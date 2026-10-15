package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestYayBootstrapBuildsSourcePackage(t *testing.T) {
	home := t.TempDir()
	bin := t.TempDir()
	logPath := filepath.Join(t.TempDir(), "commands.log")
	writeTestExecutable(t, bin, "sudo", `printf 'sudo %s\n' "$*" >> "$COMMAND_LOG"`)
	writeTestExecutable(t, bin, "git", `printf 'git %s\n' "$*" >> "$COMMAND_LOG"
mkdir -p yay/.git`)
	writeTestExecutable(t, bin, "makepkg", `printf 'makepkg %s cwd=%s\n' "$*" "$PWD" >> "$COMMAND_LOG"`)
	writeTestExecutable(t, bin, "pacman", `printf 'pacman %s\n' "$*" >> "$COMMAND_LOG"
[ "$1" = -Q ] && [ "$2" = yay ]`)
	writeTestExecutable(t, bin, "yay", `exit 0`)
	t.Setenv("HOME", home)
	t.Setenv("COMMAND_LOG", logPath)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("TERM", "xterm")
	steps := ensureYayPkgs("dotfiles-test-package")
	want := []string{"bash", "-c", yayBootstrapScript}
	if got := commandTail(t, steps[1], len(want)); !slices.Equal(got, want) {
		t.Fatalf("bootstrap inesperado: %v", got)
	}
	if out, err := steps[1].cmd().CombinedOutput(); err != nil {
		t.Fatalf("bootstrap falhou: %v: %s", err, out)
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"sudo pacman -S --needed git base-devel",
		"git clone https://aur.archlinux.org/yay.git",
		"makepkg -si cwd=" + filepath.Join(home, ".aur", "yay"),
		"pacman -Q yay",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("comando ausente: %s\n%s", want, data)
		}
	}
	if strings.Contains(string(data), "yay-bin") {
		t.Fatal("bootstrap não deve instalar yay-bin")
	}
}

func TestYayBootstrapChecksInstalledPackageAtExecutionTime(t *testing.T) {
	bin := t.TempDir()
	state := filepath.Join(t.TempDir(), "installed")
	writeTestExecutable(t, bin, "yay", `exit 0`)
	writeTestExecutable(t, bin, "pacman", `[ "$2" = yay ] && [ -f "$YAY_STATE" ]`)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("YAY_STATE", state)
	steps := ensureYayPkgs("dotfiles-test-package")
	for _, s := range steps[:2] {
		if s.skip() {
			t.Fatal("bootstrap ignorado sem pacote yay instalado")
		}
	}
	if err := os.WriteFile(state, nil, 0600); err != nil {
		t.Fatal(err)
	}
	for _, s := range steps[:2] {
		if !s.skip() {
			t.Fatal("bootstrap deve ignorar yay já instalado")
		}
	}
}

func TestYayInstallSuccessDoesNotCloneAur(t *testing.T) {
	bin := t.TempDir()
	writeTestExecutable(t, bin, "yay", `exit 0`)
	writeTestExecutable(t, bin, "pacman", `exit 0`)
	writeTestExecutable(t, bin, "git", `exit 1`)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("TERM", "xterm")
	if out, err := yayInstallStep("sample").cmd().CombinedOutput(); err != nil {
		t.Fatalf("instalação falhou: %v: %s", err, out)
	}
}

func TestYayBootstrapFailsWithoutBuildingOnDependencyError(t *testing.T) {
	bin := t.TempDir()
	home := t.TempDir()
	writeTestExecutable(t, bin, "sudo", `exit 1`)
	t.Setenv("HOME", home)
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	// Executa diretamente para não aguardar a pausa interativa em caso de erro.
	cmd := terminalStepWithPause("bootstrap", false, "bash", "-c", yayBootstrapScript).cmd()
	if err := cmd.Run(); err == nil {
		t.Fatal("falha de dependências ignorada")
	}
	if _, err := os.Stat(filepath.Join(home, ".aur")); !os.IsNotExist(err) {
		t.Fatalf("bootstrap prosseguiu após falha: %v", err)
	}
}
