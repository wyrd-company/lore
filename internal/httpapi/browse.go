package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/wyrd-company/lore/internal/browse"
)

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request) {
	projects, err := s.browse.Projects(r.Context())
	if err != nil {
		writeProblem(w, http.StatusServiceUnavailable, "project store unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": projects})
}

func (s *Server) browseProject(w http.ResponseWriter, r *http.Request) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return
	}
	response, err := s.browse.Browse(r.Context(), project.ID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "browse failed")
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (s *Server) documentDetail(w http.ResponseWriter, r *http.Request) {
	project, documentID, ok := scopedDocument(w, r)
	if !ok {
		return
	}
	document, err := s.browse.Document(r.Context(), project.ID, documentID)
	if browse.IsNotFound(err) {
		writeProblem(w, http.StatusNotFound, "document not found")
		return
	}
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "document retrieval failed")
		return
	}
	writeJSON(w, http.StatusOK, document)
}

func (s *Server) documentRevisions(w http.ResponseWriter, r *http.Request) {
	project, documentID, ok := scopedDocument(w, r)
	if !ok {
		return
	}
	revisions, err := s.browse.Revisions(r.Context(), project.ID, documentID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "revision retrieval failed")
		return
	}
	if len(revisions) == 0 {
		writeProblem(w, http.StatusNotFound, "document not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"documentId": documentID, "revisions": revisions})
}

func scopedDocument(w http.ResponseWriter, r *http.Request) (Project, uuid.UUID, bool) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return Project{}, uuid.Nil, false
	}
	documentID, err := uuid.Parse(r.PathValue("document"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "document must be a UUID")
		return Project{}, uuid.Nil, false
	}
	return project, documentID, true
}
