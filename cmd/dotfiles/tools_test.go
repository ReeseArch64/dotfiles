package main

import (
	"errors"
	"slices"
	"testing"
)

func TestTerminalToolsIncludeRequestedPackages(t *testing.T) {
	wantShelly := []string{"neovim", "scc"}
	if !slices.Equal(terminalToolPackages, wantShelly) {
		t.Fatalf("pacotes Shelly inesperados: %v", terminalToolPackages)
	}
	wantAur := []string{"viddy", "mprocs", "posting", "usql-bin", "proton-pass-cli-bin"}
	if !slices.Equal(terminalToolAurPackages, wantAur) {
		t.Fatalf("pacotes AUR inesperados: %v", terminalToolAurPackages)
	}
	wantPacman := []string{"yazi", "hurl", "glow", "ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "ncdu", "tealdeer", "hyperfine", "atuin", "zoxide", "starship", "btop", "yq", "jq", "fd", "ripgrep", "fzf", "wl-clipboard", "just", "rate-mirrors", "cmake", "git-delta", "ventoy", "eza", "bat", "wget", "curl"}
	if !slices.Equal(terminalToolPacmanPackages, wantPacman) {
		t.Fatalf("pacotes Pacman inesperados: %v", terminalToolPacmanPackages)
	}
}

func TestTerminalToolsExcludeUnsupportedViddyBinPackage(t *testing.T) {
	if slices.Contains(terminalToolAurPackages, "viddy-bin") {
		t.Fatal("viddy-bin usa uma expansão de array incompatível com o Shelly")
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
	configured := terminalToolsJobFor("/tmp/dotfiles", "/tmp/home", func(string) (string, error) {
		return "", errors.New("ausente")
	})
	if len(configured.steps) == 0 || configured.steps[len(configured.steps)-1].label != "Instalar LunarVim" {
		t.Fatal("instalação do LunarVim ausente")
	}
}

func TestTerminalToolsJobConfiguresFastfetchAndBtop(t *testing.T) {
	configured := terminalToolsJobFor("/tmp/dotfiles", "/tmp/home", func(string) (string, error) {
		return "/home/user/.local/bin/lvim", nil
	})
	want := map[string]bool{
		"Copiar configuração para ~/.config/fastfetch": false,
		"Copiar configuração para ~/.config/btop":      false,
	}
	for _, current := range configured.steps {
		if _, ok := want[current.label]; ok {
			want[current.label] = true
		}
	}
	for label, found := range want {
		if !found {
			t.Fatalf("etapa ausente: %s", label)
		}
	}
}

func TestTerminalToolsJobKeepsExistingLunarVim(t *testing.T) {
	configured := terminalToolsJobFor("/tmp/dotfiles", "/tmp/home", func(string) (string, error) {
		return "/home/user/.local/bin/lvim", nil
	})
	for _, current := range configured.steps {
		if current.label == "Instalar LunarVim" {
			t.Fatal("LunarVim instalado novamente")
		}
	}
}
