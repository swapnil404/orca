package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/swapnil404/orca/server/internal/auth"
	"github.com/swapnil404/orca/server/internal/store"
)

const (
	hostIDBytes   = 18
	tokenLifetime = 24 * time.Hour
)

type hostCreator interface {
	CreateHost(context.Context, store.CreateHostParams) (store.Host, error)
	RotateHostToken(context.Context, string, string, []byte, time.Time) (bool, error)
	DeleteUnusedHost(context.Context, string, string) (bool, error)
}

// HostRegistrationHandler registers hosts and returns their one-time agent command.
type HostRegistrationHandler struct {
	hosts     hostCreator
	serverURL string
	now       func() time.Time
	random    io.Reader
}

// NewHostRegistrationHandler creates a host registration endpoint.
func NewHostRegistrationHandler(hosts hostCreator, serverURL string) *HostRegistrationHandler {
	return &HostRegistrationHandler{
		hosts:     hosts,
		serverURL: serverURL,
		now:       time.Now,
		random:    rand.Reader,
	}
}

// RegisterRoutes registers authenticated host lifecycle routes.
func (h *HostRegistrationHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("POST /hosts", h)
	mux.HandleFunc("POST /hosts/{hostID}/token", h.rotateToken)
	mux.HandleFunc("DELETE /hosts/{hostID}", h.deleteUnused)
}

// ServeHTTP registers a host for the current user.
func (h *HostRegistrationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	hostID, err := randomID(h.random)
	if err != nil {
		http.Error(w, "failed to generate host ID", http.StatusInternalServerError)
		return
	}
	token, err := auth.GenerateAgentToken()
	if err != nil {
		http.Error(w, "failed to generate host token", http.StatusInternalServerError)
		return
	}

	now := h.now().UTC()
	host, err := h.hosts.CreateHost(r.Context(), store.CreateHostParams{
		ID:             hostID,
		UserID:         userID,
		TokenHash:      auth.HashAgentToken(token),
		TokenExpiresAt: now.Add(tokenLifetime),
		Status:         store.HostStatusNeverConnected,
	})
	if err != nil {
		http.Error(w, "failed to register host", http.StatusInternalServerError)
		return
	}

	h.writeRegistration(w, host.ID, host.Status, token)
}

func (h *HostRegistrationHandler) rotateToken(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	token, err := auth.GenerateAgentToken()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate host token")
		return
	}
	updated, err := h.hosts.RotateHostToken(r.Context(), r.PathValue("hostID"), userID, auth.HashAgentToken(token), h.now().UTC().Add(tokenLifetime))
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !updated {
		writeError(w, http.StatusNotFound, "host not found or already connected")
		return
	}
	h.writeRegistration(w, r.PathValue("hostID"), store.HostStatusNeverConnected, token)
}

func (h *HostRegistrationHandler) deleteUnused(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	deleted, err := h.hosts.DeleteUnusedHost(r.Context(), r.PathValue("hostID"), userID)
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if deleted {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// Soft-deleted clusters retain host foreign keys, so revoke any never-used
	// credential when the host record cannot be physically removed.
	revocationToken, tokenErr := auth.GenerateAgentToken()
	if tokenErr != nil {
		writeError(w, http.StatusInternalServerError, "failed to revoke host token")
		return
	}
	revoked, err := h.hosts.RotateHostToken(r.Context(), r.PathValue("hostID"), userID, auth.HashAgentToken(revocationToken), h.now().UTC())
	if err != nil {
		writeStoreError(w, err)
		return
	}
	if !revoked {
		writeError(w, http.StatusNotFound, "unused host not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *HostRegistrationHandler) writeRegistration(w http.ResponseWriter, hostID string, status store.HostStatus, token string) {
	response := struct {
		HostID           string           `json:"host_id"`
		Status           store.HostStatus `json:"status"`
		DockerRunCommand string           `json:"docker_run_command"`
	}{
		HostID:           hostID,
		Status:           status,
		DockerRunCommand: fmt.Sprintf("docker run -d \\\n  -e ORCA_TOKEN=%s \\\n  -e ORCA_SERVER_URL=%s \\\n  -v /var/run/docker.sock:/var/run/docker.sock \\\n  -v /proc:/host/proc:ro \\\n  -v /var/orca/data:/var/orca/data \\\n  orca/agent", token, h.serverURL),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

func randomID(random io.Reader) (string, error) {
	value := make([]byte, hostIDBytes)
	if _, err := io.ReadFull(random, value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}
