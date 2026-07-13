package annotations

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

func (repository *Repository) Get(ctx context.Context, projectID, annotationID uuid.UUID) (Record, error) {
	var record Record
	if err := scanRecord(repository.pool.QueryRow(ctx, recordSelect+` WHERE a.project_id = $1 AND a.id = $2`, projectID, annotationID), &record); err != nil {
		return record, notFound(err)
	}
	return record, nil
}

func (repository *Repository) List(ctx context.Context, projectID uuid.UUID, filters Filters) ([]Record, error) {
	rows, err := repository.pool.Query(ctx, recordSelect+`
		WHERE a.project_id = $1
		  AND ($2::uuid IS NULL OR a.document_id = $2)
		  AND ($3::uuid IS NULL OR a.revision_identity = $3)
		  AND ($4 = '' OR a.status::text = $4)
		  AND a.change_sequence > $5
		ORDER BY a.change_sequence, a.id`, projectID, filters.DocumentID, filters.RevisionID, filters.Status, filters.After)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Record, 0)
	for rows.Next() {
		var record Record
		if err := scanRecord(rows, &record); err != nil {
			return nil, err
		}
		result = append(result, record)
	}
	return result, rows.Err()
}

func (repository *Repository) Events(ctx context.Context, projectID, annotationID uuid.UUID) ([]Event, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT id, operation, attributed_username, details, created_at
		FROM annotation_events WHERE project_id = $1 AND annotation_id = $2 ORDER BY id`, projectID, annotationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Event, 0)
	for rows.Next() {
		var event Event
		if err := rows.Scan(&event.ID, &event.Operation, &event.AttributedUsername, &event.Details, &event.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, event)
	}
	return result, rows.Err()
}

func (repository *Repository) Export(ctx context.Context, projectID uuid.UUID, projectSlug string, after int64) (Export, error) {
	annotations, err := repository.List(ctx, projectID, Filters{After: after})
	if err != nil {
		return Export{}, err
	}
	next := after
	for _, annotation := range annotations {
		if annotation.ChangeSequence > next {
			next = annotation.ChangeSequence
		}
	}
	mode := "snapshot"
	if after > 0 {
		mode = "incremental"
	}
	return Export{FormatVersion: "lore.annotations/v1", Project: projectSlug, Mode: mode, AfterCursor: after, NextCursor: next, Annotations: annotations}, nil
}

const recordSelect = `
	SELECT a.id, a.project_id, a.document_id, d.source_identity, d.title, d.source_type, si.external_key,
		a.revision_id, a.revision_identity, a.body, a.status::text, a.attributed_username,
		a.updated_by, a.originating_operation, a.selector, a.selected_quote, a.quote_prefix,
		a.quote_suffix, a.structural_location, a.original_content_hash, a.source_provenance,
		a.copied_from_annotation_id, a.prior_target, a.resolved_at, a.resolved_by,
		a.tombstoned_at, a.created_at, a.updated_at, a.change_sequence
	FROM annotations a
	JOIN documents d ON d.id = a.document_id AND d.project_id = a.project_id
	JOIN source_instances si ON si.id = d.source_instance_id AND si.project_id = d.project_id`

type scanner interface{ Scan(...any) error }

func scanRecord(row scanner, record *Record) error {
	return row.Scan(&record.ID, &record.ProjectID, &record.DocumentID, &record.DocumentIdentity, &record.DocumentTitle,
		&record.SourceType, &record.SourceInstance, &record.RevisionID, &record.RevisionIdentity, &record.Body, &record.Status,
		&record.AttributedUsername, &record.UpdatedBy, &record.OriginatingOperation, &record.Selector,
		&record.SelectedQuote, &record.QuotePrefix, &record.QuoteSuffix, &record.StructuralLocation,
		&record.OriginalContentHash, &record.SourceProvenance, &record.CopiedFromAnnotation, &record.PriorTarget,
		&record.ResolvedAt, &record.ResolvedBy, &record.TombstonedAt, &record.CreatedAt, &record.UpdatedAt,
		&record.ChangeSequence)
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
