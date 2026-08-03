package pgbackrest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/agent/internal/postgres"
	"github.com/swapnil404/orca/agent/internal/state"
	orcatypes "github.com/swapnil404/orca/pkg/types"
	"google.golang.org/protobuf/proto"
)

const (
	restoreJournalName = "restore-operations.json"
	restoreSourcePath  = "/var/orca/restore-source"
)

// RestoreDocker provides the host mutations used by durable restore operations.
type RestoreDocker interface {
	RecoveryExecutor
	RestartContainer(ctx context.Context, containerID string) error
	EnsureVolume(ctx context.Context, name string) error
	RemoveVolume(ctx context.Context, name string) error
	RemoveNetwork(ctx context.Context, name string) error
	ListOrcaContainers(ctx context.Context) ([]orcadocker.ContainerInfo, error)
}

type restoreJournal struct {
	Version    int                       `json:"version"`
	Operations map[string]*restoreRecord `json:"operations"`
}

type restoreRecord struct {
	Fingerprint string                            `json:"fingerprint"`
	Mode        string                            `json:"mode"`
	Source      string                            `json:"source"`
	Target      string                            `json:"target,omitempty"`
	TargetTime  string                            `json:"target_time,omitempty"`
	BackupLabel string                            `json:"backup_label,omitempty"`
	Step        int                               `json:"step"`
	SourceSpec  *orcatypes.ClusterSpec            `json:"source_spec,omitempty"`
	TargetSpec  *orcatypes.ClusterSpec            `json:"target_spec,omitempty"`
	Report      *orcatypes.RestoreOperationReport `json:"report"`
}

type restoreInfoStanza struct {
	Name   string `json:"name"`
	Status struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"status"`
	Repo []struct {
		Status struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"status"`
	} `json:"repo"`
	DB []struct {
		Version string `json:"version"`
	} `json:"db"`
	Backup []restoreInfoBackup `json:"backup"`
}

type restoreInfoBackup struct {
	Error bool   `json:"error"`
	Label string `json:"label"`
	Info  struct {
		Size       uint64 `json:"size"`
		Repository struct {
			Size uint64 `json:"size"`
		} `json:"repository"`
	} `json:"info"`
	Timestamp struct {
		Start int64 `json:"start"`
		Stop  int64 `json:"stop"`
	} `json:"timestamp"`
}

// RestoreManager journals and resumes host-local restore operations.
type RestoreManager struct {
	docker      RestoreDocker
	journalPath string
	journal     restoreJournal
	now         func() time.Time
}

// NewRestoreManager creates a manager whose journal is beside ORCA_STATE_PATH.
func NewRestoreManager(statePath string, docker RestoreDocker) *RestoreManager {
	if statePath == "" {
		statePath = state.DefaultPath()
	}
	return &RestoreManager{
		docker: docker, journalPath: filepath.Join(filepath.Dir(statePath), restoreJournalName),
		journal: restoreJournal{Version: 1, Operations: make(map[string]*restoreRecord)}, now: time.Now,
	}
}

// Process consumes the complete restore-operation snapshot and advances each intent.
func (m *RestoreManager) Process(ctx context.Context, desired *orcatypes.DesiredState) error {
	if m == nil || m.docker == nil {
		return errors.New("restore manager Docker client is nil")
	}
	if err := m.load(); err != nil {
		return err
	}
	present := make(map[string]struct{}, len(desired.GetRestoreOperations()))
	operations := append([]*orcatypes.RestoreOperation(nil), desired.GetRestoreOperations()...)
	sort.SliceStable(operations, func(i, j int) bool { return operations[i].GetId() < operations[j].GetId() })
	for _, operation := range operations {
		if operation == nil || operation.GetId() == "" {
			continue
		}
		present[operation.GetId()] = struct{}{}
		if err := m.processOne(ctx, desired, operation); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			record := m.journal.Operations[operation.GetId()]
			if record == nil || record.Report == nil || record.Report.Status != "failed" {
				return err
			}
		}
	}
	changed := false
	for id, record := range m.journal.Operations {
		if _, ok := present[id]; !ok {
			status := record.Report.GetStatus()
			if status == "cancelled" || status == "finalized" || status == "rolled_back" ||
				status == "succeeded" && record.Mode == "clone" || !record.Report.GetDestructiveStarted() {
				delete(m.journal.Operations, id)
				changed = true
			}
		}
	}
	if changed {
		return m.save(ctx)
	}
	return nil
}

