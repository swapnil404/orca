// Package hostmetrics collects resource usage from the agent's Linux host.
package hostmetrics

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/swapnil404/orca/pkg/types"
)

const (
	hostProcRoot = "/host/proc"
	procRoot     = "/proc"
	defaultData  = "/var/orca/data"
)

// Collector samples CPU, memory, and data-filesystem usage.
type Collector struct {
	procRoot  string
	dataPath  string
	mu        sync.Mutex
	lastIdle  uint64
	lastTotal uint64
}

// NewCollector creates a collector for the mounted host procfs and Orca data filesystem.
func NewCollector(dataPath string) *Collector {
	root := hostProcRoot
	if _, err := os.Stat(filepath.Join(root, "stat")); err != nil {
		root = procRoot
	}
	if dataPath == "" {
		dataPath = defaultData
	}
	return &Collector{procRoot: root, dataPath: dataPath}
}

// Collect returns all measurements that could be read and joins collection errors.
func (c *Collector) Collect() (*types.HostMetrics, error) {
	metrics := &types.HostMetrics{}
	var errs []error
	if idle, total, err := readCPU(filepath.Join(c.procRoot, "stat")); err != nil {
		errs = append(errs, err)
	} else {
		metrics.CpuUsagePercent = c.cpuPercent(idle, total)
	}
	if used, total, err := readMemory(filepath.Join(c.procRoot, "meminfo")); err != nil {
		errs = append(errs, err)
	} else {
		metrics.MemoryUsedBytes, metrics.MemoryTotalBytes = used, total
	}
	if used, total, err := readDisk(c.dataPath); err != nil {
		errs = append(errs, err)
	} else {
		metrics.DiskUsedBytes, metrics.DiskTotalBytes = used, total
	}
	return metrics, errors.Join(errs...)
}

func (c *Collector) cpuPercent(idle, total uint64) float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	deltaIdle, deltaTotal := idle, total
	if total >= c.lastTotal && idle >= c.lastIdle && c.lastTotal != 0 {
		deltaIdle, deltaTotal = idle-c.lastIdle, total-c.lastTotal
	}
	c.lastIdle, c.lastTotal = idle, total
	if deltaTotal == 0 || deltaIdle > deltaTotal {
		return 0
	}
	return float64(deltaTotal-deltaIdle) * 100 / float64(deltaTotal)
}

func readCPU(path string) (uint64, uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, fmt.Errorf("read CPU metrics: %w", err)
	}
	line, _, _ := strings.Cut(string(data), "\n")
	fields := strings.Fields(line)
	if len(fields) < 5 || fields[0] != "cpu" {
		return 0, 0, errors.New("read CPU metrics: invalid aggregate row")
	}
	var total uint64
	values := make([]uint64, 0, len(fields)-1)
	for _, field := range fields[1:] {
		value, err := strconv.ParseUint(field, 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("read CPU metrics: %w", err)
		}
		values = append(values, value)
		total += value
	}
	idle := values[3]
	if len(values) > 4 {
		idle += values[4]
	}
	return idle, total, nil
}

func readMemory(path string) (uint64, uint64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, 0, fmt.Errorf("read memory metrics: %w", err)
	}
	values := make(map[string]uint64)
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		key := strings.TrimSuffix(fields[0], ":")
		if key != "MemTotal" && key != "MemAvailable" {
			continue
		}
		value, err := strconv.ParseUint(fields[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("read memory metrics: %w", err)
		}
		values[key] = value * 1024
	}
	total, totalOK := values["MemTotal"]
	available, availableOK := values["MemAvailable"]
	if !totalOK || !availableOK || available > total {
		return 0, 0, errors.New("read memory metrics: required values unavailable")
	}
	return total - available, total, nil
}

func readDisk(path string) (uint64, uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0, fmt.Errorf("read disk metrics: %w", err)
	}
	total := stat.Blocks * uint64(stat.Bsize)
	available := stat.Bavail * uint64(stat.Bsize)
	return total - available, total, nil
}
