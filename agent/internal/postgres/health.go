package postgres

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/betterorca/betterorca/pkg/types"
)

const (
	unknownStreamingState = "unknown"
	unknownLagStatus      = "unknown"
	knownLagStatus        = "known"
)

const primaryReplicationHealthQuery = `
SELECT s.slot_name,
       r.state,
       COALESCE(GREATEST(pg_wal_lsn_diff(pg_current_wal_lsn(), r.replay_lsn), 0)::numeric::text, '')
FROM pg_stat_replication AS r
JOIN pg_replication_slots AS s ON s.active_pid = r.pid
WHERE s.slot_type = 'physical'
ORDER BY s.slot_name`

const replicaReceiverHealthQuery = `
SELECT COALESCE(w.status, 'disconnected'),
       COALESCE(w.slot_name, ''),
       COALESCE(pg_last_wal_receive_lsn()::text, ''),
       COALESCE(pg_last_wal_replay_lsn()::text, '')
FROM (SELECT 1) AS one
LEFT JOIN LATERAL (
  SELECT status, slot_name
  FROM pg_stat_wal_receiver
  LIMIT 1
) AS w ON true`

// HealthDockerClient is the Docker functionality needed to inspect PostgreSQL health.
type HealthDockerClient interface {
	ExecContainer(ctx context.Context, containerID string, command []string) (string, error)
}

type primaryReplicationRow struct {
	slotName       string
	streamingState string
	lagBytes       *uint64
}

type replicaReceiverRow struct {
	streamingState string
	slotName       string
	receivedLSN    string
	replayedLSN    string
}

// PopulateReplicaHealth augments the existing actual-state report with PostgreSQL replication observations.
func PopulateReplicaHealth(ctx context.Context, docker HealthDockerClient, actual *types.ActualState) {
	if docker == nil || actual == nil {
		return
	}

	for _, cluster := range actual.Clusters {
		populateClusterReplicaHealth(ctx, docker, cluster)
	}
}

func populateClusterReplicaHealth(ctx context.Context, docker HealthDockerClient, cluster *types.ActualCluster) {
	if cluster == nil || len(cluster.Replicas) == 0 {
		return
	}

	primaryRows := map[string]primaryReplicationRow(nil)
	if cluster.ContainerId != "" {
		output, err := docker.ExecContainer(ctx, cluster.ContainerId, psqlCommand(primaryReplicationHealthQuery))
		if err == nil {
			primaryRows, err = parsePrimaryReplicationRows(output)
		}
		if err != nil {
			primaryRows = nil
		}
	}

	for _, replica := range cluster.Replicas {
		if replica == nil {
			continue
		}
		replica.StreamingState = unknownStreamingState
		replica.ReplicationLagStatus = unknownLagStatus

		var receiver replicaReceiverRow
		receiverKnown := false
		if replica.ContainerId != "" {
			output, err := docker.ExecContainer(ctx, replica.ContainerId, psqlCommand(replicaReceiverHealthQuery))
			if err == nil {
				receiver, err = parseReplicaReceiverRow(output)
			}
			if err == nil {
				receiverKnown = true
				replica.StreamingState = receiver.streamingState
				replica.LastWalReceivedLsn = receiver.receivedLSN
				replica.LastWalReplayedLsn = receiver.replayedLSN
			}
		}

		if primaryRows == nil {
			continue
		}
		connected := false
		replica.StandbyConnected = &connected
		if !receiverKnown || receiver.slotName == "" {
			replica.StreamingState = "disconnected"
			continue
		}
		primary, ok := primaryRows[receiver.slotName]
		if !ok {
			replica.StreamingState = "disconnected"
			continue
		}

		connected = true
		replica.StandbyConnected = &connected
		replica.StreamingState = primary.streamingState
		replica.ReplicationLagBytes = primary.lagBytes
		if primary.lagBytes != nil {
			replica.ReplicationLagStatus = knownLagStatus
		}
	}
}

func parsePrimaryReplicationRows(output string) (map[string]primaryReplicationRow, error) {
	rows := make(map[string]primaryReplicationRow)
	if strings.TrimSpace(output) == "" {
		return rows, nil
	}

	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Split(line, "|")
		if len(fields) != 3 {
			return nil, fmt.Errorf("parse pg_stat_replication row: got %d fields", len(fields))
		}
		slotName := strings.TrimSpace(fields[0])
		state := strings.TrimSpace(fields[1])
		if slotName == "" || state == "" {
			return nil, errors.New("parse pg_stat_replication row: slot name and state are required")
		}

		var lagBytes *uint64
		if value := strings.TrimSpace(fields[2]); value != "" {
			lag, err := strconv.ParseUint(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse pg_stat_replication lag %q: %w", value, err)
			}
			lagBytes = &lag
		}
		rows[slotName] = primaryReplicationRow{
			slotName: slotName, streamingState: state, lagBytes: lagBytes,
		}
	}
	return rows, nil
}

func parseReplicaReceiverRow(output string) (replicaReceiverRow, error) {
	fields := strings.Split(strings.TrimSpace(output), "|")
	if len(fields) != 4 {
		return replicaReceiverRow{}, fmt.Errorf("parse pg_stat_wal_receiver row: got %d fields", len(fields))
	}
	state := strings.TrimSpace(fields[0])
	if state == "" {
		return replicaReceiverRow{}, errors.New("parse pg_stat_wal_receiver row: state is required")
	}
	return replicaReceiverRow{
		streamingState: state,
		slotName:       strings.TrimSpace(fields[1]),
		receivedLSN:    strings.TrimSpace(fields[2]),
		replayedLSN:    strings.TrimSpace(fields[3]),
	}, nil
}
