// Package postgresconfig defines the PostgreSQL configuration policy shared by
// the control plane and agent.
package postgresconfig

import (
	"fmt"
	"net/netip"
	"regexp"
	"sort"
	"strings"
)

const (
	maxEntries    = 100
	maxHBAListLen = 256
	maxValueLen   = 4096

	// ConvergenceUnknown means live parameter state is unavailable.
	ConvergenceUnknown = "unknown"
	// ConvergencePending means desired parameters have not reached every node.
	ConvergencePending = "pending"
	// ConvergenceFailed means PostgreSQL rejected or could not apply a parameter.
	ConvergenceFailed = "failed"
	// ConvergenceRestartPending means a parameter is waiting for a restart.
	ConvergenceRestartPending = "restart_pending"
	// ConvergenceConverged means every PostgreSQL node has applied the desired parameters.
	ConvergenceConverged = "converged"
)

var parameterNamePattern = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_]*$`)

var orcaManagedParameters = map[string]struct{}{
	"archive_command": {}, "archive_mode": {}, "config_file": {}, "data_directory": {},
	"hba_file": {}, "hot_standby": {}, "ident_file": {}, "listen_addresses": {}, "port": {},
	"primary_conninfo": {}, "primary_slot_name": {}, "recovery_target": {}, "recovery_target_action": {},
	"recovery_target_inclusive": {}, "recovery_target_lsn": {}, "recovery_target_name": {},
	"recovery_target_time": {}, "recovery_target_timeline": {}, "recovery_target_xid": {},
	"shared_preload_libraries": {}, "unix_socket_directories": {}, "wal_level": {},
}

// HBARule contains one ordered PostgreSQL client authentication rule.
type HBARule struct {
	Type     string `json:"type"`
	Database string `json:"database"`
	User     string `json:"user"`
	Address  string `json:"address"`
	Method   string `json:"method"`
}

// ValidateHBARules normalizes and validates rules that Orca can safely render.
func ValidateHBARules(rules []HBARule) ([]HBARule, error) {
	if len(rules) > maxEntries {
		return nil, fmt.Errorf("pg_hba rules cannot exceed %d entries", maxEntries)
	}
	normalized := make([]HBARule, len(rules))
	for index, rule := range rules {
		rule = HBARule{
			Type: strings.TrimSpace(rule.Type), Database: strings.TrimSpace(rule.Database),
			User: strings.TrimSpace(rule.User), Address: strings.TrimSpace(rule.Address), Method: strings.TrimSpace(rule.Method),
		}
		if rule.Type != "host" && rule.Type != "hostssl" && rule.Type != "local" {
			return nil, fmt.Errorf("pg_hba rule %d has invalid type %q", index+1, rule.Type)
		}
		if !validHBAList(rule.Database) || !validHBAList(rule.User) {
			return nil, fmt.Errorf("pg_hba rule %d has an invalid database or user", index+1)
		}
		if rule.Type == "local" {
			if rule.Address != "" {
				return nil, fmt.Errorf("pg_hba rule %d: local rules cannot have an address", index+1)
			}
		} else if !validHBAAddress(rule.Address) {
			return nil, fmt.Errorf("pg_hba rule %d has invalid address %q", index+1, rule.Address)
		}
		switch rule.Method {
		case "trust", "md5", "scram-sha-256", "reject":
		default:
			return nil, fmt.Errorf("pg_hba rule %d has invalid method %q", index+1, rule.Method)
		}
		normalized[index] = rule
	}
	return normalized, nil
}

// ValidateParameters normalizes parameter names and validates Orca's static policy.
func ValidateParameters(parameters map[string]string) (map[string]string, error) {
	if len(parameters) > maxEntries {
		return nil, fmt.Errorf("parameters cannot exceed %d entries", maxEntries)
	}
	normalized := make(map[string]string, len(parameters))
	for original, value := range parameters {
		name := NormalizeParameterName(original)
		if !ValidParameterName(name) {
			return nil, fmt.Errorf("invalid PostgreSQL parameter name %q", original)
		}
		if _, duplicate := normalized[name]; duplicate {
			return nil, fmt.Errorf("duplicate PostgreSQL parameter name %q", name)
		}
		if _, managed := orcaManagedParameters[name]; managed {
			return nil, fmt.Errorf("PostgreSQL parameter %q is managed by Orca", name)
		}
		if len(value) > maxValueLen || strings.ContainsAny(value, "\x00\r\n") {
			return nil, fmt.Errorf("PostgreSQL parameter %q must be single-line and at most %d bytes", name, maxValueLen)
		}
		normalized[name] = value
	}
	return normalized, nil
}

// NormalizeParameterName returns the canonical spelling of a parameter name.
func NormalizeParameterName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

// ValidParameterName reports whether a normalized name is safe to use in SQL and config files.
func ValidParameterName(name string) bool {
	return parameterNamePattern.MatchString(name)
}

// ChangedParameters returns sorted parameter names whose desired and applied values differ.
func ChangedParameters(desired, applied map[string]string) []string {
	desired = normalizeParameters(desired)
	applied = normalizeParameters(applied)
	changed := make(map[string]struct{})
	for name, value := range desired {
		if appliedValue, exists := applied[name]; !exists || appliedValue != value {
			changed[name] = struct{}{}
		}
	}
	for name := range applied {
		if _, exists := desired[name]; !exists {
			changed[name] = struct{}{}
		}
	}
	result := make([]string, 0, len(changed))
	for name := range changed {
		result = append(result, name)
	}
	sort.Strings(result)
	return result
}

// ParametersEqual reports whether desired and applied parameter maps are equivalent.
func ParametersEqual(desired, applied map[string]string) bool {
	return len(ChangedParameters(desired, applied)) == 0
}

// ValidConvergence reports whether value is a defined parameter convergence status.
func ValidConvergence(value string) bool {
	switch value {
	case ConvergenceUnknown, ConvergencePending, ConvergenceFailed, ConvergenceRestartPending, ConvergenceConverged:
		return true
	default:
		return false
	}
}

func normalizeParameters(parameters map[string]string) map[string]string {
	normalized := make(map[string]string, len(parameters))
	for name, value := range parameters {
		normalized[NormalizeParameterName(name)] = value
	}
	return normalized
}

func validHBAAddress(value string) bool {
	if _, err := netip.ParsePrefix(value); err == nil {
		return true
	}
	_, err := netip.ParseAddr(value)
	return err == nil
}

func validHBAList(value string) bool {
	if value == "" || len(value) > maxHBAListLen || strings.ContainsAny(value, "\x00\r\n\t ") {
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
