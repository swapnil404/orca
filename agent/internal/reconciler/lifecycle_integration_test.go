package reconciler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/state"
	"github.com/swapnil404/orca/pkg/types"
)

// Scope observation to this test's unique cluster. The normal runner removes
// observed resources absent from desired state, so passing an unfiltered host
// inventory would put unrelated Orca deployments at risk.
type scopedIntegrationDocker struct {
	*orcadocker.Client
	clusterID string
}

func (d *scopedIntegrationDocker) ListOrcaContainers(ctx context.Context) ([]orcadocker.ContainerInfo, error) {
	all, err := d.Client.ListOrcaContainers(ctx)
	if err != nil {
		return nil, err
	}
	var own []orcadocker.ContainerInfo
	for _, container := range all {
		if container.ClusterID == d.clusterID {
			own = append(own, container)
		}
	}
	return own, nil
}
func (d *scopedIntegrationDocker) ListOrcaVolumes(ctx context.Context) ([]orcadocker.VolumeInfo, error) {
	all, err := d.Client.ListOrcaVolumes(ctx)
	if err != nil {
		return nil, err
	}
	var own []orcadocker.VolumeInfo
	for _, volume := range all {
		if volume.ClusterID == d.clusterID {
			own = append(own, volume)
		}
	}
	return own, nil
}

