package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

type launcherFixture struct {
	cache    *Cache
	provider launcherProvider
	main     []byte
}

func productFileFor(name string, body []byte) ProductFile {
	return ProductFile{
		Source: name, Destination: name, Mode: "0644",
		Size: int64(len(body)), SHA256: fmt.Sprintf("%x", sha256.Sum256(body)), Install: true,
	}
}

// newLauncherFixture serves a launchers registry holding Tatami, plus a copy of
// its row under each id in shadows.
func newLauncherFixture(t *testing.T, shadows ...string) launcherFixture {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	main := []byte("import QtQuick\nItem { readonly property bool shown: false }\n")
	preview := []byte("import QtQuick\nItem { implicitWidth: 720; implicitHeight: 250 }\n")
	manifest := ProductManifest{
		Schema: 1, ID: "tatami", Category: "launchers", Version: "1.0.0",
		Destination: "ryoku/launchers/tatami",
		Files:       []ProductFile{productFileFor("Main.qml", main), productFileFor("Preview.qml", preview)},
	}
	manifestRaw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	entry := ProductEntry{
		ID: "tatami", Name: "Tatami", Version: "1.0.0", Path: "launchers/tatami",
		Author: "Ryoku Team", Summary: "App grid", Description: "A full-screen app grid.",
		Tags: []string{"grid"}, Accent: "#c8b98f", Surface: "#0f0e0c",
		Preview: "assets/preview.png", Screenshots: []string{}, Manifest: "manifest.json",
		ManifestSHA256: fmt.Sprintf("%x", sha256.Sum256(manifestRaw)),
	}
	rows := []ProductEntry{entry}
	for _, id := range shadows {
		shadow := entry
		shadow.ID, shadow.Path = id, "launchers/"+id
		rows = append(rows, shadow)
	}
	registryRaw, err := json.Marshal(map[string]any{"schema": 1, "launchers": rows})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/launchers/registry.json":
			_, _ = w.Write(registryRaw)
		case "/launchers/tatami/manifest.json":
			_, _ = w.Write(manifestRaw)
		case "/launchers/tatami/Main.qml":
			_, _ = w.Write(main)
		case "/launchers/tatami/Preview.qml":
			_, _ = w.Write(preview)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	cache := &Cache{client: server.Client(), base: server.URL, dir: t.TempDir(), memo: map[string]memoEntry{}}
	return launcherFixture{cache: cache, provider: launcherProvider{cache: cache, config: defaultLauncherConfigPath()}, main: main}
}

func writeLauncherConfig(t *testing.T, state map[string]any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(defaultLauncherConfigPath()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeConfigState(defaultLauncherConfigPath(), state); err != nil {
		t.Fatal(err)
	}
}

func readLauncherConfig(t *testing.T) map[string]any {
	t.Helper()
	state, err := readConfigState(defaultLauncherConfigPath())
	if err != nil {
		t.Fatal(err)
	}
	return state
}

func readLauncherIndex(t *testing.T) []launcherIndexRow {
	t.Helper()
	raw, err := os.ReadFile(launcherIndexPath())
	if err != nil {
		t.Fatal(err)
	}
	var rows []launcherIndexRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		t.Fatal(err)
	}
	return rows
}

func launcherByID(items []Item, id string) *Item {
	for i := range items {
		if items[i].ID == id {
			return &items[i]
		}
	}
	return nil
}

