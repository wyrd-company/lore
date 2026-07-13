package annotations

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (repository *Repository) Cleanup(ctx context.Context, projectID uuid.UUID, revisionID *uuid.UUID, username string) (CleanupResult, error) {
	if strings.TrimSpace(username) == "" {
		return CleanupResult{}, fmt.Errorf("%w: attributedUsername is required", ErrInvalid)
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return CleanupResult{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
	rows, err := tx.Query(ctx, `
		SELECT r.id
		FROM revisions r
		JOIN documents d ON d.id = r.document_id AND d.project_id = r.project_id
		WHERE r.project_id = $1
		  AND ($2::uuid IS NULL OR r.id = $2)
		  AND r.id IS DISTINCT FROM d.current_revision_id
		  AND EXISTS (SELECT 1 FROM annotations a WHERE a.project_id = r.project_id AND a.revision_id = r.id)
		  AND NOT EXISTS (SELECT 1 FROM annotations a WHERE a.project_id = r.project_id AND a.revision_id = r.id AND a.status = 'open')
		FOR UPDATE OF r`, projectID, revisionID)
	if err != nil {
		return CleanupResult{}, err
	}
	ids := make([]uuid.UUID, 0)
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return CleanupResult{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return CleanupResult{}, err
	}
	result := CleanupResult{}
	for _, id := range ids {
		annotationRows, err := tx.Query(ctx, `SELECT id FROM annotations WHERE project_id = $1 AND revision_id = $2 FOR UPDATE`, projectID, id)
		if err != nil {
			return result, err
		}
		annotationIDs := make([]uuid.UUID, 0)
		for annotationRows.Next() {
			var annotationID uuid.UUID
			if err := annotationRows.Scan(&annotationID); err != nil {
				annotationRows.Close()
				return result, err
			}
			annotationIDs = append(annotationIDs, annotationID)
		}
		annotationRows.Close()
		if _, err := tx.Exec(ctx, `
			UPDATE annotations SET revision_id = NULL, tombstoned_at = now(), updated_by = $1
			WHERE project_id = $2 AND revision_id = $3`, strings.TrimSpace(username), projectID, id); err != nil {
			return result, err
		}
		for _, annotationID := range annotationIDs {
			if err := addEvent(ctx, tx, projectID, annotationID, "cleanup", username, nil); err != nil {
				return result, err
			}
		}
		if _, err := tx.Exec(ctx, `DELETE FROM revisions WHERE project_id = $1 AND id = $2`, projectID, id); err != nil {
			return result, err
		}
		result.RevisionsRemoved++
		result.AnnotationsTombstoned += len(annotationIDs)
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}
