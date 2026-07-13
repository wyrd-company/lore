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
	for _, document := range manifest.Documents {
		identities = append(identities, document.Identity)
		created, changed, unchanged, applyErr := applyDocument(ctx, tx, projectID, sourceInstanceID, manifest.SourceType, document)
		if applyErr != nil {
			return result, applyErr
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

	if err = tx.Commit(ctx); err != nil {
		return result, fmt.Errorf("commit synchronization: %w", err)
	}
	return result, nil
}

func applyDocument(ctx context.Context, tx pgx.Tx, projectID, sourceInstanceID uuid.UUID, sourceType string, document Document) (created, changed, unchanged bool, err error) {
	var documentID uuid.UUID
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
		return false, false, false, fmt.Errorf("upsert document %q: %w", document.Identity, err)
	}

	if currentRevisionID != nil {
		var currentHash string
		if err := tx.QueryRow(ctx, `SELECT content_hash FROM revisions WHERE id = $1 AND document_id = $2`, *currentRevisionID, documentID).Scan(&currentHash); err != nil {
			return false, false, false, fmt.Errorf("read current revision for %q: %w", document.Identity, err)
		}
		if currentHash == document.ContentHash {
			return inserted, false, true, nil
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
		return false, false, false, fmt.Errorf("create revision for %q: %w", document.Identity, err)
	}

	if newRevision {
		for _, chunk := range document.Chunks {
			if _, err := tx.Exec(ctx, `
				INSERT INTO chunks (project_id, revision_id, ordinal, normalized_text, structural_location, token_count)
				VALUES ($1, $2, $3, $4, $5, $6)`, projectID, revisionID, chunk.Ordinal, chunk.NormalizedText,
				jsonOrEmpty(chunk.StructuralLocation), chunk.TokenCount); err != nil {
				return false, false, false, fmt.Errorf("create chunk %d for %q: %w", chunk.Ordinal, document.Identity, err)
			}
		}
	}

	if _, err := tx.Exec(ctx, `UPDATE documents SET current_revision_id = $1, deleted_at = NULL, updated_at = now() WHERE id = $2`, revisionID, documentID); err != nil {
		return false, false, false, fmt.Errorf("make revision current for %q: %w", document.Identity, err)
	}
	return inserted, !inserted, false, nil
}
