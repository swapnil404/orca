package api

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/swapnil404/orca/server/internal/store"
)

const maxIdempotencyKeyBytes = 200

type restoreStore interface {
	CreateRestoreOperation(context.Context, store.CreateRestoreOperationParams) (store.RestoreOperation, bool, error)
	ListRestoreOperations(context.Context, string, string) ([]store.RestoreOperation, error)
	GetRestoreOperation(context.Context, string, string) (store.RestoreOperation, error)
	ConfirmRestoreOperation(context.Context, string, string, string) (store.RestoreOperation, error)
	CancelRestoreOperation(context.Context, string, string) (store.RestoreOperation, error)
	RollbackRestoreOperation(context.Context, string, string, string) (store.RestoreOperation, error)
	FinalizeRestoreOperation(context.Context, string, string, string) (store.RestoreOperation, error)
}

// RestoreHandler serves durable restore operation lifecycle endpoints.
type RestoreHandler struct {
	store    restoreStore
	random   io.Reader
	pusher   desiredStatePusher
	notifier projectChangeNotifier
}

// NewRestoreHandler creates the restore control-plane API.
func NewRestoreHandler(restores restoreStore, pusher desiredStatePusher, notifier projectChangeNotifier) *RestoreHandler {
	return &RestoreHandler{store: restores, random: rand.Reader, pusher: pusher, notifier: notifier}
}

// RegisterRoutes registers authenticated restore routes.
func (h *RestoreHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /clusters/{clusterID}/restore-operations", h.create)
	mux.HandleFunc("GET /projects/{projectID}/restore-operations", h.list)
	mux.HandleFunc("GET /restore-operations/{operationID}", h.get)
	mux.HandleFunc("POST /restore-operations/{operationID}/confirm", h.confirm)
	mux.HandleFunc("POST /restore-operations/{operationID}/cancel", h.cancel)
	mux.HandleFunc("POST /restore-operations/{operationID}/rollback", h.rollback)
	mux.HandleFunc("POST /restore-operations/{operationID}/finalize", h.finalize)
}

type createRestoreRequest struct {
	Mode              string `json:"mode"`
	TargetTime        string `json:"target_time"`
	TargetClusterName string `json:"target_cluster_name,omitempty"`
}

type restoreConfirmationRequest struct {
	Confirmation string `json:"confirmation"`
}

func (h *RestoreHandler) create(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	idempotencyKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" || len(idempotencyKey) > maxIdempotencyKeyBytes || strings.ContainsAny(idempotencyKey, "\x00\r\n") {
		writeError(w, http.StatusBadRequest, "a valid Idempotency-Key header is required")
		return
	}
	var request createRestoreRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	if request.Mode != "in_place" && request.Mode != "clone" {
		writeError(w, http.StatusBadRequest, "mode must be in_place or clone")
		return
	}
	targetTime, err := time.Parse(time.RFC3339Nano, request.TargetTime)
	if err != nil {
		writeError(w, http.StatusBadRequest, "target_time must be RFC3339")
		return
	}
	request.TargetClusterName = strings.TrimSpace(request.TargetClusterName)
	if request.Mode == "clone" && request.TargetClusterName == "" {
		writeError(w, http.StatusBadRequest, "target_cluster_name is required for clone mode")
		return
	}
	if request.Mode == "in_place" && request.TargetClusterName != "" {
		writeError(w, http.StatusBadRequest, "target_cluster_name is only valid for clone mode")
		return
	}
	operationID, err := randomID(h.random)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate operation ID")
		return
	}
	targetID := ""
	if request.Mode == "clone" {
		targetID, err = randomID(h.random)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to reserve target cluster ID")
			return
		}
	}
	fingerprintPayload, _ := json.Marshal(struct {
		SourceClusterID   string `json:"source_cluster_id"`
		Mode              string `json:"mode"`
		TargetTime        string `json:"target_time"`
		TargetClusterName string `json:"target_cluster_name,omitempty"`
	}{r.PathValue("clusterID"), request.Mode, targetTime.UTC().Format(time.RFC3339Nano), request.TargetClusterName})
	fingerprintBytes := sha256.Sum256(fingerprintPayload)
	operation, created, err := h.store.CreateRestoreOperation(r.Context(), store.CreateRestoreOperationParams{
		ID: operationID, UserID: userID, SourceClusterID: r.PathValue("clusterID"),
		TargetClusterID: targetID, TargetClusterName: request.TargetClusterName, Mode: request.Mode,
		TargetTime: targetTime.UTC(), RequestFingerprint: hex.EncodeToString(fingerprintBytes[:]), IdempotencyKey: idempotencyKey,
	})
	if err != nil {
		writeRestoreError(w, err)
		return
	}
	h.publish(r.Context(), operation)
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	writeJSON(w, status, operation)
}

func (h *RestoreHandler) list(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	operations, err := h.store.ListRestoreOperations(r.Context(), userID, r.PathValue("projectID"))
	if err != nil {
		writeRestoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operations)
}

func (h *RestoreHandler) get(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	operation, err := h.store.GetRestoreOperation(r.Context(), userID, r.PathValue("operationID"))
	if err != nil {
		writeRestoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, operation)
}

func (h *RestoreHandler) confirm(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request restoreConfirmationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	operation, err := h.store.ConfirmRestoreOperation(r.Context(), userID, r.PathValue("operationID"), request.Confirmation)
	if err != nil {
		writeRestoreError(w, err)
		return
	}
	h.publish(r.Context(), operation)
	writeJSON(w, http.StatusOK, operation)
}

func (h *RestoreHandler) cancel(w http.ResponseWriter, r *http.Request) {
	h.mutate(w, r, h.store.CancelRestoreOperation)
}

func (h *RestoreHandler) rollback(w http.ResponseWriter, r *http.Request) {
	h.confirmedMutation(w, r, h.store.RollbackRestoreOperation)
}

func (h *RestoreHandler) finalize(w http.ResponseWriter, r *http.Request) {
	h.confirmedMutation(w, r, h.store.FinalizeRestoreOperation)
}

func (h *RestoreHandler) mutate(w http.ResponseWriter, r *http.Request, mutation func(context.Context, string, string) (store.RestoreOperation, error)) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	operation, err := mutation(r.Context(), userID, r.PathValue("operationID"))
	if err != nil {
		writeRestoreError(w, err)
		return
	}
	h.publish(r.Context(), operation)
	writeJSON(w, http.StatusOK, operation)
}

func (h *RestoreHandler) confirmedMutation(w http.ResponseWriter, r *http.Request, mutation func(context.Context, string, string, string) (store.RestoreOperation, error)) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var request restoreConfirmationRequest
	if !decodeJSON(w, r, &request) {
		return
	}
	operation, err := mutation(r.Context(), userID, r.PathValue("operationID"), request.Confirmation)
	if err != nil {
		writeRestoreError(w, err)
		return
	}
	h.publish(r.Context(), operation)
	writeJSON(w, http.StatusOK, operation)
}

func (h *RestoreHandler) publish(ctx context.Context, operation store.RestoreOperation) {
	if h.pusher != nil {
		_ = h.pusher.PushDesiredState(ctx, operation.HostID)
	}
	if h.notifier != nil {
		_ = h.notifier.NotifyProjectChange(ctx, operation.ProjectID)
	}
}

func writeRestoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrRestoreMutationForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, store.ErrRestoreInvalidConfirmation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrRestoreIdempotencyConflict), errors.Is(err, store.ErrRestoreOperationConflict), errors.Is(err, store.ErrRestoreInvalidTransition):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, store.ErrRestorePgBackRestRequired):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		writeStoreError(w, err)
	}
}
