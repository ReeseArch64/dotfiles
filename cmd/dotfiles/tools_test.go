package main

import (
	"errors"
	"slices"
	"testing"
)

func TestTerminalToolsIncludeRequestedPackages(t *testing.T) {
	want := []string{"neovim", "wget", "curl", "bat", "eza"}
	if !slices.Equal(terminalToolPackages, want) {
		t.Fatalf("pacotes inesperados: %v", terminalToolPackages)
	}
}

func TestLunarVimInstallStepUsesReleaseBranch(t *testing.T) {
	current := lunarVimInstallStep()
	want := []string{"bash", "-c", lunarVimInstallCommand}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando LunarVim inesperado: %v", got)
	}
}

func TestTerminalToolsJobInstallsMissingLunarVim(t *testing.T) {
	configured := terminalToolsJobFor(func(string) (string, error) {
		return "", errors.New("ausente")
	})
	if len(configured.steps) == 0 || configured.steps[len(configured.steps)-1].label != "Instalar LunarVim" {
		t.Fatal("instalação do LunarVim ausente")
	}
}

func TestTerminalToolsJobKeepsExistingLunarVim(t *testing.T) {
	configured := terminalToolsJobFor(func(string) (string, error) {
		return "/home/user/.local/bin/lvim", nil
	})
	for _, current := range configured.steps {
		if current.label == "Instalar LunarVim" {
			t.Fatal("LunarVim instalado novamente")
		}
	}
}
