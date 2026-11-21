package main

import (
	"slices"
	"testing"
)

func TestIdesJobIncludesAndroidStudio(t *testing.T) {
	current := idesJob()
	if current.title != "Instalar IDEs" {
		t.Fatalf("título inesperado: %s", current.title)
	}
	if len(missingPkgs("android-studio")) == 0 {
		t.Skip("Android Studio já instalado; a CLI omite a instalação")
	}
	for _, step := range current.steps {
		if step.cmd == nil {
			continue
		}
		want := []string{"bash", "-c", yayFallbackScript, "dotfiles-yay-fallback", "android-studio"}
		args := step.cmd().Args
		if len(args) >= len(want) && slices.Equal(args[len(args)-len(want):], want) {
			return
		}
	}
	t.Fatal("instalação do Android Studio via yay/AUR ausente")
}
