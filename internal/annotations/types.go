package annotations

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
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