// Reports returns all durable reports, including terminal reports awaiting removal/finalization.
func (m *RestoreManager) Reports() []*orcatypes.RestoreOperationReport {
	if m == nil {
		return nil
	}
	ids := make([]string, 0, len(m.journal.Operations))
	for id := range m.journal.Operations {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	reports := make([]*orcatypes.RestoreOperationReport, 0, len(ids))
	for _, id := range ids {
		if report := m.journal.Operations[id].Report; report != nil {
			reports = append(reports, proto.Clone(report).(*orcatypes.RestoreOperationReport))
		}
	}
	return reports
}

// BlockedClusters returns clusters ordinary reconciliation must not mutate.
func (m *RestoreManager) BlockedClusters(desired *orcatypes.DesiredState) map[string]struct{} {
	blocked := make(map[string]struct{})
	if m == nil {
		return blocked
	}
	for _, record := range m.journal.Operations {
		status := record.Report.GetStatus()
		if status == "ready" || !terminalStatus(status) || status == "failed" && record.Report.GetDestructiveStarted() {
			blocked[record.Source] = struct{}{}
		}
		targetActivated := record.Mode == "clone" && status == "succeeded" && clusterByID(desired, record.Target) != nil
		if record.Target != "" && !targetActivated && status != "cancelled" && status != "finalized" && status != "rolled_back" {
			blocked[record.Target] = struct{}{}
		}
	}
	return blocked
}

func (m *RestoreManager) processOne(ctx context.Context, desired *orcatypes.DesiredState, operation *orcatypes.RestoreOperation) error {
	if err := validateOperationIdentity(operation); err != nil {
		return m.reject(ctx, operation, "invalid_operation", err)
	}
	record := m.journal.Operations[operation.Id]
	if record != nil && record.Fingerprint != operation.RequestFingerprint {
		if record.Report.GetErrorCode() == "fingerprint_conflict" {
			return nil
		}
		return m.reject(ctx, operation, "fingerprint_conflict", errors.New("operation ID was already journaled with a different fingerprint"))
	}
	if record != nil && (record.Mode != normalizedMode(operation.Mode) || record.Source != operation.SourceClusterId || record.Target != operation.GetTargetCluster().GetId()) {
		if record.Report.GetErrorCode() == "fingerprint_payload_mismatch" {
			return nil
		}
		return m.reject(ctx, operation, "fingerprint_payload_mismatch", errors.New("journaled operation payload changed without a new fingerprint"))
	}
	if record == nil {
		target := ""
		if operation.TargetCluster != nil {
			target = operation.TargetCluster.Id
		}
		record = &restoreRecord{
			Fingerprint: operation.RequestFingerprint, Mode: normalizedMode(operation.Mode), Source: operation.SourceClusterId,
			Target: target, TargetTime: operation.TargetTime,
			Report: &orcatypes.RestoreOperationReport{OperationId: operation.Id, Status: "pending", Phase: "accepted", Cancellable: true},
		}
		record.Report.StartedAt = m.now().UTC().Format(time.RFC3339Nano)
		m.journal.Operations[operation.Id] = record
		if err := m.save(ctx); err != nil {
			return err
		}
	}

	intent := strings.ToLower(strings.TrimSpace(operation.Intent))
	switch intent {
	case "preflight":
		if record.Step >= 1 || terminalStatus(record.Report.Status) {
			return nil
		}
		return m.preflight(ctx, desired, operation, record)
	case "execute":
		if record.Report.Status == "succeeded" || record.Report.Status == "finalized" {
			return nil
		}
		if record.Step < 1 {
			if err := m.preflight(ctx, desired, operation, record); err != nil {
				return err
			}
		}
		if record.Report.Status == "failed" && record.Report.DestructiveStarted {
			return nil
		}
		return m.execute(ctx, desired, operation, record)
	case "rollback":
		if record.Report.Status == "rolled_back" {
			return nil
		}
		return m.rollback(ctx, desired, operation, record)
	case "cancel":
		if record.Report.Status == "cancelled" {
			return nil
		}
		return m.cancel(ctx, record)
	case "finalize":
		if record.Report.Status == "finalized" {
			return nil
		}
		return m.finalize(ctx, operation, record)
	default:
		return m.fail(ctx, record, "invalid_intent", fmt.Errorf("unsupported restore intent %q", operation.Intent))
	}
}

func (m *RestoreManager) preflight(ctx context.Context, desired *orcatypes.DesiredState, operation *orcatypes.RestoreOperation, record *restoreRecord) error {
	record.Report.Status = "running"
	record.Report.Phase = "preflight_inspecting_repository"
	record.Report.Cancellable = true
	record.Report.ErrorCode = ""
	record.Report.Error = ""
	record.Report.CompletedAt = ""
	if err := m.bumpAndSave(ctx, record); err != nil {
		return err
	}
	source := clusterByID(desired, operation.SourceClusterId)
	if source == nil || source.PgBackRest == nil {
		return m.fail(ctx, record, "source_not_configured", errors.New("source cluster with pgBackRest configuration is required"))
	}
	if source.Version == "" {
		return m.fail(ctx, record, "postgres_version_required", errors.New("source cluster must pin a PostgreSQL version for restore"))
	}
	targetTime, err := time.Parse(time.RFC3339Nano, operation.TargetTime)
	if err != nil {
		return m.fail(ctx, record, "invalid_target_time", fmt.Errorf("target time must be RFC3339: %w", err))
	}
	targetTime = targetTime.UTC()
	if targetTime.After(m.now().UTC()) {
		return m.fail(ctx, record, "target_in_future", errors.New("target time must not be in the future"))
	}
	if err := validateRecoveryRepository(source); err != nil {
		return m.fail(ctx, record, "repository_placement", err)
	}
	if record.Mode == "clone" {
		if err := m.validateClonePreflight(ctx, desired, operation, source); err != nil {
			return m.fail(ctx, record, "clone_target_invalid", err)
		}
	} else if record.Mode != "in_place" {
		return m.fail(ctx, record, "invalid_mode", fmt.Errorf("unsupported restore mode %q", operation.Mode))
	}

	primary, _ := primaryContainerName(source.Id)
	output, err := m.docker.ExecContainer(ctx, primary, []string{
		"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(source.Id), "--stanza=" + source.Id, "--output=json", "info",
	})
	if err != nil {
		return m.fail(ctx, record, "repository_unavailable", fmt.Errorf("inspect pgBackRest JSON: %w", err))
	}
	stanza, err := decodeRestoreInfo(output, source.Id)
	if err != nil {
		return m.fail(ctx, record, "repository_invalid", err)
	}
	backup, err := selectBackup(stanza, targetTime)
	if err != nil {
		return m.fail(ctx, record, "target_out_of_range", err)
	}
	if _, err := m.docker.ExecContainer(ctx, primary, []string{
		"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(source.Id), "--stanza=" + source.Id, "--set=" + backup.Label, "verify",
	}); err != nil {
		return m.fail(ctx, record, "backup_verify_failed", fmt.Errorf("verify selected backup %q: %w", backup.Label, err))
	}
	version := ""
	if len(stanza.DB) > 0 {
		version = stanza.DB[len(stanza.DB)-1].Version
	}
	if version == "" {
		return m.fail(ctx, record, "postgres_version_unknown", errors.New("pgBackRest info did not report a PostgreSQL version"))
	}
	if source.Version != "" && version != "" && majorVersion(source.Version) != majorVersion(version) {
		return m.fail(ctx, record, "postgres_version_mismatch", fmt.Errorf("repository PostgreSQL version %q does not match source version %q", version, source.Version))
	}
	if operation.TargetCluster != nil && operation.TargetCluster.Version != "" && version != "" && majorVersion(operation.TargetCluster.Version) != majorVersion(version) {
		return m.fail(ctx, record, "postgres_version_mismatch", fmt.Errorf("repository PostgreSQL version %q does not match target version %q", version, operation.TargetCluster.Version))
	}
	available, err := availableBytes(ctx, m.docker, primary, source.PgBackRest.RepoPath)
	if record.Mode == "clone" {
		target := operation.TargetCluster
		if volumeErr := m.docker.EnsureVolume(ctx, orcadocker.VolumeName(target.Id)); volumeErr != nil {
			return m.fail(ctx, record, "capacity_unknown", volumeErr)
		}
		targetPath := orcadocker.VolumeMountPath(target.Id)
		capacityCommand := `set -eu; root="$1"; test -z "$(find "$root" -mindepth 1 -maxdepth 1 -print -quit)"; exec df -Pk -- "$root"`
		output, volumeErr := m.runMaintenanceOutput(ctx, target, nil, []string{"sh", "-c", capacityCommand, "sh", targetPath})
		if volumeErr != nil {
			return m.fail(ctx, record, "capacity_unknown", volumeErr)
		}
		available, err = parseAvailableBytes(output)
	}
	if err != nil {
		return m.fail(ctx, record, "capacity_unknown", err)
	}
	required := backup.Info.Size
	if required == 0 {
		required = backup.Info.Repository.Size
	}
	if available < required {
		return m.fail(ctx, record, "insufficient_space", fmt.Errorf("restore requires %d bytes but only %d are available", required, available))
	}
	record.BackupLabel = backup.Label
	record.TargetTime = targetTime.Format(time.RFC3339Nano)
	record.SourceSpec = proto.Clone(source).(*orcatypes.ClusterSpec)
	if operation.TargetCluster != nil {
		record.TargetSpec = proto.Clone(operation.TargetCluster).(*orcatypes.ClusterSpec)
	}
	record.Step = 1
	record.Report.Status = "ready"
	record.Report.Phase = "preflight_complete"
	record.Report.BackupLabel = backup.Label
	record.Report.RecoveryEarliest = time.Unix(backup.Timestamp.Stop, 0).UTC().Format(time.RFC3339)
	record.Report.RecoveryLatest = m.now().UTC().Format(time.RFC3339)
	record.Report.PostgresVersion = version
	record.Report.RequiredBytes = required
	record.Report.AvailableBytes = available
	return m.bumpAndSave(ctx, record)
}

func (m *RestoreManager) execute(ctx context.Context, desired *orcatypes.DesiredState, operation *orcatypes.RestoreOperation, record *restoreRecord) error {
	source := record.SourceSpec
	if source == nil {
		return m.fail(ctx, record, "preflight_state_missing", errors.New("journal does not contain the preflight source specification"))
	}
	if err := m.validateSelectedBackup(ctx, source, record); err != nil {
		return m.fail(ctx, record, "backup_no_longer_available", err)
	}
	record.Report.Status = "running"
	record.Report.Error = ""
	record.Report.ErrorCode = ""
	if record.Mode == "clone" {
		return m.executeClone(ctx, source, record.TargetSpec, record)
	}
	return m.executeInPlace(ctx, source, record)
}

func (m *RestoreManager) validateSelectedBackup(ctx context.Context, source *orcatypes.ClusterSpec, record *restoreRecord) error {
	if record.BackupLabel == "" {
		return errors.New("preflight did not select a backup set")
	}
	primary, err := primaryContainerName(source.Id)
	if err != nil {
		return err
	}
	output, err := m.docker.ExecContainer(ctx, primary, []string{
		"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(source.Id), "--stanza=" + source.Id, "--output=json", "info",
	})
	if err != nil {
		return fmt.Errorf("reinspect pgBackRest repository: %w", err)
	}
	stanza, err := decodeRestoreInfo(output, source.Id)
	if err != nil {
		return err
	}
	available := false
	for _, backup := range stanza.Backup {
		if backup.Label == record.BackupLabel && !backup.Error {
			available = true
			break
		}
	}
	if !available {
		return fmt.Errorf("preflight backup set %q is no longer retained; run a new preflight", record.BackupLabel)
	}
	if _, err := m.docker.ExecContainer(ctx, primary, []string{
		"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(source.Id), "--stanza=" + source.Id, "--set=" + record.BackupLabel, "verify",
	}); err != nil {
		return fmt.Errorf("reverify preflight backup set %q: %w", record.BackupLabel, err)
	}
	return nil
}

func (m *RestoreManager) executeInPlace(ctx context.Context, source *orcatypes.ClusterSpec, record *restoreRecord) error {
	if record.Step < 10 {
		if err := m.checkpoint(ctx, record, 2, "stopping_clients_and_replicas", false, true); err != nil {
			return err
		}
		if err := m.removeDependents(ctx, source.Id, true); err != nil {
			return m.fail(ctx, record, "stop_dependents_failed", err)
		}
		record.Step = 10
		if err := m.bumpAndSave(ctx, record); err != nil {
			return err
		}
	}
	original := "restore-original-" + record.Report.OperationId
	if record.Step < 20 {
		if err := m.checkpoint(ctx, record, 3, "preserving_original_primary", true, false); err != nil {
			return err
		}
		command := `set -eu; root="$1"; original="$2"; if [ -d "$root/primary" ] && [ ! -e "$root/$original" ]; then mv "$root/primary" "$root/$original"; fi; test -d "$root/$original"; install -d -m 0700 -o postgres -g postgres "$root/primary"`
		if err := m.runMaintenance(ctx, source, nil, []string{"sh", "-c", command, "sh", orcadocker.VolumeMountPath(source.Id), original}); err != nil {
			return m.fail(ctx, record, "preserve_primary_failed", err)
		}
		record.Step = 20
		if err := m.bumpAndSave(ctx, record); err != nil {
			return err
		}
	}
	if record.Step < 30 {
		if err := m.checkpoint(ctx, record, 4, "restoring_backup", true, false); err != nil {
			return err
		}
		if err := m.restoreVolume(ctx, source, source, record, false); err != nil {
			return m.fail(ctx, record, "restore_failed", err)
		}
		record.Step = 30
		if err := m.bumpAndSave(ctx, record); err != nil {
			return err
		}
	}
	return m.startAndVerify(ctx, source, record, false)
}

func (m *RestoreManager) executeClone(ctx context.Context, source, target *orcatypes.ClusterSpec, record *restoreRecord) error {
	if target == nil {
		return m.fail(ctx, record, "target_missing", errors.New("clone target cluster is required"))
	}
	if record.Step < 20 {
		if err := m.checkpoint(ctx, record, 3, "preparing_empty_clone_target", true, false); err != nil {
			return err
		}
		if err := m.docker.EnsureVolume(ctx, orcadocker.VolumeName(target.Id)); err != nil {
			return m.fail(ctx, record, "target_volume_failed", err)
		}
		prepareMarker := orcadocker.VolumeMountPath(target.Id) + "/.orca-restore-" + record.Report.OperationId + "-prepared"
		prepareCommand := `set -eu; root="$1"; marker="$2"; if [ -f "$marker" ]; then test -d "$root/primary"; exit 0; fi; test -z "$(find "$root" -mindepth 1 -maxdepth 1 -print -quit)"; install -d -m 0700 -o postgres -g postgres "$root/primary"; touch "$marker"`
		if err := m.runMaintenance(ctx, target, nil, []string{"sh", "-c", prepareCommand, "sh", orcadocker.VolumeMountPath(target.Id), prepareMarker}); err != nil {
			return m.fail(ctx, record, "target_not_empty", err)
		}
		record.Step = 20
		if err := m.bumpAndSave(ctx, record); err != nil {
			return err
		}
	}
	if record.Step < 30 {
		if err := m.checkpoint(ctx, record, 4, "restoring_clone_from_read_only_source", true, false); err != nil {
			return err
		}
		if err := m.restoreVolume(ctx, source, target, record, true); err != nil {
			return m.fail(ctx, record, "restore_failed", err)
		}
		record.Step = 30
		if err := m.bumpAndSave(ctx, record); err != nil {
			return err
		}
	}
	return m.startAndVerify(ctx, target, record, true)
}

func (m *RestoreManager) startAndVerify(ctx context.Context, target *orcatypes.ClusterSpec, record *restoreRecord, clone bool) error {
	primary, _ := primaryContainerName(target.Id)
	if record.Step < 40 {
		if err := m.checkpoint(ctx, record, 5, "starting_recovered_primary", true, false); err != nil {
			return err
		}
		if err := m.removeContainerByName(ctx, target.Id, orcadocker.ContainerKindPrimary); err != nil {
			return m.fail(ctx, record, "replace_primary_failed", err)
		}
		containerID, err := m.docker.CreateContainer(ctx, restoredPrimarySpec(target))
		if err != nil {
			return m.fail(ctx, record, "create_primary_failed", err)
		}
		if err := m.docker.StartContainer(ctx, containerID); err != nil {
			return m.fail(ctx, record, "start_primary_failed", err)
		}
		record.Step = 40
		if err := m.bumpAndSave(ctx, record); err != nil {
			return err
		}
	}
	if record.Step < 50 {
		if err := m.checkpoint(ctx, record, 6, "verifying_recovery_target", true, false); err != nil {
			return err
		}
		readWrite, observationErr := primaryIsReadWrite(ctx, m.docker, primary)
		if !readWrite {
			if err := waitForRecoveryTarget(ctx, m.docker, primary); err != nil {
				return m.fail(ctx, record, "recovery_verification_failed", errors.Join(observationErr, err))
			}
			if _, err := m.docker.ExecContainer(ctx, primary, psqlCommand("SELECT pg_promote(true, 60)")); err != nil {
				return m.fail(ctx, record, "promotion_failed", err)
			}
			if err := waitForReadWrite(ctx, m.docker, primary); err != nil {
				return m.fail(ctx, record, "read_write_verification_failed", err)
			}
		}
		if clone {
			mode, err := m.docker.ExecContainer(ctx, primary, psqlCommand("SHOW archive_mode"))
			if err != nil || strings.TrimSpace(mode) != "off" {
				return m.fail(ctx, record, "clone_archive_enabled", errors.Join(err, fmt.Errorf("clone archive_mode is %q", strings.TrimSpace(mode))))
			}
		}
		record.Step = 50
		record.Report.Phase = "verification_complete"
		return m.bumpAndSave(ctx, record)
	}
	record.Report.Status = "succeeded"
	record.Report.Phase = "restore_complete"
	record.Report.Cancellable = false
	record.Report.RollbackAvailable = !clone
	record.Report.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
	return m.bumpAndSave(ctx, record)
}

func (m *RestoreManager) rollback(ctx context.Context, desired *orcatypes.DesiredState, operation *orcatypes.RestoreOperation, record *restoreRecord) error {
	if !record.Report.DestructiveStarted && record.Mode != "clone" {
		record.Report.Status = "rolled_back"
		record.Report.Phase = "rollback_complete"
		record.Report.Cancellable = false
		record.Report.RollbackAvailable = false
		record.Report.ErrorCode = ""
		record.Report.Error = ""
		record.Report.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
		return m.bumpAndSave(ctx, record)
	}
	record.Report.Status = "running"
	record.Report.Phase = "rolling_back"
	record.Report.Cancellable = false
	if err := m.bumpAndSave(ctx, record); err != nil {
		return err
	}
	if record.Mode == "clone" {
		if err := m.removeDependents(ctx, record.Target, true); err != nil {
			return m.fail(ctx, record, "rollback_failed", err)
		}
		if err := m.docker.RemoveNetwork(ctx, orcadocker.NetworkName(record.Target)); err != nil && !isMissingDockerObject(err) {
			return m.fail(ctx, record, "rollback_failed", err)
		}
		if err := m.docker.RemoveVolume(ctx, orcadocker.VolumeName(record.Target)); err != nil && !isMissingDockerObject(err) {
			return m.fail(ctx, record, "rollback_failed", err)
		}
	} else {
		source := record.SourceSpec
		if source == nil {
			return m.fail(ctx, record, "rollback_source_missing", errors.New("source cluster is required to roll back"))
		}
		if err := m.removeDependents(ctx, source.Id, true); err != nil {
			return m.fail(ctx, record, "rollback_failed", err)
		}
		original := "restore-original-" + operation.Id
		command := `set -eu; root="$1"; original="$2"; if [ -d "$root/$original" ]; then rm -rf "$root/primary"; mv "$root/$original" "$root/primary"; else test -d "$root/primary"; fi`
		if err := m.runMaintenance(ctx, source, nil, []string{"sh", "-c", command, "sh", orcadocker.VolumeMountPath(source.Id), original}); err != nil {
			return m.fail(ctx, record, "rollback_failed", err)
		}
		id, err := m.docker.CreateContainer(ctx, restoredPrimarySpec(source))
		if err != nil {
			return m.fail(ctx, record, "rollback_failed", err)
		}
		if err := m.docker.StartContainer(ctx, id); err != nil {
			return m.fail(ctx, record, "rollback_failed", err)
		}
		if err := waitForReadWrite(ctx, m.docker, id); err != nil {
			return m.fail(ctx, record, "rollback_verification_failed", err)
		}
	}
	record.Report.Status = "rolled_back"
	record.Report.Phase = "rollback_complete"
	record.Report.ErrorCode = ""
	record.Report.Error = ""
	record.Report.RollbackAvailable = false
	record.Report.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
	return m.bumpAndSave(ctx, record)
}

func (m *RestoreManager) cancel(ctx context.Context, record *restoreRecord) error {
	if record.Report.DestructiveStarted {
		return m.fail(ctx, record, "cancel_not_allowed", errors.New("restore cancellation is no longer safe after destructive work started"))
	}
	if record.Mode == "clone" && record.Target != "" {
		if err := m.removeDependents(ctx, record.Target, true); err != nil {
			return m.fail(ctx, record, "cancel_cleanup_failed", err)
		}
		if err := m.docker.RemoveNetwork(ctx, orcadocker.NetworkName(record.Target)); err != nil && !isMissingDockerObject(err) {
			return m.fail(ctx, record, "cancel_cleanup_failed", err)
		}
		if err := m.docker.RemoveVolume(ctx, orcadocker.VolumeName(record.Target)); err != nil && !isMissingDockerObject(err) {
			return m.fail(ctx, record, "cancel_cleanup_failed", err)
		}
	}
	record.Report.Status = "cancelled"
	record.Report.Phase = "cancelled"
	record.Report.Cancellable = false
	record.Report.RollbackAvailable = false
	record.Report.ErrorCode = ""
	record.Report.Error = ""
	record.Report.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
	return m.bumpAndSave(ctx, record)
}

func (m *RestoreManager) finalize(ctx context.Context, operation *orcatypes.RestoreOperation, record *restoreRecord) error {
	if record.Report.Status != "succeeded" && record.Report.Status != "rolled_back" {
		return m.fail(ctx, record, "finalize_not_allowed", errors.New("only a succeeded or rolled-back restore can be finalized"))
	}
	if record.Mode == "in_place" && record.Report.Status == "succeeded" {
		source := &orcatypes.ClusterSpec{Id: record.Source, Version: record.Report.PostgresVersion}
		original := "restore-original-" + operation.Id
		command := `set -eu; root="$1"; original="$2"; rm -rf "$root/$original"`
		if err := m.runMaintenance(ctx, source, nil, []string{"sh", "-c", command, "sh", orcadocker.VolumeMountPath(source.Id), original}); err != nil {
			return m.fail(ctx, record, "finalize_failed", err)
		}
	}
	record.Report.Status = "finalized"
	record.Report.Phase = "finalized"
	record.Report.ErrorCode = ""
	record.Report.Error = ""
	record.Report.RollbackAvailable = false
	record.Report.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
	return m.bumpAndSave(ctx, record)
}

func (m *RestoreManager) restoreVolume(ctx context.Context, source, target *orcatypes.ClusterSpec, record *restoreRecord, clone bool) error {
	config, mounts, err := restoreConfigAndMounts(source, target, clone)
	if err != nil {
		return err
	}
	spec := orcadocker.ContainerSpec{
		ClusterID: target.Id, Kind: orcadocker.ContainerKindPgBackRest, Image: restoreImage(target.Version),
		Command: []string{"sleep", "infinity"}, Volumes: mounts,
		Config: &orcadocker.ConfigMount{RelativePath: configRelativePath, ContainerPath: recoveryConfigPath, Content: config},
	}
	_ = m.removeContainerByName(ctx, target.Id, orcadocker.ContainerKindPgBackRest)
	id, err := m.docker.CreateContainer(ctx, spec)
	if err != nil {
		return fmt.Errorf("create recovery container: %w", err)
	}
	if err := m.docker.StartContainer(ctx, id); err != nil {
		return fmt.Errorf("start recovery container: %w", err)
	}
	marker := orcadocker.VolumeMountPath(target.Id) + "/.orca-restore-" + record.Report.OperationId + "-complete"
	command := restoreSetCommand(source.Id, target.Id, record.BackupLabel, record.TargetTime, marker, clone)
	_, restoreErr := m.docker.ExecContainer(ctx, id, command)
	cleanupErr := m.removeContainer(ctx, id)
	return errors.Join(restoreErr, cleanupErr)
}

func (m *RestoreManager) runMaintenance(ctx context.Context, target *orcatypes.ClusterSpec, volumes []orcadocker.VolumeMount, command []string) error {
	_, err := m.runMaintenanceOutput(ctx, target, volumes, command)
	return err
}

func (m *RestoreManager) runMaintenanceOutput(ctx context.Context, target *orcatypes.ClusterSpec, volumes []orcadocker.VolumeMount, command []string) (string, error) {
	if len(volumes) == 0 {
		volumes = []orcadocker.VolumeMount{{Name: orcadocker.VolumeName(target.Id), Path: orcadocker.VolumeMountPath(target.Id)}}
	}
	_ = m.removeContainerByName(ctx, target.Id, orcadocker.ContainerKindPgBackRest)
	id, err := m.docker.CreateContainer(ctx, orcadocker.ContainerSpec{
		ClusterID: target.Id, Kind: orcadocker.ContainerKindPgBackRest, Image: restoreImage(target.Version),
		Command: []string{"sleep", "infinity"}, Volumes: volumes,
	})
	if err != nil {
		return "", fmt.Errorf("create maintenance container: %w", err)
	}
	if err := m.docker.StartContainer(ctx, id); err != nil {
		return "", errors.Join(err, m.removeContainer(ctx, id))
	}
	output, commandErr := m.docker.ExecContainer(ctx, id, command)
	return output, errors.Join(commandErr, m.removeContainer(ctx, id))
}

func (m *RestoreManager) removeDependents(ctx context.Context, clusterID string, includePrimary bool) error {
	containers, err := m.docker.ListOrcaContainers(ctx)
	if err != nil {
		return err
	}
	order := map[orcadocker.ContainerKind]int{orcadocker.ContainerKindPgBouncer: 0, orcadocker.ContainerKindReplica: 1, orcadocker.ContainerKindPrimary: 2}
	sort.SliceStable(containers, func(i, j int) bool { return order[containers[i].Kind] < order[containers[j].Kind] })
	var result error
	for _, container := range containers {
		if container.ClusterID != clusterID || container.Kind == orcadocker.ContainerKindPrimary && !includePrimary {
			continue
		}
		result = errors.Join(result, m.removeContainer(ctx, container.ID))
	}
	return result
}

func (m *RestoreManager) removeContainerByName(ctx context.Context, clusterID string, kind orcadocker.ContainerKind) error {
	name, err := orcadocker.ContainerName(orcadocker.ContainerSpec{ClusterID: clusterID, Kind: kind})
	if err != nil {
		return err
	}
	return m.removeContainer(ctx, name)
}

func (m *RestoreManager) removeContainer(ctx context.Context, id string) error {
	stopErr := m.docker.StopContainer(ctx, id)
	removeErr := m.docker.RemoveContainer(ctx, id)
	if isMissingDockerObject(stopErr) {
		stopErr = nil
	}
	if isMissingDockerObject(removeErr) {
		removeErr = nil
	}
	return errors.Join(stopErr, removeErr)
}

func (m *RestoreManager) validateClonePreflight(ctx context.Context, desired *orcatypes.DesiredState, operation *orcatypes.RestoreOperation, source *orcatypes.ClusterSpec) error {
	target := operation.TargetCluster
	if target == nil || target.Id == "" || target.Id == source.Id {
		return errors.New("clone requires a distinct reserved target ClusterSpec")
	}
	if target.Version == "" {
		return errors.New("clone target must pin a PostgreSQL version")
	}
	if existing := clusterByID(desired, target.Id); existing != nil {
		return errors.New("clone target is already active in desired state")
	}
	containers, err := m.docker.ListOrcaContainers(ctx)
	if err != nil {
		return err
	}
	for _, container := range containers {
		if container.ClusterID == target.Id {
			return fmt.Errorf("clone target %q already has containers", target.Id)
		}
	}
	return nil
}

func (m *RestoreManager) checkpoint(ctx context.Context, record *restoreRecord, sequence uint64, phase string, destructive, cancellable bool) error {
	if sequence <= record.Report.Sequence {
		sequence = record.Report.Sequence + 1
	}
	record.Report.Sequence = sequence
	record.Report.Phase = phase
	record.Report.DestructiveStarted = record.Report.DestructiveStarted || destructive
	record.Report.Cancellable = cancellable
	record.Report.RollbackAvailable = record.Report.DestructiveStarted && record.Mode == "in_place"
	return m.save(ctx)
}

func (m *RestoreManager) fail(ctx context.Context, record *restoreRecord, code string, err error) error {
	record.Report.Sequence++
	record.Report.Status = "failed"
	record.Report.Phase = "failed"
	record.Report.ErrorCode = code
	record.Report.Error = err.Error()
	record.Report.Cancellable = false
	record.Report.RollbackAvailable = record.Report.DestructiveStarted
	record.Report.CompletedAt = m.now().UTC().Format(time.RFC3339Nano)
	if saveErr := m.save(ctx); saveErr != nil {
		return errors.Join(err, saveErr)
	}
	return err
}

func (m *RestoreManager) reject(ctx context.Context, operation *orcatypes.RestoreOperation, code string, err error) error {
	record := m.journal.Operations[operation.GetId()]
	if record == nil {
		record = &restoreRecord{Fingerprint: operation.GetRequestFingerprint(), Source: operation.GetSourceClusterId(), Mode: normalizedMode(operation.GetMode())}
		record.Report = &orcatypes.RestoreOperationReport{OperationId: operation.GetId(), StartedAt: m.now().UTC().Format(time.RFC3339Nano)}
		m.journal.Operations[operation.GetId()] = record
	}
	return m.fail(ctx, record, code, err)
}

func (m *RestoreManager) bumpAndSave(ctx context.Context, record *restoreRecord) error {
	record.Report.Sequence++
	return m.save(ctx)
}

func (m *RestoreManager) load() error {
	data, err := os.ReadFile(m.journalPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read restore journal: %w", err)
	}
	var journal restoreJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return fmt.Errorf("decode restore journal: %w", err)
	}
	if journal.Version != 1 {
		return fmt.Errorf("unsupported restore journal version %d", journal.Version)
	}
	if journal.Operations == nil {
		journal.Operations = make(map[string]*restoreRecord)
	}
	for id, record := range journal.Operations {
		if record == nil || record.Report == nil {
			return fmt.Errorf("restore journal operation %q has no report", id)
		}
	}
	m.journal = journal
	return nil
}

