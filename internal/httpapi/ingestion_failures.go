/*
---
relationships:

	implements: system

---
*/
package httpapi

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/wyrd-company/lore/internal/ingestfailures"
)

func (server *Server) listIngestionFailures(writer http.ResponseWriter, request *http.Request) {
	project, ok := projectFromContext(request.Context())
	if !ok {
		writeProblem(writer, http.StatusInternalServerError, "project scope missing")
		return
	}
	filters := ingestfailures.Filters{
		SourceType: request.URL.Query().Get("sourceType"), SourceInstance: request.URL.Query().Get("sourceInstance"),
	}
	records, err := server.failures.List(request.Context(), project.ID, filters)
	if err != nil {
		writeProblem(writer, http.StatusInternalServerError, "ingestion failure retrieval failed")
		return
	}
	writeJSON(writer, http.StatusOK, map[string]any{"project": project.Slug, "failures": records})
}

func (server *Server) deleteIngestionFailure(writer http.ResponseWriter, request *http.Request) {
	project, ok := projectFromContext(request.Context())
	if !ok {
		writeProblem(writer, http.StatusInternalServerError, "project scope missing")
		return
	}
	failureID, err := uuid.Parse(request.PathValue("failure"))
	if err != nil {
		writeProblem(writer, http.StatusBadRequest, "failure must be a UUID")
		return
	}
	if err := server.failures.Delete(request.Context(), project.ID, failureID); err != nil {
		if ingestfailures.IsNotFound(err) {
			writeProblem(writer, http.StatusNotFound, "ingestion failure not found")
			return
		}
		writeProblem(writer, http.StatusInternalServerError, "ingestion failure removal failed")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}
