package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/wyrd-company/lore/internal/browse"
	"github.com/wyrd-company/lore/internal/embedding"
	"github.com/wyrd-company/lore/internal/retrieval"
	"github.com/wyrd-company/lore/internal/synchronization"
	webassets "github.com/wyrd-company/lore/web"
)

type Server struct {
	pool        *pgxpool.Pool
	sync        *synchronization.Repository
	search      *retrieval.Repository
	browse      *browse.Repository
	embedder    *embedding.Client
	ingestToken string
	adminToken  string
}

func New(pool *pgxpool.Pool, ingestToken, adminToken string, embedders ...*embedding.Client) http.Handler {
	var embedder *embedding.Client
	if len(embedders) > 0 {
		embedder = embedders[0]
	}
	server := &Server{
		pool: pool, sync: synchronization.NewRepository(pool), search: retrieval.NewRepository(pool), browse: browse.NewRepository(pool), embedder: embedder,
		ingestToken: ingestToken, adminToken: adminToken,
	}
	mux := http.NewServeMux()

	// Browse/search boundary. All handlers receive a resolved project in context.
	mux.Handle("GET /api/projects", http.HandlerFunc(server.listProjects))
	mux.Handle("GET /api/projects/{project}/browse", projectScope(pool, http.HandlerFunc(server.browseProject)))
	mux.Handle("GET /api/projects/{project}/search", projectScope(pool, http.HandlerFunc(server.searchProject)))
	mux.Handle("GET /api/projects/{project}/documents/{document}", projectScope(pool, http.HandlerFunc(server.documentDetail)))
	mux.Handle("GET /api/projects/{project}/documents/{document}/revisions", projectScope(pool, http.HandlerFunc(server.documentRevisions)))

	// Annotation boundary.
	mux.Handle("GET /api/projects/{project}/annotations", projectScope(pool, http.HandlerFunc(server.stubAnnotations)))
	mux.Handle("POST /api/projects/{project}/annotations", projectScope(pool, http.HandlerFunc(server.stubAnnotations)))
	mux.Handle("PATCH /api/projects/{project}/annotations/{annotation}", projectScope(pool, http.HandlerFunc(server.stubAnnotations)))
	mux.Handle("GET /api/projects/{project}/annotations/export", projectScope(pool, http.HandlerFunc(server.stubAnnotations)))

	// Synchronization boundary, protected by the ingest credential.
	mux.Handle("POST /api/projects/{project}/synchronizations",
		bearerToken(ingestToken, projectScope(pool, http.HandlerFunc(server.synchronize))))

	// Administrative operations have their own credential and remain project scoped.
	mux.Handle("POST /api/projects/{project}/admin/cleanup",
		bearerToken(adminToken, projectScope(pool, http.HandlerFunc(server.stubAdmin))))

	mux.HandleFunc("GET /health/live", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) })
	mux.HandleFunc("GET /health/ready", server.ready)
	mux.Handle("/", spaHandler())
	return mux
}

func (s *Server) synchronize(w http.ResponseWriter, r *http.Request) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return
	}
	reader := http.MaxBytesReader(w, r.Body, 256<<20)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var manifest synchronization.Manifest
	if err := decoder.Decode(&manifest); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid synchronization manifest")
		return
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, http.StatusBadRequest, "synchronization request must contain one manifest")
		return
	}
	if manifest.Project != project.Slug {
		writeProblem(w, http.StatusBadRequest, "manifest project does not match route project")
		return
	}
	if err := manifest.Validate(); err != nil {
		writeProblem(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	result, err := s.sync.Apply(r.Context(), project.ID, manifest)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "synchronization failed")
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) ready(w http.ResponseWriter, r *http.Request) {
	if err := s.pool.Ping(r.Context()); err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "database unavailable")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) stubAnnotations(w http.ResponseWriter, r *http.Request) {
	writeStub(w, r, "annotations")
}

func (s *Server) stubAdmin(w http.ResponseWriter, r *http.Request) {
	writeStub(w, r, "administration")
}

func writeStub(w http.ResponseWriter, r *http.Request, boundary string) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return
	}
	writeJSON(w, http.StatusNotImplemented, map[string]string{
		"boundary": boundary, "project": project.Slug, "status": "planned for a later milestone",
	})
}

func writeProblem(w http.ResponseWriter, status int, detail string) {
	writeJSON(w, status, map[string]any{"status": status, "detail": detail})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value) //nolint:errcheck
}

func spaHandler() http.Handler {
	dist, err := fs.Sub(webassets.Dist, "dist")
	if err != nil {
		panic(err)
	}
	files := http.FileServer(http.FS(dist))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "." {
			name = "index.html"
		}
		file, openErr := dist.Open(name)
		if openErr == nil {
			file.Close()
			files.ServeHTTP(w, r)
			return
		}
		if !errors.Is(openErr, fs.ErrNotExist) {
			writeProblem(w, http.StatusInternalServerError, "static asset unavailable")
			return
		}
		index, indexErr := fs.ReadFile(dist, "index.html")
		if indexErr != nil {
			writeProblem(w, http.StatusInternalServerError, "web application unavailable")
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(index) //nolint:errcheck
	})
}
