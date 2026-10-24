package sys

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPicturesDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("XDG_PICTURES_DIR", "")

	if got, want := PicturesDir(), filepath.Join(home, "Pictures"); got != want {
		t.Errorf("no env, no file: got %q, want %q", got, want)
	}

	write := func(body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(home, ".config"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(home, ".config", "user-dirs.dirs"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("# written by xdg-user-dirs-update\nXDG_DOWNLOAD_DIR=\"$HOME/Descargas\"\nXDG_PICTURES_DIR=\"$HOME/Imágenes\"\n")
	if got, want := PicturesDir(), filepath.Join(home, "Imágenes"); got != want {
		t.Errorf("user-dirs.dirs: got %q, want %q", got, want)
	}

	// xdg-user-dirs writes $HOME itself for a folder the user disabled; that is
	// not a Pictures folder, so the default wins.
	write("XDG_PICTURES_DIR=\"$HOME/\"\n")
	if got, want := PicturesDir(), filepath.Join(home, "Pictures"); got != want {
		t.Errorf("disabled folder: got %q, want %q", got, want)
	}

	t.Setenv("XDG_PICTURES_DIR", "/data/pics")
	if got := PicturesDir(); got != "/data/pics" {
		t.Errorf("session env wins: got %q", got)
	}
}
