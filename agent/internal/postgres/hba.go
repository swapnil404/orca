package postgres

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"sort"
	"strings"

	orcadocker "github.com/swapnil404/orca/agent/internal/docker"
	"github.com/swapnil404/orca/pkg/types"
)

const (
	hbaRulesBegin = "# BEGIN ORCA USER RULES"
	hbaRulesEnd   = "# END ORCA USER RULES"
)

// HBAExecutor provides the container operations needed to manage pg_hba.conf.
type HBAExecutor interface {
	ExecContainer(context.Context, string, []string) (string, error)
	ContainerNetworkCIDRs(context.Context, string) ([]string, error)
	RestartContainer(context.Context, string) error
}

// HBAObservation describes the Orca-managed portions of an active HBA file.
type HBAObservation struct {
	Rules            []*types.PgHbaRule
	ReplicationCIDRs []string
	PoolCIDRs        []string
}

// DesiredHBARules returns the managed rules, or nil when this server does not manage HBA.
func DesiredHBARules(desired *types.ClusterSpec) []*types.PgHbaRule {
	if desired == nil || desired.PgHba == nil {
		return nil
	}
	return desired.PgHba.Rules
}

// ValidateHBARules rejects rules that cannot be rendered safely.
func ValidateHBARules(rules []*types.PgHbaRule) error {
	if len(rules) > 100 {
		return errors.New("pg_hba rules cannot exceed 100 entries")
	}
	for index, rule := range rules {
		if rule == nil {
			return fmt.Errorf("pg_hba rule %d is nil", index+1)
		}
		if rule.Type != "host" && rule.Type != "hostssl" && rule.Type != "local" {
			return fmt.Errorf("pg_hba rule %d has invalid type %q", index+1, rule.Type)
		}
		if !validHBAList(rule.Database) || !validHBAList(rule.User) {
			return fmt.Errorf("pg_hba rule %d has an invalid database or user", index+1)
		}
		if rule.Type == "local" {
			if rule.Address != "" {
				return fmt.Errorf("pg_hba rule %d: local rules cannot have an address", index+1)
			}
		} else if !validHBAAddress(rule.Address) {
			return fmt.Errorf("pg_hba rule %d has invalid address %q", index+1, rule.Address)
		}
		switch rule.Method {
		case "trust", "md5", "scram-sha-256", "reject":
		default:
			return fmt.Errorf("pg_hba rule %d has invalid method %q", index+1, rule.Method)
		}
	}
	return nil
}

// RulesEqual reports whether two ordered HBA rule lists are identical.
func RulesEqual(left, right []*types.PgHbaRule) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] == nil || right[index] == nil ||
			left[index].Type != right[index].Type || left[index].Database != right[index].Database ||
			left[index].User != right[index].User || left[index].Address != right[index].Address ||
			left[index].Method != right[index].Method {
			return false
		}
	}
	return true
}

// StringsEqual reports whether two ordered string lists are identical.
func StringsEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// ObserveHBA reads the active file and returns the applied Orca user rules.
func ObserveHBA(ctx context.Context, executor HBAExecutor, containerID string) (HBAObservation, error) {
	content, err := readHBA(ctx, executor, containerID)
	if err != nil {
		return HBAObservation{}, err
	}
	return parseManagedRules(content)
}

