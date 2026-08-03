package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/swapnil404/orca/pkg/types"
	"github.com/swapnil404/orca/server/internal/store/sqlcdb"
)

var (
	// ErrRestoreIdempotencyConflict indicates reuse of a key for a different request.
	ErrRestoreIdempotencyConflict = errors.New("idempotency key was already used for a different restore request")
	// ErrRestoreOperationConflict indicates another active operation touches the source or target.
	ErrRestoreOperationConflict = errors.New("source or target already has an active restore operation")
	// ErrRestorePgBackRestRequired indicates that the source has no backup configuration.
	ErrRestorePgBackRestRequired = errors.New("source cluster must have pgBackRest enabled")
	// ErrRestoreInvalidTransition indicates that an intent is not valid for the current state.
	ErrRestoreInvalidTransition = errors.New("invalid restore operation transition")
	// ErrRestoreInvalidConfirmation indicates that typed confirmation does not match the operation target.
	ErrRestoreInvalidConfirmation = errors.New("invalid restore operation confirmation")
)

// RestoreOperation is the durable API representation of one recovery operation.
type RestoreOperation struct {
	ID                 string                        `json:"id"`
	ProjectID          string                        `json:"project_id"`
	HostID             string                        `json:"host_id"`
	SourceClusterID    string                        `json:"source_cluster_id"`
	TargetClusterID    string                        `json:"target_cluster_id,omitempty"`
	TargetClusterName  string                        `json:"target_cluster_name,omitempty"`
	TargetCluster      *types.ClusterSpec            `json:"target_cluster,omitempty"`
	Mode               string                        `json:"mode"`
	Intent             string                        `json:"intent"`
	Status             string                        `json:"status"`
	TargetTime         time.Time                     `json:"target_time"`
	RequestFingerprint string                        `json:"request_fingerprint"`
	RequestedByUserID  string                        `json:"requested_by_user_id"`
	ReportSequence     uint64                        `json:"report_sequence"`
	Report             *types.RestoreOperationReport `json:"report,omitempty"`
	CreatedAt          time.Time                     `json:"created_at"`
	UpdatedAt          time.Time                     `json:"updated_at"`
	FinalizedAt        *time.Time                    `json:"finalized_at,omitempty"`
}

// CreateRestoreOperationParams contains an authenticated restore request.
type CreateRestoreOperationParams struct {
	ID                 string
	UserID             string
	SourceClusterID    string
	TargetClusterID    string
	TargetClusterName  string
	Mode               string
	TargetTime         time.Time
	RequestFingerprint string
	IdempotencyKey     string
}

