package channelgen

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

func schedulePath(stateDir, id string) string {
	return filepath.Join(stateDir, "channels", id+".json")
}

// PruneSchedules removes schedule files for channels no longer configured, keep being
// the configured channel ids.
func PruneSchedules(stateDir string, keep []string) error {
	root := filepath.Join(stateDir, "channels")
	known := make(map[string]struct{}, len(keep))
	for _, id := range keep {
		known[id+".json"] = struct{}{}
	}

	var errs []error
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		if _, ok := known[filepath.Base(path)]; ok {
			return nil
		}
		if err := os.Remove(path); err != nil {
			errs = append(errs, err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return errors.Join(errs...)
		}
		errs = append(errs, err)
	}

	pruneEmptyDirs(root)
	return errors.Join(errs...)
}

func pruneEmptyDirs(root string) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sub := filepath.Join(root, e.Name())
		if children, err := os.ReadDir(sub); err == nil && len(children) == 0 {
			_ = os.Remove(sub)
		}
	}
}

func loadSchedule(stateDir, id string) (*Schedule, error) {
	data, err := os.ReadFile(schedulePath(stateDir, id))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}

	var s Schedule
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("parse schedule: %w", err)
	}
	// Rejects manually copied or renamed state files.
	if s.Channel != id {
		return nil, nil
	}
	return &s, nil
}

func saveSchedule(stateDir string, s *Schedule) error {
	path := schedulePath(stateDir, s.Channel)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}

	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}
