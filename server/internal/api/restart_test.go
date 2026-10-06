package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/swapnil404/orca/server/internal/auth"
	"github.com/swapnil404/orca/server/internal/store"
)

type restartTestStore struct {
	resourceStore
	err    error
	params store.RestartProjectParams
}

func (s *restartTestStore) RestartProject(_ context.Context, params store.RestartProjectParams) ([]store.Cluster, error) {
	s.params = params
	return []store.Cluster{{ID: "cluster", Name: "main", HostID: "host", RestartGeneration: 7, DesiredRevision: "42"}}, s.err
}

func TestRestartReturnsCommittedTrackingTargets(t *testing.T) {
	state := &restartTestStore{}
	handler := NewResourceHandler(state)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)
	request := httptest.NewRequest(http.MethodPost, "/projects/project/restart", nil)
	request = request.WithContext(auth.WithUserID(request.Context(), "user"))
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
	var targets []struct {
		ID         string `json:"id"`
		Generation int64  `json:"restart_generation"`
		Revision   string `json:"desired_revision"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &targets); err != nil {
		t.Fatal(err)
	}
	if len(targets) != 1 || targets[0].ID != "cluster" || targets[0].Generation != 7 || targets[0].Revision != "42" {
		t.Fatalf("missing tracking targets: %+v", targets)
	}
	if state.params.ProjectID != "project" || state.params.UserID != "user" {
		t.Fatalf("incorrect scope: %+v", state.params)
	}
}

func TestRestartFailureDoesNotReturnTrackingTargets(t *testing.T) {
	handler := NewResourceHandler(&restartTestStore{err: errors.New("store unavailable")})
	request := httptest.NewRequest(http.MethodPost, "/projects/project/restart", nil)
	request = request.WithContext(auth.WithUserID(request.Context(), "user"))
	request.SetPathValue("projectID", "project")
	response := httptest.NewRecorder()
	handler.restartProject(response, request)
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status %d: %s", response.Code, response.Body.String())
	}
}
