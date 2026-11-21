package main

import (
	"slices"
	"testing"
)

func TestThunderbirdJobUsesDedicatedPacmanPackage(t *testing.T) {
	install := nativeStep("Instalar thunderbird", func() error { return nil })
	configured := thunderbirdJobFor([]step{install})
	if thunderbirdPackage != "thunderbird" {
		t.Fatalf("pacote inesperado: %s", thunderbirdPackage)
	}
	if configured.title != "Instalar Thunderbird" {
		t.Fatalf("título inesperado: %s", configured.title)
	}
	if got := jobStepLabels(configured); !slices.Equal(got, []string{"Instalar thunderbird"}) {
		t.Fatalf("passos inesperados: %v", got)
	}
}

func TestDesktopMenuKeepsThunderbirdSeparateFromTerminalTools(t *testing.T) {
	model := newModel(t.TempDir())
	model.screen = screenDesktop
	for _, current := range model.items() {
		if current.title == "Thunderbird" {
			if current.job == nil {
				t.Fatal("ação Thunderbird não inicia um job")
			}
			if configured := current.job(); configured.title != "Instalar Thunderbird" {
				t.Fatalf("job inesperado: %s", configured.title)
			}
			return
		}
	}
	t.Fatal("ação Thunderbird ausente do menu Desktop")
}
