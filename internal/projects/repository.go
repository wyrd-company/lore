package projects

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Project struct {
	ID        uuid.UUID `json:"id"`
	Slug      string    `json:"slug"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type CreateRequest struct {
	Slug string `json:"slug"`
	Name string `json:"name"`
}

func (request CreateRequest) Validate() error {
	if !slugPattern.MatchString(request.Slug) {
		return errors.New("slug must contain lowercase letters, numbers, and hyphens")
	}
	if strings.TrimSpace(request.Name) == "" {
		return errors.New("name is required")
	}
	return nil
}

type Repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func (repository *Repository) Create(ctx context.Context, request CreateRequest) (Project, bool, error) {
	if err := request.Validate(); err != nil {
		return Project{}, false, err
	}
	var project Project
	err := repository.pool.QueryRow(ctx, `
		INSERT INTO projects (slug, name)
		VALUES ($1, $2)
		ON CONFLICT (slug) DO NOTHING
		RETURNING id, slug, name, created_at, updated_at`, request.Slug, strings.TrimSpace(request.Name)).
		Scan(&project.ID, &project.Slug, &project.Name, &project.CreatedAt, &project.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		err = repository.pool.QueryRow(ctx, `
			SELECT id, slug, name, created_at, updated_at FROM projects WHERE slug = $1`, request.Slug).
			Scan(&project.ID, &project.Slug, &project.Name, &project.CreatedAt, &project.UpdatedAt)
		return project, false, err
	}
	return project, true, err
}
