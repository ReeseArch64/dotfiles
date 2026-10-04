package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestDockerCredentialsSkipLoginAndBrowserRequirement(t *testing.T) {
	home := t.TempDir()
	if dockerHasCredentials(home) {
		t.Fatal("credencial inexistente")
	}
	if err := os.MkdirAll(filepath.Join(home, ".docker"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".docker", "config.json"), []byte(`{"auths":{"private.example.com":{"auth":"secret"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	if dockerHasCredentials(home) {
		t.Fatal("credenciais de outro registro não autenticam no Docker Hub")
	}
	if err := os.WriteFile(filepath.Join(home, ".docker", "config.json"), []byte(`{"auths":{"https://index.docker.io/v1/":{"auth":"secret"}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	steps := dockerJobForUser("dev", home, nil, func(string) (string, error) { return "", errors.New("ausente") }).steps
	if !steps[0].skip() || !steps[len(steps)-1].skip() {
		t.Fatal("login existente não foi ignorado")
	}
}

func TestDockerPackages(t *testing.T) {
	want := []string{"docker", "docker-compose", "lazydocker", "docker-buildx", "util-linux", "xdg-utils"}
	if packages := dockerRequiredPackages(); !slices.Equal(packages, want) {
		t.Fatalf("pacotes inesperados: %v", packages)
	}
}

func dockerBrowserTest(t *testing.T) (string, func(string) (string, error)) {
	t.Helper()
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".config", "zen"), 0755); err != nil {
		t.Fatal(err)
	}
	return home, func(name string) (string, error) {
		if name != "zen-browser" {
			t.Fatalf("executável inesperado: %s", name)
		}
		return "/usr/bin/zen-browser", nil
	}
}

func TestValidateDockerLoginBrowserRequiresExecutableAndProfile(t *testing.T) {
	home := t.TempDir()
	lookPath := func(string) (string, error) { return "", errors.New("não encontrado") }

	err := validateDockerLoginBrowser(home, lookPath)
	if err == nil {
		t.Fatal("esperava bloqueio sem Zen Browser")
	}
	for _, requirement := range []string{"zen-browser", filepath.Join(home, ".config", "zen")} {
		if !strings.Contains(err.Error(), requirement) {
			t.Fatalf("erro não menciona %s: %v", requirement, err)
		}
	}
}

func TestDockerJobValidatesBrowserAndSetsDefaultBeforeLogin(t *testing.T) {
	home, lookPath := dockerBrowserTest(t)
	job := dockerJobForUser("dev", home, nil, lookPath)
	if len(job.steps) != 5 {
		t.Fatalf("quantidade inesperada de passos: %d", len(job.steps))
	}
	if job.steps[0].run == nil || job.steps[0].label != "Validar Zen Browser para Docker login" {
		t.Fatalf("validação inicial inesperada: %#v", job.steps[0])
	}
	want := [][]string{
		{"sudo", "usermod", "-aG", "docker", "dev"},
		{"sudo", "systemctl", "enable", "--now", "docker.service"},
		{"xdg-settings", "set", "default-web-browser", "zen.desktop"},
		{"newgrp", "docker", "-c", "docker login"},
	}
	for i, expected := range want {
		stepIndex := i + 1
		if job.steps[stepIndex].cmd == nil {
			t.Fatalf("passo %d não é interativo", stepIndex)
		}
		args := job.steps[stepIndex].cmd().Args
		actual := args[len(args)-len(expected):]
		if !slices.Equal(actual, expected) {
			t.Fatalf("comando %d inesperado: %v", stepIndex, actual)
		}
	}
}

func TestDockerJobValidatesBrowserBeforePackageSteps(t *testing.T) {
	home, lookPath := dockerBrowserTest(t)
	install := nativeStep("Instalar pacotes Docker", func() error { return nil })
	job := dockerJobForUser("dev", home, []step{install}, lookPath)
	if len(job.steps) != 6 {
		t.Fatalf("quantidade inesperada de passos: %d", len(job.steps))
	}
	if job.steps[0].label != "Validar Zen Browser para Docker login" || job.steps[1].label != install.label {
		t.Fatalf("ordem inicial inesperada: %s, %s", job.steps[0].label, job.steps[1].label)
	}
}
