package httpui

import (
	"net/http"
	"strconv"
)

func (h *Handler) UnitHistory(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.UnitHistory(r.Context(), r.PathValue("id"), r.PathValue("unitID"))
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) RelationHistory(w http.ResponseWriter, r *http.Request) {
	result, err := h.service.RelationHistory(r.Context(), r.PathValue("id"), r.PathValue("relationID"))
	respond(w, result, err, http.StatusOK)
}

func (h *Handler) AuditPage(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	before, _ := strconv.ParseInt(r.URL.Query().Get("before"), 10, 64)
	result, err := h.service.AuditPage(r.Context(), r.PathValue("id"), limit, before)
	respond(w, result, err, http.StatusOK)
}
