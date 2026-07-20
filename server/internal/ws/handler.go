package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/betterorca/betterorca/pkg/types"
	"github.com/betterorca/betterorca/server/internal/auth"
	"github.com/betterorca/betterorca/server/internal/store"
)

const (
	defaultAuthenticationTimeout = 10 * time.Second
	statusUpdateTimeout          = 5 * time.Second
	reportStoreTimeout           = 5 * time.Second
	maxAgentMessageBytes         = 1024 * 1024
)

type agentHostStore interface {
	HostByTokenHash(context.Context, []byte) (store.Host, error)
	UpdateHostStatus(context.Context, string, store.HostStatus) error
}

type desiredStatePusher interface {
	PushDesiredState(context.Context, string) error
}

type agentReportStore interface {
	StoreAgentReport(context.Context, string, *types.AgentReportMessage, time.Time) error
}

// AgentHandler upgrades and authenticates agent WebSocket connections.
type AgentHandler struct {
	hub         *Hub
	hosts       agentHostStore
	now         func() time.Time
	authTimeout time.Duration
	upgrader    websocket.Upgrader
	pusher      desiredStatePusher
	reports     agentReportStore
	logger      *slog.Logger
}

// NewAgentHandler creates an authenticated agent WebSocket endpoint.
func NewAgentHandler(hub *Hub, hosts agentHostStore, pushers ...desiredStatePusher) *AgentHandler {
	handler := &AgentHandler{
		hub:         hub,
		hosts:       hosts,
		now:         time.Now,
		authTimeout: defaultAuthenticationTimeout,
		logger:      slog.Default(),
	}
	handler.reports, _ = hosts.(agentReportStore)
	if len(pushers) > 0 {
		handler.pusher = pushers[0]
	}
	return handler
}

// ServeHTTP upgrades an agent connection and registers its authenticated session.
func (h *AgentHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	connection, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}

	connection.SetReadLimit(maxAgentMessageBytes)
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
	if h.pusher != nil {
		if err := h.pusher.PushDesiredState(r.Context(), host.ID); err != nil {
			return
		}
	}

	for {
		messageType, payload, err := connection.ReadMessage()
		if err != nil {
			return
		}
		h.handleReportFrame(host.ID, messageType, payload)
	}
}

func (h *AgentHandler) handleReportFrame(hostID string, messageType int, payload []byte) {
	if messageType != websocket.BinaryMessage {
		h.logger.Warn("dropping unexpected agent WebSocket message", "host_id", hostID, "message_type", messageType)
		return
	}
	if h.reports == nil {
		h.logger.Error("dropping agent report because no report store is configured", "host_id", hostID)
		return
	}

	report := &types.AgentReportMessage{}
	if err := proto.Unmarshal(payload, report); err != nil {
		h.logger.Warn("dropping malformed agent report", "host_id", hostID, "error", err)
		return
	}
	if err := validateAgentReport(report); err != nil {
		h.logger.Warn("dropping invalid agent report", "host_id", hostID, "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), reportStoreTimeout)
	defer cancel()
	if err := h.reports.StoreAgentReport(ctx, hostID, report, h.now().UTC()); err != nil {
		h.logger.Error("failed to store agent report", "host_id", hostID, "error", err)
	}
}

func validateAgentReport(report *types.AgentReportMessage) error {
	if report.GetActualState() == nil || report.GetHealthReport() == nil {
		return fmt.Errorf("actual_state and health_report are required")
	}
	if report.GetHealthReport().GetHostMetrics() == nil {
		return fmt.Errorf("host_metrics is required")
	}
	actualIDs := make(map[string]struct{}, len(report.GetActualState().GetClusters()))
	for _, cluster := range report.GetActualState().GetClusters() {
		if cluster.GetId() == "" {
			return fmt.Errorf("actual cluster ID is required")
		}
		if _, exists := actualIDs[cluster.GetId()]; exists {
			return fmt.Errorf("duplicate actual cluster ID %q", cluster.GetId())
		}
		actualIDs[cluster.GetId()] = struct{}{}
	}
	healthIDs := make(map[string]struct{}, len(report.GetHealthReport().GetClusters()))
	for _, health := range report.GetHealthReport().GetClusters() {
		if health.GetClusterId() == "" {
			return fmt.Errorf("health cluster ID is required")
		}
		if health.GetStatus() < types.ClusterStatus_CLUSTER_STATUS_PENDING || health.GetStatus() > types.ClusterStatus_CLUSTER_STATUS_DOWN {
			return fmt.Errorf("health status for cluster %q is invalid", health.GetClusterId())
		}
		if _, exists := healthIDs[health.GetClusterId()]; exists {
			return fmt.Errorf("duplicate health cluster ID %q", health.GetClusterId())
		}
		healthIDs[health.GetClusterId()] = struct{}{}
	}
	return nil
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