func (m *RestoreManager) save(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m.journal, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal restore journal: %w", err)
	}
	data = append(data, '\n')
	dir := filepath.Dir(m.journalPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create restore journal directory: %w", err)
	}
	temporary, err := os.CreateTemp(dir, ".restore-operations-*.tmp")
	if err != nil {
		return fmt.Errorf("create restore journal temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, m.journalPath); err != nil {
		return fmt.Errorf("replace restore journal: %w", err)
	}
	directory, err := os.Open(dir)
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	closeErr := directory.Close()
	return errors.Join(syncErr, closeErr)
}

func decodeRestoreInfo(output, stanzaName string) (*restoreInfoStanza, error) {
	var stanzas []restoreInfoStanza
	if err := json.Unmarshal([]byte(output), &stanzas); err != nil {
		return nil, fmt.Errorf("decode pgBackRest info JSON: %w", err)
	}
	for index := range stanzas {
		stanza := &stanzas[index]
		if stanza.Name != stanzaName {
			continue
		}
		if stanza.Status.Code != 0 {
			return nil, fmt.Errorf("stanza status %d: %s", stanza.Status.Code, stanza.Status.Message)
		}
		if len(stanza.Repo) == 0 {
			return nil, errors.New("pgBackRest info did not report a repository")
		}
		for _, repository := range stanza.Repo {
			if repository.Status.Code != 0 {
				return nil, fmt.Errorf("repository status %d: %s", repository.Status.Code, repository.Status.Message)
			}
		}
		return stanza, nil
	}
	return nil, fmt.Errorf("pgBackRest info did not contain stanza %q", stanzaName)
}

