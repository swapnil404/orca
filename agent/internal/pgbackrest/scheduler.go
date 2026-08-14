package pgbackrest

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	orcatypes "github.com/swapnil404/orca/pkg/types"
	"google.golang.org/protobuf/proto"
)

// BackupType identifies a pgBackRest backup type.
type BackupType string

const (
	// BackupTypeFull creates a full backup.
	BackupTypeFull BackupType = "full"
	// BackupTypeDiff creates a differential backup.
	BackupTypeDiff BackupType = "diff"
	// BackupTypeIncr creates an incremental backup.
	BackupTypeIncr BackupType = "incr"

	maxScheduleIntervalSeconds = uint64((1<<63 - 1) / 1_000_000_000)
)

type scheduleTicker interface {
	Chan() <-chan time.Time
	Stop()
}

type realTicker struct{ *time.Ticker }

func (t realTicker) Chan() <-chan time.Time { return t.C }

// OperationGate serializes scheduled backups with reconciliation passes.
type OperationGate struct{ token chan struct{} }

// NewOperationGate creates an unlocked operation gate.
func NewOperationGate() *OperationGate {
	gate := &OperationGate{token: make(chan struct{}, 1)}
	gate.token <- struct{}{}
	return gate
}

// Acquire waits for exclusive access or context cancellation.
func (g *OperationGate) Acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.token:
		return nil
	}
}

// Release relinquishes exclusive access.
func (g *OperationGate) Release() { g.token <- struct{}{} }

type scheduledCluster struct {
	spec   *orcatypes.ClusterSpec
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// Result reports a completed scheduled backup attempt.
type Result struct {
	ClusterID  string
	BackupType BackupType
	Err        error
}

// Scheduler owns ticker workers after configuration has been reconciled successfully.
type Scheduler struct {
	executor  PrimaryExecutor
	logger    *slog.Logger
	newTicker func(time.Duration) scheduleTicker
	gate      *OperationGate
	root      context.Context
	cancel    context.CancelFunc

	mu       sync.Mutex
	clusters map[string]*scheduledCluster
	results  []Result
}

// NewScheduler creates a backup scheduler.
func NewScheduler(executor PrimaryExecutor) *Scheduler {
	root, cancel := context.WithCancel(context.Background())
	return &Scheduler{
		executor: executor, logger: slog.Default(), root: root, cancel: cancel,
		newTicker: func(interval time.Duration) scheduleTicker { return realTicker{time.NewTicker(interval)} },
		clusters:  make(map[string]*scheduledCluster),
	}
}

// SetOperationGate configures the gate shared with the reconciliation runner.
func (s *Scheduler) SetOperationGate(gate *OperationGate) {
	s.mu.Lock()
	s.gate = gate
	s.mu.Unlock()
}

// SetSchedule replaces ticker workers for a successfully applied cluster configuration.
func (s *Scheduler) SetSchedule(cluster *orcatypes.ClusterSpec) {
	if cluster == nil || cluster.PgBackRest == nil {
		return
	}
	copy := proto.Clone(cluster).(*orcatypes.ClusterSpec)
	intervals := scheduleIntervals(copy.PgBackRest.Schedule)
	s.mu.Lock()
	previous := s.clusters[cluster.Id]
	if previous != nil && proto.Equal(previous.spec.PgBackRest, copy.PgBackRest) {
		s.mu.Unlock()
		return
	}
	clusterCtx, cancel := context.WithCancel(s.root)
	scheduled := &scheduledCluster{spec: copy, ctx: clusterCtx, cancel: cancel}
	scheduled.wg.Add(len(intervals))
	s.clusters[cluster.Id] = scheduled
	for backupType, interval := range intervals {
		go func() {
			defer scheduled.wg.Done()
			s.runTicker(scheduled, backupType, interval)
		}()
	}
	s.mu.Unlock()
	if previous != nil {
		stopSchedule(previous)
	}
}

// RemoveSchedule stops ticker workers for a cluster.
func (s *Scheduler) RemoveSchedule(clusterID string) {
	s.mu.Lock()
	cluster := s.clusters[clusterID]
	delete(s.clusters, clusterID)
	s.mu.Unlock()
	if cluster != nil {
		stopSchedule(cluster)
	}
}

// Run keeps the scheduler alive until ctx is canceled.
func (s *Scheduler) Run(ctx context.Context) error {
	if s.executor == nil {
		return fmt.Errorf("executor is nil")
	}
	<-ctx.Done()
	s.cancel()
	s.mu.Lock()
	clusters := s.clusters
	s.clusters = make(map[string]*scheduledCluster)
	s.mu.Unlock()
	cancelSchedules(clusters)
	return ctx.Err()
}

// PendingResults returns asynchronous backup outcomes awaiting report delivery.
func (s *Scheduler) PendingResults() []Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Result(nil), s.results...)
}

