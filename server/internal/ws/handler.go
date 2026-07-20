package ws

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/betterorca/betterorca/server/internal/auth"
	"github.com/betterorca/betterorca/server/internal/store"
)

const (
	defaultAuthenticationTimeout = 10 * time.Second
	statusUpdateTimeout          = 5 * time.Second
)

type agentHostStore interface {
	HostByTokenHash(context.Context, []byte) (store.Host, error)
	UpdateHostStatus(context.Context, string, store.HostStatus) error
}

// AgentHandler upgrades and authenticates agent WebSocket connections.
type AgentHandler struct {
	hub         *Hub
	hosts       agentHostStore
	now         func() time.Time
	authTimeout time.Duration
	upgrader    websocket.Upgrader
}

// NewAgentHandler creates an authenticated agent WebSocket endpoint.
func NewAgentHandler(hub *Hub, hosts agentHostStore) *AgentHandler {
	return &AgentHandler{
		hub:         hub,
		hosts:       hosts,
		now:         time.Now,
		authTimeout: defaultAuthenticationTimeout,
	}
}

// ServeHTTP upgrades an agent connection and registers its authenticated session.
func (h *AgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	connection, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	connection.SetReadLimit(4096)
	_ = connection.SetReadDeadline(h.now().Add(h.authTimeout))

	var message struct {
		Token string `json:"token"`
	}
	if err := connection.ReadJSON(&message); err != nil || message.Token == "" {
		closeConnection(connection, websocket.ClosePolicyViolation, "authentication required")
		return
	}

	host, err := h.hosts.HostByTokenHash(r.Context(), auth.HashAgentToken(message.Token))
	if err != nil || !h.now().Before(host.TokenExpiresAt) {
		closeConnection(connection, websocket.ClosePolicyViolation, "invalid or expired token")
		return
	}
	if err := h.hosts.UpdateHostStatus(r.Context(), host.ID, store.HostStatusOnline); err != nil {
		closeConnection(connection, websocket.CloseInternalServerErr, "failed to activate host")
		return
	}

	_ = connection.SetReadDeadline(time.Time{})
	session := NewSession(connection)
	h.hub.Register(host.ID, session)
	defer h.disconnect(host.ID, session)

	for {
		if _, _, err := connection.ReadMessage(); err != nil {
			return
		}
	}
}

func (h *AgentHandler) disconnect(hostID string, session *Session) {
	removed := h.hub.UnregisterSession(hostID, session)
	session.Close()
	if !removed {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), statusUpdateTimeout)
	defer cancel()
	_ = h.hosts.UpdateHostStatus(ctx, hostID, store.HostStatusOffline)
}

func closeConnection(connection *websocket.Conn, code int, reason string) {
	deadline := time.Now().Add(time.Second)
	payload, err := json.Marshal(struct {
		Error string `json:"error"`
	}{Error: reason})
	if err == nil {
		_ = connection.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(code, string(payload)), deadline)
	}
	_ = connection.Close()
}