func selectBackup(stanza *restoreInfoStanza, target time.Time) (*restoreInfoBackup, error) {
	var selected *restoreInfoBackup
	for index := range stanza.Backup {
		backup := &stanza.Backup[index]
		if backup.Error || backup.Label == "" || backup.Timestamp.Stop <= 0 || backup.Timestamp.Stop > target.Unix() {
			continue
		}
		if selected == nil || backup.Timestamp.Stop > selected.Timestamp.Stop {
			selected = backup
		}
	}
	if selected == nil {
		return nil, errors.New("no successful backup completed at or before the recovery target")
	}
	return selected, nil
}

func availableBytes(ctx context.Context, executor Executor, containerID, path string) (uint64, error) {
	output, err := executor.ExecContainer(ctx, containerID, []string{"df", "-Pk", "--", path})
	if err != nil {
		return 0, fmt.Errorf("inspect repository filesystem capacity: %w", err)
	}
	return parseAvailableBytes(output)
}

func parseAvailableBytes(output string) (uint64, error) {
	fields := strings.Fields(output)
	if len(fields) < 6 {
		return 0, fmt.Errorf("unexpected df output %q", output)
	}
	availableKB, err := strconv.ParseUint(fields[len(fields)-3], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse available filesystem capacity: %w", err)
	}
	return availableKB * 1024, nil
}

