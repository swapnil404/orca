package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/swapnil404/orca/server/internal/api"
	"github.com/swapnil404/orca/server/internal/metrics"
	"github.com/swapnil404/orca/server/internal/orchestrator"
	"github.com/swapnil404/orca/server/internal/store"
	"github.com/swapnil404/orca/server/internal/ws"
)

const (
	defaultPort       = 8080
	shutdownTimeout   = 10 * time.Second
	databaseTimeout   = 10 * time.Second
	readHeaderTimeout = 10 * time.Second
)

type config struct {
	databaseURL string
	port        int
	serverURL   string
	logLevel    slog.Level
}

func main() {
	configuration, err := loadConfig()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: configuration.logLevel})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := run(ctx, configuration); err != nil {
		slog.Error("server stopped", "error", err)
		os.Exit(1)
	}
}

func loadConfig() (config, error) {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return config{}, errors.New("DATABASE_URL is required")
	}

	port := defaultPort
	if value := os.Getenv("ORCA_PORT"); value != "" {
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 65535 {
			return config{}, errors.New("ORCA_PORT must be a valid TCP port")
		}
		port = parsed
	}

	serverURL := os.Getenv("ORCA_SERVER_URL")
	if serverURL == "" {
		serverURL = fmt.Sprintf("ws://localhost:%d/agent", port)
	}
	parsedServerURL, err := url.Parse(serverURL)
	if err != nil || (parsedServerURL.Scheme != "ws" && parsedServerURL.Scheme != "wss") || parsedServerURL.Host == "" {
		return config{}, errors.New("ORCA_SERVER_URL must be a ws:// or wss:// URL")
	}

	logLevel := slog.LevelInfo
	switch value := os.Getenv("ORCA_LOG_LEVEL"); value {
	case "", "info":
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		return config{}, errors.New("ORCA_LOG_LEVEL must be debug, info, warn, or error")
	}

	return config{databaseURL: databaseURL, port: port, serverURL: serverURL, logLevel: logLevel}, nil
}

func run(ctx context.Context, configuration config) error {
	runCtx, cancelRun := context.WithCancel(ctx)
	defer cancelRun()

	database, err := sql.Open("pgx", configuration.databaseURL)
	if err != nil {
		return fmt.Errorf("open metadata database: %w", err)
	}
	defer database.Close()

	pingCtx, cancelPing := context.WithTimeout(runCtx, databaseTimeout)
	defer cancelPing()
	if err := database.PingContext(pingCtx); err != nil {
		return fmt.Errorf("connect metadata database: %w", err)
	}

	metadata := store.NewPostgres(database)
	hub := ws.NewHub()
	desiredStates := orchestrator.New(metadata, hub)
	projectEvents := api.NewProjectEventHandler(metadata)
	agentHandler := ws.NewAgentHandler(hub, metadata, desiredStates)
	agentHandler.SetReportNotifier(projectEvents)

	mux := http.NewServeMux()
	api.NewResourceHandler(metadata, desiredStates).RegisterRoutes(mux)
	mux.Handle("POST /hosts", api.NewHostRegistrationHandler(metadata, configuration.serverURL))
	projectEvents.RegisterRoutes(mux)
	mux.Handle("GET /agent", agentHandler)
	metrics.NewHandler(metadata).RegisterRoutes(mux)

	evaluator := metrics.NewEvaluator(metadata, metadata, 0)
	go evaluator.Run(runCtx)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", configuration.port),
		Handler:           mux,
		ReadHeaderTimeout: readHeaderTimeout,
	}
	serveErrors := make(chan error, 1)
	go func() {
		slog.Info("server listening", "address", server.Addr)
		serveErrors <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-runCtx.Done():
		shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancelShutdown()
		shutdownErr := server.Shutdown(shutdownCtx)
		serveErr := <-serveErrors
		if !errors.Is(serveErr, http.ErrServerClosed) {
			shutdownErr = errors.Join(shutdownErr, serveErr)
		}
		return shutdownErr
	}
}
