package synchronization

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/indexing"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func IsRetryable(err error) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && (postgresError.Code == "40001" || postgresError.Code == "40P01")
}

func (r *Repository) Apply(ctx context.Context, projectID uuid.UUID, manifest Manifest) (result Result, err error) {
	if err := manifest.Validate(); err != nil {
		return result, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return result, fmt.Errorf("begin synchronization: %w", err)
	}
	defer func() {
		if err != nil {
			tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
		}
	}()
	var projectSlug string
	if err = tx.QueryRow(ctx, `SELECT slug FROM projects WHERE id = $1`, projectID).Scan(&projectSlug); err != nil {
		return result, fmt.Errorf("resolve manifest project: %w", err)
	}
	if projectSlug != manifest.Project {
		return result, fmt.Errorf("manifest project %q does not match database project %q", manifest.Project, projectSlug)
	}

	var sourceInstanceID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO source_instances (project_id, source_type, external_key, metadata)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (project_id, source_type, external_key)
		DO UPDATE SET metadata = EXCLUDED.metadata, updated_at = now()
		RETURNING id`, projectID, manifest.SourceType, manifest.SourceInstance, jsonOrEmpty(manifest.Metadata)).Scan(&sourceInstanceID)
	if err != nil {
		return result, fmt.Errorf("resolve source instance: %w", err)
	}
	for _, failure := range manifest.Failures {
		if _, err = tx.Exec(ctx, `
			INSERT INTO ingestion_failures (project_id, source_instance_id, path, message)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (project_id, source_instance_id, path)
			DO UPDATE SET message = EXCLUDED.message, updated_at = now()`, projectID, sourceInstanceID, failure.Path, failure.Message); err != nil {
			return result, fmt.Errorf("record ingestion failure for %q: %w", failure.Path, err)
		}
	}
	result.Failed = len(manifest.Failures)

	identities := make([]string, 0, len(manifest.Documents))
	documentIDs := make(map[string]uuid.UUID, len(manifest.Documents))
	for _, document := range manifest.Documents {
		identities = append(identities, document.Identity)
		documentID, revisionID, newRevision, created, changed, unchanged, applyErr := applyDocument(ctx, tx, projectID, sourceInstanceID, manifest.SourceType, document)
		if applyErr != nil {
			return result, applyErr
		}
		documentIDs[document.Identity] = documentID
		if applyErr := replaceTags(ctx, tx, projectID, documentID, document.Tags); applyErr != nil {
			return result, fmt.Errorf("replace tags for %q: %w", document.Identity, applyErr)
		}
		if applyErr := replaceTerms(ctx, tx, projectID, documentID, document.Terms, document.DefinesTerm); applyErr != nil {
			return result, fmt.Errorf("replace terms for %q: %w", document.Identity, applyErr)
		}
		indexInput := indexing.RevisionInput{
			ProjectID: projectID, RevisionID: revisionID, SourceType: manifest.SourceType,
			Title: document.Title, NormalizedText: document.NormalizedText, Metadata: document.Metadata, Tags: document.Tags,
		}
		if newRevision {
			applyErr = indexing.IndexRevision(ctx, tx, indexInput)
		} else {
			applyErr = indexing.RefreshKeywords(ctx, tx, indexInput)
		}
		if applyErr != nil {
			return result, fmt.Errorf("index document %q: %w", document.Identity, applyErr)
		}
		if created {
			result.Created++
		} else if changed {
			result.Updated++
		} else if unchanged {
			result.Unchanged++
		}
	}

	if manifest.Boundary == BoundaryComplete {
		rows, deleteErr := tx.Query(ctx, `
			UPDATE documents
			SET current_revision_id = NULL, deleted_at = now(), updated_at = now()
			WHERE project_id = $1
			  AND source_instance_id = $2
			  AND deleted_at IS NULL
			  AND NOT (source_identity = ANY($3::text[]))
			  AND NOT EXISTS (
				SELECT 1
				FROM ingestion_failures failure
				JOIN revisions current_revision ON current_revision.id = documents.current_revision_id
				WHERE failure.project_id = documents.project_id
				  AND failure.source_instance_id = documents.source_instance_id
				  AND failure.path = current_revision.provenance->>'path'
			  )
			RETURNING id`, projectID, sourceInstanceID, identities)
		if deleteErr != nil {
			return result, fmt.Errorf("delete documents absent from complete manifest: %w", deleteErr)
		}
		deletedDocumentIDs := make([]uuid.UUID, 0)
		for rows.Next() {
			var documentID uuid.UUID
			if err := rows.Scan(&documentID); err != nil {
				rows.Close()
				return result, fmt.Errorf("read deleted document: %w", err)
			}
			deletedDocumentIDs = append(deletedDocumentIDs, documentID)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return result, fmt.Errorf("read deleted documents: %w", err)
		}
		result.Deleted = len(deletedDocumentIDs)
		if err := deleteUnannotatedRevisions(ctx, tx, projectID, deletedDocumentIDs, uuid.Nil); err != nil {
			return result, fmt.Errorf("retain annotated deleted documents: %w", err)
		}
		if _, err = tx.Exec(ctx, `UPDATE source_instances SET last_complete_sync_at = now(), updated_at = now() WHERE id = $1`, sourceInstanceID); err != nil {
			return result, fmt.Errorf("record complete synchronization: %w", err)
		}
	}
	if err = replaceRelationships(ctx, tx, projectID, sourceInstanceID, manifest.Boundary, documentIDs, manifest.Relationships); err != nil {
		return result, err
	}

	if err = tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("commit synchronization: %w", err)
	}
	return result, nil
}

func applyDocument(ctx context.Context, tx pgx.Tx, projectID, sourceInstanceID uuid.UUID, sourceType string, document Document) (documentID, revisionID uuid.UUID, newRevision, created, changed, unchanged bool, err error) {
	var currentRevisionID *uuid.UUID
	var inserted bool
	err = tx.QueryRow(ctx, `
		INSERT INTO documents (project_id, source_instance_id, source_type, source_identity, title)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (project_id, source_type, source_instance_id, source_identity)
		DO UPDATE SET title = EXCLUDED.title, deleted_at = NULL, updated_at = now()
		RETURNING id, current_revision_id, (xmax = 0)`, projectID, sourceInstanceID, sourceType, document.Identity, document.Title).
		Scan(&documentID, &currentRevisionID, &inserted)
	if err != nil {
		return uuid.Nil, uuid.Nil, false, false, false, false, fmt.Errorf("upsert document %q: %w", document.Identity, err)
	}

	if currentRevisionID != nil {
		var currentHash string
		if err := tx.QueryRow(ctx, `SELECT content_hash FROM revisions WHERE id = $1 AND document_id = $2`, *currentRevisionID, documentID).Scan(&currentHash); err != nil {
			return uuid.Nil, uuid.Nil, false, false, false, false, fmt.Errorf("read current revision for %q: %w", document.Identity, err)
		}
		if currentHash == document.ContentHash {
			return documentID, *currentRevisionID, false, inserted, false, true, nil
		}
	}

	err = tx.QueryRow(ctx, `
		INSERT INTO revisions (project_id, document_id, content_hash, normalized_text, rendered_content, renderer, metadata, provenance)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (document_id, content_hash) DO NOTHING
		RETURNING id`, projectID, documentID, document.ContentHash, document.NormalizedText, document.RenderedContent,
		document.Renderer, jsonOrEmpty(document.Metadata), jsonOrEmpty(document.Provenance)).Scan(&revisionID)
	newRevision = true
	if errors.Is(err, pgx.ErrNoRows) {
		newRevision = false
		err = tx.QueryRow(ctx, `SELECT id FROM revisions WHERE document_id = $1 AND content_hash = $2`, documentID, document.ContentHash).Scan(&revisionID)
	}
	if err != nil {
		return uuid.Nil, uuid.Nil, false, false, false, false, fmt.Errorf("create revision for %q: %w", document.Identity, err)
	}

	if _, err := tx.Exec(ctx, `UPDATE documents SET current_revision_id = $1, deleted_at = NULL, updated_at = now() WHERE id = $2`, revisionID, documentID); err != nil {
		return uuid.Nil, uuid.Nil, false, false, false, false, fmt.Errorf("make revision current for %q: %w", document.Identity, err)
	}
	if err := deleteUnannotatedRevisions(ctx, tx, projectID, []uuid.UUID{documentID}, revisionID); err != nil {
		return uuid.Nil, uuid.Nil, false, false, false, false, fmt.Errorf("remove superseded revisions for %q: %w", document.Identity, err)
	}
	return documentID, revisionID, newRevision, inserted, !inserted, false, nil
}

func deleteUnannotatedRevisions(ctx context.Context, tx pgx.Tx, projectID uuid.UUID, documentIDs []uuid.UUID, retainedRevisionID uuid.UUID) error {
	if len(documentIDs) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		DELETE FROM revisions r
		WHERE r.project_id = $1
		  AND r.document_id = ANY($2::uuid[])
		  AND ($3::uuid = '00000000-0000-0000-0000-000000000000' OR r.id <> $3)
		  AND NOT EXISTS (SELECT 1 FROM annotations a WHERE a.project_id = r.project_id AND a.revision_id = r.id)`,
		projectID, documentIDs, retainedRevisionID)
	return err
}

