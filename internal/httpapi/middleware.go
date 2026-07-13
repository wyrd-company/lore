package httpapi

import (
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func projectScope(pool *pgxpool.Pool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slug := r.PathValue("project")
		if slug == "" {
			writeProblem(w, http.StatusBadRequest, "project is required")
			return
		}
		var project Project
		if err := pool.QueryRow(r.Context(), `SELECT id, slug, name FROM projects WHERE slug = $1`, slug).
			Scan(&project.ID, &project.Slug, &project.Name); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeProblem(w, http.StatusNotFound, "project not found")
			} else {
				writeProblem(w, http.StatusServiceUnavailable, "project store unavailable")
			}
			return
		}
		next.ServeHTTP(w, r.WithContext(withProject(r.Context(), project)))
	})
}

func bearerToken(expected string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if provided == r.Header.Get("Authorization") || expected == "" || len(provided) != len(expected) ||
			subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeProblem(w, http.StatusUnauthorized, "valid bearer token required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