func restoreConfigAndMounts(source, target *orcatypes.ClusterSpec, clone bool) (string, []orcadocker.VolumeMount, error) {
	if source == nil || target == nil || source.PgBackRest == nil {
		return "", nil, errors.New("source, target, and source repository configuration are required")
	}
	repoPath := filepath.Clean(source.PgBackRest.RepoPath)
	mounts := []orcadocker.VolumeMount{{Name: orcadocker.VolumeName(target.Id), Path: orcadocker.VolumeMountPath(target.Id)}}
	if clone {
		relative, err := filepath.Rel(filepath.Clean(orcadocker.VolumeMountPath(source.Id)), repoPath)
		if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", nil, errors.New("source repository must be inside the source cluster volume")
		}
		repoPath = filepath.Join(restoreSourcePath, relative)
		mounts = append(mounts, orcadocker.VolumeMount{Name: orcadocker.VolumeName(source.Id), Path: restoreSourcePath, ReadOnly: true})
	}
	config := fmt.Sprintf("[global]\nrepo1-path=%s\n\n[%s]\npg1-path=%s/primary\n", repoPath, source.Id, orcadocker.VolumeMountPath(target.Id))
	return config, mounts, nil
}

func restoreSetCommand(sourceID, targetID, label, targetTime, marker string, clone bool) []string {
	arguments := []string{
		"--config=" + recoveryConfigPath, "--stanza=" + sourceID, "--set=" + label,
		"--type=time", "--target=" + targetTime, "--target-action=pause",
	}
	if clone {
		arguments = append(arguments, "--archive-mode=off")
	}
	arguments = append(arguments, "restore")
	dataPath := orcadocker.VolumeMountPath(targetID) + "/primary"
	command := []string{
		"sh", "-c",
		`set -eu; data="$1"; marker="$2"; shift 2; if [ -f "$marker" ]; then exit 0; fi; rm -rf "$data"; install -d -m 0700 -o postgres -g postgres "$data"; gosu postgres pgbackrest "$@"; touch "$marker"`,
		"sh", dataPath, marker,
	}
	return append(command, arguments...)
}

