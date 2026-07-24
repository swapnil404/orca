// Package extensions reconciles supported PostgreSQL extensions on a cluster primary.
package extensions

import (
	"fmt"
	"sort"
)

// ActionType identifies an extension reconciliation operation.
type ActionType string

// UpdateMethod identifies how an extension change is applied.
type UpdateMethod string

const (
	// ActionCreate enables an extension.
	ActionCreate ActionType = "create"
	// ActionDrop disables an extension.
	ActionDrop ActionType = "drop"

	// UpdateMethodHotApply applies SQL without restarting PostgreSQL.
	UpdateMethodHotApply UpdateMethod = "hot_apply"
	// UpdateMethodRestart applies shared preload configuration and restarts PostgreSQL.
	UpdateMethodRestart UpdateMethod = "restart"
)

// Action describes one extension reconciliation operation.
type Action struct {
	Type      ActionType
	Extension string
	Method    UpdateMethod
}

type definition struct {
	SQLName         string
	RequiresRestart bool
}

var definitions = map[string]definition{
	"pgvector":    {SQLName: "vector", RequiresRestart: false},
	"powa":        {SQLName: "powa", RequiresRestart: true},
	"timescaledb": {SQLName: "timescaledb", RequiresRestart: true},
	"pg_partman":  {SQLName: "pg_partman", RequiresRestart: false},
	"postgis":     {SQLName: "postgis", RequiresRestart: false},
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
			actions = append(actions, newAction(ActionCreate, extension))
		}
	}
	for extension := range actualSet {
		if _, exists := desiredSet[extension]; !exists {
			actions = append(actions, newAction(ActionDrop, extension))
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

func newAction(actionType ActionType, extension string) Action {
	return Action{Type: actionType, Extension: extension, Method: ClassifyUpdate(extension)}
}

// ClassifyUpdate determines whether an extension change can be hot-applied or
// requires a PostgreSQL restart.
func ClassifyUpdate(extension string) UpdateMethod {
	if definitions[extension].RequiresRestart {
		return UpdateMethodRestart
	}
	return UpdateMethodHotApply
}

func extensionSet(extensions []string) (map[string]struct{}, error) {
	set := make(map[string]struct{}, len(extensions))
	for _, extension := range extensions {
		if _, supported := definitions[extension]; !supported {
			return nil, fmt.Errorf("unsupported extension %q", extension)
		}
		set[extension] = struct{}{}
	}
	return set, nil
}
