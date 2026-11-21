package main

import (
	"slices"
	"testing"
)

func TestAndroidCLIJob(t *testing.T) {
	current := androidCLIJob()
	if current.title != "Instalar Android CLI" || len(current.steps) != 1 {
		t.Fatalf("job inesperado: %v", current)
	}
	want := []string{"bash", "-o", "pipefail", "-c", "curl -fsSL https://dl.google.com/android/cli/latest/linux_x86_64/install.sh | bash"}
	if got := commandTail(t, current.steps[0], len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando de instalação inesperado: %v", got)
	}
}
