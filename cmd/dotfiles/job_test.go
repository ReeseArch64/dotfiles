package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func commandTail(t *testing.T, current step, length int) []string {
	t.Helper()
	if current.cmd == nil {
		t.Fatal("esperava passo interativo")
	}
	args := current.cmd().Args
	if len(args) < length {
		t.Fatalf("comando incompleto: %v", args)
	}
	return args[len(args)-length:]
}

func TestJobSkipsConfiguredStepAtExecutionTime(t *testing.T) {
	configured := false
	called := false
	r := newJobRun(job{steps: []step{skipWhen(nativeStep("login", func() error {
		called = true
		return nil
	}), func() bool { return configured })}})
	configured = true
	if command := r.startStep(); command != nil || !r.done || called || r.states[0] != stepSkipped {
		t.Fatalf("passo não foi ignorado: done=%v called=%v states=%v", r.done, called, r.states)
	}
	configured = false
	r = newJobRun(r.job)
	message := r.startStep()()
	if _, ok := message.(stepDoneMsg); !ok {
		t.Fatalf("mensagem inesperada: %T", message)
	}
	if !called || r.states[0] != stepRunning {
		t.Fatal("passo ausente não foi executado")
	}
}

func TestPacmanSyncStepRefreshesAllDatabases(t *testing.T) {
	current := pacmanSyncStep()
	want := []string{"sudo", "pacman", "-Syy"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("sincronização do Pacman inesperada: %v", got)
	}
}

func TestEnsurePkgsRefreshesBeforeInstall(t *testing.T) {
	steps := ensurePkgs("dotfiles-test-missing-package")
	if len(steps) != 2 {
		t.Fatalf("passos inesperados: %v", jobStepLabels(job{steps: steps}))
	}
	if got := commandTail(t, steps[0], 3); !slices.Equal(got, []string{"sudo", "pacman", "-Syy"}) {
		t.Fatalf("primeiro comando inesperado: %v", got)
	}
	want := []string{"sudo", "pacman", "-S", "--needed", "dotfiles-test-missing-package"}
	if got := commandTail(t, steps[1], len(want)); !slices.Equal(got, want) {
		t.Fatalf("instalação Pacman inesperada: %v", got)
	}
}

func TestEnsureShellyPkgsRefreshesBeforeEachPackageFlow(t *testing.T) {
	steps := ensureShellyPkgs("aur", "dotfiles-test-one", "dotfiles-test-two")
	if len(steps) != 4 {
		t.Fatalf("passos inesperados: %v", jobStepLabels(job{steps: steps}))
	}
	for i, pkg := range []string{"dotfiles-test-one", "dotfiles-test-two"} {
		syncStep := steps[i*2]
		installStep := steps[i*2+1]
		if got := commandTail(t, syncStep, 3); !slices.Equal(got, []string{"sudo", "pacman", "-Syy"}) {
			t.Fatalf("sincronização ausente antes de %s: %v", pkg, got)
		}
		want := []string{"bash", "-c", shellyFallbackScript, "dotfiles-shelly-fallback", "aur", pkg}
		if got := commandTail(t, installStep, len(want)); !slices.Equal(got, want) {
			t.Fatalf("comando Shelly inesperado para %s: %v", pkg, got)
		}
	}
}

func TestShellyInstallStepFallsBackToAurAndVerifiesPackage(t *testing.T) {
	home := t.TempDir()
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "commands.log")
	writeTestExecutable(t, bin, "shelly", `printf 'shelly %s\n' "$*" >> "$COMMAND_LOG"
exit 1`)
	writeTestExecutable(t, bin, "git", `printf 'git %s\n' "$*" >> "$COMMAND_LOG"
repo=${2##*/}
repo=${repo%.git}
mkdir -p "$repo/.git"`)
	writeTestExecutable(t, bin, "makepkg", `printf 'makepkg %s cwd=%s\n' "$*" "$PWD" >> "$COMMAND_LOG"`)
	writeTestExecutable(t, bin, "pacman", `printf 'pacman %s\n' "$*" >> "$COMMAND_LOG"
[ "$1" = "-Q" ] && [ "$2" = "sample-aur-package" ]`)
	t.Setenv("HOME", home)
	t.Setenv("COMMAND_LOG", logPath)
	t.Setenv("TERM", "xterm-256color")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	if out, err := shellyInstallStep("aur", "sample-aur-package").cmd().CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	if info, err := os.Stat(filepath.Join(home, ".aur", "sample-aur-package", ".git")); err != nil || !info.IsDir() {
		t.Fatalf("clone AUR ausente: info=%v err=%v", info, err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(content)
	for _, want := range []string{
		"shelly install aur sample-aur-package",
		"git clone https://aur.archlinux.org/sample-aur-package.git",
		"makepkg -si cwd=" + filepath.Join(home, ".aur", "sample-aur-package"),
		"pacman -Q sample-aur-package",
	} {
		if !strings.Contains(log, want) {
			t.Fatalf("comando %q ausente:\n%s", want, log)
		}
	}
}

func writeTestExecutable(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0755); err != nil {
		t.Fatal(err)
	}
}
