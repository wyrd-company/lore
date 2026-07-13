package indexing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const Model = "voyage/voyage-4"
const Dimensions = 1024

type RevisionInput struct {
	ProjectID      uuid.UUID
	RevisionID     uuid.UUID
	SourceType     string
	Title          string
	NormalizedText string
	Metadata       json.RawMessage
	Tags           []string
}

func IndexRevision(ctx context.Context, tx pgx.Tx, input RevisionInput) error {
	chunks := ChunkDocument(input.SourceType, input.NormalizedText, input.Metadata)
	if len(chunks) == 0 && input.Title != "" {
		chunks = chunkText(input.Title, "body", map[string]any{"titleOnly": true})
	}
	for ordinal, chunk := range chunks {
		location, err := json.Marshal(chunk.Location)
		if err != nil {
			return fmt.Errorf("encode chunk location: %w", err)
		}
		var chunkID uuid.UUID
		err = tx.QueryRow(ctx, `
			INSERT INTO chunks (
				project_id, revision_id, ordinal, normalized_text, structural_location,
				token_count, content_kind, search_vector
			)
			VALUES ($1, $2, $3, $4, $5, $6, $7, `+keywordVectorSQL("$4", "$7", "$8", "$9", "$10", "$11")+`)
			RETURNING id`, input.ProjectID, input.RevisionID, ordinal, chunk.Text, location,
			chunk.TokenCount, chunk.Kind, input.Title, strings.Join(input.Tags, " "), metadataText(input.Metadata),
			input.SourceType).Scan(&chunkID)
		if err != nil {
			return fmt.Errorf("create search chunk %d: %w", ordinal, err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO embedding_jobs (chunk_id, project_id, model)
			VALUES ($1, $2, $3)
			ON CONFLICT (chunk_id) DO NOTHING`, chunkID, input.ProjectID, Model); err != nil {
			return fmt.Errorf("queue chunk embedding: %w", err)
		}
	}
	return nil
}

func RefreshKeywords(ctx context.Context, tx pgx.Tx, input RevisionInput) error {
	_, err := tx.Exec(ctx, `
		UPDATE chunks
		SET search_vector = `+keywordVectorSQL("normalized_text", "content_kind", "$3", "$4", "$5", "$6")+`
		WHERE revision_id = $2 AND project_id = $1`,
		input.ProjectID, input.RevisionID, input.Title, strings.Join(input.Tags, " "),
		metadataText(input.Metadata), input.SourceType)
	if err != nil {
		return fmt.Errorf("refresh keyword index: %w", err)
	}
	return nil
}

func keywordVectorSQL(text, kind, title, tags, metadata, sourceType string) string {
	return fmt.Sprintf(`
		setweight(to_tsvector('english', coalesce(%s, '')),
			CASE WHEN %s = 'thinking' THEN 'C'::"char" ELSE 'B'::"char" END) ||
		setweight(to_tsvector('english', coalesce(%s, '')), 'A') ||
		setweight(to_tsvector('english', coalesce(%s, '')),
			CASE WHEN %s IN ('task', 'note') THEN 'A'::"char" ELSE 'C'::"char" END) ||
		setweight(to_tsvector('english', coalesce(%s, '')), 'C')`,
		text, kind, title, tags, sourceType, metadata)
}

func metadataText(metadata json.RawMessage) string {
	if len(metadata) == 0 {
		return ""
	}
	var value any
	if json.Unmarshal(metadata, &value) != nil {
		return ""
	}
	return flattenJSON(value)
}

func flattenJSON(value any) string {
	switch typed := value.(type) {
	case map[string]any:
		parts := make([]string, 0, len(typed)*2)
		for key, child := range typed {
			parts = append(parts, key, flattenJSON(child))
		}
		return strings.Join(parts, " ")
	case []any:
		parts := make([]string, 0, len(typed))
		for _, child := range typed {
			parts = append(parts, flattenJSON(child))
		}
		return strings.Join(parts, " ")
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprint(typed)
	}
}
