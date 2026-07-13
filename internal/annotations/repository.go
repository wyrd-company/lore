package annotations

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrNotFound = errors.New("annotation not found")
	ErrConflict = errors.New("annotation conflict")
	ErrInvalid  = errors.New("invalid annotation")
)

type Record struct {
	ID                   uuid.UUID       `json:"id"`
	ProjectID            uuid.UUID       `json:"projectId"`
	DocumentID           uuid.UUID       `json:"documentId"`
	DocumentIdentity     string          `json:"documentIdentity"`
	DocumentTitle        string          `json:"documentTitle"`
	SourceType           string          `json:"sourceType"`
	SourceInstance       string          `json:"sourceInstance"`
	RevisionID           *uuid.UUID      `json:"revisionId,omitempty"`
	RevisionIdentity     uuid.UUID       `json:"revisionIdentity"`
	Body                 string          `json:"body"`
	Status               string          `json:"status"`
	AttributedUsername   string          `json:"attributedUsername"`
	UpdatedBy            string          `json:"updatedBy"`
	OriginatingOperation string          `json:"originatingOperation"`
	Selector             json.RawMessage `json:"selector"`
	SelectedQuote        *string         `json:"selectedQuote,omitempty"`
	QuotePrefix          *string         `json:"quotePrefix,omitempty"`
	QuoteSuffix          *string         `json:"quoteSuffix,omitempty"`
	StructuralLocation   json.RawMessage `json:"structuralLocation"`
	OriginalContentHash  string          `json:"originalContentHash"`
	SourceProvenance     json.RawMessage `json:"sourceProvenance"`
	CopiedFromAnnotation *uuid.UUID      `json:"copiedFromAnnotationId,omitempty"`
	PriorTarget          json.RawMessage `json:"priorTarget,omitempty"`
	ResolvedAt           *time.Time      `json:"resolvedAt,omitempty"`
	ResolvedBy           *string         `json:"resolvedBy,omitempty"`
	TombstonedAt         *time.Time      `json:"tombstonedAt,omitempty"`
	CreatedAt            time.Time       `json:"createdAt"`
	UpdatedAt            time.Time       `json:"updatedAt"`
	ChangeSequence       int64           `json:"changeSequence"`
}

type Event struct {
	ID                 int64           `json:"id"`
	Operation          string          `json:"operation"`
	AttributedUsername string          `json:"attributedUsername"`
	Details            json.RawMessage `json:"details"`
	CreatedAt          time.Time       `json:"createdAt"`
}

type CreateRequest struct {
	DocumentID           uuid.UUID       `json:"documentId"`
	RevisionID           uuid.UUID       `json:"revisionId"`
	Body                 string          `json:"body"`
	AttributedUsername   string          `json:"attributedUsername"`
	OriginatingOperation string          `json:"originatingOperation"`
	Selector             json.RawMessage `json:"selector"`
	SelectedQuote        *string         `json:"selectedQuote,omitempty"`
	QuotePrefix          *string         `json:"quotePrefix,omitempty"`
	QuoteSuffix          *string         `json:"quoteSuffix,omitempty"`
	StructuralLocation   json.RawMessage `json:"structuralLocation,omitempty"`
	OriginalContentHash  string          `json:"originalContentHash"`
}

func (request CreateRequest) validate() error {
	if request.DocumentID == uuid.Nil || request.RevisionID == uuid.Nil {
		return fmt.Errorf("%w: documentId and revisionId are required", ErrInvalid)
	}
	if strings.TrimSpace(request.Body) == "" {
		return fmt.Errorf("%w: body is required", ErrInvalid)
	}
	if strings.TrimSpace(request.AttributedUsername) == "" {
		return fmt.Errorf("%w: attributedUsername is required", ErrInvalid)
	}
	if strings.TrimSpace(request.OriginatingOperation) == "" {
		return fmt.Errorf("%w: originatingOperation is required", ErrInvalid)
	}
	if !validObject(request.Selector) {
		return fmt.Errorf("%w: selector must be a JSON object", ErrInvalid)
	}
	if request.OriginalContentHash == "" {
		return fmt.Errorf("%w: originalContentHash is required", ErrInvalid)
	}
	var selector struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(request.Selector, &selector)
	if selector.Kind == "text-quote" && (request.SelectedQuote == nil || strings.TrimSpace(*request.SelectedQuote) == "" || (request.QuotePrefix == nil && request.QuoteSuffix == nil)) {
		return fmt.Errorf("%w: text-quote selectors require selectedQuote and surrounding context", ErrInvalid)
	}
	return nil
}

