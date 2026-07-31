package tunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/gorilla/websocket"
	"google.golang.org/protobuf/proto"

	"github.com/swapnil404/orca/agent/internal/reconciler"
	"github.com/swapnil404/orca/pkg/types"
)

const (
	defaultMinBackoff        = time.Second
	defaultMaxBackoff        = 30 * time.Second
	defaultReconcileInterval = 30 * time.Second
	maxServerMessage         = 4 * 1024 * 1024
	connectionDeadline       = 10 * time.Second
)

type reconcileRunner interface {
	Reconcile(context.Context, *reconciler.DesiredState) (reconciler.Pass, error)
	ReconcileCached(context.Context) (reconciler.Pass, error)
}

// Config contains the agent tunnel connection settings.
type Config struct {
	ServerURL         string
	Token             string
	MinBackoff        time.Duration
	MaxBackoff        time.Duration
	ReconcileInterval time.Duration
}

// Client maintains an authenticated agent tunnel and reconciles full desired snapshots.
type Client struct {
	config Config
	runner reconcileRunner
	dialer *websocket.Dialer
	logger *slog.Logger
}

// NewClient validates config and creates an agent tunnel client.
func NewClient(config Config, runner reconcileRunner) (*Client, error) {
	if config.ServerURL == "" {
		return nil, errors.New("ORCA_SERVER_URL is required")
	}
	parsed, err := url.Parse(config.ServerURL)
	if err != nil || (parsed.Scheme != "ws" && parsed.Scheme != "wss") || parsed.Host == "" {
		return nil, errors.New("ORCA_SERVER_URL must be a ws:// or wss:// URL")
	}
	if config.Token == "" {
		return nil, errors.New("ORCA_TOKEN is required")
	}
	if runner == nil {
		return nil, errors.New("reconciler is required")
	}
	if config.MinBackoff <= 0 {
		config.MinBackoff = defaultMinBackoff
	}
	if config.MaxBackoff <= 0 {
		config.MaxBackoff = defaultMaxBackoff
	}
	if config.MaxBackoff < config.MinBackoff {
		return nil, errors.New("maximum reconnect backoff cannot be less than minimum backoff")
	}
	if config.ReconcileInterval <= 0 {
		config.ReconcileInterval = defaultReconcileInterval
	}
	return &Client{
		config: config, runner: runner, dialer: websocket.DefaultDialer, logger: slog.Default(),
	}, nil
}

// Run connects until ctx is canceled, retrying disconnected sessions with exponential backoff.
func (c *Client) Run(ctx context.Context) error {
	backoff := c.config.MinBackoff
	for {
		reconciled, err := c.runSession(ctx)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if reconciled {
			backoff = c.config.MinBackoff
		}
		c.logger.Warn("agent tunnel disconnected", "error", err, "retry_in", backoff)
		if _, err := c.runner.ReconcileCached(ctx); err != nil {
			c.logger.Warn("offline reconciliation failed", "error", err)
		}
		if err := wait(ctx, backoff); err != nil {
			return err
		}
		if !reconciled && backoff < c.config.MaxBackoff {
			backoff *= 2
			if backoff > c.config.MaxBackoff {
				backoff = c.config.MaxBackoff
			}
		}
	}
}