func primaryIsReadWrite(ctx context.Context, executor Executor, primary string) (bool, error) {
	output, err := executor.ExecContainer(ctx, primary, psqlCommand("SELECT pg_is_in_recovery()::text || '|' || current_setting('transaction_read_only')"))
	if err != nil {
		return false, err
	}
	state := strings.TrimSpace(output)
	return state == "false|off" || state == "f|off", nil
}

func restoredPrimarySpec(cluster *orcatypes.ClusterSpec) orcadocker.ContainerSpec {
	config, _ := postgres.RenderConfig(cluster.Id, cluster.Params)
	return orcadocker.ContainerSpec{
		ClusterID: cluster.Id, Kind: orcadocker.ContainerKindPrimary, Image: restoreImage(cluster.Version), UseVolume: true,
		Env:     []string{"POSTGRES_HOST_AUTH_METHOD=reject", "PGDATA=" + orcadocker.VolumeMountPath(cluster.Id) + "/primary"},
		Command: []string{"postgres", "-c", "config_file=" + orcadocker.PostgresConfigContainerPath},
		Config:  &orcadocker.ConfigMount{RelativePath: orcadocker.PostgresConfigRelativePath, ContainerPath: orcadocker.PostgresConfigContainerPath, Content: config},
	}
}

func restoreImage(version string) string {
	if version == "" {
		return "orca-postgres:latest"
	}
	return "orca-postgres:" + version
}

