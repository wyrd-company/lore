package httpapi

import (
	"context"

	"github.com/google/uuid"
)

type Project struct {
	ID   uuid.UUID
	Slug string
	Name string
}

type projectContextKey struct{}

func withProject(ctx context.Context, project Project) context.Context {
	return context.WithValue(ctx, projectContextKey{}, project)
}

func projectFromContext(ctx context.Context) (Project, bool) {
	project, ok := ctx.Value(projectContextKey{}).(Project)
	return project, ok
}