// CreateRestoreOperation creates a prepare intent or returns its idempotent predecessor.
func (s *Postgres) CreateRestoreOperation(ctx context.Context, params CreateRestoreOperationParams) (RestoreOperation, bool, error) {
	existing, err := s.queries.FindRestoreOperationByIdempotencyKey(ctx, sqlcdb.FindRestoreOperationByIdempotencyKeyParams{
		UserID: params.UserID, IdempotencyKey: params.IdempotencyKey,
	})
	if err == nil {
		operation, decodeErr := restoreOperationFromSQLC(existing)
		if decodeErr != nil {
			return RestoreOperation{}, false, decodeErr
		}
		if operation.RequestFingerprint != params.RequestFingerprint {
			return RestoreOperation{}, false, ErrRestoreIdempotencyConflict
		}
		return operation, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RestoreOperation{}, false, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RestoreOperation{}, false, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	source, err := queries.GetRestoreMutationContext(ctx, sqlcdb.GetRestoreMutationContextParams{
		SourceClusterID: params.SourceClusterID, UserID: params.UserID,
	})
	if err != nil {
		return RestoreOperation{}, false, err
	}
	if err := requireMutationRole(source.Role); err != nil {
		return RestoreOperation{}, false, err
	}
	if !source.PgbackrestEnabled {
		return RestoreOperation{}, false, ErrRestorePgBackRestRequired
	}

	var targetID, targetName sql.NullString
	targetJSON := json.RawMessage("null")
	if params.Mode == string(sqlcdb.RestoreOperationModeClone) {
		targetID = sql.NullString{String: params.TargetClusterID, Valid: true}
		targetName = sql.NullString{String: params.TargetClusterName, Valid: true}
		target, buildErr := cloneTargetFromSource(source, params.TargetClusterID)
		if buildErr != nil {
			return RestoreOperation{}, false, buildErr
		}
		payload, marshalErr := protojson.Marshal(target)
		if marshalErr != nil {
			return RestoreOperation{}, false, fmt.Errorf("marshal clone target: %w", marshalErr)
		}
		targetJSON = payload
	}
	resources := []string{params.SourceClusterID}
	if targetID.Valid {
		resources = append(resources, targetID.String)
	}
	sort.Strings(resources)
	for _, resourceID := range resources {
		if err := queries.LockRestoreResource(ctx, resourceID); err != nil {
			return RestoreOperation{}, false, err
		}
	}
	existing, err = queries.FindRestoreOperationByIdempotencyKey(ctx, sqlcdb.FindRestoreOperationByIdempotencyKeyParams{
		UserID: params.UserID, IdempotencyKey: params.IdempotencyKey,
	})
	if err == nil {
		operation, decodeErr := restoreOperationFromSQLC(existing)
		if decodeErr != nil {
			return RestoreOperation{}, false, decodeErr
		}
		if operation.RequestFingerprint != params.RequestFingerprint {
			return RestoreOperation{}, false, ErrRestoreIdempotencyConflict
		}
		return operation, false, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return RestoreOperation{}, false, err
	}
	conflict, err := queries.HasActiveRestoreConflict(ctx, sqlcdb.HasActiveRestoreConflictParams{
		SourceClusterID: params.SourceClusterID, TargetClusterID: targetID,
	})
	if err != nil {
		return RestoreOperation{}, false, err
	}
	if conflict {
		return RestoreOperation{}, false, ErrRestoreOperationConflict
	}
	row, err := queries.CreateRestoreOperation(ctx, sqlcdb.CreateRestoreOperationParams{
		ID: params.ID, ProjectID: source.ProjectID, HostID: source.HostID,
		SourceClusterID: params.SourceClusterID, TargetClusterID: targetID,
		TargetClusterName: targetName, TargetSpec: targetJSON,
		Mode: sqlcdb.RestoreOperationMode(params.Mode), TargetTime: params.TargetTime,
		RequestFingerprint: params.RequestFingerprint, IdempotencyKey: params.IdempotencyKey,
		RequestedByUserID: params.UserID,
	})
	if err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr == nil {
			existing, lookupErr := s.queries.FindRestoreOperationByIdempotencyKey(ctx, sqlcdb.FindRestoreOperationByIdempotencyKeyParams{
				UserID: params.UserID, IdempotencyKey: params.IdempotencyKey,
			})
			if lookupErr == nil {
				operation, decodeErr := restoreOperationFromSQLC(existing)
				if decodeErr != nil {
					return RestoreOperation{}, false, decodeErr
				}
				if operation.RequestFingerprint != params.RequestFingerprint {
					return RestoreOperation{}, false, ErrRestoreIdempotencyConflict
				}
				return operation, false, nil
			}
		}
		if restoreConstraintConflict(err) {
			return RestoreOperation{}, false, ErrRestoreOperationConflict
		}
		return RestoreOperation{}, false, err
	}
	if _, err := queries.CreateRestoreOperationEvent(ctx, sqlcdb.CreateRestoreOperationEventParams{
		OperationID: row.ID, HostID: row.HostID, EventType: "created", Payload: json.RawMessage(`{"intent":"preflight"}`),
	}); err != nil {
		return RestoreOperation{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return RestoreOperation{}, false, err
	}
	operation, err := restoreOperationFromSQLC(row)
	return operation, true, err
}

// ListRestoreOperations returns project operations visible to an organization member.
func (s *Postgres) ListRestoreOperations(ctx context.Context, userID, projectID string) ([]RestoreOperation, error) {
	rows, err := s.queries.ListRestoreOperations(ctx, sqlcdb.ListRestoreOperationsParams{ProjectID: projectID, UserID: userID})
	if err != nil {
		return nil, err
	}
	operations := make([]RestoreOperation, 0, len(rows))
	for _, row := range rows {
		operation, err := restoreOperationFromSQLC(row)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, nil
}

// GetRestoreOperation returns an operation visible to an organization member.
func (s *Postgres) GetRestoreOperation(ctx context.Context, userID, operationID string) (RestoreOperation, error) {
	row, err := s.queries.GetRestoreOperation(ctx, sqlcdb.GetRestoreOperationParams{ID: operationID, UserID: userID})
	if err != nil {
		return RestoreOperation{}, err
	}
	return restoreOperationFromSQLC(row)
}

// ConfirmRestoreOperation changes an operation's intent to execute after validating typed confirmation.
func (s *Postgres) ConfirmRestoreOperation(ctx context.Context, userID, operationID, confirmation string) (RestoreOperation, error) {
	return s.changeRestoreIntent(ctx, userID, operationID, sqlcdb.RestoreOperationIntentExecute, confirmation)
}

// CancelRestoreOperation asks the agent to cancel an operation.
func (s *Postgres) CancelRestoreOperation(ctx context.Context, userID, operationID string) (RestoreOperation, error) {
	return s.changeRestoreIntent(ctx, userID, operationID, sqlcdb.RestoreOperationIntentCancel, "")
}

// RollbackRestoreOperation asks the agent to roll back an operation after validating typed confirmation.
func (s *Postgres) RollbackRestoreOperation(ctx context.Context, userID, operationID, confirmation string) (RestoreOperation, error) {
	return s.changeRestoreIntent(ctx, userID, operationID, sqlcdb.RestoreOperationIntentRollback, confirmation)
}

func (s *Postgres) changeRestoreIntent(ctx context.Context, userID, operationID string, intent sqlcdb.RestoreOperationIntent, confirmation string) (RestoreOperation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RestoreOperation{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	current, err := queries.GetRestoreOperationForUpdate(ctx, sqlcdb.GetRestoreOperationForUpdateParams{ID: operationID, UserID: userID})
	if err != nil {
		return RestoreOperation{}, err
	}
	if err := requireMutationRole(current.Role); err != nil {
		return RestoreOperation{}, err
	}
	if (intent == sqlcdb.RestoreOperationIntentExecute || intent == sqlcdb.RestoreOperationIntentRollback) && !validRestoreConfirmation(current.RestoreOperation, confirmation) {
		return RestoreOperation{}, ErrRestoreInvalidConfirmation
	}
	if !validIntentTransition(current.RestoreOperation, intent) {
		return RestoreOperation{}, ErrRestoreInvalidTransition
	}
	row, err := queries.UpdateRestoreIntent(ctx, sqlcdb.UpdateRestoreIntentParams{Intent: intent, ID: operationID})
	if err != nil {
		if restoreConstraintConflict(err) {
			return RestoreOperation{}, ErrRestoreOperationConflict
		}
		return RestoreOperation{}, err
	}
	payload, _ := json.Marshal(map[string]string{"intent": string(intent)})
	if _, err := queries.CreateRestoreOperationEvent(ctx, sqlcdb.CreateRestoreOperationEventParams{
		OperationID: row.ID, HostID: row.HostID, EventType: "intent_changed", Payload: payload,
	}); err != nil {
		return RestoreOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return RestoreOperation{}, err
	}
	return restoreOperationFromSQLC(row)
}

// FinalizeRestoreOperation archives a terminal operation after validating typed confirmation.
func (s *Postgres) FinalizeRestoreOperation(ctx context.Context, userID, operationID, confirmation string) (RestoreOperation, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return RestoreOperation{}, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	current, err := queries.GetRestoreOperationForUpdate(ctx, sqlcdb.GetRestoreOperationForUpdateParams{ID: operationID, UserID: userID})
	if err != nil {
		return RestoreOperation{}, err
	}
	if err := requireMutationRole(current.Role); err != nil {
		return RestoreOperation{}, err
	}
	if !validRestoreConfirmation(current.RestoreOperation, confirmation) {
		return RestoreOperation{}, ErrRestoreInvalidConfirmation
	}
	row, err := queries.RequestRestoreFinalization(ctx, operationID)
	if errors.Is(err, sql.ErrNoRows) {
		return RestoreOperation{}, ErrRestoreInvalidTransition
	}
	if err != nil {
		return RestoreOperation{}, err
	}
	if _, err := queries.CreateRestoreOperationEvent(ctx, sqlcdb.CreateRestoreOperationEventParams{
		OperationID: row.ID, HostID: row.HostID, EventType: "intent_changed", Payload: json.RawMessage(`{"intent":"finalize"}`),
	}); err != nil {
		return RestoreOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return RestoreOperation{}, err
	}
	return restoreOperationFromSQLC(row)
}

func validRestoreConfirmation(operation sqlcdb.RestoreOperation, confirmation string) bool {
	expected := operation.SourceClusterID
	if operation.Mode == sqlcdb.RestoreOperationModeClone {
		expected = operation.TargetClusterName.String
	}
	return confirmation == expected
}

// ListActionableRestoreOperationsForHost returns the full current operation set for an agent.
func (s *Postgres) ListActionableRestoreOperationsForHost(ctx context.Context, hostID string) ([]*types.RestoreOperation, error) {
	rows, err := s.queries.ListActionableRestoreOperationsForHost(ctx, hostID)
	if err != nil {
		return nil, err
	}
	operations := make([]*types.RestoreOperation, 0, len(rows))
	for _, row := range rows {
		operation, err := restoreOperationFromSQLC(row)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation.toProto())
	}
	return operations, nil
}

// GetRestoreRevisionForHost returns the latest append-only operation event sequence.
func (s *Postgres) GetRestoreRevisionForHost(ctx context.Context, hostID string) (int64, error) {
	return s.queries.GetRestoreRevisionForHost(ctx, hostID)
}

// ApplyRestoreOperationReports applies strictly increasing agent report sequences atomically.
func (s *Postgres) ApplyRestoreOperationReports(ctx context.Context, hostID string, reports []*types.RestoreOperationReport) ([]string, error) {
	projects := make(map[string]struct{})
	for _, report := range reports {
		projectID, applied, err := s.applyRestoreOperationReport(ctx, hostID, report)
		if err != nil {
			return nil, err
		}
		if applied {
			projects[projectID] = struct{}{}
		}
	}
	ids := make([]string, 0, len(projects))
	for projectID := range projects {
		ids = append(ids, projectID)
	}
	return ids, nil
}

func (s *Postgres) applyRestoreOperationReport(ctx context.Context, hostID string, report *types.RestoreOperationReport) (string, bool, error) {
	if report == nil || report.GetOperationId() == "" || report.GetSequence() == 0 || report.GetSequence() > uint64(^uint64(0)>>1) || report.GetPhase() == "" {
		return "", false, ErrRestoreInvalidTransition
	}
	status, ok := restoreReportStatus(report.GetStatus())
	if !ok {
		return "", false, fmt.Errorf("%w: unknown report status %q", ErrRestoreInvalidTransition, report.GetStatus())
	}
	payload, err := protojson.Marshal(report)
	if err != nil {
		return "", false, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", false, err
	}
	defer tx.Rollback()
	queries := s.queries.WithTx(tx)
	current, err := queries.LockRestoreOperationForAgentReport(ctx, sqlcdb.LockRestoreOperationForAgentReportParams{ID: report.GetOperationId(), HostID: hostID})
	if err != nil {
		return "", false, err
	}
	if report.GetSequence() <= uint64(current.ReportSequence) {
		return current.ProjectID, false, nil
	}
	if current.Status == sqlcdb.RestoreOperationStatusCancelled || current.Status == sqlcdb.RestoreOperationStatusFinalized {
		return current.ProjectID, false, nil
	}
	if !validReportTransition(current, status) {
		return "", false, ErrRestoreInvalidTransition
	}
	row, err := queries.ApplyRestoreOperationReport(ctx, sqlcdb.ApplyRestoreOperationReportParams{
		ReportSequence: int64(report.GetSequence()), Status: status,
		Report: payload, ID: current.ID,
	})
	if err != nil {
		return "", false, err
	}
	if row.Mode == sqlcdb.RestoreOperationModeClone && status == sqlcdb.RestoreOperationStatusSucceeded && current.Status != sqlcdb.RestoreOperationStatusSucceeded {
		if err := activateClone(ctx, queries, row); err != nil {
			return "", false, err
		}
	}
	if _, err := queries.CreateRestoreOperationEvent(ctx, sqlcdb.CreateRestoreOperationEventParams{
		OperationID: row.ID, HostID: row.HostID, EventType: "agent_report", Payload: payload,
	}); err != nil {
		return "", false, err
	}
	if err := tx.Commit(); err != nil {
		return "", false, err
	}
	return row.ProjectID, true, nil
}

func activateClone(ctx context.Context, queries *sqlcdb.Queries, operation sqlcdb.RestoreOperation) error {
	if !operation.TargetClusterID.Valid || !operation.TargetClusterName.Valid || string(operation.TargetSpec) == "null" {
		return fmt.Errorf("clone operation %q has no target specification", operation.ID)
	}
	var spec types.ClusterSpec
	if err := protojson.Unmarshal(operation.TargetSpec, &spec); err != nil {
		return fmt.Errorf("decode clone target: %w", err)
	}
	parameters, _ := json.Marshal(spec.GetParams())
	replicaIDs := make([]string, len(spec.GetReplicas()))
	for index, replica := range spec.GetReplicas() {
		replicaIDs[index] = replica.GetId()
	}
	replicas, _ := json.Marshal(replicaIDs)
	extensions, _ := json.Marshal(spec.GetEnabledExtensions())
	hbaRules, _ := json.Marshal(spec.GetPgHba().GetRules())
	params := sqlcdb.CreateActivatedCloneClusterParams{
		ID: operation.TargetClusterID.String, ProjectID: operation.ProjectID, HostID: operation.HostID,
		Name: operation.TargetClusterName.String, PostgresVersion: spec.GetVersion(), Parameters: parameters,
		ReplicaCount: int32(len(spec.GetReplicas())), ReplicaIds: replicas, EnabledExtensions: extensions,
		PgbouncerEnabled: spec.GetPgBouncer() != nil, PgHbaRules: hbaRules,
		PgbouncerPoolMode: defaultPgBouncerPoolMode, PgbouncerMaxConnections: defaultPgBouncerMaxConnections,
		PgbouncerPublishAddress: defaultPgBouncerPublishAddress, PgbouncerPublishPort: defaultPgBouncerPublishPort,
		RestartGeneration: int64(spec.GetRestartGeneration()),
	}
	if pool := spec.GetPgBouncer(); pool != nil {
		params.PgbouncerPoolMode = pool.GetPoolMode()
		params.PgbouncerMaxConnections = int32(pool.GetMaxConnections())
		params.PgbouncerPublishAddress = pool.GetPublishAddress()
		params.PgbouncerPublishPort = int32(pool.GetPublishPort())
	}
	clusterRow, err := queries.CreateActivatedCloneCluster(ctx, params)
	if err != nil {
		return fmt.Errorf("activate clone cluster: %w", err)
	}
	cluster, err := clusterFromSQLC(clusterRow)
	if err != nil {
		return err
	}
	_, err = createClusterUpsertState(ctx, queries, cluster)
	return err
}

func cloneTargetFromSource(source sqlcdb.GetRestoreMutationContextRow, targetID string) (*types.ClusterSpec, error) {
	cluster := Cluster{
		ID: targetID, HostID: source.HostID, PostgresVersion: source.PostgresVersion,
		ReplicaCount: source.ReplicaCount, PgBouncerEnabled: source.PgbouncerEnabled,
		PgBouncer: PgBouncerConfig{
			PoolMode: source.PgbouncerPoolMode, MaxConnections: source.PgbouncerMaxConnections,
			PublishAddress: source.PgbouncerPublishAddress, PublishPort: source.PgbouncerPublishPort,
		},
		RestartGeneration: source.RestartGeneration,
	}
	if err := json.Unmarshal(source.Parameters, &cluster.Parameters); err != nil {
		return nil, err
	}
	var sourceReplicaIDs []string
	if err := json.Unmarshal(source.ReplicaIds, &sourceReplicaIDs); err != nil {
		return nil, err
	}
	cluster.Replicas = make([]Replica, len(sourceReplicaIDs))
	for index := range cluster.Replicas {
		cluster.Replicas[index].ID = fmt.Sprintf("restore-%d", index+1)
	}
	if err := json.Unmarshal(source.EnabledExtensions, &cluster.EnabledExtensions); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(source.PgHbaRules, &cluster.PgHbaRules); err != nil {
		return nil, err
	}
	// A clone cannot use the source volume's repository path after activation.
	cluster.PgBackRest = nil
	// The source's host port cannot also be assigned to a same-host clone.
	cluster.PgBouncerEnabled = false
	payload, err := clusterDesiredStatePayload(cluster)
	if err != nil {
		return nil, err
	}
	var target types.ClusterSpec
	if err := protojson.Unmarshal(payload, &target); err != nil {
		return nil, err
	}
	return &target, nil
}

func restoreOperationFromSQLC(row sqlcdb.RestoreOperation) (RestoreOperation, error) {
	operation := RestoreOperation{
		ID: row.ID, ProjectID: row.ProjectID, HostID: row.HostID, SourceClusterID: row.SourceClusterID,
		Mode: string(row.Mode), Intent: string(row.Intent), Status: string(row.Status), TargetTime: row.TargetTime,
		RequestFingerprint: row.RequestFingerprint, RequestedByUserID: row.RequestedByUserID,
		ReportSequence: uint64(row.ReportSequence), CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}
	if row.TargetClusterID.Valid {
		operation.TargetClusterID = row.TargetClusterID.String
	}
	if row.TargetClusterName.Valid {
		operation.TargetClusterName = row.TargetClusterName.String
	}
	if string(row.TargetSpec) != "null" {
		operation.TargetCluster = &types.ClusterSpec{}
		if err := protojson.Unmarshal(row.TargetSpec, operation.TargetCluster); err != nil {
			return RestoreOperation{}, fmt.Errorf("decode restore target: %w", err)
		}
	}
	if string(row.Report) != "null" {
		operation.Report = &types.RestoreOperationReport{}
		if err := protojson.Unmarshal(row.Report, operation.Report); err != nil {
			return RestoreOperation{}, fmt.Errorf("decode restore report: %w", err)
		}
	}
	if row.FinalizedAt.Valid {
		operation.FinalizedAt = &row.FinalizedAt.Time
	}
	return operation, nil
}

func (operation RestoreOperation) toProto() *types.RestoreOperation {
	return &types.RestoreOperation{
		Id: operation.ID, Mode: operation.Mode, Intent: operation.Intent,
		SourceClusterId: operation.SourceClusterID, TargetCluster: operation.TargetCluster,
		TargetTime: operation.TargetTime.UTC().Format(time.RFC3339Nano), RequestFingerprint: operation.RequestFingerprint,
	}
}

func validIntentTransition(operation sqlcdb.RestoreOperation, intent sqlcdb.RestoreOperationIntent) bool {
	switch intent {
	case sqlcdb.RestoreOperationIntentExecute:
		return operation.Intent == sqlcdb.RestoreOperationIntentPreflight && operation.Status == sqlcdb.RestoreOperationStatusReady
	case sqlcdb.RestoreOperationIntentCancel:
		if operation.Status == sqlcdb.RestoreOperationStatusPending || operation.Status == sqlcdb.RestoreOperationStatusReady {
			return true
		}
		if operation.Status != sqlcdb.RestoreOperationStatusRunning || string(operation.Report) == "null" {
			return false
		}
		var report types.RestoreOperationReport
		return protojson.Unmarshal(operation.Report, &report) == nil && report.GetCancellable() && !report.GetDestructiveStarted()
	case sqlcdb.RestoreOperationIntentRollback:
		if operation.Status == sqlcdb.RestoreOperationStatusFailed {
			return true
		}
		if operation.Mode != sqlcdb.RestoreOperationModeInPlace || operation.Status != sqlcdb.RestoreOperationStatusSucceeded || string(operation.Report) == "null" {
			return false
		}
		var report types.RestoreOperationReport
		return protojson.Unmarshal(operation.Report, &report) == nil && report.GetRollbackAvailable()
	default:
		return false
	}
}

func restoreReportStatus(status string) (sqlcdb.RestoreOperationStatus, bool) {
	value := sqlcdb.RestoreOperationStatus(status)
	switch value {
	case sqlcdb.RestoreOperationStatusPending, sqlcdb.RestoreOperationStatusReady, sqlcdb.RestoreOperationStatusRunning,
		sqlcdb.RestoreOperationStatusSucceeded, sqlcdb.RestoreOperationStatusFailed,
		sqlcdb.RestoreOperationStatusCancelled, sqlcdb.RestoreOperationStatusRolledBack, sqlcdb.RestoreOperationStatusFinalized:
		return value, true
	default:
		return "", false
	}
}

func validReportTransition(operation sqlcdb.RestoreOperation, next sqlcdb.RestoreOperationStatus) bool {
	if operation.Status == next {
		return true
	}
	switch operation.Status {
	case sqlcdb.RestoreOperationStatusPending:
		return next == sqlcdb.RestoreOperationStatusReady || next == sqlcdb.RestoreOperationStatusRunning || next == sqlcdb.RestoreOperationStatusFailed || next == sqlcdb.RestoreOperationStatusCancelled || operation.Intent == sqlcdb.RestoreOperationIntentRollback && next == sqlcdb.RestoreOperationStatusRolledBack
	case sqlcdb.RestoreOperationStatusReady:
		return next == sqlcdb.RestoreOperationStatusRunning || next == sqlcdb.RestoreOperationStatusFailed || next == sqlcdb.RestoreOperationStatusCancelled
	case sqlcdb.RestoreOperationStatusRunning:
		return operation.Intent == sqlcdb.RestoreOperationIntentPreflight && next == sqlcdb.RestoreOperationStatusReady || next == sqlcdb.RestoreOperationStatusSucceeded || next == sqlcdb.RestoreOperationStatusFailed || next == sqlcdb.RestoreOperationStatusCancelled || next == sqlcdb.RestoreOperationStatusRolledBack
	case sqlcdb.RestoreOperationStatusFailed:
		return operation.Intent == sqlcdb.RestoreOperationIntentRollback && (next == sqlcdb.RestoreOperationStatusRunning || next == sqlcdb.RestoreOperationStatusRolledBack)
	case sqlcdb.RestoreOperationStatusSucceeded:
		return operation.Intent == sqlcdb.RestoreOperationIntentFinalize && next == sqlcdb.RestoreOperationStatusFinalized ||
			operation.Intent == sqlcdb.RestoreOperationIntentRollback && operation.Mode == sqlcdb.RestoreOperationModeInPlace && (next == sqlcdb.RestoreOperationStatusRunning || next == sqlcdb.RestoreOperationStatusRolledBack)
	case sqlcdb.RestoreOperationStatusRolledBack:
		return operation.Intent == sqlcdb.RestoreOperationIntentFinalize && next == sqlcdb.RestoreOperationStatusFinalized
	default:
		return false
	}
}

func restoreConstraintConflict(err error) bool {
	var postgresError *pgconn.PgError
	if !errors.As(err, &postgresError) || postgresError.Code != "23505" {
		return false
	}
	return postgresError.ConstraintName == "restore_operations_active_source_idx" ||
		postgresError.ConstraintName == "restore_operations_active_target_idx" ||
		postgresError.ConstraintName == "restore_operations_target_cluster_id_key"
}