// ApplyHBA replaces and reloads pg_hba.conf on the primary and existing replicas.
func ApplyHBA(ctx context.Context, executor HBAExecutor, desired *types.ClusterSpec, actual *types.ActualCluster) error {
	if executor == nil || desired == nil || desired.PgHba == nil {
		return errors.New("pg_hba executor and desired cluster are required")
	}
	rules := DesiredHBARules(desired)
	if err := ValidateHBARules(rules); err != nil {
		return err
	}
	primary, err := orcadocker.ContainerName(orcadocker.ContainerSpec{ClusterID: desired.Id, Kind: orcadocker.ContainerKindPrimary})
	if err != nil {
		return err
	}
	if err := WaitForPrimaryReady(ctx, executor, primary); err != nil {
		return err
	}
	cidrs, err := executor.ContainerNetworkCIDRs(ctx, primary)
	if err != nil {
		return fmt.Errorf("inspect primary network: %w", err)
	}
	type target struct {
		name     string
		content  string
		previous string
	}
	targets := []target{{name: primary, content: renderHBA(rules, cidrs, len(desired.Replicas) > 0, desired.PgBouncer != nil)}}
	if actual != nil {
		for _, replica := range actual.Replicas {
			if replica == nil || replica.ContainerId == "" || replica.Status != "running" {
				continue
			}
			cidrs, err := executor.ContainerNetworkCIDRs(ctx, replica.ContainerId)
			if err != nil {
				return fmt.Errorf("inspect replica network: %w", err)
			}
			targets = append(targets, target{name: replica.ContainerId, content: renderHBA(rules, cidrs, false, desired.PgBouncer != nil)})
		}
	}
	for index := range targets {
		previous, err := readHBA(ctx, executor, targets[index].name)
		if err != nil {
			return err
		}
		targets[index].previous = previous
	}
	for index := range targets {
		if err := applyHBAFile(ctx, executor, targets[index].name, targets[index].content); err != nil {
			var rollbackErr error
			for rollbackIndex := index - 1; rollbackIndex >= 0; rollbackIndex-- {
				if restoreErr := writeAndReloadHBA(context.WithoutCancel(ctx), executor, targets[rollbackIndex].name, targets[rollbackIndex].previous); restoreErr != nil {
					rollbackErr = errors.Join(rollbackErr, fmt.Errorf("restore node %q: %w", targets[rollbackIndex].name, restoreErr))
				}
			}
			return errors.Join(err, rollbackErr)
		}
	}
	return nil
}

// ApplyReplicaHBA replaces and reloads pg_hba.conf on one newly bootstrapped replica.
func ApplyReplicaHBA(ctx context.Context, executor HBAExecutor, desired *types.ClusterSpec, containerID string) error {
	if executor == nil || desired == nil || desired.PgHba == nil || containerID == "" {
		return errors.New("pg_hba executor, desired cluster, and replica container are required")
	}
	rules := DesiredHBARules(desired)
	if err := ValidateHBARules(rules); err != nil {
		return err
	}
	if err := WaitForPrimaryReady(ctx, executor, containerID); err != nil {
		return err
	}
	cidrs, err := executor.ContainerNetworkCIDRs(ctx, containerID)
	if err != nil {
		return fmt.Errorf("inspect replica network: %w", err)
	}
	return applyHBAFile(ctx, executor, containerID, renderHBA(rules, cidrs, false, desired.PgBouncer != nil))
}

func renderHBA(rules []*types.PgHbaRule, networkCIDRs []string, replication, pooling bool) string {
	var builder strings.Builder
	builder.WriteString("# Managed by Orca. Manual changes are replaced.\n")
	builder.WriteString("local all postgres trust\n")
	if replication {
		cidrs := append([]string(nil), networkCIDRs...)
		sort.Strings(cidrs)
		for _, cidr := range cidrs {
			fmt.Fprintf(&builder, "host replication postgres %s trust\n", cidr)
		}
	}
	if pooling {
		cidrs := append([]string(nil), networkCIDRs...)
		sort.Strings(cidrs)
		for _, cidr := range cidrs {
			fmt.Fprintf(&builder, "host all postgres %s scram-sha-256\n", cidr)
		}
	}
	builder.WriteString(hbaRulesBegin + "\n")
	for _, rule := range rules {
		if rule.Type == "local" {
			fmt.Fprintf(&builder, "%s %s %s %s\n", rule.Type, rule.Database, rule.User, rule.Method)
		} else {
			fmt.Fprintf(&builder, "%s %s %s %s %s\n", rule.Type, rule.Database, rule.User, rule.Address, rule.Method)
		}
	}
	builder.WriteString(hbaRulesEnd + "\n")
	return builder.String()
}

func applyHBAFile(ctx context.Context, executor HBAExecutor, containerID, content string) error {
	previous, err := readHBA(ctx, executor, containerID)
	if err != nil {
		return err
	}
	if previous == content {
		return nil
	}
	if err := writeAndReloadHBA(ctx, executor, containerID, content); err != nil {
		rollbackErr := writeAndReloadHBA(context.WithoutCancel(ctx), executor, containerID, previous)
		if rollbackErr != nil {
			return errors.Join(err, fmt.Errorf("restore previous pg_hba.conf: %w", rollbackErr))
		}
		return err
	}
	return nil
}

func readHBA(ctx context.Context, executor HBAExecutor, containerID string) (string, error) {
	content, err := executor.ExecContainer(ctx, containerID, []string{"sh", "-c", `hba_file="$(psql -U postgres -Atqc 'SHOW hba_file')"; cat -- "$hba_file"`})
	if err != nil {
		return "", fmt.Errorf("read active pg_hba.conf: %w", err)
	}
	return content + "\n", nil
}

