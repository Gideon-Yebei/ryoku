// The launcher provider adapts community app launchers from the ryostore
// launchers registry into receipt-owned Store products. The shell ships its own
// launchers (catalog.json beside the launcher host); a store launcher installs
// into dataHome/ryoku/launchers/<id>, runs from a content-addressed view, and
// joins the launcher host and Ryoku Settings -> App Launcher through the
// launchers.json index. Install never selects it: the App Launcher page saves
// the choice as variant in launcher.json.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	launcherEntry   = "Main.qml"
	launcherPreview = "Preview.qml"
)

// builtinLauncherIDs are the launchers the shell ships. A store product never
// takes one of their ids: the host would always pick the shipped one.
var builtinLauncherIDs = map[string]bool{
	"main": true, "hero": true, "okshell": true, "kairos": true, "spotlight": true, "python": true,
}

// launcherIndexRow is one runnable store launcher: Main.qml, and Preview.qml
// when the product ships one, both inside View.
type launcherIndexRow struct {
	ID      string `json:"id"`
	Version string `json:"version"`
	Preview string `json:"preview"`
	View    string `json:"view"`
}

type launcherProvider struct {
	cache  *Cache
	config string
}

func newLauncherProvider(cache *Cache) launcherProvider {
	if cache == nil {
		cache = newCache()
	}
	return launcherProvider{cache: cache, config: defaultLauncherConfigPath()}
}

func defaultLauncherConfigPath() string {
	return filepath.Join(configHome(), "ryoku", "launcher.json")
}

func (launcherProvider) Category() Category {
	return Category{
		ID:          "launchers",
		Name:        "App launchers",
		Group:       "wear",
		Description: "Community launchers for Super+Space. Install here, pick one in Settings -> App Launcher.",
	}
}

func (p launcherProvider) Load(ctx context.Context, refresh bool) ([]Item, SourceState, error) {
	local, err := rebuildLauncherIndex()
	if err != nil {
		return nil, SourceState{}, err
	}
	rows := make(map[string]launcherIndexRow, len(local))
	for _, row := range local {
		rows[row.ID] = row
	}
	active := readConfigString(p.config, "variant")

	entries, state, registryErr := loadProductRegistry(ctx, p.cache, "launchers", refresh)
	if productRegistryAbsent(registryErr) {
		entries, registryErr = nil, nil
	}
	if registryErr != nil && !productRegistryUnavailable(registryErr) {
		return nil, state, registryErr
	}
	items := make([]Item, 0, len(entries)+len(local))
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if builtinLauncherIDs[entry.ID] {
			continue
		}
		item, err := productEntryItem(p.cache.base, "launchers", entry)
		if err != nil {
			return nil, state, fmt.Errorf("launchers/%s: installed state: %w", entry.ID, err)
		}
		if row, runnable := rows[entry.ID]; runnable {
			item.Active = item.Installed && entry.ID == active
			item.Metadata = launcherMetadata(row)
		}
		items = append(items, item)
		seen[entry.ID] = true
	}
	// A launcher that is installed but no longer listed, or whose source is out
	// of reach, stays usable and removable.
	for _, row := range local {
		if seen[row.ID] {
			continue
		}
		items = append(items, Item{
			ID: row.ID, Category: "launchers", Name: row.ID,
			Version: row.Version, InstalledVersion: row.Version,
			Installed: true, Active: row.ID == active,
			Metadata: launcherMetadata(row),
		})
	}
	if registryErr != nil {
		state.Offline = true
	}
	return items, state, nil
}

func launcherMetadata(row launcherIndexRow) map[string]any {
	return map[string]any{"entry": launcherEntry, "preview": row.Preview, "view": row.View}
}

func (p launcherProvider) Install(ctx context.Context, id string) error {
	if builtinLauncherIDs[id] {
		return fmt.Errorf("the %s launcher ships with the shell", id)
	}
	entries, _, err := loadProductRegistry(ctx, p.cache, "launchers", false)
	if err != nil {
		return err
	}
	entry, err := findProductEntry(entries, id)
	if err != nil {
		return err
	}
	return installProduct(ctx, p.cache, "launchers", entry)
}

func (p launcherProvider) Remove(ctx context.Context, id string) error {
	if builtinLauncherIDs[id] {
		return fmt.Errorf("the %s launcher ships with the shell and is not removable", id)
	}
	return removeProduct(ctx, "launchers", id)
}

func launcherIndexPath() string {
	return filepath.Join(storeStateDir(), "launchers.json")
}

func rebuildLauncherIndex() ([]launcherIndexRow, error) {
	unlock, err := lockTree(storeTransactionLockPath())
	if err != nil {
		return nil, err
	}
	defer unlock()
	if err := recoverStoreTransactions(); err != nil {
		return nil, err
	}
	return rebuildLauncherIndexLocked()
}

func writeLauncherIndexLocked() error {
	_, err := rebuildLauncherIndexLocked()
	return err
}

func rebuildLauncherIndexLocked() ([]launcherIndexRow, error) {
	entries, err := os.ReadDir(filepath.Join(storeStateDir(), "launchers"))
	if os.IsNotExist(err) {
		entries = nil
	} else if err != nil {
		return nil, err
	}
	rows := make([]launcherIndexRow, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		// A broken receipt (bad name, unreadable, no Main.qml, or drifted from
		// disk) is quarantined: it reads as installable again and a reinstall
		// heals it, while the rest of the index stays usable.
		if !productIDPattern.MatchString(id) || builtinLauncherIDs[id] {
			continue
		}
		receipt, err := readReceipt("launchers", id)
		if err != nil || !receiptOwnsFile(receipt, launcherEntry) {
			continue
		}
		destination, _, err := productDestination("launchers", id)
		if err != nil {
			continue
		}
		if err := verifyInstalledReceipt(destination, receipt); err != nil {
			continue
		}
		row := launcherIndexRow{ID: id, Version: receipt.Version, View: launcherViews.path(id, receipt)}
		if receiptOwnsFile(receipt, launcherPreview) {
			row.Preview = launcherPreview
		}
		if err := launcherViews.ensure(id, row.View, receipt); err != nil {
			continue
		}
		rows = append(rows, row)
	}
	live := make(map[string]string, len(rows))
	for _, row := range rows {
		live[row.ID] = row.View
	}
	if err := launcherViews.prune(live); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(rows)
	if err != nil {
		return nil, err
	}
	if err := atomicWrite(launcherIndexPath(), append(raw, '\n'), 0o600); err != nil {
		return nil, err
	}
	return rows, nil
}
