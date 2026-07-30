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
	"strings"
	"syscall"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/swapnil404/orca/server/internal/api"
	"github.com/swapnil404/orca/server/internal/auth"
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
	databaseURL        string
	port               int
	serverURL          string
	logLevel           slog.Level
	jwtSecret          string
	githubClientID     string
	githubClientSecret string
	googleClientID     string
	googleClientSecret string
	oauthCallbackBase  string
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
	jwtSecret := os.Getenv("ORCA_JWT_SECRET")
	if strings.TrimSpace(jwtSecret) == "" {
		return config{}, errors.New("ORCA_JWT_SECRET is required")
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
	oauthCallbackBase := *parsedServerURL
	if oauthCallbackBase.Scheme == "ws" {
		oauthCallbackBase.Scheme = "http"
	} else {
		oauthCallbackBase.Scheme = "https"
	}
	oauthCallbackBase.Path = ""
	oauthCallbackBase.RawPath = ""
	oauthCallbackBase.RawQuery = ""
	oauthCallbackBase.Fragment = ""

	githubClientID, githubClientSecret := os.Getenv("ORCA_GITHUB_CLIENT_ID"), os.Getenv("ORCA_GITHUB_CLIENT_SECRET")
	if err := validateOAuthCredentials("GITHUB", githubClientID, githubClientSecret); err != nil {
		return config{}, err
	}
	googleClientID, googleClientSecret := os.Getenv("ORCA_GOOGLE_CLIENT_ID"), os.Getenv("ORCA_GOOGLE_CLIENT_SECRET")
	if err := validateOAuthCredentials("GOOGLE", googleClientID, googleClientSecret); err != nil {
		return config{}, err
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

	return config{
		databaseURL: databaseURL, port: port, serverURL: serverURL, logLevel: logLevel, jwtSecret: jwtSecret,
		githubClientID: githubClientID, githubClientSecret: githubClientSecret,
		googleClientID: googleClientID, googleClientSecret: googleClientSecret,
		oauthCallbackBase: strings.TrimSuffix(oauthCallbackBase.String(), "/"),
	}, nil
}

func validateOAuthCredentials(provider, clientID, clientSecret string) error {
	if (clientID == "") != (clientSecret == "") {
		return fmt.Errorf("ORCA_%s_CLIENT_ID and ORCA_%s_CLIENT_SECRET must be set together", provider, provider)
	}
	return nil
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
	tokens, err := auth.NewJWTManager(configuration.jwtSecret, metadata, configuration.oauthCallbackBase)
	if err != nil {
		return err
	}
	hub := ws.NewHub()
	desiredStates := orchestrator.New(metadata, hub)
	projectEvents := api.NewProjectEventHandler(metadata, tokens)
	agentHandler := ws.NewAgentHandler(hub, metadata, desiredStates)
	agentHandler.SetReportNotifier(projectEvents)

	protected := http.NewServeMux()
	userAuth := api.NewUserAuthHandler(metadata, tokens)
	userAuth.RegisterProtectedRoutes(protected)
	api.NewOrganizationHandler(metadata).RegisterRoutes(protected)
	api.NewResourceHandler(metadata, desiredStates).RegisterRoutes(protected)
	api.NewBackupHandler(metadata).RegisterRoutes(protected)
	api.NewAlertHandler(metadata).RegisterRoutes(protected)
	protected.Handle("POST /hosts", api.NewHostRegistrationHandler(metadata, configuration.serverURL))
	metrics.NewHandler(metadata).RegisterRoutes(protected)

	mux := http.NewServeMux()
	userAuth.RegisterRoutes(mux)
	api.NewOAuthHandler(metadata, tokens, api.OAuthConfig{
		CallbackBaseURL: configuration.oauthCallbackBase, CookieSecret: configuration.jwtSecret,
		GitHubClientID: configuration.githubClientID, GitHubClientSecret: configuration.githubClientSecret,
		GoogleClientID: configuration.googleClientID, GoogleClientSecret: configuration.googleClientSecret,
	}).RegisterRoutes(mux)
	projectEvents.RegisterRoutes(mux)
	mux.Handle("GET /agent", agentHandler)
	mux.Handle("/", tokens.Middleware(protected))

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
