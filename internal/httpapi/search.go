package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wyrd-company/lore/internal/retrieval"
)

func (s *Server) searchProject(w http.ResponseWriter, r *http.Request) {
	project, ok := projectFromContext(r.Context())
	if !ok {
		writeProblem(w, http.StatusInternalServerError, "project scope missing")
		return
	}
	request, err := parseSearchRequest(r)
	if err != nil {
		writeProblem(w, http.StatusBadRequest, err.Error())
		return
	}
	var queryVector []float32
	var warning string
	if s.embedder != nil {
		embedContext, cancel := context.WithTimeout(r.Context(), 8*time.Second)
		vectors, embedErr := s.embedder.Embed(embedContext, []string{request.Query})
		cancel()
		if embedErr == nil && len(vectors) == 1 {
			queryVector = vectors[0]
		} else {
			warning = "semantic retrieval is temporarily unavailable; results use keyword ranking"
		}
	} else {
		warning = "semantic retrieval is not configured; results use keyword ranking"
	}
	response, err := s.search.Search(r.Context(), project.ID, request, queryVector)
	if err != nil {
		writeProblem(w, http.StatusInternalServerError, "search failed")
		return
	}
	if warning != "" {
		response.Warnings = []string{warning}
	}
	writeJSON(w, http.StatusOK, response)
}

func parseSearchRequest(r *http.Request) (retrieval.Request, error) {
	query := r.URL.Query()
	request := retrieval.Request{
		Query: query.Get("q"),
		Filters: retrieval.Filters{
			SourceTypes: listValues(query["sourceType"]), Branches: listValues(query["branch"]),
			Repositories: listValues(query["repository"]), Tags: listValues(query["tag"]),
		},
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit <= 0 {
			return request, fmt.Errorf("limit must be a positive integer")
		}
		request.Limit = limit
	}
	var err error
	request.Filters.CreatedFrom, err = parseOptionalTime(query.Get("createdFrom"))
	if err != nil {
		return request, fmt.Errorf("createdFrom must be RFC3339")
	}
	request.Filters.CreatedTo, err = parseOptionalTime(query.Get("createdTo"))
	if err != nil {
		return request, fmt.Errorf("createdTo must be RFC3339")
	}
	if strings.TrimSpace(request.Query) == "" {
		return request, fmt.Errorf("q is required")
	}
	return request, nil
}

func listValues(values []string) []string {
	var result []string
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				result = append(result, part)
			}
		}
	}
	return result
}

func parseOptionalTime(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	return &parsed, err
}
