package main

import (
	"os"
	"path/filepath"
	"testing"
)

// The wall preview, pinned down: a preview is a paint, never an apply. It
// publishes the previewed still flagged as a preview, and it leaves the
// catalog, the stored choice, and the applied state exactly as the last
// commit left them. Clearing repaints the committed wallpaper.

func previewTestDaemon(t *testing.T) *daemon {
	t.Helper()
	root := t.TempDir()
	var cfg config
	cfg.Paths.Wallpaper = filepath.Join(root, "wall")
	cfg.Paths.Cache = filepath.Join(root, "cache")
	if err := os.MkdirAll(cfg.Paths.Wallpaper, 0o755); err != nil {
		t.Fatal(err)
	}
	return &daemon{
		cfg:            cfg,
		surface:        newWallSurface(),
		store:          openStore(cfg.Paths.Cache),
		events:         newEventHub(),
		video:          newVideoPlayer(),
		lastTransition: -1,
	}
}

func previewTestStill(t *testing.T, d *daemon, name string) string {
	t.Helper()
	p := filepath.Join(d.config().wallpaperDir(), name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	d.store.entries[name] = Entry{Key: name, Name: name, Type: "static", Path: p}
	return p
}

func TestPreviewPaintsFlaggedAndRecordsNothing(t *testing.T) {
	d := previewTestDaemon(t)
	committed := previewTestStill(t, d, "committed.png")
	preview := previewTestStill(t, d, "preview.png")
	if err := d.applyWallpaper("static", committed, "set", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	savedBefore, err := os.ReadFile(filepath.Join(d.config().cacheDir(), "outputs.json"))
	if err != nil {
		t.Fatal(err)
	}

	if err := d.previewWallpaper(preview, nil); err != nil {
		t.Fatal(err)
	}
	frame := d.surface.snapshot()
	if frame.Default.Path != preview {
		t.Fatalf("frame path=%q, want the previewed still %q", frame.Default.Path, preview)
	}
	if !frame.Default.Preview {
		t.Fatal("frame entry is not flagged as a preview")
	}
	savedAfter, err := os.ReadFile(filepath.Join(d.config().cacheDir(), "outputs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(savedAfter) != string(savedBefore) {
		t.Fatalf("preview rewrote the stored choice:\nbefore %s\nafter  %s", savedBefore, savedAfter)
	}
	if e, _ := d.store.get("preview.png"); e.ApplyCount != 0 {
		t.Fatalf("preview bumped its apply count to %d", e.ApplyCount)
	}
	if e, _ := d.store.get("committed.png"); e.ApplyCount != 1 {
		t.Fatalf("committed apply count=%d, want 1", e.ApplyCount)
	}
}

func TestPreviewReplacesPreview(t *testing.T) {
	d := previewTestDaemon(t)
	first := previewTestStill(t, d, "first.png")
	second := previewTestStill(t, d, "second.png")
	if err := d.previewWallpaper(first, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.previewWallpaper(second, nil); err != nil {
		t.Fatal(err)
	}
	frame := d.surface.snapshot()
	if frame.Default.Path != second || !frame.Default.Preview {
		t.Fatalf("frame=(%q, preview=%v), want the second still flagged", frame.Default.Path, frame.Default.Preview)
	}
	st := d.preview
	if st == nil || st.path != second {
		t.Fatalf("preview state=%+v, want it tracking the second still", st)
	}
}

func TestClearPreviewRepaintsCommitted(t *testing.T) {
	d := previewTestDaemon(t)
	committed := previewTestStill(t, d, "committed.png")
	preview := previewTestStill(t, d, "preview.png")
	if err := d.applyWallpaper("static", committed, "set", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.previewWallpaper(preview, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.clearPreview(); err != nil {
		t.Fatal(err)
	}
	if d.preview != nil {
		t.Fatal("preview state survived the clear")
	}
	frame := d.surface.snapshot()
	if frame.Default.Path != committed {
		t.Fatalf("frame path=%q, want the committed still %q back", frame.Default.Path, committed)
	}
	if frame.Default.Preview {
		t.Fatal("repainted committed entry is still flagged as a preview")
	}
	if got := d.currentName(); got != "committed.png" {
		t.Fatalf("current name=%q, want the pre-preview name restored", got)
	}
}

func TestApplySupersedesPreview(t *testing.T) {
	d := previewTestDaemon(t)
	preview := previewTestStill(t, d, "preview.png")
	other := previewTestStill(t, d, "other.png")
	if err := d.previewWallpaper(preview, nil); err != nil {
		t.Fatal(err)
	}
	if err := d.applyWallpaper("static", other, "set", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if d.preview != nil {
		t.Fatal("a committed apply left preview state behind")
	}
	frame := d.surface.snapshot()
	if frame.Default.Path != other || frame.Default.Preview {
		t.Fatalf("frame=(%q, preview=%v), want the applied still unflagged", frame.Default.Path, frame.Default.Preview)
	}
}

func TestClearPreviewWithoutPreviewIsNoOp(t *testing.T) {
	d := previewTestDaemon(t)
	if err := d.clearPreview(); err != nil {
		t.Fatalf("clear with no live preview: %v", err)
	}
}

func TestPreviewRejectsNonStaticAndMissing(t *testing.T) {
	d := previewTestDaemon(t)
	clip := filepath.Join(d.config().wallpaperDir(), "clip.mp4")
	if err := os.WriteFile(clip, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := d.previewWallpaper(clip, nil); err == nil {
		t.Fatal("preview accepted a video")
	}
	if err := d.previewWallpaper(filepath.Join(d.config().wallpaperDir(), "gone.png"), nil); err == nil {
		t.Fatal("preview accepted a missing file")
	}
	if err := d.previewWallpaper("", nil); err == nil {
		t.Fatal("preview accepted an empty path")
	}
	if d.preview != nil {
		t.Fatal("a rejected preview left state behind")
	}
}