func TestIntegrationClusterLifecycleAndOfflineCache(t *testing.T) {
	if os.Getenv("ORCA_INTEGRATION_TESTS") != "1" {
		t.Skip("set ORCA_INTEGRATION_TESTS=1 to use disposable Docker/Postgres resources")
	}
	root := t.TempDir()
	t.Setenv("ORCA_DATA_DIR", root)
	cachePath := filepath.Join(root, "desired.json")
	t.Setenv("ORCA_STATE_PATH", cachePath)
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		t.Fatal(err)
	}
	id := "revival-" + hex.EncodeToString(suffix[:])
	client, err := orcadocker.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	docker := &scopedIntegrationDocker{Client: client, clusterID: id}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		containers, err := docker.ListOrcaContainers(ctx)
		if err != nil {
			t.Error(err)
			return
		}
		for _, container := range containers {
			if err := docker.StopContainer(ctx, container.ID); err != nil {
				t.Error(err)
			}
			if err := docker.RemoveContainer(ctx, container.ID); err != nil {
				t.Error(err)
			}
		}
		volumes, err := docker.ListOrcaVolumes(ctx)
		if err != nil {
			t.Error(err)
		}
		for _, volume := range volumes {
			if err := docker.RemoveVolume(ctx, volume.Name); err != nil {
				t.Error(err)
			}
		}
		if err := docker.RemoveNetwork(ctx, orcadocker.NetworkName(id)); err != nil {
			t.Error(err)
		}
	})
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	desired := &DesiredState{Revision: "initial", Clusters: []*ClusterSpec{{Id: id, Version: "17", Databases: []*types.DatabaseSpec{{Name: "postgres"}}, PgHba: &types.PgHbaSpec{Rules: []*types.PgHbaRule{{Type: "host", Database: "all", User: "postgres", Address: "0.0.0.0/0", Method: "scram-sha-256"}}}, PgBouncer: &PgBouncerSpec{PoolMode: "transaction", MaxConnections: 30, PublishAddress: "127.0.0.1", PublishPort: uint32(port)}}}}
	runner := NewRunner(state.NewFileCache(cachePath), docker)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	pass, err := runner.Reconcile(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	assertPassSucceeded(t, pass)
	if len(pass.Report.ActualState.Clusters) != 1 {
		t.Fatalf("expected one observed cluster: %v", pass.Report.ActualState)
	}
	before := pass.Report.ActualState.Clusters[0].ContainerId
	password, err := docker.EnsureClusterPassword(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	config, err := pgx.ParseConfig("postgres://postgres@127.0.0.1:" + strconv.Itoa(port) + "/postgres?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}
	config.Password = password
	connection, err := pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Exec(ctx, "CREATE TABLE revival_probe (value integer); INSERT INTO revival_probe VALUES (42)"); err != nil {
		connection.Close(ctx)
		t.Fatal(err)
	}
	var value int
	if err := connection.QueryRow(ctx, "SELECT value FROM revival_probe").Scan(&value); err != nil || value != 42 {
		connection.Close(ctx)
		t.Fatalf("pool query value = %d, error = %v", value, err)
	}
	if err := connection.Close(ctx); err != nil {
		t.Fatal(err)
	}
	restarted := NewRunner(state.NewFileCache(cachePath), docker)
	pass, err = restarted.ReconcileCached(ctx)
	if err != nil {
		t.Fatal(err)
	}
	assertPassSucceeded(t, pass)
	if len(pass.Results) != 0 || pass.Report.ActualState.Clusters[0].ContainerId != before || pass.Report.DesiredStateRevision != "initial" {
		t.Fatalf("cached/offline pass changed settled resources: %+v", pass.Results)
	}
	t.Log("SQL through authenticated PgBouncer and offline cache restart passed")
	desired.Revision = "replica-added"
	desired.Clusters[0].Replicas = []*ReplicaSpec{{Id: "standby"}}
	pass, err = restarted.Reconcile(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	assertPassSucceeded(t, pass)
	observed := pass.Report.ActualState.Clusters[0]
	if len(observed.Replicas) != 1 || observed.ContainerId != before {
		t.Fatal("replica addition replaced the primary or lost the replica")
	}
	replicaID := observed.Replicas[0].ContainerId
	connection, err = pgx.ConnectConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = connection.Exec(ctx, "INSERT INTO revival_probe VALUES (99)"); err != nil {
		connection.Close(ctx)
		t.Fatal(err)
	}
	if err := connection.Close(ctx); err != nil {
		t.Fatal(err)
	}
	replicationDeadline := time.Now().Add(20 * time.Second)
	for {
		output, queryErr := docker.ExecContainer(ctx, replicaID, []string{"psql", "-U", "postgres", "-Atqc", "SELECT pg_is_in_recovery()::text || '|' || (SELECT count(*)::text FROM revival_probe)"})
		if queryErr == nil && strings.TrimSpace(output) == "true|2" {
			break
		}
		if time.Now().After(replicationDeadline) {
			t.Fatalf("streaming data did not reach standby: %q, %v", output, queryErr)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(100 * time.Millisecond):
		}
	}
	desired.Revision = "parameter-update"
	desired.Clusters[0].Params = map[string]string{"log_min_duration_statement": "250"}
	pass, err = restarted.Reconcile(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	assertPassSucceeded(t, pass)
	for _, containerID := range []string{before, replicaID} {
		output, queryErr := docker.ExecContainer(ctx, containerID, []string{"psql", "-U", "postgres", "-Atqc", "SHOW log_min_duration_statement"})
		if queryErr != nil || strings.TrimSpace(output) != "250ms" {
			t.Fatalf("parameter not applied to node: %q, %v", output, queryErr)
		}
	}
	desired.Revision = "restart-request"
	desired.Clusters[0].RestartGeneration = 1
	pass, err = restarted.Reconcile(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	assertPassSucceeded(t, pass)
	if pass.Report.ActualState.Clusters[0].AppliedRestartGeneration != 1 {
		t.Fatal("restart request was not acknowledged")
	}
	desired.Revision = "replica-removed"
	desired.Clusters[0].Replicas = nil
	pass, err = restarted.Reconcile(ctx, desired)
	if err != nil {
		t.Fatal(err)
	}
	assertPassSucceeded(t, pass)
	if len(pass.Report.ActualState.Clusters[0].Replicas) != 0 {
		t.Fatal("replica persisted after removal")
	}
	output, queryErr := docker.ExecContainer(ctx, before, []string{"psql", "-U", "postgres", "-Atqc", "SELECT count(*) FROM pg_replication_slots"})
	if queryErr != nil || strings.TrimSpace(output) != "0" {
		t.Fatalf("replica removal leaked a replication slot: %q, %v", output, queryErr)
	}
	t.Log("streaming replication, parameter convergence on both nodes, restart acknowledgement, and replica/slot removal passed")
	pass, err = restarted.Reconcile(ctx, &DesiredState{Revision: "removed"})
	if err != nil {
		t.Fatal(err)
	}
	assertPassSucceeded(t, pass)
	containers, err := docker.ListOrcaContainers(ctx)
	if err != nil || len(containers) != 0 {
		t.Fatalf("containers after deletion = %v, error = %v", containers, err)
	}
	volumes, err := docker.ListOrcaVolumes(ctx)
	if err != nil || len(volumes) != 0 {
		t.Fatalf("volumes after deletion = %v, error = %v", volumes, err)
	}
}

func assertPassSucceeded(t *testing.T, pass Pass) {
	t.Helper()
	for _, result := range pass.Results {
		if result.Status != ApplyStatusSuccess {
			t.Fatalf("action %s: %s: %v", result.Action.Type, result.Status, result.Err)
		}
	}
}
