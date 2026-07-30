package api

import (
	"context"
	"net/http"
	"time"

	"github.com/swapnil404/orca/server/internal/store"
)

type backupStore interface {
	ListBackupJobs(context.Context, string, string, time.Time) ([]store.BackupJob, error)
}

// BackupHandler serves organization-membership-scoped backup rollups.
type BackupHandler struct {
	store backupStore
	now   func() time.Time
}

// NewBackupHandler creates the global backup API handler.
func NewBackupHandler(backups backupStore) *BackupHandler {
	return &BackupHandler{store: backups, now: time.Now}
}

// RegisterRoutes registers authenticated backup routes on mux.
func (h *BackupHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /backups", h.list)
}

func (h *BackupHandler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	jobs, err := h.store.ListBackupJobs(r.Context(), userID, r.URL.Query().Get("project_id"), h.now())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, jobs)
}
