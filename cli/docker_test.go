package main

import (
	"slices"
	"testing"
)

func TestDockerPackages(t *testing.T) {
	want := []string{"docker", "docker-compose", "lazydocker", "docker-buildx", "kind", "util-linux"}
	if packages := dockerRequiredPackages(); !slices.Equal(packages, want) {
		t.Fatalf("pacotes inesperados: %v", packages)
	}
}

func TestDockerJobCommands(t *testing.T) {
	job := dockerJobForUser("dev", nil)
	if len(job.steps) != 3 {
		t.Fatalf("quantidade inesperada de passos: %d", len(job.steps))
	}
	want := [][]string{
		{"sudo", "usermod", "-aG", "docker", "dev"},
		{"sudo", "systemctl", "enable", "--now", "docker.service"},
		{"newgrp", "docker", "-c", "docker login"},
	}
	for i, expected := range want {
		if job.steps[i].cmd == nil {
			t.Fatalf("passo %d não é interativo", i)
		}
		args := job.steps[i].cmd().Args
		if len(args) < len(expected) {
			t.Fatalf("comando %d incompleto: %v", i, args)
		}
		actual := args[len(args)-len(expected):]
		if !slices.Equal(actual, expected) {
			t.Fatalf("comando %d inesperado: %v", i, actual)
		}
	}
}

func TestDockerJobKeepsPackageStepsFirst(t *testing.T) {
	install := nativeStep("Instalar pacotes Docker", func() error { return nil })
	job := dockerJobForUser("dev", []step{install})
	if len(job.steps) != 4 {
		t.Fatalf("quantidade inesperada de passos: %d", len(job.steps))
	}
	if job.steps[0].label != install.label {
		t.Fatalf("primeiro passo inesperado: %s", job.steps[0].label)
	}
}
