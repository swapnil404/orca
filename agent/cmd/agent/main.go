package main

import (
	"log"
	"net/http"
	"os"

	"github.com/betterorca/betterorca/agent/internal/devrpc"
	orcadocker "github.com/betterorca/betterorca/agent/internal/docker"
	"github.com/betterorca/betterorca/agent/internal/state"
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
	server := devrpc.NewServer(cache, dockerClient)

	log.Printf("dev-only agent endpoint listening on http://%s/dev/desired-state", address)
	if err := http.ListenAndServe(address, server); err != nil {
		log.Fatalf("serve dev RPC: %v", err)
	}
}