type UpdateRequest struct {
	Body               *string `json:"body,omitempty"`
	Status             *string `json:"status,omitempty"`
	AttributedUsername string  `json:"attributedUsername"`
}

type RetargetRequest struct {
	TargetRevisionID    uuid.UUID       `json:"targetRevisionId"`
	AttributedUsername  string          `json:"attributedUsername"`
	Selector            json.RawMessage `json:"selector"`
	SelectedQuote       *string         `json:"selectedQuote,omitempty"`
	QuotePrefix         *string         `json:"quotePrefix,omitempty"`
	QuoteSuffix         *string         `json:"quoteSuffix,omitempty"`
	StructuralLocation  json.RawMessage `json:"structuralLocation,omitempty"`
	OriginalContentHash string          `json:"originalContentHash"`
}

type Filters struct {
	DocumentID *uuid.UUID
	RevisionID *uuid.UUID
	Status     string
	After      int64
}

type Export struct {
	FormatVersion string   `json:"formatVersion"`
	Project       string   `json:"project"`
	Mode          string   `json:"mode"`
	AfterCursor   int64    `json:"afterCursor,omitempty"`
	NextCursor    int64    `json:"nextCursor"`
	Annotations   []Record `json:"annotations"`
}

type CleanupResult struct {
	RevisionsRemoved      int `json:"revisionsRemoved"`
	AnnotationsTombstoned int `json:"annotationsTombstoned"`
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) Create(ctx context.Context, projectID uuid.UUID, request CreateRequest) (Record, error) {
	if err := request.validate(); err != nil {
		return Record{}, err
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
	provenance, err := revisionTarget(ctx, tx, projectID, request.DocumentID, request.RevisionID, request.OriginalContentHash)
	if err != nil {
		return Record{}, err
	}
	var id uuid.UUID
	err = tx.QueryRow(ctx, `
		INSERT INTO annotations (
			project_id, document_id, revision_id, revision_identity, body, status,
			attributed_username, updated_by, originating_operation, selector, selected_quote,
			quote_prefix, quote_suffix, structural_location, original_content_hash, source_provenance)
		VALUES ($1, $2, $3, $3, $4, 'open', $5, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id`, projectID, request.DocumentID, request.RevisionID, strings.TrimSpace(request.Body),
		strings.TrimSpace(request.AttributedUsername), strings.TrimSpace(request.OriginatingOperation), request.Selector,
		request.SelectedQuote, request.QuotePrefix, request.QuoteSuffix, objectOrEmpty(request.StructuralLocation),
		request.OriginalContentHash, provenance).Scan(&id)
	if err != nil {
		return Record{}, err
	}
	if err := addEvent(ctx, tx, projectID, id, "create", request.AttributedUsername, nil); err != nil {
		return Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return repository.Get(ctx, projectID, id)
}

func (repository *Repository) Update(ctx context.Context, projectID, annotationID uuid.UUID, request UpdateRequest) (Record, error) {
	if strings.TrimSpace(request.AttributedUsername) == "" {
		return Record{}, fmt.Errorf("%w: attributedUsername is required", ErrInvalid)
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Record{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
	var body, status string
	var tombstonedAt *time.Time
	var resolvedAt *time.Time
	var resolvedBy *string
	if err := tx.QueryRow(ctx, `SELECT body, status, tombstoned_at, resolved_at, resolved_by FROM annotations WHERE project_id = $1 AND id = $2 FOR UPDATE`, projectID, annotationID).
		Scan(&body, &status, &tombstonedAt, &resolvedAt, &resolvedBy); err != nil {
		return Record{}, notFound(err)
	}
	if request.Body != nil {
		body = strings.TrimSpace(*request.Body)
		if body == "" {
			return Record{}, fmt.Errorf("%w: body must not be empty", ErrInvalid)
		}
	}
	operation := "update"
	if request.Status != nil {
		if *request.Status != "open" && *request.Status != "resolved" && *request.Status != "dismissed" {
			return Record{}, fmt.Errorf("%w: status must be open, resolved, or dismissed", ErrInvalid)
		}
		if tombstonedAt != nil && *request.Status == "open" {
			return Record{}, fmt.Errorf("%w: a tombstoned annotation cannot be reopened", ErrConflict)
		}
		status = *request.Status
		operation = map[string]string{"open": "reopen", "resolved": "resolve", "dismissed": "dismiss"}[status]
	}
	if request.Status != nil {
		if status == "open" {
			resolvedAt = nil
			resolvedBy = nil
		} else {
			now := time.Now().UTC()
			username := strings.TrimSpace(request.AttributedUsername)
			resolvedAt = &now
			resolvedBy = &username
		}
	}
	if _, err := tx.Exec(ctx, `
		UPDATE annotations SET body = $1, status = $2, updated_by = $3, resolved_at = $4, resolved_by = $5
		WHERE project_id = $6 AND id = $7`, body, status, strings.TrimSpace(request.AttributedUsername), resolvedAt, resolvedBy, projectID, annotationID); err != nil {
		return Record{}, err
	}
	details, _ := json.Marshal(map[string]any{"bodyChanged": request.Body != nil, "status": status})
	if err := addEvent(ctx, tx, projectID, annotationID, operation, request.AttributedUsername, details); err != nil {
		return Record{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Record{}, err
	}
	return repository.Get(ctx, projectID, annotationID)
}

func (repository *Repository) Copy(ctx context.Context, projectID, annotationID uuid.UUID, request RetargetRequest) (Record, error) {
	return repository.retarget(ctx, projectID, annotationID, request, false)
}

func (repository *Repository) Move(ctx context.Context, projectID, annotationID uuid.UUID, request RetargetRequest) (Record, error) {
	return repository.retarget(ctx, projectID, annotationID, request, true)
}

func (repository *Repository) retarget(ctx context.Context, projectID, annotationID uuid.UUID, request RetargetRequest, move bool) (Record, error) {
	if request.TargetRevisionID == uuid.Nil || strings.TrimSpace(request.AttributedUsername) == "" || !validObject(request.Selector) || request.OriginalContentHash == "" {
		return Record{}, fmt.Errorf("%w: targetRevisionId, attributedUsername, selector, and originalContentHash are required", ErrInvalid)
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
	provenance, err := revisionTarget(ctx, tx, projectID, source.DocumentID, request.TargetRevisionID, request.OriginalContentHash)
	if err != nil {
		return Record{}, err
	}
	operation := "copy"
	resultID := uuid.Nil
	if move {
		operation = "move"
		prior, _ := json.Marshal(map[string]any{
			"revisionIdentity": source.RevisionIdentity, "selector": source.Selector,
			"originalContentHash": source.OriginalContentHash, "sourceProvenance": source.SourceProvenance,
		})
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

func revisionTarget(ctx context.Context, tx pgx.Tx, projectID, documentID, revisionID uuid.UUID, expectedHash string) (json.RawMessage, error) {
	var hash string
	var provenance json.RawMessage
	err := tx.QueryRow(ctx, `
		SELECT r.content_hash, r.provenance
		FROM revisions r JOIN documents d ON d.id = r.document_id AND d.project_id = r.project_id
		WHERE r.project_id = $1 AND r.document_id = $2 AND r.id = $3`, projectID, documentID, revisionID).
		Scan(&hash, &provenance)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if hash != expectedHash {
		return nil, fmt.Errorf("%w: originalContentHash does not match revision", ErrConflict)
	}
	return provenance, nil
}

func addEvent(ctx context.Context, tx pgx.Tx, projectID, annotationID uuid.UUID, operation, username string, details json.RawMessage) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO annotation_events (project_id, annotation_id, operation, attributed_username, details)
		VALUES ($1, $2, $3, $4, $5)`, projectID, annotationID, operation, strings.TrimSpace(username), objectOrEmpty(details))
	return err
}

func validObject(value json.RawMessage) bool {
	var object map[string]any
	return len(value) > 0 && json.Unmarshal(value, &object) == nil && object != nil
}

func objectOrEmpty(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
