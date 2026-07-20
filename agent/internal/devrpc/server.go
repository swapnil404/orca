package devrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	orcadocker "github.com/betterorca/betterorca/agent/internal/docker"
	"github.com/betterorca/betterorca/agent/internal/reconciler"
	"github.com/betterorca/betterorca/agent/internal/state"
)

const desiredStatePath = "/dev/desired-state"

// Server exposes the local development reconciliation endpoint.
type Server struct {
	cache  state.StateCache
	docker orcadocker.DockerClient
	mu     sync.Mutex
}

// NewServer creates a dev RPC server with explicit state and Docker dependencies.
func NewServer(cache state.StateCache, docker orcadocker.DockerClient) *Server {
	return &Server{cache: cache, docker: docker}
}

// ServeHTTP serves the dev-only desired-state endpoint.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != desiredStatePath {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	s.handleDesiredState(w, r)
}

func (s *Server) handleDesiredState(w http.ResponseWriter, r *http.Request) {
	var requested state.DesiredState
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := decoder.Decode(&requested); err != nil {
		http.Error(w, fmt.Sprintf("decode desired state: %v", err), http.StatusBadRequest)
		return
	}
	if err := ensureJSONEnd(decoder); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.cache.Save(r.Context(), requested); err != nil {
		http.Error(w, fmt.Sprintf("save desired state: %v", err), http.StatusInternalServerError)
		return
	}

	desired, err := s.cache.Load(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("load desired state: %v", err), http.StatusInternalServerError)
		return
	}
	containers, err := s.docker.ListOrcaContainers(r.Context())
	if err != nil {
		http.Error(w, fmt.Sprintf("load actual state: %v", err), http.StatusInternalServerError)
		return
	}

	actions := reconciler.Diff(desired, actualState(containers))
	results := reconciler.Apply(r.Context(), s.docker, actions)

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(results); err != nil {
		return
	}
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode desired state: %w", err)
	}

	return errors.New("decode desired state: request body must contain one JSON value")
}

func actualState(containers []orcadocker.ContainerInfo) reconciler.ActualState {
	clusters := make(map[string]*reconciler.ActualCluster)
	order := make([]string, 0)
	for _, container := range containers {
		cluster, exists := clusters[container.ClusterID]
		if !exists {
			cluster = &reconciler.ActualCluster{Id: container.ClusterID}
			clusters[container.ClusterID] = cluster
			order = append(order, container.ClusterID)
		}

		switch container.Kind {
		case orcadocker.ContainerKindPrimary:
			cluster.ContainerId = container.ID
			cluster.Status = container.Status
			cluster.Version = postgresVersion(container.Image)
		case orcadocker.ContainerKindReplica:
			cluster.Replicas = append(cluster.Replicas, &reconciler.ActualReplica{
				Id:          container.ReplicaID,
				ContainerId: container.ID,
				Status:      container.Status,
			})
		case orcadocker.ContainerKindPgBouncer:
			cluster.PgBouncer = &reconciler.ActualPgBouncer{
				ContainerId: container.ID,
				Status:      container.Status,
			}
		}
	}

	actual := reconciler.ActualState{Clusters: make([]*reconciler.ActualCluster, 0, len(order))}
	for _, clusterID := range order {
		actual.Clusters = append(actual.Clusters, clusters[clusterID])
	}

	return actual
}

func postgresVersion(image string) string {
	image = strings.TrimPrefix(image, "docker.io/library/")
	version, found := strings.CutPrefix(image, "postgres:")
	if !found || version == "latest" {
		return ""
	}

	version, _, _ = strings.Cut(version, "@")
	return version
}