func TestLauncherInstallIsRunnableAndRemovalFallsBackToTheDefault(t *testing.T) {
	fixture := newLauncherFixture(t)
	ctx := context.Background()
	writeLauncherConfig(t, map[string]any{"variant": "tatami", "radius": 12})

	items, state, err := fixture.provider.Load(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if tatami := launcherByID(items, "tatami"); len(items) != 1 || state.Offline || tatami == nil || tatami.Installed || tatami.Active {
		t.Fatalf("catalogue before install = %+v (offline %v)", items, state.Offline)
	}

	if err := fixture.provider.Install(ctx, "tatami"); err != nil {
		t.Fatal(err)
	}
	rows := readLauncherIndex(t)
	if len(rows) != 1 || rows[0].ID != "tatami" || rows[0].Version != "1.0.0" || rows[0].Preview != "Preview.qml" {
		t.Fatalf("index after install = %+v", rows)
	}
	view := filepath.Join(storeStateDir(), filepath.FromSlash(rows[0].View))
	if body, err := os.ReadFile(filepath.Join(view, "Main.qml")); err != nil || string(body) != string(fixture.main) {
		t.Fatalf("view Main.qml = %q, err=%v", body, err)
	}

	items, _, err = fixture.provider.Load(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	tatami := launcherByID(items, "tatami")
	if tatami == nil || !tatami.Installed || !tatami.Active || tatami.Metadata["view"] != rows[0].View || tatami.Metadata["preview"] != "Preview.qml" {
		t.Fatalf("installed item = %+v", tatami)
	}

	if err := fixture.provider.Remove(ctx, "tatami"); err != nil {
		t.Fatal(err)
	}
	if rows := readLauncherIndex(t); len(rows) != 0 {
		t.Fatalf("index after removal = %+v", rows)
	}
	if _, err := os.Stat(filepath.Join(storeStateDir(), "launcher-views", "tatami")); !os.IsNotExist(err) {
		t.Fatalf("removed launcher kept its view: %v", err)
	}
	config := readLauncherConfig(t)
	if config["variant"] != "" || config["radius"] != float64(12) {
		t.Fatalf("launcher.json after removing the worn launcher = %#v", config)
	}
	if _, marked := config[launcherTransactionKey]; marked {
		t.Fatalf("committed removal left its marker: %#v", config)
	}
}

// Removing a launcher nobody wears must not touch the user's pick.
func TestLauncherRemovalKeepsAnotherSelection(t *testing.T) {
	fixture := newLauncherFixture(t)
	ctx := context.Background()
	if err := fixture.provider.Install(ctx, "tatami"); err != nil {
		t.Fatal(err)
	}
	writeLauncherConfig(t, map[string]any{"variant": "okshell"})
	if err := fixture.provider.Remove(ctx, "tatami"); err != nil {
		t.Fatal(err)
	}
	if config := readLauncherConfig(t); config["variant"] != "okshell" {
		t.Fatalf("launcher.json = %#v", config)
	}
}

func TestLauncherRemovalRollbackRestoresTheSelection(t *testing.T) {
	fixture := newLauncherFixture(t)
	ctx := context.Background()
	if err := fixture.provider.Install(ctx, "tatami"); err != nil {
		t.Fatal(err)
	}
	writeLauncherConfig(t, map[string]any{"variant": "tatami"})
	previous := productTransactionCheckpoint
	productTransactionCheckpoint = func(phase string) error {
		if phase == "derived-state" {
			return errors.New("injected derived-state failure")
		}
		return nil
	}
	t.Cleanup(func() { productTransactionCheckpoint = previous })

	if err := fixture.provider.Remove(ctx, "tatami"); err == nil {
		t.Fatal("removal accepted a derived-state failure")
	}
	if _, err := readReceipt("launchers", "tatami"); err != nil {
		t.Fatalf("rolled-back receipt: %v", err)
	}
	config := readLauncherConfig(t)
	if config["variant"] != "tatami" {
		t.Fatalf("rolled-back selection = %#v", config)
	}
	if _, marked := config[launcherTransactionKey]; marked {
		t.Fatalf("rolled-back settings retain the marker: %#v", config)
	}
	if rows := readLauncherIndex(t); len(rows) != 1 || rows[0].ID != "tatami" {
		t.Fatalf("rolled-back index = %+v", rows)
	}
}

func TestLauncherIdsOfShippedLaunchersAreRefused(t *testing.T) {
	fixture := newLauncherFixture(t, "hero")
	ctx := context.Background()
	items, _, err := fixture.provider.Load(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if launcherByID(items, "hero") != nil {
		t.Fatalf("a store row shadows the shipped Hero launcher: %+v", items)
	}
	if err := fixture.provider.Install(ctx, "hero"); err == nil {
		t.Fatal("the shipped Hero launcher was installable from the store")
	}
	if err := fixture.provider.Remove(ctx, "hero"); err == nil {
		t.Fatal("the shipped Hero launcher was removable")
	}
}

// A source without a launchers registry yet shows an empty shelf rather than an
// error or an offline store.
func TestLauncherCatalogueWithoutRegistryIsEmpty(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", filepath.Join(t.TempDir(), "state"))
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "config"))
	server := httptest.NewServer(http.NotFoundHandler())
	t.Cleanup(server.Close)
	cache := &Cache{client: server.Client(), base: server.URL, dir: t.TempDir(), memo: map[string]memoEntry{}}
	items, state, err := (launcherProvider{cache: cache, config: defaultLauncherConfigPath()}).Load(context.Background(), false)
	if err != nil || len(items) != 0 || state.Offline {
		t.Fatalf("items=%+v offline=%v err=%v", items, state.Offline, err)
	}
}

// The store refuses the shell's own launcher ids, so the list must follow the
// shell's catalog whenever a launcher ships or retires.
func TestBuiltinLauncherIDsMatchTheShellCatalog(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "shell", "quickshell", "shell", "modules", "launcher", "catalog.json"))
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Variants []struct {
			ID string `json:"id"`
		} `json:"variants"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	var shipped, refused []string
	for _, variant := range catalog.Variants {
		shipped = append(shipped, variant.ID)
	}
	for id := range builtinLauncherIDs {
		refused = append(refused, id)
	}
	sort.Strings(shipped)
	sort.Strings(refused)
	if fmt.Sprint(shipped) != fmt.Sprint(refused) {
		t.Fatalf("shell ships %v, the store refuses %v", shipped, refused)
	}
}