func writeAndReloadHBA(ctx context.Context, executor HBAExecutor, containerID, content string) error {
	_, err := executor.ExecContainer(ctx, containerID, []string{
		"sh", "-c",
		`set -eu; hba_file="$(psql -U postgres -Atqc 'SHOW hba_file')"; tmp="${hba_file}.orca.tmp"; printf '%s' "$1" > "$tmp"; chown --reference="$hba_file" "$tmp"; chmod --reference="$hba_file" "$tmp"; mv -f -- "$tmp" "$hba_file"`,
		"sh", content,
	})
	if err != nil {
		return fmt.Errorf("write active pg_hba.conf: %w", err)
	}
	result, err := executor.ExecContainer(ctx, containerID, psqlCommand("SELECT pg_reload_conf()"))
	if err != nil || strings.TrimSpace(result) != "t" {
		return fmt.Errorf("reload pg_hba.conf: result %q: %w", strings.TrimSpace(result), err)
	}
	parseErrors, err := executor.ExecContainer(ctx, containerID, psqlCommand("SELECT count(*) FROM pg_hba_file_rules WHERE error IS NOT NULL"))
	if err != nil || strings.TrimSpace(parseErrors) != "0" {
		return fmt.Errorf("validate pg_hba.conf: %s parse errors: %w", strings.TrimSpace(parseErrors), err)
	}
	return nil
}

func parseManagedRules(content string) (HBAObservation, error) {
	start := strings.Index(content, hbaRulesBegin+"\n")
	end := strings.Index(content, hbaRulesEnd+"\n")
	if start < 0 || end < start {
		return HBAObservation{}, errors.New("active pg_hba.conf is not managed by Orca")
	}
	replicationCIDRs := make([]string, 0)
	poolCIDRs := make([]string, 0)
	for _, line := range strings.Split(content[:start], "\n") {
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[0] == "host" && fields[1] == "replication" && fields[2] == "postgres" && fields[4] == "trust" {
			if _, err := netip.ParsePrefix(fields[3]); err != nil {
				return HBAObservation{}, fmt.Errorf("invalid managed replication CIDR %q", fields[3])
			}
			replicationCIDRs = append(replicationCIDRs, fields[3])
		} else if len(fields) == 5 && fields[0] == "host" && fields[1] == "all" && fields[2] == "postgres" && fields[4] == "scram-sha-256" {
			if _, err := netip.ParsePrefix(fields[3]); err != nil {
				return HBAObservation{}, fmt.Errorf("invalid managed pool CIDR %q", fields[3])
			}
			poolCIDRs = append(poolCIDRs, fields[3])
		}
	}
	sort.Strings(replicationCIDRs)
	sort.Strings(poolCIDRs)
	block := content[start+len(hbaRulesBegin)+1 : end]
	rules := make([]*types.PgHbaRule, 0)
	for _, line := range strings.Split(strings.TrimSuffix(block, "\n"), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 4 && len(fields) != 5 {
			return HBAObservation{}, fmt.Errorf("invalid managed pg_hba line %q", line)
		}
		rule := &types.PgHbaRule{Type: fields[0], Database: fields[1], User: fields[2]}
		if rule.Type == "local" && len(fields) == 4 {
			rule.Method = fields[3]
		} else if len(fields) == 5 {
			rule.Address, rule.Method = fields[3], fields[4]
		} else {
			return HBAObservation{}, fmt.Errorf("invalid managed pg_hba line %q", line)
		}
		rules = append(rules, rule)
	}
	if err := ValidateHBARules(rules); err != nil {
		return HBAObservation{}, err
	}
	return HBAObservation{Rules: rules, ReplicationCIDRs: replicationCIDRs, PoolCIDRs: poolCIDRs}, nil
}

func validHBAAddress(value string) bool {
	if _, err := netip.ParsePrefix(value); err == nil {
		return true
	}
	_, err := netip.ParseAddr(value)
	return err == nil
}

func validHBAList(value string) bool {
	if value == "" || len(value) > 256 || strings.ContainsAny(value, "\x00\r\n\t ") {
		return false
	}
	for _, item := range strings.Split(value, ",") {
		if item == "" {
			return false
		}
		for _, char := range item {
			if char != '_' && char != '-' && char != '.' && (char < '0' || char > '9') && (char < 'A' || char > 'Z') && (char < 'a' || char > 'z') {
				return false
			}
		}
	}
	return true
}
