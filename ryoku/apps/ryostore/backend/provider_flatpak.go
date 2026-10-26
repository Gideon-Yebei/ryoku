// The flatpak provider lists the most popular Flathub applications beside
// whatever Flathub applications are already installed, and installs or removes
// them in the user's own Flatpak installation. Flathub is a third-party source:
// nothing here passes through the ryostore registry, so the catalogue marks each
// application with Flathub's own verification state and never claims the
// screening the registry products get.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	flatpakCategory = "flatpak"
	flatpakRemote   = "flathub"
	// flathubRepoFile is the same remote file `ryoku doctor` adds system-wide.
	flathubRepoFile = "https://flathub.org/repo/flathub.flatpakrepo"
	flathubAPI      = "https://flathub.org/api/v2"
	flathubPageSize = 300
	// A search sends at most flathubSearchHits results and refuses a query
	// shorter than flathubQueryMin, which would match nearly everything.
	flathubSearchHits = 30
	flathubQueryMin   = 2
	flathubQueryMax   = 100
	flathubCache      = "flathub-popular.json"
	flatpakTimeout    = 30 * time.Minute
	// flatpakUpdateProbe bounds the update check of one installation, so an
	// unreachable remote cannot hold the catalogue.
	flatpakUpdateProbe = 20 * time.Second
)

