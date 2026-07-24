package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/swapnil404/orca/agent/internal/devrpc"
	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/pgbackrest"
	"github.com/swapnil404/orca/agent/internal/reconciler"
	"github.com/swapnil404/orca/agent/internal/state"
	"github.com/swapnil404/orca/agent/internal/tunnel"
)

const defaultDevAddress = "127.0.0.1:8080"

func main() {
	dockerClient, err := orcadocker.NewClient()
	if err != nil {
		log.Fatalf("create Docker client: %v", err)
	}

	address := os.Getenv("ORCA_DEV_ADDRESS")
	if address == "" {
		address = defaultDevAddress
	}
	cache := state.NewFileCache(os.Getenv("ORCA_STATE_PATH"))
	scheduler := pgbackrest.NewScheduler(cache, dockerClient)
	runner := reconciler.NewRunner(cache, dockerClient, scheduler)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		if err := scheduler.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Printf("run backup scheduler: %v", err)
		}
	}()
	serverURL := os.Getenv("ORCA_SERVER_URL")
	if serverURL != "" {
		client, err := tunnel.NewClient(tunnel.Config{
			ServerURL: serverURL,
			Token:     os.Getenv("ORCA_TOKEN"),
		}, runner)
		if err != nil {
			log.Fatalf("configure agent tunnel: %v", err)
		}
		log.Printf("connecting agent tunnel to %s", serverURL)
		if err := client.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("run agent tunnel: %v", err)
		}
		return
	}

	server := &http.Server{Addr: address, Handler: devrpc.NewServerWithRunner(runner)}
	go func() {
		<-ctx.Done()
		_ = server.Shutdown(context.Background())
	}()

	log.Printf("dev-only agent endpoint listening on http://%s/dev/desired-state", address)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("serve dev RPC: %v", err)
	}
}
