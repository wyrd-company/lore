package synchronization

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
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

	identities := make([]string, 0, len(manifest.Documents))
	documentIDs := make(map[string]uuid.UUID, len(manifest.Documents))
	for _, document := range manifest.Documents {
		identities = append(identities, document.Identity)
		documentID, created, changed, unchanged, applyErr := applyDocument(ctx, tx, projectID, sourceInstanceID, manifest.SourceType, document)
		if applyErr != nil {
			return result, applyErr
		}
		documentIDs[document.Identity] = documentID
		if applyErr := replaceTags(ctx, tx, projectID, documentID, document.Tags); applyErr != nil {
			return result, fmt.Errorf("replace tags for %q: %w", document.Identity, applyErr)
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
		commandTag, deleteErr := tx.Exec(ctx, `
			UPDATE documents
			SET deleted_at = now(), updated_at = now()
			WHERE project_id = $1
			  AND source_instance_id = $2
			  AND deleted_at IS NULL
			  AND NOT (source_identity = ANY($3::text[]))`, projectID, sourceInstanceID, identities)
		if deleteErr != nil {
			return result, fmt.Errorf("delete documents absent from complete manifest: %w", deleteErr)
		}
		result.Deleted = int(commandTag.RowsAffected())
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

func applyDocument(ctx context.Context, tx pgx.Tx, projectID, sourceInstanceID uuid.UUID, sourceType string, document Document) (documentID uuid.UUID, created, changed, unchanged bool, err error) {
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
		return uuid.Nil, false, false, false, fmt.Errorf("upsert document %q: %w", document.Identity, err)
	}

	if currentRevisionID != nil {
		var currentHash string
		if err := tx.QueryRow(ctx, `SELECT content_hash FROM revisions WHERE id = $1 AND document_id = $2`, *currentRevisionID, documentID).Scan(&currentHash); err != nil {
			return uuid.Nil, false, false, false, fmt.Errorf("read current revision for %q: %w", document.Identity, err)
		}
		if currentHash == document.ContentHash {
			return documentID, inserted, false, true, nil
		}
	}

	var revisionID uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO revisions (project_id, document_id, content_hash, normalized_text, rendered_content, renderer, metadata, provenance)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (document_id, content_hash) DO NOTHING
		RETURNING id`, projectID, documentID, document.ContentHash, document.NormalizedText, document.RenderedContent,
		document.Renderer, jsonOrEmpty(document.Metadata), jsonOrEmpty(document.Provenance)).Scan(&revisionID)
	newRevision := true
	if errors.Is(err, pgx.ErrNoRows) {
		newRevision = false
		err = tx.QueryRow(ctx, `SELECT id FROM revisions WHERE document_id = $1 AND content_hash = $2`, documentID, document.ContentHash).Scan(&revisionID)
	}
	if err != nil {
		return uuid.Nil, false, false, false, fmt.Errorf("create revision for %q: %w", document.Identity, err)
	}

	if newRevision {
		for _, chunk := range document.Chunks {
			if _, err := tx.Exec(ctx, `
				INSERT INTO chunks (project_id, revision_id, ordinal, normalized_text, structural_location, token_count)
				VALUES ($1, $2, $3, $4, $5, $6)`, projectID, revisionID, chunk.Ordinal, chunk.NormalizedText,
				jsonOrEmpty(chunk.StructuralLocation), chunk.TokenCount); err != nil {
				return uuid.Nil, false, false, false, fmt.Errorf("create chunk %d for %q: %w", chunk.Ordinal, document.Identity, err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE documents SET current_revision_id = $1, deleted_at = NULL, updated_at = now() WHERE id = $2`, revisionID, documentID); err != nil {
		return uuid.Nil, false, false, false, fmt.Errorf("make revision current for %q: %w", document.Identity, err)
	}
	return documentID, inserted, !inserted, false, nil
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
