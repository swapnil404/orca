package api

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/swapnil404/orca/server/internal/auth"
	"github.com/swapnil404/orca/server/internal/store"
)

type replicaTestStore struct {
	resourceStore
	user, cluster, replica string
	err                    error
	calls                  int
}

func (s *replicaTestStore) RemoveReplica(_ context.Context, user, cluster, replica string) (store.Cluster, error) {
	s.user, s.cluster, s.replica = user, cluster, replica
	s.calls++
	return store.Cluster{ID: cluster, Replicas: []store.Replica{{ID: "keep"}}, ReplicaCount: 1, DesiredRevision: "43"}, s.err
}
func TestReplicaRemovalScopesSelectedIdentity(t *testing.T) {
	for _, scenario := range []struct {
		name          string
		authenticated bool
		err           error
		status        int
	}{
		{"success", true, nil, http.StatusOK}, {"missing", true, sql.ErrNoRows, http.StatusNotFound}, {"unauthenticated", false, nil, http.StatusUnauthorized},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			state := &replicaTestStore{err: scenario.err}
			mux := http.NewServeMux()
			NewResourceHandler(state).RegisterRoutes(mux)
			request := httptest.NewRequest(http.MethodDelete, "/clusters/cluster/replicas/selected", nil)
			if scenario.authenticated {
				request = request.WithContext(auth.WithUserID(request.Context(), "user"))
			}
			response := httptest.NewRecorder()
			mux.ServeHTTP(response, request)
			if response.Code != scenario.status {
				t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
			}
			if !scenario.authenticated && state.calls != 0 {
				t.Fatal("unauthenticated removal reached store")
			}
			if scenario.authenticated && (state.user != "user" || state.cluster != "cluster" || state.replica != "selected") {
				t.Fatalf("incorrect scope: %+v", state)
			}
			if scenario.status == http.StatusOK && (!strings.Contains(response.Body.String(), `"desired_revision":"43"`) || !strings.Contains(response.Body.String(), `"id":"keep"`)) {
				t.Fatalf("missing updated desired state: %s", response.Body.String())
			}
		})
	}
}
