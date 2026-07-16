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
	if err := validateSelector(request.Selector, request.SelectedQuote, request.QuotePrefix, request.QuoteSuffix); err != nil {
		return err
	}
	if request.OriginalContentHash == "" {
		return fmt.Errorf("%w: originalContentHash is required", ErrInvalid)
	}
	return nil
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
	if request.Body == nil && request.Status == nil {
		return Record{}, fmt.Errorf("%w: body or status is required", ErrInvalid)
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

func (repository *Repository) Reply(ctx context.Context, projectID, annotationID uuid.UUID, request ReplyRequest) (Reply, error) {
	body := strings.TrimSpace(request.Body)
	username := strings.TrimSpace(request.AttributedUsername)
	if body == "" || username == "" {
		return Reply{}, fmt.Errorf("%w: body and attributedUsername are required", ErrInvalid)
	}
	tx, err := repository.pool.Begin(ctx)
	if err != nil {
		return Reply{}, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
	if err := tx.QueryRow(ctx, `SELECT id FROM annotations WHERE project_id = $1 AND id = $2 FOR UPDATE`, projectID, annotationID).Scan(&annotationID); err != nil {
		return Reply{}, notFound(err)
	}
	var reply Reply
	err = tx.QueryRow(ctx, `
		INSERT INTO annotation_replies (project_id, annotation_id, body, attributed_username)
		VALUES ($1, $2, $3, $4)
		RETURNING id, annotation_id, body, attributed_username, created_at, updated_at`, projectID, annotationID, body, username).
		Scan(&reply.ID, &reply.AnnotationID, &reply.Body, &reply.AttributedUsername, &reply.CreatedAt, &reply.UpdatedAt)
	if err != nil {
		return Reply{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE annotations SET updated_by = $1 WHERE project_id = $2 AND id = $3`, username, projectID, annotationID); err != nil {
		return Reply{}, err
	}
	details, _ := json.Marshal(map[string]any{"replyId": reply.ID})
	if err := addEvent(ctx, tx, projectID, annotationID, "reply", username, details); err != nil {
		return Reply{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Reply{}, err
	}
	return reply, nil
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

func validateSelector(selector json.RawMessage, selectedQuote, quotePrefix, quoteSuffix *string) error {
	if !validObject(selector) {
		return fmt.Errorf("%w: selector must be a JSON object", ErrInvalid)
	}
	var value struct {
		Kind string `json:"kind"`
	}
	_ = json.Unmarshal(selector, &value)
	if value.Kind == "text-quote" && (selectedQuote == nil || strings.TrimSpace(*selectedQuote) == "" || (quotePrefix == nil && quoteSuffix == nil)) {
		return fmt.Errorf("%w: text-quote selectors require selectedQuote and surrounding context", ErrInvalid)
	}
	return nil
}

func objectOrEmpty(value json.RawMessage) json.RawMessage {
	if len(value) == 0 {
		return json.RawMessage(`{}`)
	}
	return value
}
