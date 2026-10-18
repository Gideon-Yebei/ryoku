package doctor

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestVesktopPaletteMigration(t *testing.T) {
	root := t.TempDir()
	config, state := filepath.Join(root, "config"), filepath.Join(root, "state")
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("XDG_STATE_HOME", state)
	template := filepath.Join(config, "ryoku/user_edits/matugen/templates/vesktop-colors.css")
	ownership := filepath.Join(state, "ryoku/palette-bridge/owned-files.tsv")
	settings := filepath.Join(config, "vesktop/settings/settings.json")
	overlay := filepath.Join(config, "ryoku/user_edits/matugen/apps.toml")
	write := func(path, content string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	legacy := fmt.Sprintf("[config]\n\n[templates.vesktop]\ninput_path = %q\noutput_path = %q\n", filepath.Join(config, "matugen/templates/vesktop-colors.css"), filepath.Join(config, "vesktop/settings/quickCss.css"))
	write(overlay, legacy)
	write(settings, `{"useQuickCss":true}`)
	if legacyVesktopPalette(config, state) {
		t.Fatal("unowned integration selected")
	}
	write(ownership, "vesktop\t"+template+"\n")
	if !legacyVesktopPalette(config, state) {
		t.Fatal("owned legacy registration not selected")
	}
	for _, raw := range []string{`{}`, `{"useQuickCss":false}`, `invalid`} {
		write(settings, raw)
		if legacyVesktopPalette(config, state) {
			t.Fatalf("disabled/invalid settings selected: %s", raw)
		}
	}
	write(settings, `{"useQuickCss":true}`)
	for _, raw := range []string{legacy + "post_hook = \"custom\"\n", "[config]\n", legacy + "# customized\n"} {
		write(overlay, raw)
		if legacyVesktopPalette(config, state) {
			t.Fatal("edited/missing registration selected")
		}
	}
	write(overlay, legacy)
	installer := filepath.Join(root, "installer.sh")
	write(installer, "#!/bin/sh\n[ \"$1\" = --vesktop ] || exit 2\nprintf '[config]\\n' > \"$XDG_CONFIG_HOME/ryoku/user_edits/matugen/apps.toml\"\n")
	old := vesktopPaletteInstaller
	vesktopPaletteInstaller = installer
	t.Cleanup(func() { vesktopPaletteInstaller = old })
	if r := reconcileVesktopPalette(true); r.status != recWouldFix {
		t.Fatalf("check-only: %+v", r)
	}
	if !legacyVesktopPalette(config, state) {
		t.Fatal("check-only mutated registration")
	}
	if r := reconcileVesktopPalette(false); r.status != recFixed {
		t.Fatalf("repair: %+v", r)
	}
	if legacyVesktopPalette(config, state) {
		t.Fatal("migration did not invoke installer")
	}
	write(installer, "exit 99\n")
	if r := reconcileVesktopPalette(false); r.status != recOK {
		t.Fatalf("repeat: %+v", r)
	}
}
