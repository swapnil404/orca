package devrpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/swapnil404/orca/agent/internal/reconciler"
	"github.com/swapnil404/orca/agent/internal/state"
	"io"
	"net/http"
)

const desiredStatePath = "/dev/desired-state"

// Server exposes the local development reconciliation endpoint.
type Server struct {
	runner *reconciler.Runner
}

// NewServerWithRunner creates a dev RPC server using the shared reconciliation path.
func NewServerWithRunner(runner *reconciler.Runner) *Server {
	return &Server{runner: runner}
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

	pass, err := s.runner.Reconcile(r.Context(), &requested)
	if err != nil {
		http.Error(w, fmt.Sprintf("reconcile desired state: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	for _, result := range pass.Results {
		if result.Status == reconciler.ApplyStatusFailed {
			w.WriteHeader(http.StatusInternalServerError)
			break
		}
	}
	if err := json.NewEncoder(w).Encode(pass.Results); err != nil {
		return
	}
	pass.Acknowledge()
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
