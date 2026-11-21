package main

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestInstallCursorValidatesAndPreservesBackups(t *testing.T) {
	home := t.TempDir()
	assets := filepath.Join(home, ".dotfiles", "cachyos")
	niri := filepath.Join(home, ".config", "niri")
	config := filepath.Join(niri, "config.kdl")
	misc := filepath.Join(niri, "cfg", "misc.kdl")
	asset := filepath.Join(assets, "niri", "cfg", "misc.kdl")
	theme := filepath.Join(home, ".icons", cursorTheme)
	originalConfig := "include \"./cfg/misc.kdl\" // keep this config\ninclude \"cfg/input.kdl\"\n"
	for path, content := range map[string]string{
		config:                                  originalConfig,
		misc:                                    "old settings",
		filepath.Join(niri, "cfg", "input.kdl"): "keep input",
		asset:                                   "new settings",
		filepath.Join(assets, "cursor", cursorTheme, "cursors", "default"): "new cursor",
		filepath.Join(assets, "cursor", cursorTheme, "index.theme"):        "theme",
		filepath.Join(theme, "index.theme"):                                "old theme",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	checkFile := func(path, want string) {
		t.Helper()
		if contents, err := os.ReadFile(path); err != nil || string(contents) != want {
			t.Fatalf("%s contém %q, esperado %q: %v", path, contents, want, err)
		}
	}

	invalid := errors.New("validation failed")
	if _, err := installCursor(assets, config, home, func(string) error { return invalid }); !errors.Is(err, invalid) {
		t.Fatalf("resultado de validação inválida: %v", err)
	}
	checkFile(misc, "old settings")
	checkFile(filepath.Join(theme, "index.theme"), "old theme")

	validate := func(candidate string) error {
		data, err := os.ReadFile(candidate)
		if err != nil {
			return err
		}
		if filepath.Dir(candidate) != niri || !strings.Contains(string(data), "include "+strconv.Quote(asset)) ||
			!strings.Contains(string(data), "include \"cfg/input.kdl\"") {
			t.Fatalf("candidato incorreto: %s", data)
		}
		return nil
	}
	backups, err := installCursor(assets, config, home, validate)
	if err != nil || len(backups) != 2 {
		t.Fatalf("instalação: backups %v, erro %v", backups, err)
	}
	checkFile(filepath.Join(backups[0], "index.theme"), "old theme")
	checkFile(backups[1], "old settings")
	checkFile(config, originalConfig)
	checkFile(filepath.Join(niri, "cfg", "input.kdl"), "keep input")
	checkFile(misc, "new settings")
	checkFile(filepath.Join(theme, "cursors", "default"), "new cursor")

	if backups, err := installCursor(assets, config, home, validate); err != nil || len(backups) != 0 {
		t.Fatalf("reaplicação: backups %v, erro %v", backups, err)
	}
}
