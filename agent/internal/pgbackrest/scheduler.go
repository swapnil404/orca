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

// DesiredStateSource loads the last desired state cached by the agent.
type DesiredStateSource interface {
	Load(ctx context.Context) (orcatypes.DesiredState, error)
}

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

type realTicker struct {
	*time.Ticker
}

func (t realTicker) Chan() <-chan time.Time { return t.C }

type scheduledCluster struct {
	spec   *orcatypes.ClusterSpec
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
	wg     sync.WaitGroup
}

// Scheduler runs desired pgBackRest backup schedules without relying on cron.
type Scheduler struct {
	source    DesiredStateSource
	executor  PrimaryExecutor
	logger    *slog.Logger
	newTicker func(time.Duration) scheduleTicker

	mu      sync.Mutex
	desired *orcatypes.DesiredState
	changed chan struct{}
}

// NewScheduler creates a backup scheduler backed by the local desired-state cache.
func NewScheduler(source DesiredStateSource, executor PrimaryExecutor) *Scheduler {
	return &Scheduler{
		source: source, executor: executor, logger: slog.Default(),
		newTicker: func(interval time.Duration) scheduleTicker { return realTicker{time.NewTicker(interval)} },
		changed:   make(chan struct{}, 1),
	}
}

// Update applies a fresh complete desired-state snapshot to the scheduler.
func (s *Scheduler) Update(desired *orcatypes.DesiredState) {
	if desired == nil {
		return
	}
	s.mu.Lock()
	s.desired = proto.Clone(desired).(*orcatypes.DesiredState)
	s.mu.Unlock()
	select {
	case s.changed <- struct{}{}:
	default:
	}
}

// Run schedules backups until ctx is canceled.
func (s *Scheduler) Run(ctx context.Context) error {
	if s.source == nil {
		return fmt.Errorf("desired-state source is nil")
	}
	if s.executor == nil {
		return fmt.Errorf("executor is nil")
	}

	desired, err := s.source.Load(ctx)
	if err != nil {
		s.logger.Warn("load backup schedules from cached desired state", "error", err)
	}
	clusters := make(map[string]*scheduledCluster)
	s.syncSchedules(ctx, clusters, &desired)
	defer cancelSchedules(clusters)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.changed:
			s.mu.Lock()
			latest := s.desired
			s.mu.Unlock()
			s.syncSchedules(ctx, clusters, latest)
		}
	}
}

func (s *Scheduler) syncSchedules(ctx context.Context, current map[string]*scheduledCluster, desired *orcatypes.DesiredState) {
	wanted := make(map[string]*orcatypes.ClusterSpec)
	activeClusters := make(map[string]struct{})
	if desired == nil {
		cancelSchedules(current)
		clear(current)
		return
	}
	for _, cluster := range desired.Clusters {
		if cluster == nil {
			continue
		}
		activeClusters[cluster.Id] = struct{}{}
		if cluster.PgBackRest != nil {
			wanted[cluster.Id] = cluster
		}
	}

	for clusterID, scheduled := range current {
		desiredCluster, exists := wanted[clusterID]
		if exists && proto.Equal(scheduled.spec.PgBackRest, desiredCluster.PgBackRest) {
			scheduled.wg.Add(1)
			go func() {
				defer scheduled.wg.Done()
				s.prepare(scheduled.ctx, scheduled)
			}()
			delete(wanted, clusterID)
			continue
		}
		stopSchedule(scheduled)
		delete(current, clusterID)
		if _, clusterStillExists := activeClusters[clusterID]; clusterStillExists && !exists {
			if err := DisableWALArchiving(ctx, s.executor, clusterID); err != nil {
				s.logger.Error("disable pgBackRest WAL archiving", "cluster_id", clusterID, "error", err)
			}
		}
	}

	for clusterID, cluster := range wanted {
		if _, err := GeneratePgBackRestConfig(*cluster); err != nil {
			s.logger.Error("configure pgBackRest schedule", "cluster_id", clusterID, "error", err)
			continue
		}
		clusterCtx, cancel := context.WithCancel(ctx)
		scheduled := &scheduledCluster{spec: cluster, ctx: clusterCtx, cancel: cancel}
		current[clusterID] = scheduled
		scheduled.wg.Add(1)
		go func() {
			defer scheduled.wg.Done()
			s.prepare(clusterCtx, scheduled)
		}()
		for backupType, interval := range scheduleIntervals(cluster.PgBackRest.Schedule) {
			scheduled.wg.Add(1)
			go func() {
				defer scheduled.wg.Done()
				s.runTicker(clusterCtx, scheduled, backupType, interval)
			}()
		}
	}
}

func (s *Scheduler) prepare(ctx context.Context, cluster *scheduledCluster) {
	cluster.mu.Lock()
	defer cluster.mu.Unlock()
	if err := s.prepareLocked(ctx, cluster); err != nil {
		s.logger.Error("prepare pgBackRest cluster", "cluster_id", cluster.spec.Id, "error", err)
	}
}

func (s *Scheduler) prepareLocked(ctx context.Context, cluster *scheduledCluster) error {
	if err := InstallConfig(ctx, s.executor, cluster.spec); err != nil {
		return fmt.Errorf("install config: %w", err)
	}
	if err := ConfigureWALArchiving(ctx, s.executor, cluster.spec); err != nil {
		return fmt.Errorf("configure WAL archiving: %w", err)
	}
	if err := InitializeStanza(ctx, s.executor, cluster.spec); err != nil {
		return fmt.Errorf("initialize stanza: %w", err)
	}
	return nil
}

func (s *Scheduler) runTicker(ctx context.Context, cluster *scheduledCluster, backupType BackupType, interval time.Duration) {
	// v1 shortcut: ticker state is intentionally in memory only. Agent restarts
	// reset the interval and can delay the next backup by up to one full interval.
	ticker := s.newTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.Chan():
			cluster.mu.Lock()
			err := s.prepareLocked(ctx, cluster)
			if err == nil {
				err = RunBackup(ctx, s.executor, cluster.spec.Id, backupType)
			}
			cluster.mu.Unlock()
			if err != nil {
				s.logger.Error("run scheduled pgBackRest backup", "cluster_id", cluster.spec.Id, "type", backupType, "error", err)
			}
		}
	}
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
	command := []string{"gosu", postgresUser, "pgbackrest", "--stanza=" + clusterID, "--type=" + string(backupType), "backup"}
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
