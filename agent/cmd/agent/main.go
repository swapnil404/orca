package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/betterorca/betterorca/agent/internal/devrpc"
	orcadocker "github.com/betterorca/betterorca/agent/internal/docker"
	"github.com/betterorca/betterorca/agent/internal/reconciler"
	"github.com/betterorca/betterorca/agent/internal/state"
	"github.com/betterorca/betterorca/agent/internal/tunnel"
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
	runner := reconciler.NewRunner(cache, dockerClient)
	serverURL := os.Getenv("ORCA_SERVER_URL")
	if serverURL != "" {
		client, err := tunnel.NewClient(tunnel.Config{
			ServerURL: serverURL,
			Token:     os.Getenv("ORCA_TOKEN"),
		}, runner)
		if err != nil {
			log.Fatalf("configure agent tunnel: %v", err)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		log.Printf("connecting agent tunnel to %s", serverURL)
		if err := client.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Fatalf("run agent tunnel: %v", err)
		}
		return
	}

	server := devrpc.NewServerWithRunner(runner)

	log.Printf("dev-only agent endpoint listening on http://%s/dev/desired-state", address)
	if err := http.ListenAndServe(address, server); err != nil {
		log.Fatalf("serve dev RPC: %v", err)
	}
}
