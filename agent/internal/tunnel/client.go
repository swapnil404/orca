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
	Reconcile(context.Context, reconciler.DesiredState) (reconciler.Pass, error)
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
		c.logger.Debug("agent tunnel disconnected", "error", err, "retry_in", backoff)
		if _, err := c.runner.ReconcileCached(ctx); err != nil {
			c.logger.Debug("offline reconciliation failed", "error", err)
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
	connection, _, err := c.dialer.DialContext(ctx, c.config.ServerURL, nil)
	if err != nil {
		return false, fmt.Errorf("connect agent tunnel: %w", err)
	}
	defer connection.Close()
	connection.SetReadLimit(maxServerMessage)
	if err := connection.SetWriteDeadline(time.Now().Add(connectionDeadline)); err != nil {
		return false, err
	}
	if err := connection.WriteJSON(struct {
		Token string `json:"token"`
	}{Token: c.config.Token}); err != nil {
		return false, fmt.Errorf("authenticate agent tunnel: %w", err)
	}
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
	for {
		select {
		case <-ctx.Done():
			return reconciled, ctx.Err()
		case incoming := <-frames:
			if incoming.err != nil {
				return reconciled, fmt.Errorf("read desired state: %w", incoming.err)
			}
			if incoming.messageType != websocket.BinaryMessage {
				return reconciled, fmt.Errorf("unexpected desired-state message type %d", incoming.messageType)
			}
			message := &types.DesiredStateMessage{}
			if err := proto.Unmarshal(incoming.payload, message); err != nil {
				return reconciled, fmt.Errorf("decode desired state: %w", err)
			}
			if message.GetDesiredState() == nil {
				return reconciled, errors.New("desired state is required")
			}
			pass, err := c.runner.Reconcile(ctx, *message.GetDesiredState())
			if err != nil {
				return reconciled, fmt.Errorf("reconcile desired state: %w", err)
			}
			if err := writeReport(connection, pass); err != nil {
				return reconciled, err
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
	report, err := proto.Marshal(pass.Report)
	if err != nil {
		return fmt.Errorf("encode agent report: %w", err)
	}
	if err := connection.SetWriteDeadline(time.Now().Add(connectionDeadline)); err != nil {
		return err
	}
	if err := connection.WriteMessage(websocket.BinaryMessage, report); err != nil {
		return fmt.Errorf("send agent report: %w", err)
	}
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
