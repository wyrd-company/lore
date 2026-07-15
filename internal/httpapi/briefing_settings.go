/*
---
relationships:

	implements: system

---
*/
package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/wyrd-company/lore/internal/briefings"
)

func (server *Server) updateBriefingSetting(writer http.ResponseWriter, request *http.Request) {
	project, ok := projectFromContext(request.Context())
	if !ok {
		writeProblem(writer, http.StatusInternalServerError, "project scope missing")
		return
	}
	documentID, err := uuid.Parse(request.PathValue("document"))
	if err != nil {
		writeProblem(writer, http.StatusBadRequest, "document must be a UUID")
		return
	}
	var update briefings.UpdateSettingRequest
	if !decodeOne(writer, request, &update) {
		return
	}
	setting, err := server.briefings.UpdateSetting(request.Context(), project.ID, documentID, update)
	if err != nil {
		switch {
		case briefings.IsSettingNotFound(err):
			writeProblem(writer, http.StatusNotFound, "briefing not found")
		case errors.Is(err, briefings.ErrInvalidSetting):
			writeProblem(writer, http.StatusUnprocessableEntity, "category or home is required")
		default:
			writeProblem(writer, http.StatusInternalServerError, "briefing setting update failed")
		}
		return
	}
	writeJSON(writer, http.StatusOK, setting)
}
