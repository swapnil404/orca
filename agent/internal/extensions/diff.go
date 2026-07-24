// Package extensions reconciles supported PostgreSQL extensions on a cluster primary.
package extensions

import (
	"fmt"
	"sort"
)

// ActionType identifies an extension reconciliation operation.
type ActionType string

const (
	// ActionCreate enables an extension.
	ActionCreate ActionType = "create"
	// ActionDrop disables an extension.
	ActionDrop ActionType = "drop"
)

// Action describes one extension reconciliation operation.
type Action struct {
	Type      ActionType
	Extension string
}

var sqlNames = map[string]string{
	"pgvector":    "vector",
	"powa":        "powa",
	"timescaledb": "timescaledb",
	"pg_partman":  "pg_partman",
	"postgis":     "postgis",
}

// Diff returns the operations needed to make actual match desired.
func Diff(desired, actual []string) ([]Action, error) {
	desiredSet, err := extensionSet(desired)
	if err != nil {
		return nil, fmt.Errorf("desired extensions: %w", err)
	}
	actualSet, err := extensionSet(actual)
	if err != nil {
		return nil, fmt.Errorf("actual extensions: %w", err)
	}

	actions := make([]Action, 0)
	for extension := range desiredSet {
		if _, exists := actualSet[extension]; !exists {
			actions = append(actions, Action{Type: ActionCreate, Extension: extension})
		}
	}
	for extension := range actualSet {
		if _, exists := desiredSet[extension]; !exists {
			actions = append(actions, Action{Type: ActionDrop, Extension: extension})
		}
	}
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Type != actions[j].Type {
			return actions[i].Type < actions[j].Type
		}
		return actions[i].Extension < actions[j].Extension
	})
	return actions, nil
}

func extensionSet(extensions []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(extensions))
	for _, extension := range extensions {
		if _, supported := sqlNames[extension]; !supported {
			return nil, fmt.Errorf("unsupported extension %q", extension)
		}
		set[extension] = struct{}{}
	}
	return set, nil
}
