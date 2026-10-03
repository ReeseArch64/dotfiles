package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDevelopmentPacmanPackagesIncludeRustupAndTk(t *testing.T) {
	want := []string{"rustup", "tk"}
	if !slices.Equal(developmentPacmanPackages, want) {
		t.Fatalf("pacotes do ambiente inesperados: %v", developmentPacmanPackages)
	}
}

func TestDevelopmentToolsInstallStepUsesMiseConfig(t *testing.T) {
	current := developmentToolsInstallStep("/tmp/dotfiles")
	want := []string{"mise", "--cd", "/tmp/dotfiles", "install"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando mise inesperado: %v", got)
	}
}

func TestNpmLoginStepUsesMiseShim(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	current := npmLoginStep("/tmp/home")
	want := []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "npm", "login"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando npm login inesperado: %v", got)
	}
}

func TestBetterStackInstallStepUsesOfficialInstaller(t *testing.T) {
	current := betterStackInstallStep()
	want := []string{"sh", "-c", betterStackInstallCommand}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando de instalação inesperado: %v", got)
	}
}

func TestBetterStackLoginStepUsesLocalBin(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	current := betterStackLoginStep("/tmp/home")
	want := []string{"env", "PATH=/tmp/home/.local/bin:/usr/bin", "bs", "auth", "init"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando de autenticação inesperado: %v", got)
	}
}

func TestRustupToolchainStepInstallsStable(t *testing.T) {
	current := rustupToolchainStep()
	want := []string{"rustup", "default", "stable"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando rustup inesperado: %v", got)
	}
}

func TestVerifyRustToolsRunsRustcAndCargo(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(t.TempDir(), "rust.log")
	for _, name := range []string{"rustc", "cargo"} {
		writeTestExecutable(t, bin, name, `printf '%s %s\n' "$(basename "$0")" "$*" >> "$RUST_LOG"
printf '%s 1.0.0\n' "$(basename "$0")"`)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("RUST_LOG", logPath)

	if err := verifyRustTools(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	log := string(content)
	for _, want := range []string{"rustc --version", "cargo --version"} {
		if !strings.Contains(log, want) {
			t.Fatalf("verificação %q ausente:\n%s", want, log)
		}
	}
}

func TestDevelopmentEnvironmentJobConfiguresAllTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	steps := developmentEnvironmentSteps("/tmp/dotfiles", home, []step{{label: "Instalar rustup, tk"}})
	labels := jobStepLabels(job{steps: steps})
	for _, want := range []string{
		"Symlink ~/.config/mise/config.toml",
		"Ativar ferramentas do mise no Fish",
		"Instalar ferramentas configuradas via mise",
		"Instalar rustup, tk",
		"Instalar toolchain Rust estável",
		"Verificar rustc e cargo",
		"Instalar Better Stack CLI",
		"Autenticar no Better Stack",
		"Autenticar no npm",
	} {
		if !slices.Contains(labels, want) {
			t.Fatalf("passo %q ausente: %v", want, labels)
		}
	}
	if slices.Index(labels, "Instalar rustup, tk") > slices.Index(labels, "Instalar toolchain Rust estável") {
		t.Fatalf("rustup deve ser instalado antes da toolchain: %v", labels)
	}
	if slices.Index(labels, "Instalar Better Stack CLI") > slices.Index(labels, "Autenticar no Better Stack") {
		t.Fatalf("Better Stack deve ser instalado antes da autenticação: %v", labels)
	}
	if labels[len(labels)-1] != "Autenticar no npm" {
		t.Fatalf("último passo inesperado: %s", labels[len(labels)-1])
	}
}
