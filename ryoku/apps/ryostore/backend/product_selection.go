package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

const launcherTransactionKey = "ryoStoreLauncherTransaction"

// productSelection is the user setting that wears a product: shell.json names
// the bar style the desktop draws, launcher.json the launcher Super+Space opens.
// Removing the worn product moves that setting to its fallback inside the
// removal's own transaction, so the desktop never keeps naming what is gone and
// a later reinstall does not put it back on by itself.
type productSelection struct {
	path     func() string
	key      string
	fallback string
	// marker is a key the fallback write leaves beside the setting, so recovery
	// reverts only its own fallback and preserves a newer user choice.
	marker string
}

func selectionFor(category string) (productSelection, bool) {
	switch category {
	case "barstyles":
		return productSelection{path: defaultShellConfigPath, key: "barStyle", fallback: "sumi", marker: barStyleTransactionKey}, true
	case "launchers":
		// An empty variant opens the launcher the shell ships as its default.
		return productSelection{path: defaultLauncherConfigPath, key: "variant", fallback: "", marker: launcherTransactionKey}, true
	}
	return productSelection{}, false
}

func (s productSelection) applyFallback(id, token string) error {
	return mutateConfigState(s.path(), func(state map[string]any) bool {
		if state[s.key] != id {
			return false
		}
		state[s.key] = s.fallback
		state[s.marker] = token
		return true
	})
}

func (s productSelection) finishFallback(id, token string, committed bool) error {
	return mutateConfigState(s.path(), func(state map[string]any) bool {
		marker, owned := state[s.marker].(string)
		owned = owned && marker == token
		changed := false
		if committed {
			if state[s.key] == id {
				state[s.key] = s.fallback
				changed = true
			}
		} else if owned && state[s.key] == s.fallback {
			state[s.key] = id
			changed = true
		}
		if owned {
			delete(state, s.marker)
			changed = true
		}
		return changed
	})
}

// readConfigString is one top-level string setting of a JSON config, or "" when
// the file, the key or a string value is absent.
func readConfigString(path, key string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var state map[string]any
	if decodeOneJSON(raw, &state) != nil {
		return ""
	}
	value, _ := state[key].(string)
	return value
}

func lockConfigFile(path string) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX); err != nil {
		file.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}

func readConfigState(path string) (map[string]any, error) {
	state := map[string]any{}
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if err := decodeOneJSON(raw, &state); err != nil {
		return nil, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}
	if state == nil {
		return nil, fmt.Errorf("parse %s: root must be an object", filepath.Base(path))
	}
	return state, nil
}

func writeConfigState(path string, state map[string]any) error {
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(raw, '\n'), 0o644)
}

func mutateConfigState(path string, mutate func(map[string]any) bool) error {
	unlock, err := lockConfigFile(path)
	if err != nil {
		return err
	}
	defer unlock()
	state, err := readConfigState(path)
	if err != nil {
		return err
	}
	if !mutate(state) {
		return nil
	}
	return writeConfigState(path, state)
}
