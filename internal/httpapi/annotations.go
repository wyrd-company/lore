package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/wyrd-company/lore/internal/annotations"
)

func (s *Server) listAnnotations(w http.ResponseWriter, r *http.Request) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return
	}
	filters, ok := annotationFilters(w, r)
	if !ok {
		return
	}
	records, err := s.annotations.List(r.Context(), project.ID, filters)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "annotation retrieval failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": project.Slug, "annotations": records})
}

func (s *Server) createAnnotation(w http.ResponseWriter, r *http.Request) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return
	}
	var request annotations.CreateRequest
	if !decodeOne(w, r, &request) {
		return
	}
	record, err := s.annotations.Create(r.Context(), project.ID, request)
	if annotationError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, record)
}

func (s *Server) getAnnotation(w http.ResponseWriter, r *http.Request) {
	project, annotationID, ok := scopedAnnotation(w, r)
	if !ok {
		return
	}
	record, err := s.annotations.Get(r.Context(), project.ID, annotationID)
	if annotationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) updateAnnotation(w http.ResponseWriter, r *http.Request) {
	project, annotationID, ok := scopedAnnotation(w, r)
	if !ok {
		return
	}
	var request annotations.UpdateRequest
	if !decodeOne(w, r, &request) {
		return
	}
	record, err := s.annotations.Update(r.Context(), project.ID, annotationID, request)
	if annotationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, record)
}

func (s *Server) copyAnnotation(w http.ResponseWriter, r *http.Request) {
	s.retargetAnnotation(w, r, false)
}

func (s *Server) moveAnnotation(w http.ResponseWriter, r *http.Request) {
	s.retargetAnnotation(w, r, true)
}

func (s *Server) retargetAnnotation(w http.ResponseWriter, r *http.Request, move bool) {
	project, annotationID, ok := scopedAnnotation(w, r)
	if !ok {
		return
	}
	var request annotations.RetargetRequest
	if !decodeOne(w, r, &request) {
		return
	}
	var record annotations.Record
	var err error
	if move {
		record, err = s.annotations.Move(r.Context(), project.ID, annotationID, request)
	} else {
		record, err = s.annotations.Copy(r.Context(), project.ID, annotationID, request)
	}
	if annotationError(w, err) {
		return
	}
	status := http.StatusOK
	if !move {
		status = http.StatusCreated
	}
	writeJSON(w, status, record)
}

func (s *Server) annotationEvents(w http.ResponseWriter, r *http.Request) {
	project, annotationID, ok := scopedAnnotation(w, r)
	if !ok {
		return
	}
	if _, err := s.annotations.Get(r.Context(), project.ID, annotationID); annotationError(w, err) {
		return
	}
	events, err := s.annotations.Events(r.Context(), project.ID, annotationID)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "annotation event retrieval failed")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"annotationId": annotationID, "events": events})
}

func (s *Server) exportAnnotations(w http.ResponseWriter, r *http.Request) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return
	}
	after, err := nonnegativeInt64(r.URL.Query().Get("after"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "after must be a non-negative integer")
		return
	}
	export, err := s.annotations.Export(r.Context(), project.ID, project.Slug, after)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "annotation export failed")
		return
	}
	writeJSON(w, http.StatusOK, export)
}

func (s *Server) cleanupRevisions(w http.ResponseWriter, r *http.Request) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return
	}
	var request struct {
		RevisionID         *uuid.UUID `json:"revisionId,omitempty"`
		AttributedUsername string     `json:"attributedUsername"`
	}
	if !decodeOne(w, r, &request) {
		return
	}
	result, err := s.annotations.Cleanup(r.Context(), project.ID, request.RevisionID, request.AttributedUsername)
	if annotationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func scopedAnnotation(w http.ResponseWriter, r *http.Request) (Project, uuid.UUID, bool) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return Project{}, uuid.Nil, false
	}
	id, err := uuid.Parse(r.PathValue("annotation"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "annotation must be a UUID")
		return Project{}, uuid.Nil, false
	}
	return project, id, true
}

func annotationFilters(w http.ResponseWriter, r *http.Request) (annotations.Filters, bool) {
	filters := annotations.Filters{Status: r.URL.Query().Get("status")}
	if filters.Status != "" && filters.Status != "open" && filters.Status != "resolved" && filters.Status != "dismissed" {
		writeProblem(w, http.StatusBadRequest, "status must be open, resolved, or dismissed")
		return filters, false
	}
	for name, target := range map[string]**uuid.UUID{"documentId": &filters.DocumentID, "revisionId": &filters.RevisionID} {
		value := r.URL.Query().Get(name)
		if value == "" {
			continue
		}
		parsed, err := uuid.Parse(value)
		if err != nil {
			writeProblem(w, http.StatusBadRequest, name+" must be a UUID")
			return filters, false
		}
		*target = &parsed
	}
	after, err := nonnegativeInt64(r.URL.Query().Get("after"))
	if err != nil {
		writeProblem(w, http.StatusBadRequest, "after must be a non-negative integer")
		return filters, false
	}
	filters.After = after
	return filters, true
}

func nonnegativeInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 {
		return 0, errors.New("invalid non-negative integer")
	}
	return parsed, nil
}

func decodeOne(w http.ResponseWriter, r *http.Request, target any) bool {
	reader := http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeProblem(w, http.StatusBadRequest, "invalid request body")
		return false
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		writeProblem(w, http.StatusBadRequest, "request must contain one JSON object")
		return false
	}
	return true
}

func annotationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, annotations.ErrNotFound):
		writeProblem(w, http.StatusNotFound, "annotation or target revision not found")
	case errors.Is(err, annotations.ErrConflict):
		writeProblem(w, http.StatusConflict, err.Error())
	case errors.Is(err, annotations.ErrInvalid):
		writeProblem(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeProblem(w, http.StatusInternalServerError, "annotation operation failed")
	}
	return true
}
