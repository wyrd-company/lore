package embedding

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/indexing"
)

type Worker struct {
	pool   *pgxpool.Pool
	client *Client
}

type job struct {
	ChunkID   uuid.UUID
	ProjectID uuid.UUID
	Text      string
	Attempts  int
}

func NewWorker(pool *pgxpool.Pool, client *Client) *Worker {
	return &Worker{pool: pool, client: client}
}

func (w *Worker) Run(ctx context.Context) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		processed, err := w.ProcessOnce(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Warn("embedding backfill failed", "error", err)
		}
		if processed > 0 {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (w *Worker) ProcessOnce(ctx context.Context) (int, error) {
	jobs, err := w.claim(ctx, maxBatchSize)
	if err != nil || len(jobs) == 0 {
		return len(jobs), err
	}
	inputs := make([]string, len(jobs))
	for index := range jobs {
		inputs[index] = jobs[index].Text
	}
	vectors, err := w.client.Embed(ctx, inputs)
	if err != nil {
		if recordErr := w.recordFailure(context.WithoutCancel(ctx), jobs, err); recordErr != nil {
			return len(jobs), fmt.Errorf("embed chunks: %v; record failure: %w", err, recordErr)
		}
		return len(jobs), fmt.Errorf("embed chunks: %w", err)
	}
	if err := w.store(ctx, jobs, vectors); err != nil {
		return len(jobs), err
	}
	return len(jobs), nil
}

func (w *Worker) claim(ctx context.Context, limit int) ([]job, error) {
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
	rows, err := tx.Query(ctx, `
		SELECT j.chunk_id, j.project_id, c.normalized_text, j.attempts
		FROM embedding_jobs j
		JOIN chunks c ON c.id = j.chunk_id AND c.project_id = j.project_id
		WHERE j.available_at <= now()
		  AND (j.locked_until IS NULL OR j.locked_until < now())
		ORDER BY j.created_at
		LIMIT $1
		FOR UPDATE OF j SKIP LOCKED`, limit)
	if err != nil {
		return nil, err
	}
	jobs, err := pgx.CollectRows(rows, pgx.RowToStructByPos[job])
	if err != nil {
		return nil, err
	}
	for _, item := range jobs {
		if _, err := tx.Exec(ctx, `UPDATE embedding_jobs SET locked_until = now() + interval '2 minutes', updated_at = now() WHERE chunk_id = $1`, item.ChunkID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (w *Worker) store(ctx context.Context, jobs []job, vectors [][]float32) error {
	tx, err := w.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(context.WithoutCancel(ctx)) //nolint:errcheck
	for index, item := range jobs {
		if len(vectors[index]) != indexing.Dimensions {
			return fmt.Errorf("embedding for chunk %s has %d dimensions", item.ChunkID, len(vectors[index]))
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO embeddings (project_id, chunk_id, model, dimensions, embedding)
			SELECT c.project_id, c.id, $3, $4, $5::vector
			FROM chunks c
			WHERE c.project_id = $1 AND c.id = $2
			ON CONFLICT (chunk_id, model) DO UPDATE
			SET embedding = EXCLUDED.embedding, dimensions = EXCLUDED.dimensions, created_at = now()`,
			item.ProjectID, item.ChunkID, indexing.Model, indexing.Dimensions, vectorLiteral(vectors[index])); err != nil {
			return fmt.Errorf("store chunk embedding: %w", err)
		}
		if _, err := tx.Exec(ctx, `DELETE FROM embedding_jobs WHERE chunk_id = $1`, item.ChunkID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (w *Worker) recordFailure(ctx context.Context, jobs []job, cause error) error {
	for _, item := range jobs {
		delay := min(time.Second*time.Duration(1<<min(item.Attempts, 10)), time.Hour)
		if _, err := w.pool.Exec(ctx, `
			UPDATE embedding_jobs
			SET attempts = attempts + 1, available_at = now() + $2::interval,
				locked_until = NULL, last_error = $3, updated_at = now()
			WHERE chunk_id = $1`, item.ChunkID, delay.String(), truncateError(cause)); err != nil {
			return err
		}
	}
	return nil
}

func vectorLiteral(vector []float32) string {
	var output strings.Builder
	output.WriteByte('[')
	for index, value := range vector {
		if index > 0 {
			output.WriteByte(',')
		}
		fmt.Fprintf(&output, "%g", value)
	}
	output.WriteByte(']')
	return output.String()
}

func truncateError(err error) string {
	value := err.Error()
	if len(value) > 2000 {
		return value[:2000]
	}
	return value
}