// AcknowledgeResults removes outcomes included in a successfully delivered report.
func (s *Scheduler) AcknowledgeResults(count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if count <= 0 {
		return
	}
	if count >= len(s.results) {
		s.results = nil
		return
	}
	s.results = append([]Result(nil), s.results[count:]...)
}

func (s *Scheduler) runTicker(cluster *scheduledCluster, backupType BackupType, interval time.Duration) {
	ticker := s.newTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-cluster.ctx.Done():
			return
		case <-ticker.Chan():
			if err := s.acquire(cluster.ctx); err != nil {
				return
			}
			err := s.prepareAndRun(cluster.ctx, cluster.spec, backupType)
			s.release()
			s.record(Result{ClusterID: cluster.spec.Id, BackupType: backupType, Err: err})
			if err != nil {
				s.logger.Error("run scheduled pgBackRest backup", "cluster_id", cluster.spec.Id, "type", backupType, "error", err)
			}
		}
	}
}

func (s *Scheduler) prepareAndRun(ctx context.Context, cluster *orcatypes.ClusterSpec, backupType BackupType) error {
	if err := InstallConfig(ctx, s.executor, cluster); err != nil {
		return fmt.Errorf("install config: %w", err)
	}
	if err := ConfigureWALArchiving(ctx, s.executor, cluster); err != nil {
		return fmt.Errorf("configure WAL archiving: %w", err)
	}
	if err := InitializeStanza(ctx, s.executor, cluster); err != nil {
		return fmt.Errorf("initialize stanza: %w", err)
	}
	return RunBackup(ctx, s.executor, cluster.Id, backupType)
}

func (s *Scheduler) acquire(ctx context.Context) error {
	s.mu.Lock()
	gate := s.gate
	s.mu.Unlock()
	if gate == nil {
		return nil
	}
	return gate.Acquire(ctx)
}

func (s *Scheduler) release() {
	s.mu.Lock()
	gate := s.gate
	s.mu.Unlock()
	if gate != nil {
		gate.Release()
	}
}

func (s *Scheduler) record(result Result) {
	s.mu.Lock()
	s.results = append(s.results, result)
	s.mu.Unlock()
}

// RunBackup executes one pgBackRest backup against the primary.
func RunBackup(ctx context.Context, executor Executor, clusterID string, backupType BackupType) error {
	if executor == nil {
		return fmt.Errorf("executor is nil")
	}
	if err := validateClusterID(clusterID); err != nil {
		return err
	}
	switch backupType {
	case BackupTypeFull, BackupTypeDiff, BackupTypeIncr:
	default:
		return fmt.Errorf("invalid backup type %q", backupType)
	}
	primary, err := primaryContainerName(clusterID)
	if err != nil {
		return err
	}
	command := []string{"gosu", postgresUser, "pgbackrest", "--config=" + clusterConfigPath(clusterID), "--stanza=" + clusterID, "--type=" + string(backupType), "backup"}
	if _, err := executor.ExecContainer(ctx, primary, command); err != nil {
		return fmt.Errorf("run %s backup for stanza %q: %w", backupType, clusterID, err)
	}
	return nil
}

func scheduleIntervals(schedule *orcatypes.BackupSchedule) map[BackupType]time.Duration {
	intervals := make(map[BackupType]time.Duration)
	if schedule == nil {
		return intervals
	}
	if schedule.FullIntervalSeconds > 0 {
		intervals[BackupTypeFull] = time.Duration(schedule.FullIntervalSeconds) * time.Second
	}
	if schedule.DiffIntervalSeconds > 0 {
		intervals[BackupTypeDiff] = time.Duration(schedule.DiffIntervalSeconds) * time.Second
	}
	if schedule.IncrIntervalSeconds > 0 {
		intervals[BackupTypeIncr] = time.Duration(schedule.IncrIntervalSeconds) * time.Second
	}
	return intervals
}

func cancelSchedules(clusters map[string]*scheduledCluster) {
	for _, cluster := range clusters {
		stopSchedule(cluster)
	}
}

func stopSchedule(cluster *scheduledCluster) {
	cluster.cancel()
	cluster.wg.Wait()
}
