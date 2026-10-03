package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDevelopmentPacmanPackagesIncludeFishRustupAndTk(t *testing.T) {
	want := []string{"fish", "rustup", "tk"}
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

func TestCloudLoginStepsUseMiseShims(t *testing.T) {
	t.Setenv("PATH", "/usr/bin")
	steps := cloudLoginSteps("/tmp/home")
	want := []struct {
		label string
		args  []string
	}{
		{"Autenticar na AWS", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "aws", "login"}},
		{"Autenticar no Google Cloud", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "gcloud", "auth", "login"}},
		{"Autenticar no Railway", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "railway", "login"}},
		{"Autenticar no Firebase", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "firebase", "login"}},
		{"Autenticar no Azure", []string{"env", "PATH=/tmp/home/.local/share/mise/shims:/usr/bin", "az", "login"}},
	}
	if len(steps) != len(want) {
		t.Fatalf("quantidade de autenticações inesperada: %d", len(steps))
	}
	for i, expected := range want {
		if steps[i].label != expected.label {
			t.Fatalf("rótulo inesperado: %s", steps[i].label)
		}
		if got := commandTail(t, steps[i], len(expected.args)); !slices.Equal(got, expected.args) {
			t.Fatalf("comando de autenticação inesperado para %s: %v", expected.label, got)
		}
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
	steps := developmentEnvironmentSteps("/tmp/dotfiles", home, []step{{label: "Instalar fish, rustup, tk"}})
	labels := jobStepLabels(job{steps: steps})
	for _, want := range []string{
		"Symlink ~/.config/mise/config.toml",
		"Instalar ferramentas configuradas via mise",
		"Autenticar na AWS",
		"Autenticar no Google Cloud",
		"Autenticar no Railway",
		"Autenticar no Firebase",
		"Autenticar no Azure",
		"Instalar fish, rustup, tk",
		"Copiar configuração para ~/.config/fish/config.fish",
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
	if slices.Index(labels, "Instalar ferramentas configuradas via mise") > slices.Index(labels, "Autenticar na AWS") {
		t.Fatalf("ferramentas do Mise devem ser instaladas antes das autenticações: %v", labels)
	}
	if slices.Index(labels, "Autenticar no Azure") > slices.Index(labels, "Instalar fish, rustup, tk") {
		t.Fatalf("autenticações cloud devem ocorrer antes das instalações do Pacman: %v", labels)
	}
	if slices.Index(labels, "Instalar fish, rustup, tk") > slices.Index(labels, "Copiar configuração para ~/.config/fish/config.fish") {
		t.Fatalf("Fish deve ser instalado antes de copiar sua configuração: %v", labels)
	}
	if slices.Index(labels, "Instalar fish, rustup, tk") > slices.Index(labels, "Instalar toolchain Rust estável") {
		t.Fatalf("rustup deve ser instalado antes da toolchain: %v", labels)
	}
	if slices.Index(labels, "Instalar Better Stack CLI") > slices.Index(labels, "Autenticar no Better Stack") {
		t.Fatalf("Better Stack deve ser instalado antes da autenticação: %v", labels)
	}
	if labels[len(labels)-1] != "Autenticar no npm" {
		t.Fatalf("último passo inesperado: %s", labels[len(labels)-1])
	}
}
