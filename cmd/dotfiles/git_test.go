package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteGitConfigFromEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(configPath(dir, "git"), 0755); err != nil {
		t.Fatal(err)
	}
	env := "GIT_USER_EMAIL=dev@example.com\nGIT_USERNAME=octocat\nGIT_USER_NAME='Test User'\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeGitConfig(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(configPath(dir, "git", ".gitconfig"))
	if err != nil {
		t.Fatal(err)
	}
	want := "[user]\n    signingkey =\n    email = \"dev@example.com\"\n    username = \"octocat\"\n    name = \"Test User\"\n"
	if string(got) != want {
		t.Fatalf("conteúdo inesperado:\n%s", got)
	}
}

func TestWriteGitConfigPreservesSigningKey(t *testing.T) {
	dir := t.TempDir()
	gitDir := configPath(dir, "git")
	if err := os.MkdirAll(gitDir, 0755); err != nil {
		t.Fatal(err)
	}
	fingerprint := "0123456789ABCDEF0123456789ABCDEF01234567"
	config := "[user]\n    signingkey = " + fingerprint + "\n    email = old@example.com\n"
	if err := os.WriteFile(filepath.Join(gitDir, ".gitconfig"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	env := "GIT_USER_EMAIL=dev@example.com\nGIT_USERNAME=octocat\nGIT_USER_NAME='Test User'\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeGitConfig(dir); err != nil {
		t.Fatal(err)
	}
	got, err := readGitSigningKey(filepath.Join(gitDir, ".gitconfig"))
	if err != nil {
		t.Fatal(err)
	}
	if got != fingerprint {
		t.Fatalf("signingkey inesperada: %s", got)
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
	if err := os.WriteFile(source, []byte("[user]\n    signingkey =\n"), 0644); err != nil {
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

func TestWriteGitConfigRequiresIdentity(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("GIT_USER_EMAIL=dev@example.com\n"), 0600); err != nil {
		t.Fatal(err)
	}

	err := writeGitConfig(dir)
	if err == nil {
		t.Fatal("esperava erro para identidade incompleta")
	}
	if !strings.Contains(err.Error(), "GIT_USERNAME, GIT_USER_NAME") {
		t.Fatalf("erro inesperado: %v", err)
	}
}
