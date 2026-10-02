package main

import (
	"errors"
	"slices"
	"testing"
)

func TestTerminalToolsIncludeRequestedPackages(t *testing.T) {
	wantShelly := []string{"neovim", "wget", "curl", "bat", "eza", "scc"}
	if !slices.Equal(terminalToolPackages, wantShelly) {
		t.Fatalf("pacotes Shelly inesperados: %v", terminalToolPackages)
	}
	wantAur := []string{"viddy-bin"}
	if !slices.Equal(terminalToolAurPackages, wantAur) {
		t.Fatalf("pacotes AUR inesperados: %v", terminalToolAurPackages)
	}
	wantPacman := []string{"yazi", "hurl", "glow", "ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "ncdu", "tealdeer", "hyperfine", "atuin", "zoxide", "starship"}
	if !slices.Equal(terminalToolPacmanPackages, wantPacman) {
		t.Fatalf("pacotes Pacman inesperados: %v", terminalToolPacmanPackages)
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
