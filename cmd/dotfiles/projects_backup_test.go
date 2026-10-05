package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestFindProjectRepositoriesMapsEntireTree(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "alpha")
	second := filepath.Join(root, "group", "beta")
	for _, repository := range []string{first, second} {
		if err := os.MkdirAll(filepath.Join(repository, ".git"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(filepath.Join(first, ".git", "nested", ".git"), 0700); err != nil {
		t.Fatal(err)
	}

	got, err := findProjectRepositories(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{first, second}
	if !slices.Equal(got, want) {
		t.Fatalf("projetos inesperados: %v", got)
	}
}

func TestBackupProjectRepositoriesPushesGitHubBeforeGitLab(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"zeta", "alpha"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".git"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	var calls []string
	count, err := backupProjectRepositories(context.Background(), root, func(_ context.Context, repository string, args ...string) error {
		calls = append(calls, filepath.Base(repository)+" "+strings.Join(args, " "))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"alpha push origin", "alpha push backup", "zeta push origin", "zeta push backup"}
	if count != 2 || !slices.Equal(calls, want) {
		t.Fatalf("backup inesperado: count=%d calls=%v", count, calls)
	}
}

func TestBackupProjectRepositoriesContinuesAfterFailure(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"alpha", "beta"} {
		if err := os.MkdirAll(filepath.Join(root, name, ".git"), 0700); err != nil {
			t.Fatal(err)
		}
	}
	var calls []string
	count, err := backupProjectRepositories(context.Background(), root, func(_ context.Context, repository string, args ...string) error {
		call := filepath.Base(repository) + " " + strings.Join(args, " ")
		calls = append(calls, call)
		if call == "alpha push origin" {
			return errors.New("offline")
		}
		return nil
	})
	wantCalls := []string{"alpha push origin", "beta push origin", "beta push backup"}
	if count != 1 || err == nil || !strings.Contains(err.Error(), "alpha -> GitHub") {
		t.Fatalf("falha agregada inesperada: count=%d err=%v", count, err)
	}
	if !slices.Equal(calls, wantCalls) {
		t.Fatalf("comandos inesperados: %v", calls)
	}
}

func TestBackupProjectRepositoriesRejectsEmptyTree(t *testing.T) {
	_, err := backupProjectRepositories(context.Background(), t.TempDir(), func(context.Context, string, ...string) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "nenhum projeto Git") {
		t.Fatalf("erro inesperado: %v", err)
	}
}

func TestEnsureProjectsLinkCreatesAndPreservesLink(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(t.TempDir(), "workspaces")
	if err := ensureProjectsLink(home, target); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, ".projects")
	if got, err := os.Readlink(link); err != nil || got != target {
		t.Fatalf("link inesperado: %q err=%v", got, err)
	}
	if err := ensureProjectsLink(home, target); err != nil {
		t.Fatalf("segunda execução falhou: %v", err)
	}
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(link, []byte("preservar"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ensureProjectsLink(home, target); err == nil || !strings.Contains(err.Error(), "não é um link simbólico") {
		t.Fatalf("destino existente deveria ser preservado: %v", err)
	}
}

func TestProjectsWorkspaceSetupUsesSudoInstallArguments(t *testing.T) {
	want := []string{"install", "-d", "-m", "0700", "-o", "1000", "-g", "1001", "/mnt/workspaces"}
	if got := projectsWorkspaceSetupArgs("/mnt/workspaces", 1000, 1001); !slices.Equal(got, want) {
		t.Fatalf("argumentos inesperados: %v", got)
	}
}

func TestProjectsBackupJobOrdersSetupBeforePushes(t *testing.T) {
	setup := nativeStep("Preparar /mnt/workspaces", func() error { return nil })
	configured := projectsBackupJobFor(
		[]step{setup},
		func() error { return nil },
		func() error { return nil },
		func() (int, error) { return 2, nil },
	)
	want := []string{
		"Preparar /mnt/workspaces",
		"Criar /mnt/workspaces/reesearch64",
		"Criar link ~/.projects",
		"Enviar projetos ao GitHub e GitLab",
	}
	if labels := jobStepLabels(configured); !slices.Equal(labels, want) {
		t.Fatalf("passos inesperados: %v", labels)
	}
}
