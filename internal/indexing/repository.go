package indexing

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
			chunk.TokenCount, chunk.Kind, input.Title, strings.Join(input.Tags, " "), metadataText(input.Metadata, input.SourceType),
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
		metadataText(input.Metadata, input.SourceType), input.SourceType)
	if err != nil {
		return fmt.Errorf("refresh keyword index: %w", err)
	}
	return nil
}

func BackfillCurrentRevisions(ctx context.Context, pool *pgxpool.Pool) (int, error) {
	rows, err := pool.Query(ctx, `
		SELECT d.project_id, r.id, d.source_type, d.title, r.normalized_text, r.metadata,
			coalesce(array_agg(t.name ORDER BY t.name) FILTER (WHERE t.name IS NOT NULL), ARRAY[]::text[])
		FROM documents d
		JOIN revisions r ON r.id = d.current_revision_id
		LEFT JOIN chunks c ON c.revision_id = r.id
		LEFT JOIN document_tags dt ON dt.document_id = d.id AND dt.project_id = d.project_id
		LEFT JOIN tags t ON t.id = dt.tag_id AND t.project_id = d.project_id
		WHERE d.deleted_at IS NULL AND c.id IS NULL
		GROUP BY d.project_id, r.id, d.source_type, d.title, r.normalized_text, r.metadata`)
	if err != nil {
		return 0, fmt.Errorf("find revisions requiring chunk backfill: %w", err)
	}
	inputs, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (RevisionInput, error) {
		var input RevisionInput
		err := row.Scan(&input.ProjectID, &input.RevisionID, &input.SourceType, &input.Title,
			&input.NormalizedText, &input.Metadata, &input.Tags)
		return input, err
	})
	if err != nil {
		return 0, err
	}
	indexed := 0
	for _, input := range inputs {
		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `SELECT id FROM revisions WHERE id = $1 FOR UPDATE`, input.RevisionID); err != nil {
			tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
			return 0, err
		}
		var alreadyIndexed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM chunks WHERE revision_id = $1)`, input.RevisionID).Scan(&alreadyIndexed); err != nil {
			tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
			return 0, err
		}
		if alreadyIndexed {
			tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
			continue
		}
		if err := IndexRevision(ctx, tx, input); err != nil {
			tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
			return 0, err
		}
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		indexed++
	}
	return indexed, nil
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

func metadataText(metadata json.RawMessage, sourceType string) string {
	if len(metadata) == 0 {
		return ""
	}
	var value any
	if json.Unmarshal(metadata, &value) != nil {
		return ""
	}
	if sourceType == "conversation" {
		if object, ok := value.(map[string]any); ok {
			delete(object, "messages")
		}
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
