package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func preparePiAgentTest(t *testing.T) (string, string) {
	t.Helper()
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	themeDir := filepath.Join(dotfiles, "pi", "agent", "themes")
	if err := os.MkdirAll(themeDir, 0755); err != nil {
		t.Fatal(err)
	}
	settings, err := json.Marshal(piSettingsFile{Theme: "noctalia", Packages: requiredPiPackages})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dotfiles, "pi", "agent", "settings.json"), settings, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themeDir, "noctalia.json"), []byte(`{"name":"noctalia","colors":{}}`), 0644); err != nil {
		t.Fatal(err)
	}
	return dotfiles, home
}

func TestRepositoryPiSettingsDeclareRequiredPackages(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "pi", "agent", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var settings piSettingsFile
	if err := json.Unmarshal(content, &settings); err != nil {
		t.Fatal(err)
	}
	if settings.Theme != "noctalia" {
		t.Fatalf("tema inesperado: %s", settings.Theme)
	}
	if !slices.Equal(settings.Packages, requiredPiPackages) {
		t.Fatalf("pacotes inesperados: %v", settings.Packages)
	}
}

func TestInstallPiConfigCopiesRegularFilesAndPreservesExisting(t *testing.T) {
	dotfiles, home := preparePiAgentTest(t)
	agentDir := piAgentDir(home)
	if err := os.MkdirAll(filepath.Join(agentDir, "themes"), 0755); err != nil {
		t.Fatal(err)
	}
	settingsDestination := filepath.Join(agentDir, "settings.json")
	if err := os.WriteFile(settingsDestination, []byte("old settings"), 0600); err != nil {
		t.Fatal(err)
	}
	themeSource := filepath.Join(dotfiles, "pi", "agent", "themes", "noctalia.json")
	themeDestination := filepath.Join(agentDir, "themes", "noctalia.json")
	if err := os.Symlink(themeSource, themeDestination); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)

	if err := installPiConfig(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	for _, file := range piConfigFiles(dotfiles, home) {
		info, err := os.Lstat(file.destination)
		if err != nil {
			t.Fatal(err)
		}
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			t.Fatalf("%s não é arquivo regular: %v", file.name, info.Mode())
		}
		if !regularFileMatches(file.source, file.destination) {
			t.Fatalf("%s não corresponde à origem", file.name)
		}
		if _, err := os.Lstat(file.destination + ".backup-20250102-030405"); err != nil {
			t.Fatalf("backup de %s ausente: %v", file.name, err)
		}
	}
}

func TestInstallPiConfigIsIdempotent(t *testing.T) {
	dotfiles, home := preparePiAgentTest(t)
	now := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := installPiConfig(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	if err := installPiConfig(dotfiles, home, now); err != nil {
		t.Fatal(err)
	}
	for _, file := range piConfigFiles(dotfiles, home) {
		matches, err := filepath.Glob(file.destination + ".backup-*")
		if err != nil {
			t.Fatal(err)
		}
		if len(matches) != 0 {
			t.Fatalf("backups inesperados para %s: %v", file.name, matches)
		}
	}
}

func TestPiAgentJobUpdatesPackagesInConfiguredAgentDir(t *testing.T) {
	dotfiles, home := preparePiAgentTest(t)
	lookPath := func(name string) (string, error) {
		if name != "pi" {
			return "", errors.New("comando inesperado")
		}
		return "/usr/bin/pi", nil
	}
	configured := piAgentJobFor(dotfiles, home, lookPath)
	if len(configured.steps) != 3 {
		t.Fatalf("quantidade inesperada de passos: %d", len(configured.steps))
	}
	want := []string{"env", "PI_CODING_AGENT_DIR=" + piAgentDir(home), "/usr/bin/pi", "update", "--extensions"}
	args := configured.steps[2].cmd().Args
	got := args[len(args)-len(want):]
	if !slices.Equal(got, want) {
		t.Fatalf("comando inesperado: %v", got)
	}
}

func TestPiAgentJobInstallsPiWhenMissing(t *testing.T) {
	dotfiles, home := preparePiAgentTest(t)
	configured := piAgentJobFor(dotfiles, home, func(string) (string, error) {
		return "", errors.New("ausente")
	})
	if len(configured.steps) != 4 {
		t.Fatalf("quantidade inesperada de passos: %d", len(configured.steps))
	}
	installWant := []string{
		"env",
		"PI_CODING_AGENT_DIR=" + piAgentDir(home),
		"sh",
		"-c",
		"curl -fsSL https://pi.dev/install.sh | sh",
	}
	installArgs := configured.steps[1].cmd().Args
	if got := installArgs[len(installArgs)-len(installWant):]; !slices.Equal(got, installWant) {
		t.Fatalf("comando de instalação inesperado: %v", got)
	}
	updateWant := []string{
		"env",
		"PI_CODING_AGENT_DIR=" + piAgentDir(home),
		filepath.Join(piAgentDir(home), "bin", "pi"),
		"update",
		"--extensions",
	}
	updateArgs := configured.steps[3].cmd().Args
	if got := updateArgs[len(updateArgs)-len(updateWant):]; !slices.Equal(got, updateWant) {
		t.Fatalf("comando de atualização inesperado: %v", got)
	}
}

func TestLoadPiAgentStatusCountsInstalledPackages(t *testing.T) {
	dotfiles, home := preparePiAgentTest(t)
	if err := installPiConfig(dotfiles, home, time.Now()); err != nil {
		t.Fatal(err)
	}
	agentDir := piAgentDir(home)
	for _, source := range requiredPiPackages {
		if err := os.MkdirAll(piPackagePath(agentDir, source), 0755); err != nil {
			t.Fatal(err)
		}
	}
	status := loadPiAgentStatusForHome(dotfiles, home, func(string) (string, error) { return "/usr/bin/pi", nil })
	if !status.piAvailable || !status.settingsOK || !status.themeOK || status.configErr != nil {
		t.Fatalf("status inesperado: %#v", status)
	}
	if status.installed != len(requiredPiPackages) {
		t.Fatalf("pacotes instalados: %d", status.installed)
	}
}