func clusterByID(desired *orcatypes.DesiredState, id string) *orcatypes.ClusterSpec {
	if desired == nil {
		return nil
	}
	for _, cluster := range desired.Clusters {
		if cluster != nil && cluster.Id == id {
			return cluster
		}
	}
	return nil
}

func validateOperationIdentity(operation *orcatypes.RestoreOperation) error {
	if operation == nil || operation.Id == "" || filepath.Base(operation.Id) != operation.Id || operation.Id == "." || operation.Id == ".." {
		return errors.New("operation ID must be a non-empty path segment")
	}
	if operation.RequestFingerprint == "" {
		return errors.New("request fingerprint is required")
	}
	if operation.SourceClusterId == "" {
		return errors.New("source cluster ID is required")
	}
	return nil
}

func normalizedMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "in-place" || mode == "inplace" {
		return "in_place"
	}
	return mode
}

func majorVersion(version string) string {
	version = strings.TrimPrefix(strings.TrimSpace(version), "v")
	major, _, _ := strings.Cut(version, ".")
	return major
}

func terminalStatus(status string) bool {
	switch status {
	case "ready", "succeeded", "failed", "cancelled", "rolled_back", "finalized":
		return true
	default:
		return false
	}
}

func isMissingDockerObject(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "no such container") || strings.Contains(message, "no such volume") ||
		strings.Contains(message, "is not running") || strings.Contains(message, "already stopped")
}
