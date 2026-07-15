/*
---
relationships:

	implements: system

---
*/
package briefings

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrSettingNotFound = errors.New("briefing not found")
	ErrInvalidSetting  = errors.New("invalid briefing setting")
)

type Setting struct {
	DocumentID uuid.UUID `json:"documentId"`
	Category   string    `json:"category"`
	Home       bool      `json:"home"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type UpdateSettingRequest struct {
	Category *string `json:"category,omitempty"`
	Home     *bool   `json:"home,omitempty"`
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) UpdateSetting(ctx context.Context, projectID, documentID uuid.UUID, request UpdateSettingRequest) (setting Setting, err error) {
	if request.Category == nil && request.Home == nil {
		return setting, ErrInvalidSetting
	}
	tx, err := repository.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return setting, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback(context.WithoutCancel(ctx))
		}
	}()
	var sourceType string
	if err = tx.QueryRow(ctx, `SELECT source_type FROM documents WHERE project_id = $1 AND id = $2 AND deleted_at IS NULL FOR UPDATE`, projectID, documentID).Scan(&sourceType); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return setting, ErrSettingNotFound
		}
		return setting, err
	}
	if sourceType != "briefing" {
		return setting, ErrSettingNotFound
	}
	var category *string
	var home bool
	err = tx.QueryRow(ctx, `SELECT category, is_home FROM briefing_settings WHERE project_id = $1 AND document_id = $2`, projectID, documentID).Scan(&category, &home)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	} else if err != nil {
		return setting, err
	}
	if request.Category != nil {
		trimmed := strings.TrimSpace(*request.Category)
		category = nil
		if trimmed != "" {
			category = &trimmed
		}
	}
	if request.Home != nil {
		home = *request.Home
	}
	if home {
		if _, err = tx.Exec(ctx, `UPDATE briefing_settings SET is_home = false, updated_at = now() WHERE project_id = $1 AND document_id <> $2 AND is_home`, projectID, documentID); err != nil {
			return setting, err
		}
	}
	if category == nil && !home {
		if _, err = tx.Exec(ctx, `DELETE FROM briefing_settings WHERE project_id = $1 AND document_id = $2`, projectID, documentID); err != nil {
			return setting, err
		}
		setting = Setting{DocumentID: documentID}
	} else if err = tx.QueryRow(ctx, `
		INSERT INTO briefing_settings (project_id, document_id, category, is_home)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (project_id, document_id)
		DO UPDATE SET category = EXCLUDED.category, is_home = EXCLUDED.is_home, updated_at = now()
		RETURNING document_id, coalesce(category, ''), is_home, updated_at`, projectID, documentID, category, home).
		Scan(&setting.DocumentID, &setting.Category, &setting.Home, &setting.UpdatedAt); err != nil {
		return setting, err
	}
	if err = tx.Commit(ctx); err != nil {
		return setting, err
	}
	return setting, nil
}

func IsSettingNotFound(err error) bool { return errors.Is(err, ErrSettingNotFound) }
