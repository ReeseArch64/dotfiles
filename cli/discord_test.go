package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestDiscordResourcesSelectsNewestCompleteNativeVersion(t *testing.T) {
	root := t.TempDir()
	sdkTestFile(t, filepath.Join(root, "app-1.0.99", "resources", "app.asar"), "old")
	sdkTestFile(t, filepath.Join(root, "app-1.0.100", "resources", "betterdiscord.app.asar"), "patched")
	sdkTestFile(t, filepath.Join(root, "app-1.0.101", "modules", "module.json"), "incomplete")
	sdkTestFile(t, filepath.Join(root, "app-1.0.1.900", "resources", "app.asar"), "invalid version")
	got, err := discordResources(root)
	if want := filepath.Join(root, "app-1.0.100", "resources"); err != nil || got != want {
		t.Fatalf("recursos = %q, esperado %q, erro %v", got, want, err)
	}
}

func TestBetterDiscordConfigTargetsStableAndPreservesOtherFiles(t *testing.T) {
	home, _, backups := sdkTestRoots(t)
	destination := filepath.Join(home, "BetterDiscord")
	source := filepath.Join("..", "cachyos", "discord", "BetterDiscord")
	settings := filepath.Join(destination, "data", "stable", "settings.json")
	unmanaged := filepath.Join(destination, "plugins", "other.plugin.js")
	sdkTestFile(t, settings, "settings anteriores")
	sdkTestFile(t, unmanaged, "outro plugin")
	files, err := betterDiscordConfigFiles(source, destination)
	if err != nil {
		t.Fatal(err)
	}
	backup, err := applyConfigFiles(files, backupLocation{path: backups}, "betterdiscord")
	if err != nil || backup == "" {
		t.Fatalf("backup da configuração: %q %v", backup, err)
	}
	sdkTestContent(t, unmanaged, "outro plugin")
	sdkTestContent(t, filepath.Join(backup, "settings.json"), "settings anteriores")
	for _, name := range []string{"data/stable/settings.json", "data/stable/plugins.json", "data/stable/themes.json",
		"plugins/BetterAnimations.plugin.js", "themes/Material-Discord.theme.css"} {
		if _, err := os.Stat(filepath.Join(destination, name)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestInstallBetterDiscordRestoresExistingAsarOnFailure(t *testing.T) {
	root := t.TempDir()
	asar := filepath.Join(root, "betterdiscord.asar")
	original := filepath.Join(root, "external.asar")
	sdkTestFile(t, original, "previous bundle")
	if err := os.Symlink(original, asar); err != nil {
		t.Fatal(err)
	}
	for _, success := range []bool{false, true} {
		commands := sshCommands{interactive: func(args ...string) error {
			if !slices.Equal(args, []string{"env", "-u", "BDCLI_DEV_BUILD", "/usr/bin/bdcli", "install", "--path", root}) {
				t.Fatalf("comando inesperado: %v", args)
			}
			sdkTestFile(t, asar, "new bundle")
			if !success {
				return errors.New("download interrompido")
			}
			return nil
		}}
		if err := installBetterDiscord(commands, "/usr/bin/bdcli", root, asar); (err == nil) != success {
			t.Fatalf("instalação com sucesso %t: %v", success, err)
		}
		sdkTestContent(t, original, "previous bundle")
		if !success {
			if link, err := os.Readlink(asar); err != nil || link != original {
				t.Fatalf("link anterior = %q (%v)", link, err)
			}
		} else {
			sdkTestContent(t, asar, "new bundle")
		}
	}
}

func TestRetireLegacyDiscordLoaderOnlyReplacesKnownWrapper(t *testing.T) {
	home, app, backups := sdkTestRoots(t)
	asar := filepath.Join(home, "BetterDiscord", "data", "betterdiscord.asar")
	const stock = "module.exports = require('./core.asar');\n"
	legacy := fmt.Sprintf("require(%q);\n%s", asar, stock)
	legacyFile := filepath.Join(app, "modules", "discord_desktop_core-2", "discord_desktop_core", "index.js")
	unknownFile := filepath.Join(app, "modules", "discord_desktop_core-3", "discord_desktop_core", "index.js")
	sdkTestFile(t, legacyFile, legacy)
	sdkTestFile(t, unknownFile, "// outro carregador\n"+legacy)
	if err := os.Symlink("discord_desktop_core-2", filepath.Join(app, "modules", "discord_desktop_core-1")); err != nil {
		t.Fatal(err)
	}
	if err := retireLegacyDiscordLoader(&sshConsole{output: io.Discard}, app, asar, backupLocation{path: backups}); err != nil {
		t.Fatal(err)
	}
	sdkTestContent(t, legacyFile, stock)
	sdkTestContent(t, unknownFile, "// outro carregador\n"+legacy)
	entries, err := os.ReadDir(backups)
	if err != nil || len(entries) != 1 {
		t.Fatalf("backup dos carregadores: %v %v", entries, err)
	}
	sdkTestContent(t, filepath.Join(backups, entries[0].Name(), "index.js"), legacy)
}
