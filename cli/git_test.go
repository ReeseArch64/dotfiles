package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteGitConfigFromEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "git"), 0755); err != nil {
		t.Fatal(err)
	}
	env := "GIT_USER_EMAIL=dev@example.com\nGIT_USERNAME=octocat\nGIT_USER_NAME='Test User'\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0600); err != nil {
		t.Fatal(err)
	}

	if err := writeGitConfig(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "git", ".gitconfig"))
	if err != nil {
		t.Fatal(err)
	}
	want := "[user]\n    signingkey =\n    email = \"dev@example.com\"\n    username = \"octocat\"\n    name = \"Test User\"\n"
	if string(got) != want {
		t.Fatalf("conteúdo inesperado:\n%s", got)
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
