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

	"github.com/betterorca/betterorca/server/internal/auth"
	"github.com/betterorca/betterorca/server/internal/store"
)

const (
	placeholderUserID = "placeholder-user"
	hostIDBytes       = 18
	tokenLifetime     = 24 * time.Hour
)

type hostCreator interface {
	CreateHost(context.Context, store.CreateHostParams) (store.Host, error)
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

// ServeHTTP registers a host for the current user.
func (h *HostRegistrationHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
		UserID:         placeholderUserID,
		TokenHash:      auth.HashAgentToken(token),
		TokenExpiresAt: now.Add(tokenLifetime),
		Status:         store.HostStatusNeverConnected,
	})
	if err != nil {
		http.Error(w, "failed to register host", http.StatusInternalServerError)
		return
	}

	response := struct {
		HostID           string           `json:"host_id"`
		Status           store.HostStatus `json:"status"`
		DockerRunCommand string           `json:"docker_run_command"`
	}{
		HostID:           host.ID,
		Status:           host.Status,
		DockerRunCommand: fmt.Sprintf("docker run -d \\\n  -e ORCA_TOKEN=%s \\\n  -e ORCA_SERVER_URL=%s \\\n  -v /var/run/docker.sock:/var/run/docker.sock \\\n  -v /var/orca/data:/var/orca/data \\\n  orca/agent", token, h.serverURL),
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
