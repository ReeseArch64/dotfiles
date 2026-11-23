package main

import (
	"slices"
	"testing"
)

func TestZedJobInstallsZedViaYay(t *testing.T) {
	current := zedJob()
	if current.title != "Instalar Zed" {
		t.Fatalf("título inesperado: %s", current.title)
	}
	if len(missingPkgs("zed")) == 0 {
		if len(current.steps) != 0 {
			t.Fatal("Zed já instalado deveria ser ignorado")
		}
		return
	}
	for _, step := range current.steps {
		if step.cmd == nil {
			continue
		}
		want := []string{"bash", "-c", yayFallbackScript, "dotfiles-yay-fallback", "zed"}
		args := step.cmd().Args
		if len(args) >= len(want) && slices.Equal(args[len(args)-len(want):], want) {
			return
		}
	}
	t.Fatal("instalação do Zed via yay ausente")
}
