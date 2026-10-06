package pgbackrest

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

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

func parseAvailableBytes(output string) (uint64, error) {
	fields := strings.Fields(output)
	if len(fields) < 6 {
		return 0, fmt.Errorf("unexpected df output %q", output)
	}
	availableKB, err := strconv.ParseUint(fields[len(fields)-3], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse available filesystem capacity: %w", err)
	}
	if availableKB > math.MaxUint64/1024 {
		return 0, errors.New("available filesystem capacity overflows bytes")
	}
	return availableKB * 1024, nil
}

func parseDirectorySizeBytes(output string) (uint64, error) {
	fields := strings.Fields(output)
	if len(fields) < 2 {
		return 0, fmt.Errorf("unexpected du output %q", output)
	}
	sizeKB, err := strconv.ParseUint(fields[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("parse directory size: %w", err)
	}
	if sizeKB > math.MaxUint64/1024 {
		return 0, errors.New("directory size overflows bytes")
	}
	return sizeKB * 1024, nil
}

func addBytes(left, right uint64) (uint64, error) {
	if left > math.MaxUint64-right {
		return 0, errors.New("capacity calculation overflow")
	}
	return left + right, nil
}

func restoreCapacityRequired(original, restored uint64) (uint64, error) {
	total, err := addBytes(original, restored)
	if err != nil {
		return 0, err
	}
	headroom := total / 10
	if total%10 != 0 {
		headroom++
	}
	return addBytes(total, headroom)
}