func (c *Client) runSession(ctx context.Context) (bool, error) {
	c.logger.Info("dialing agent tunnel", "url", c.config.ServerURL)
	connection, response, err := c.dialer.DialContext(ctx, c.config.ServerURL, nil)
	if err != nil {
		if response != nil {
			c.logger.Warn("agent tunnel dial failed", "url", c.config.ServerURL, "http_status", response.StatusCode, "error", err)
		} else {
			c.logger.Warn("agent tunnel dial failed", "url", c.config.ServerURL, "error", err)
		}
		return false, fmt.Errorf("connect agent tunnel: %w", err)
	}
	c.logger.Info("agent tunnel WebSocket handshake succeeded", "url", c.config.ServerURL)
	defer connection.Close()
	connection.SetReadLimit(maxServerMessage)
	if err := connection.SetWriteDeadline(time.Now().Add(connectionDeadline)); err != nil {
		return false, err
	}
	if err := connection.WriteJSON(struct {
		Token string `json:"token"`
	}{Token: c.config.Token}); err != nil {
		c.logger.Warn("agent tunnel authentication request failed", "error", err)
		return false, fmt.Errorf("authenticate agent tunnel: %w", err)
	}
	c.logger.Info("agent tunnel authentication request sent")
	if err := connection.SetWriteDeadline(time.Time{}); err != nil {
		return false, err
	}

	closed := make(chan struct{})
	defer close(closed)
	go func() {
		select {
		case <-ctx.Done():
			_ = connection.Close()
		case <-closed:
		}
	}()

	timer := time.NewTimer(c.config.ReconcileInterval)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()
	var reconcileTimer <-chan time.Time
	type frame struct {
		messageType int
		payload     []byte
		err         error
	}
	frames := make(chan frame)
	go func() {
		for {
			messageType, payload, err := connection.ReadMessage()
			select {
			case frames <- frame{messageType: messageType, payload: payload, err: err}:
			case <-ctx.Done():
				return
			case <-closed:
				return
			}
			if err != nil {
				return
			}
		}
	}()

	reconciled := false
	authenticated := false
	for {
		select {
		case <-ctx.Done():
			return reconciled, ctx.Err()
		case incoming := <-frames:
			if incoming.err != nil {
				if !authenticated {
					c.logger.Warn("agent tunnel authentication failed before confirmation", "error", incoming.err)
				}
				return reconciled, fmt.Errorf("read desired state: %w", incoming.err)
			}
			if incoming.messageType != websocket.BinaryMessage {
				return reconciled, fmt.Errorf("unexpected desired-state message type %d", incoming.messageType)
			}
			if !authenticated {
				authenticated = true
				c.logger.Info("agent tunnel authentication succeeded")
			}
			message := &types.DesiredStateMessage{}
			if err := proto.Unmarshal(incoming.payload, message); err != nil {
				return reconciled, fmt.Errorf("decode desired state: %w", err)
			}
			if message.GetDesiredState() == nil {
				return reconciled, errors.New("desired state is required")
			}
			c.logger.Info("desired-state snapshot received", "revision", message.GetDesiredState().GetRevision(), "clusters", len(message.GetDesiredState().GetClusters()))
			pass, err := c.runner.Reconcile(ctx, message.GetDesiredState())
			if err != nil {
				return reconciled, fmt.Errorf("reconcile desired state: %w", err)
			}
			failedActions := 0
			for _, result := range pass.Results {
				if result.Err == nil {
					continue
				}
				failedActions++
				c.logger.Warn("reconciliation action failed", "action", result.Action.Type, "cluster_id", result.Action.ClusterID, "error", result.Err)
			}
			c.logger.Info("desired-state reconciliation completed", "revision", message.GetDesiredState().GetRevision(), "actions", len(pass.Results), "failed_actions", failedActions)
			if err := writeReport(connection, pass); err != nil {
				return reconciled, err
			}
			if !reconciled {
				c.logger.Info("agent tunnel post-auth ready", "revision", message.GetDesiredState().GetRevision())
			}
			reconciled = true
			timer.Reset(c.config.ReconcileInterval)
			reconcileTimer = timer.C
		case <-reconcileTimer:
			pass, err := c.runner.ReconcileCached(ctx)
			if err != nil {
				return reconciled, fmt.Errorf("reconcile cached desired state: %w", err)
			}
			if err := writeReport(connection, pass); err != nil {
				return reconciled, err
			}
			timer.Reset(c.config.ReconcileInterval)
		}
	}
}

func writeReport(connection *websocket.Conn, pass reconciler.Pass) error {
	if pass.Report == nil {
		return errors.New("reconciler returned no agent report")
	}
	reportMessage := proto.Clone(pass.Report).(*types.AgentReportMessage)
	reportMessage.ReconciliationResults = make([]*types.ReconciliationResult, 0, len(pass.Results))
	for _, result := range pass.Results {
		message := ""
		if result.Err != nil {
			message = result.Err.Error()
		}
		reportMessage.ReconciliationResults = append(reportMessage.ReconciliationResults, &types.ReconciliationResult{
			Action: string(result.Action.Type), ClusterId: result.Action.ClusterID,
			Status: string(result.Status), Error: message,
		})
	}
	report, err := proto.Marshal(reportMessage)
	if err != nil {
		return fmt.Errorf("encode agent report: %w", err)
	}
	if err := connection.SetWriteDeadline(time.Now().Add(connectionDeadline)); err != nil {
		return err
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, report); err != nil {
		return fmt.Errorf("send agent report: %w", err)
	}
	pass.Acknowledge()
	return connection.SetWriteDeadline(time.Time{})
}

func wait(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