func replaceTags(ctx context.Context, tx pgx.Tx, projectID, documentID uuid.UUID, tags []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM document_tags WHERE project_id = $1 AND document_id = $2`, projectID, documentID); err != nil {
		return err
	}
	for _, name := range tags {
		var tagID uuid.UUID
		if err := tx.QueryRow(ctx, `
			INSERT INTO tags (project_id, name) VALUES ($1, $2)
			ON CONFLICT (project_id, name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, projectID, name).Scan(&tagID); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO document_tags (project_id, document_id, tag_id) VALUES ($1, $2, $3)`, projectID, documentID, tagID); err != nil {
			return err
		}
	}
	return nil
}

func replaceTerms(ctx context.Context, tx pgx.Tx, projectID, documentID uuid.UUID, terms []string, definesTerm string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM document_terms WHERE project_id = $1 AND document_id = $2`, projectID, documentID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM term_definitions WHERE project_id = $1 AND document_id = $2`, projectID, documentID); err != nil {
		return err
	}
	resolve := func(name string) (uuid.UUID, error) {
		var termID uuid.UUID
		err := tx.QueryRow(ctx, `
			INSERT INTO terms (project_id, name) VALUES ($1, $2)
			ON CONFLICT (project_id, name) DO UPDATE SET name = EXCLUDED.name
			RETURNING id`, projectID, name).Scan(&termID)
		return termID, err
	}
	for _, name := range terms {
		termID, err := resolve(name)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO document_terms (project_id, document_id, term_id) VALUES ($1, $2, $3)`, projectID, documentID, termID); err != nil {
			return err
		}
	}
	if definesTerm != "" {
		termID, err := resolve(definesTerm)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `INSERT INTO term_definitions (project_id, term_id, document_id) VALUES ($1, $2, $3)`, projectID, termID, documentID); err != nil {
			return err
		}
	}
	return nil
}

