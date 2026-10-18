package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const reduxPresetKey = "parakeet-redux"

func reduxModelPath() string {
	return filepath.Join(dataHome(), "voxtype", "models", "redux-packed.gguf")
}
func reduxAdapterPath() string {
	return filepath.Join(os.Getenv("HOME"), ".local", "bin", "voxtype-redux")
}
func reduxInstalled() bool {
	return fileExists(reduxModelPath()) && fileExists(reduxAdapterPath()) &&
		fileExists(filepath.Join(configHome(), "systemd", "user", "voxtype-redux.service"))
}

// Keep the resident worker dependency only while Redux is selected. Migrate the
// previous machine-local profile pin so all Hub presets use config.toml again.
func configureReduxService(selected, enabled bool) error {
	if !hostUsesSystemd() {
		if selected {
			return fmt.Errorf("the installed Redux adapter requires systemd")
		}
		return nil
	}
	dir := filepath.Join(configHome(), "systemd", "user", "voxtype.service.d")
	for _, name := range []string{"local-parakeet-redux.conf", "redux-adapter.conf"} {
		old := filepath.Join(dir, name)
		if text := readFileString(old); text != "" {
			if !strings.Contains(text, "redux-adapter.toml") || !strings.Contains(text, "Requires=voxtype-redux.service") {
				return fmt.Errorf("unrecognized local Redux service override: %s", old)
			}
			if fileExists(old + ".pre-hub") {
				return fmt.Errorf("Redux override backup already exists: %s", old+".pre-hub")
			}
			if err := os.Rename(old, old+".pre-hub"); err != nil {
				return err
			}
		}
	}
	path := filepath.Join(dir, "ryoku-redux.conf")
	if selected {
		if err := atomicWrite(path, []byte("[Unit]\nRequires=voxtype-redux.service\nAfter=voxtype-redux.service\n\n[Service]\nExecStart=\nExecStart=/usr/bin/voxtype daemon\n"), 0o644); err != nil {
			return err
		}
	} else if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := userService("daemon-reload"); err != nil {
		return err
	}
	if selected && enabled {
		if err := userService("enable", "--now", "voxtype-redux"); err != nil {
			return err
		}
	}
	return nil
}
