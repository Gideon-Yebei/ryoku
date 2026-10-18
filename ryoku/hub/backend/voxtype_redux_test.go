package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReduxInstalledPresetAndSwitching(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HOME", root)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(root, "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	bin := filepath.Join(root, "bin")
	os.MkdirAll(bin, 0700)
	log := filepath.Join(root, "calls")
	host := "#!/bin/sh\nif [ \"$1\" = init ]; then echo systemd; exit 0; fi\nprintf '%s\\n' \"$*\" >> '" + log + "'\n"
	os.WriteFile(filepath.Join(bin, "ryoku-host"), []byte(host), 0700)
	t.Setenv("PATH", bin)
	if reduxInstalled() {
		t.Fatal("Redux installed without files")
	}
	for _, path := range []string{reduxModelPath(), reduxAdapterPath(), filepath.Join(configHome(), "systemd/user/voxtype-redux.service")} {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	p, ok := presetByKey(reduxPresetKey)
	if !ok || p.Label != "Redux Multilingual + Adapter" || p.Group != "multilingual" || p.Language != "25 languages" {
		t.Fatalf("invalid Redux preset: %#v", p)
	}
	config := buildVoxtypeConfig(p, "")
	for _, expected := range []string{"mode = \"cli\"", "language = \"auto\"", "max_duration_secs = 120", reduxAdapterPath(), reduxModelPath()} {
		if !strings.Contains(config, expected) {
			t.Fatalf("config missing %s", expected)
		}
	}
	if err := voxtypeRmModel(reduxPresetKey); err == nil {
		t.Fatal("allowed deleting shared Redux model")
	}
	dir := filepath.Join(configHome(), "systemd/user/voxtype.service.d")
	os.MkdirAll(dir, 0700)
	old := filepath.Join(dir, "local-parakeet-redux.conf")
	os.WriteFile(old, []byte("[Unit]\nRequires=voxtype-redux.service\n[Service]\nExecStart=/usr/bin/voxtype --config %h/.config/voxtype/redux-adapter.toml daemon\n"), 0600)
	if err := configureReduxService(true, true); err != nil {
		t.Fatal(err)
	}
	if !fileExists(old+".pre-hub") || fileExists(old) {
		t.Fatal("old forced profile was not migrated")
	}
	current := filepath.Join(dir, "ryoku-redux.conf")
	if !strings.Contains(readFileString(current), "Requires=voxtype-redux.service") {
		t.Fatal("missing worker dependency")
	}
	if !strings.Contains(readFileString(log), "enable --now voxtype-redux") {
		t.Fatal("worker not started")
	}
	if err := configureReduxService(false, true); err != nil {
		t.Fatal(err)
	}
	if fileExists(current) {
		t.Fatal("Redux override still forces worker after switching models")
	}
}
