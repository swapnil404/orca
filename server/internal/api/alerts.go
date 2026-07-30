package api

import (
	"context"
	"net/http"

	"github.com/swapnil404/orca/server/internal/store"
)

type alertStore interface {
	GetProject(context.Context, string, string) (store.Project, error)
	ListAlertRulesForProject(context.Context, string, string) ([]store.AlertRule, error)
	ListAlertIncidentsForProject(context.Context, string) ([]store.AlertIncident, error)
	ListAlertIncidentsForUser(context.Context, string) ([]store.GlobalAlertIncident, error)
}

// AlertHandler serves project-scoped alert rules and incident history.
type AlertHandler struct {
	store alertStore
}

// NewAlertHandler creates an alert API handler.
func NewAlertHandler(alerts alertStore) *AlertHandler {
	return &AlertHandler{store: alerts}
}

// RegisterRoutes registers project alert routes on mux.
func (h *AlertHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /alerts", h.listGlobal)
	mux.HandleFunc("GET /projects/{projectID}/alerts", h.list)
}

func (h *AlertHandler) listGlobal(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	incidents, err := h.store.ListAlertIncidentsForUser(r.Context(), userID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, incidents)
}

func (h *AlertHandler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	projectID := r.PathValue("projectID")
	if _, err := h.store.GetProject(r.Context(), userID, projectID); err != nil {
		writeStoreError(w, err)
		return
	}
	rules, err := h.store.ListAlertRulesForProject(r.Context(), userID, projectID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	incidents, err := h.store.ListAlertIncidentsForProject(r.Context(), projectID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Rules     []store.AlertRule     `json:"rules"`
		Incidents []store.AlertIncident `json:"incidents"`
	}{Rules: rules, Incidents: incidents})
}
