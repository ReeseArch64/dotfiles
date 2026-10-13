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
	for _, step := range current.steps {
		if step.cmd == nil {
			continue
		}
		want := []string{"bash", "-c", shellyFallbackScript, "dotfiles-shelly-fallback", "aur", "android-studio"}
		args := step.cmd().Args
		if len(args) >= len(want) && slices.Equal(args[len(args)-len(want):], want) {
			return
		}
	}
	t.Fatal("instalação do Android Studio via Shelly/AUR ausente")
}