// flatpakIDPattern is the reverse-DNS application id Flatpak itself accepts.
// Every id reaches an argv, so one that could parse as a flag is refused here.
var flatpakIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+(\.[A-Za-z0-9_-]+){2,}$`)

func validFlatpakID(id string) bool { return len(id) <= 255 && flatpakIDPattern.MatchString(id) }

// flathubHit is the part of a Flathub collection entry the store renders.
type flathubHit struct {
	AppID             string   `json:"app_id"`
	Name              string   `json:"name"`
	Summary           string   `json:"summary"`
	Description       string   `json:"description"`
	Type              string   `json:"type"`
	Icon              string   `json:"icon"`
	License           string   `json:"project_license"`
	Developer         string   `json:"developer_name"`
	MainCategories    any      `json:"main_categories"`
	SubCategories     any      `json:"sub_categories"`
	Verified          bool     `json:"verification_verified"`
	Arches            []string `json:"arches"`
	AddedAt           int64    `json:"added_at"`
	InstallsLastMonth int64    `json:"installs_last_month"`
}

type flathubCollection struct {
	Hits []flathubHit `json:"hits"`
}

// installedFlatpak is one installed application as `flatpak list` reports it.
type installedFlatpak struct {
	ID           string
	Name         string
	Version      string
	Installation string
	Origin       string
}

type flatpakProvider struct {
	// api is the Flathub API base; tests point it at a local server.
	api string
	// present reports whether the flatpak command exists on this host.
	present func() bool
	// installed lists the applications installed in any installation.
	installed func(ctx context.Context) ([]installedFlatpak, error)
	// updates returns the ids that have an update waiting in the named
	// installations. It is best effort: a failure reads as no updates.
	updates func(ctx context.Context, installations []string) map[string]bool
	// run executes flatpak with the given arguments and returns its output.
	run func(ctx context.Context, args ...string) ([]byte, error)
}

func newFlatpakProvider() flatpakProvider {
	p := flatpakProvider{api: flathubAPI}
	p.present = func() bool { _, err := exec.LookPath("flatpak"); return err == nil }
	p.run = runFlatpak
	p.installed = func(ctx context.Context) ([]installedFlatpak, error) {
		out, err := runFlatpak(ctx, "list", "--app", "--columns=application,name,version,installation,origin")
		if err != nil {
			return nil, err
		}
		return parseFlatpakList(out), nil
	}
	p.updates = func(ctx context.Context, installations []string) map[string]bool {
		return pendingFlatpakUpdates(ctx, p.run, installations)
	}
	return p
}

// pendingFlatpakUpdates asks each installation what it could update. remote-ls
// reads one installation at a time (the system one by default), so the user one
// has to be named; a failing installation is skipped rather than failing Load.
func pendingFlatpakUpdates(ctx context.Context, run func(context.Context, ...string) ([]byte, error), installations []string) map[string]bool {
	pending := map[string]bool{}
	for _, installation := range installations {
		ctx, cancel := context.WithTimeout(ctx, flatpakUpdateProbe)
		out, err := run(ctx, "remote-ls", "--updates", "--app", "--columns=application", installationFlag(installation))
		cancel()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			if id := strings.TrimSpace(line); validFlatpakID(id) {
				pending[id] = true
			}
		}
	}
	return pending
}

// installationFlag names the installation a flatpak command works in.
func installationFlag(installation string) string {
	switch installation {
	case "user":
		return "--user"
	case "", "system":
		return "--system"
	}
	return "--installation=" + installation
}

func (flatpakProvider) Category() Category {
	return Category{
		ID:          flatpakCategory,
		Name:        "Apps",
		Group:       "EXTEND",
		Description: "Popular applications from Flathub, installed with Flatpak. Flathub is a third-party source: Ryostore does not review these.",
	}
}

func runFlatpak(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "flatpak", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	return out.Bytes(), err
}

// parseFlatpakList reads the tab separated rows of `flatpak list --columns=...`.
func parseFlatpakList(out []byte) []installedFlatpak {
	var apps []installedFlatpak
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Split(line, "\t")
		if len(f) < 5 || !validFlatpakID(strings.TrimSpace(f[0])) {
			continue
		}
		apps = append(apps, installedFlatpak{
			ID:           strings.TrimSpace(f[0]),
			Name:         strings.TrimSpace(f[1]),
			Version:      strings.TrimSpace(f[2]),
			Installation: strings.TrimSpace(f[3]),
			Origin:       strings.TrimSpace(f[4]),
		})
	}
	return apps
}

func (p flatpakProvider) Load(ctx context.Context, refresh bool) ([]Item, SourceState, error) {
	hits, state, err := p.popular(ctx, refresh)
	var installed []installedFlatpak
	if p.present() && p.installed != nil {
		installed, _ = p.installed(ctx)
	}
	// With no network and no archive the store still shows what is installed, so
	// Remove keeps working; only a host with neither source reports the error.
	if err != nil && len(installed) == 0 {
		return nil, state, err
	}

	owned := ownedFlathub(installed)
	usable := p.present()
	updatable := map[string]bool{}
	if usable && p.updates != nil && len(owned) > 0 {
		updatable = p.updates(ctx, installationsOf(owned))
	}
	items := flatpakItems(hits, owned, usable, updatable)
	seen := map[string]bool{}
	for _, item := range items {
		seen[item.ID] = true
	}
	ids := make([]string, 0, len(owned))
	for id := range owned {
		if !seen[id] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		item := flatpakInstalledItem(owned[id])
		item.UpdateAvailable = updatable[id]
		items = append(items, item)
	}
	return items, state, nil
}

// ownedFlathub indexes the installed applications that came from Flathub.
func ownedFlathub(installed []installedFlatpak) map[string]installedFlatpak {
	owned := map[string]installedFlatpak{}
	for _, app := range installed {
		if app.Origin == flatpakRemote {
			owned[app.ID] = app
		}
	}
	return owned
}

// flatpakItems turns Flathub hits into items, dropping what this host cannot
// or should not list: ids that could be flags, repeats, anything that is not a
// desktop application, and builds for another architecture.
func flatpakItems(hits []flathubHit, owned map[string]installedFlatpak, usable bool, updatable map[string]bool) []Item {
	arch := flathubArch()
	items := make([]Item, 0, len(hits))
	seen := map[string]bool{}
	for _, hit := range hits {
		if !validFlatpakID(hit.AppID) || seen[hit.AppID] || hit.Type != "desktop-application" || !hasArch(hit.Arches, arch) {
			continue
		}
		seen[hit.AppID] = true
		item := flatpakItem(hit, owned[hit.AppID], usable)
		item.UpdateAvailable = item.Installed && updatable[hit.AppID]
		items = append(items, item)
	}
	return items
}

func installationsOf(owned map[string]installedFlatpak) []string {
	set := map[string]bool{}
	for _, app := range owned {
		set[app.Installation] = true
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func flathubArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	}
	return runtime.GOARCH
}

func hasArch(arches []string, arch string) bool {
	for _, a := range arches {
		if a == arch {
			return true
		}
	}
	return false
}

// popular fetches the top of Flathub's popularity ranking. A failed fetch serves
// the last copy on disk, flagged offline, like every registry source.
func (p flatpakProvider) popular(ctx context.Context, refresh bool) ([]flathubHit, SourceState, error) {
	file := filepath.Join(extrasCacheDir(), flathubCache)
	if !refresh {
		if info, err := os.Stat(file); err == nil && time.Since(info.ModTime()) < 6*time.Hour {
			if hits, err := decodeFlathub(mustRead(file)); err == nil {
				return hits, SourceState{}, nil
			}
		}
	}
	raw, err := p.fetchPopular(ctx)
	if err == nil {
		if hits, derr := decodeFlathub(raw); derr == nil {
			_ = atomicWrite(file, raw, 0o644)
			return hits, SourceState{}, nil
		} else {
			err = derr
		}
	}
	if archived := mustRead(file); len(archived) > 0 {
		if hits, derr := decodeFlathub(archived); derr == nil {
			state := SourceState{Offline: true}
			if info, serr := os.Stat(file); serr == nil {
				state.CachedAt = info.ModTime().UTC().Format(time.RFC3339)
			}
			return hits, state, nil
		}
	}
	return nil, SourceState{}, fmt.Errorf("flathub: %w", err)
}

func mustRead(path string) []byte {
	b, _ := os.ReadFile(path)
	return b
}

func decodeFlathub(raw []byte) ([]flathubHit, error) {
	var c flathubCollection
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if len(c.Hits) == 0 {
		return nil, errors.New("empty collection")
	}
	return c.Hits, nil
}

func (p flatpakProvider) fetchPopular(ctx context.Context) ([]byte, error) {
	return p.flathub(ctx, http.MethodGet, fmt.Sprintf("/collection/popular?page=1&per_page=%d", flathubPageSize), nil)
}

// flathub performs one bounded request against the Flathub API. payload, when
// set, is sent as the JSON body.
func (p flatpakProvider) flathub(ctx context.Context, method, path string, payload any) ([]byte, error) {
	url := strings.TrimRight(p.api, "/") + path
	ctx, cancel := context.WithTimeout(ctx, cacheTimeout)
	defer cancel()
	var body io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		body = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: %s", url, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBody {
		return nil, fmt.Errorf("%s: response over %d bytes", url, maxBody)
	}
	return data, nil
}

func flatpakItem(hit flathubHit, owned installedFlatpak, usable bool) Item {
	verification := "unverified"
	if hit.Verified {
		verification = "verified"
	}
	item := Item{
		ID:                 hit.AppID,
		Category:           flatpakCategory,
		Name:               firstNonEmpty(hit.Name, owned.Name, hit.AppID),
		Summary:            strings.TrimSpace(hit.Summary),
		Description:        strings.Join(strings.Fields(hit.Description), " "),
		Art:                hit.Icon,
		Author:             hit.Developer,
		Upstream:           "https://flathub.org/apps/" + hit.AppID,
		License:            hit.License,
		Kind:               "flatpak",
		VerificationStatus: verification,
		Tags:               flathubTags(hit),
		Installed:          owned.ID != "",
		InstalledVersion:   owned.Version,
		Metadata: map[string]any{
			"provider":          "Flathub",
			"installsLastMonth": hit.InstallsLastMonth,
		},
	}
	if hit.AddedAt > 0 {
		item.ListedAt = time.Unix(hit.AddedAt, 0).UTC().Format(time.RFC3339)
	}
	if owned.Installation != "" {
		item.Metadata["installation"] = owned.Installation
	}
	markFlatpakUsable(&item, usable)
	return item
}

// flatpakInstalledItem lists an installed application that is outside the top of
// the ranking, so it can still be found in the Library and removed.
func flatpakInstalledItem(app installedFlatpak) Item {
	item := Item{
		ID:                 app.ID,
		Category:           flatpakCategory,
		Name:               firstNonEmpty(app.Name, app.ID),
		Upstream:           "https://flathub.org/apps/" + app.ID,
		Kind:               "flatpak",
		VerificationStatus: "unverified",
		Installed:          true,
		InstalledVersion:   app.Version,
		Metadata:           map[string]any{"provider": "Flathub", "installation": app.Installation},
	}
	return item
}

func markFlatpakUsable(item *Item, usable bool) {
	if usable {
		return
	}
	item.Unavailable = true
	item.UnavailableEdition = true
	item.UnavailableReason = "Flatpak is not installed on this system."
}

func flathubTags(hit flathubHit) []string {
	var tags []string
	add := func(v any) {
		switch t := v.(type) {
		case string:
			if t != "" {
				tags = append(tags, strings.ToLower(t))
			}
		case []any:
			for _, e := range t {
				if s, ok := e.(string); ok && s != "" {
					tags = append(tags, strings.ToLower(s))
				}
			}
		}
	}
	add(hit.MainCategories)
	add(hit.SubCategories)
	return tags
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// Install puts the application in the user installation, so no privileged
// prompt is needed; the user remote is added when missing. An application that
// is already installed is updated in the installation that holds it instead,
// because that is what the store's UPDATE button runs.
func (p flatpakProvider) Install(ctx context.Context, id string) error {
	if !validFlatpakID(id) {
		return fmt.Errorf("invalid flatpak id %q", id)
	}
	if !p.present() {
		return errors.New("flatpak is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, flatpakTimeout)
	defer cancel()
	reportProgressNow(ctx, "apply", 0, 0)
	if apps, err := p.installed(ctx); err == nil {
		for _, app := range apps {
			if app.ID == id && app.Origin == flatpakRemote {
				if out, err := p.run(ctx, "update", "--noninteractive", "--assumeyes", installationFlag(app.Installation), id); err != nil {
					return flatpakError("update "+id, out, err)
				}
				return nil
			}
		}
	}
	if out, err := p.run(ctx, "remote-add", "--user", "--if-not-exists", flatpakRemote, flathubRepoFile); err != nil {
		return flatpakError("add the Flathub remote", out, err)
	}
	if out, err := p.run(ctx, "install", "--user", "--noninteractive", "--assumeyes", flatpakRemote, id); err != nil {
		return flatpakError("install "+id, out, err)
	}
	return nil
}

// Remove uninstalls the application from whichever installation holds it.
// Its user data stays, as it does for a native package. A removal from the
// user installation also frees the runtimes nothing uses any more.
func (p flatpakProvider) Remove(ctx context.Context, id string) error {
	if !validFlatpakID(id) {
		return fmt.Errorf("invalid flatpak id %q", id)
	}
	if !p.present() {
		return errors.New("flatpak is not installed")
	}
	ctx, cancel := context.WithTimeout(ctx, flatpakTimeout)
	defer cancel()
	apps, err := p.installed(ctx)
	if err != nil {
		return flatpakError("list installed applications", nil, err)
	}
	args := []string{"uninstall", "--noninteractive", "--assumeyes"}
	found := false
	scope := ""
	for _, app := range apps {
		if app.ID == id {
			found = true
			scope = app.Installation
			args = append(args, installationFlag(app.Installation))
			break
		}
	}
	if !found {
		return fmt.Errorf("%s is not installed", id)
	}
	if out, err := p.run(ctx, append(args, id)...); err != nil {
		return flatpakError("remove "+id, out, err)
	}
	if scope == "user" {
		// The runtimes an install pulled in are gigabytes and nothing else
		// frees them. Only the user installation is swept: the store is what
		// fills it, whereas the system one may hold runtimes the user
		// installed on purpose. The app is already gone, so a failure here
		// is not a failed removal.
		_, _ = p.run(ctx, "uninstall", "--unused", "--noninteractive", "--assumeyes", "--user")
	}
	return nil
}

func flatpakError(action string, out []byte, err error) error {
	detail := lastLine(out)
	if detail == "" {
		return fmt.Errorf("could not %s: %w", action, err)
	}
	return fmt.Errorf("could not %s: %s", action, detail)
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

// Search asks Flathub's own search, so an application outside the popular
// ranking can still be found. The result is live, never cached, and carries the
// installed state of this host.
func (p flatpakProvider) Search(ctx context.Context, query string) ([]Item, error) {
	query = strings.Join(strings.Fields(query), " ")
	if r := []rune(query); len(r) > flathubQueryMax {
		query = string(r[:flathubQueryMax])
	}
	if utf8.RuneCountInString(query) < flathubQueryMin {
		return []Item{}, nil
	}
	raw, err := p.flathub(ctx, http.MethodPost, "/search", map[string]any{
		"query": query, "page": 1, "hits_per_page": flathubSearchHits,
	})
	if err != nil {
		return nil, fmt.Errorf("flathub search: %w", err)
	}
	var found flathubCollection
	if err := json.Unmarshal(raw, &found); err != nil {
		return nil, fmt.Errorf("flathub search: %w", err)
	}
	usable := p.present()
	var installed []installedFlatpak
	if usable && p.installed != nil {
		installed, _ = p.installed(ctx)
	}
	return flatpakItems(found.Hits, ownedFlathub(installed), usable, nil), nil
}
