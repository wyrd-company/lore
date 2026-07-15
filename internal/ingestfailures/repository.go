/*
---
relationships:

	implements: system

---
*/
package ingestfailures

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("ingestion failure not found")

type Record struct {
	ID             uuid.UUID `json:"id"`
	ProjectID      uuid.UUID `json:"projectId"`
	SourceType     string    `json:"sourceType"`
	SourceInstance string    `json:"sourceInstance"`
	Path           string    `json:"path"`
	Message        string    `json:"message"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

type Filters struct {
	SourceType     string
	SourceInstance string
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) List(ctx context.Context, projectID uuid.UUID, filters Filters) ([]Record, error) {
	rows, err := repository.pool.Query(ctx, `
		SELECT f.id, f.project_id, s.source_type, s.external_key, f.path, f.message, f.created_at, f.updated_at
		FROM ingestion_failures f
		JOIN source_instances s ON s.id = f.source_instance_id AND s.project_id = f.project_id
		WHERE f.project_id = $1
		  AND ($2 = '' OR s.source_type = $2)
		  AND ($3 = '' OR s.external_key = $3)
		ORDER BY f.updated_at DESC, f.path, f.id`, projectID, filters.SourceType, filters.SourceInstance)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	records := make([]Record, 0)
	for rows.Next() {
		var record Record
		if err := rows.Scan(&record.ID, &record.ProjectID, &record.SourceType, &record.SourceInstance,
			&record.Path, &record.Message, &record.CreatedAt, &record.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func (repository *Repository) Delete(ctx context.Context, projectID, failureID uuid.UUID) error {
	result, err := repository.pool.Exec(ctx, `DELETE FROM ingestion_failures WHERE project_id = $1 AND id = $2`, projectID, failureID)
	if err != nil {
		return err
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func IsNotFound(err error) bool { return errors.Is(err, ErrNotFound) || errors.Is(err, pgx.ErrNoRows) }
