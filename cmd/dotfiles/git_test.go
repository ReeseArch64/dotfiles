package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestGitPacmanPackagesIncludeGitHubCLI(t *testing.T) {
	want := []string{"git", "github-cli"}
	if !slices.Equal(gitPacmanPackages, want) {
		t.Fatalf("pacotes Pacman do Git inesperados: %v", gitPacmanPackages)
	}
}

func TestGitShellyPackagesIncludeGlab(t *testing.T) {
	want := []string{"lazygit", "glab"}
	if !slices.Equal(gitShellyPackages, want) {
		t.Fatalf("pacotes Git inesperados: %v", gitShellyPackages)
	}
}

func TestGitHubLoginStepUsesSSH(t *testing.T) {
	current := githubLoginStep()
	want := []string{"gh", "auth", "login", "--git-protocol", "ssh"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando gh inesperado: %v", got)
	}
}

func TestGlabLoginStepUsesSSH(t *testing.T) {
	current := glabLoginStep()
	want := []string{"glab", "auth", "login", "--git-protocol", "ssh"}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando glab inesperado: %v", got)
	}
}

func TestValidateGitSSHRequiresConfiguredClient(t *testing.T) {
	dotfiles := t.TempDir()
	home := t.TempDir()
	if err := validateGitSSH(dotfiles, home); err == nil || !strings.Contains(err.Error(), "chaves ausentes") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestValidateGitSSHAcceptsConfiguredClient(t *testing.T) {
	dotfiles := t.TempDir()
	home := t.TempDir()
	if err := os.MkdirAll(configPath(dotfiles, "ssh"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	for _, name := range requiredSSHIdentityFiles {
		if err := os.WriteFile(filepath.Join(home, ".ssh", name), []byte(name), 0600); err != nil {
			t.Fatal(err)
		}
	}
	content := []byte("Host github.com\n")
	if err := os.WriteFile(configPath(dotfiles, "ssh", "config"), content, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".ssh", "config"), content, 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateGitSSH(dotfiles, home); err != nil {
		t.Fatal(err)
	}
}

func TestWriteGitSigningKeyPreservesIdentity(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".gitconfig")
	content := "[user]\n    signingkey =\n    email = \"dev@example.com\"\n    username = \"octocat\"\n    name = \"Test User\"\n"
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
	fingerprint := "0123456789ABCDEF0123456789ABCDEF01234567"

	if err := writeGitSigningKey(path, fingerprint); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "signingkey = "+fingerprint) {
		t.Fatalf("signingkey não atualizada:\n%s", got)
	}
	for _, identity := range []string{"dev@example.com", "octocat", "Test User"} {
		if !strings.Contains(string(got), identity) {
			t.Fatalf("identidade %q removida:\n%s", identity, got)
		}
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("permissões inesperadas: %v", info.Mode().Perm())
	}
}

func TestGitJobValidatesSSHBeforeAuthentications(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	steps := gitJob(t.TempDir()).steps
	labels := jobStepLabels(job{steps: steps})
	want := []string{"Validar SSH para GitHub e GitLab", "Autenticar no GitHub com SSH", "Autenticar no GitLab com SSH"}
	if got := labels[len(labels)-len(want):]; !slices.Equal(got, want) {
		t.Fatalf("passos finais inesperados: %v", got)
	}
}

func TestGitJobCopiesGitconfigAsRegularFile(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	gitDir := configPath(dotfiles, "git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(gitDir, ".gitconfig")
	content := "[user]\n    signingkey =\n    email = dev@example.com\n    username = octocat\n    name = Test User\n"
	if err := os.WriteFile(source, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".gitconfig")
	if err := os.Symlink(source, destination); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)

	configured := false
	for _, current := range gitJob(dotfiles).steps {
		if current.label == "Copiar ~/.gitconfig" {
			if output, err := current.run(); err != nil {
				t.Fatalf("copiar .gitconfig: %s: %v", output, err)
			}
			configured = true
			break
		}
	}
	if !configured {
		t.Fatal("passo de cópia de ~/.gitconfig ausente")
	}
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("~/.gitconfig não é arquivo regular: %v", info.Mode())
	}
	got, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != content {
		t.Fatalf("conteúdo inesperado em ~/.gitconfig:\n%s", got)
	}
}

func TestApplyGitSigningKeyUpdatesRegularHomeCopy(t *testing.T) {
	dotfiles := filepath.Join(t.TempDir(), "dotfiles")
	home := filepath.Join(t.TempDir(), "home")
	gitDir := configPath(dotfiles, "git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home, 0755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(gitDir, ".gitconfig")
	content := "[user]\n    signingkey =\n    email = \"dev@example.com\"\n"
	if err := os.WriteFile(source, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(home, ".gitconfig")
	if err := os.Symlink(source, destination); err != nil {
		t.Fatal(err)
	}
	fingerprint := "0123456789ABCDEF0123456789ABCDEF01234567"

	if err := applyGitSigningKey(dotfiles, home, fingerprint); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{source, destination} {
		got, err := readGitSigningKey(path)
		if err != nil {
			t.Fatal(err)
		}
		if got != fingerprint {
			t.Fatalf("signingkey inesperada em %s: %s", path, got)
		}
	}
	info, err := os.Lstat(destination)
	if err != nil {
		t.Fatal(err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("cópia home inválida: %v", info.Mode())
	}
}
