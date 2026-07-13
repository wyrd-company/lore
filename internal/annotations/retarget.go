package annotations

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

func (repository *Repository) Copy(ctx context.Context, projectID, annotationID uuid.UUID, request RetargetRequest) (Record, error) {
	return repository.retarget(ctx, projectID, annotationID, request, false)
}

func (repository *Repository) Move(ctx context.Context, projectID, annotationID uuid.UUID, request RetargetRequest) (Record, error) {
	return repository.retarget(ctx, projectID, annotationID, request, true)
}

func (repository *Repository) retarget(ctx context.Context, projectID, annotationID uuid.UUID, request RetargetRequest, move bool) (Record, error) {
	if request.TargetRevisionID == uuid.Nil || strings.TrimSpace(request.AttributedUsername) == "" || request.OriginalContentHash == "" {
		return Record{}, fmt.Errorf("%w: targetRevisionId, attributedUsername, selector, and originalContentHash are required", ErrInvalid)
	}
	if err := validateSelector(request.Selector, request.SelectedQuote, request.QuotePrefix, request.QuoteSuffix); err != nil {
		return Record{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
	var source Record
	if err := scanRecord(tx.QueryRow(ctx, recordSelect+` WHERE a.project_id = $1 AND a.id = $2 FOR UPDATE OF a`, projectID, annotationID), &source); err != nil {
		return Record{}, notFound(err)
	}
	if source.TombstonedAt != nil {
		return Record{}, fmt.Errorf("%w: a tombstoned annotation cannot be retargeted", ErrConflict)
	}
	if source.RevisionIdentity == request.TargetRevisionID {
		return Record{}, fmt.Errorf("%w: target revision must differ from the current target", ErrConflict)
	}
	provenance, err := revisionTarget(ctx, tx, projectID, source.DocumentID, request.TargetRevisionID, request.OriginalContentHash)
	if err != nil {
		return Record{}, err
	}
	operation := "copy"
	resultID := uuid.Nil
	if move {
		operation = "move"
		prior := source.PriorTarget
		if !validObject(prior) {
			prior, _ = json.Marshal(map[string]any{
				"revisionIdentity": source.RevisionIdentity, "selector": source.Selector,
				"originalContentHash": source.OriginalContentHash, "sourceProvenance": source.SourceProvenance,
			})
		}
		_, err = tx.Exec(ctx, `
			UPDATE annotations SET revision_id = $1, revision_identity = $1, selector = $2, selected_quote = $3,
				quote_prefix = $4, quote_suffix = $5, structural_location = $6, original_content_hash = $7,
				source_provenance = $8, prior_target = $9, updated_by = $10
			WHERE project_id = $11 AND id = $12`, request.TargetRevisionID, request.Selector, request.SelectedQuote,
			request.QuotePrefix, request.QuoteSuffix, objectOrEmpty(request.StructuralLocation), request.OriginalContentHash,
			provenance, prior, strings.TrimSpace(request.AttributedUsername), projectID, annotationID)
		resultID = annotationID
	} else {
		err = tx.QueryRow(ctx, `
			INSERT INTO annotations (
				project_id, document_id, revision_id, revision_identity, body, status, attributed_username,
				updated_by, originating_operation, selector, selected_quote, quote_prefix, quote_suffix,
				structural_location, original_content_hash, source_provenance, copied_from_annotation_id)
			VALUES ($1, $2, $3, $3, $4, 'open', $5, $5, 'copy', $6, $7, $8, $9, $10, $11, $12, $13)
			RETURNING id`, projectID, source.DocumentID, request.TargetRevisionID, source.Body,
			strings.TrimSpace(request.AttributedUsername), request.Selector, request.SelectedQuote, request.QuotePrefix,
			request.QuoteSuffix, objectOrEmpty(request.StructuralLocation), request.OriginalContentHash, provenance, annotationID).Scan(&resultID)
	}
	if err != nil {
		return Record{}, err
	}
	details, _ := json.Marshal(map[string]any{"sourceAnnotationId": annotationID, "targetRevisionId": request.TargetRevisionID})
	if err := addEvent(ctx, tx, projectID, resultID, operation, request.AttributedUsername, details); err != nil {
		return Record{}, err
	}
	if move {
		if _, err := tx.Exec(ctx, `
			DELETE FROM revisions r
			USING documents d
			WHERE r.project_id = $1 AND r.id = $2 AND d.id = r.document_id
			  AND r.id IS DISTINCT FROM d.current_revision_id
			  AND NOT EXISTS (SELECT 1 FROM annotations a WHERE a.project_id = r.project_id AND a.revision_id = r.id)`,
			projectID, source.RevisionIdentity); err != nil {
			return Record{}, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return repository.Get(ctx, projectID, resultID)
}