func replaceRelationships(ctx context.Context, tx pgx.Tx, projectID, sourceInstanceID uuid.UUID, boundary Boundary, documentIDs map[string]uuid.UUID, relationships []Relationship) error {
	if boundary == BoundaryComplete {
		if _, err := tx.Exec(ctx, `
			DELETE FROM relationships
			WHERE project_id = $1
			  AND source_document_id IN (SELECT id FROM documents WHERE source_instance_id = $2)`, projectID, sourceInstanceID); err != nil {
			return fmt.Errorf("clear complete-manifest relationships: %w", err)
		}
	} else {
		ids := make([]uuid.UUID, 0, len(documentIDs))
		for _, id := range documentIDs {
			ids = append(ids, id)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM relationships WHERE project_id = $1 AND source_document_id = ANY($2::uuid[])`, projectID, ids); err != nil {
			return fmt.Errorf("clear partial-manifest relationships: %w", err)
		}
	}

	for _, relationship := range relationships {
		sourceID := documentIDs[relationship.SourceIdentity]
		var targetID uuid.UUID
		if err := tx.QueryRow(ctx, `
			SELECT id FROM documents
			WHERE project_id = $1 AND source_instance_id = $2 AND source_identity = $3 AND deleted_at IS NULL`,
			projectID, sourceInstanceID, relationship.TargetIdentity).Scan(&targetID); err != nil {
			if relationship.Type == "note-related-to" && errors.Is(err, pgx.ErrNoRows) {
				continue
			}
			return fmt.Errorf("resolve relationship target %q: %w", relationship.TargetIdentity, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO relationships (project_id, source_document_id, target_document_id, relationship_type, metadata)
			VALUES ($1, $2, $3, $4, $5)`, projectID, sourceID, targetID, relationship.Type, jsonOrEmpty(relationship.Metadata)); err != nil {
			return fmt.Errorf("create relationship %q -> %q: %w", relationship.SourceIdentity, relationship.TargetIdentity, err)
		}
	}
	return nil
}
