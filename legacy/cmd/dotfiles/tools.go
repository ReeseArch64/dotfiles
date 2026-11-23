package main

import (
	"os"
	"os/exec"
	"slices"
)

const lunarVimInstallCommand = "LV_BRANCH='release-1.4/neovim-0.9' bash <(curl -s https://raw.githubusercontent.com/LunarVim/LunarVim/release-1.4/neovim-0.9/utils/installer/install.sh)"
const mprocsInstallCommand = `curl -fsSL https://dekit.run/install.sh | sh && printf '%s\n' '#!/bin/sh' 'exec "$(dirname "$0")/dekit" mprocs "$@"' > "$HOME/.local/bin/mprocs" && chmod 755 "$HOME/.local/bin/mprocs"`

var terminalToolPackages = []string{"neovim", "scc"}
var terminalToolAurPackages = []string{"viddy", "usql-bin", "proton-vpn-cli", "proton-pass-cli-bin"}
var terminalToolPacmanPackages = []string{"yazi", "sshfs", "hurl", "glow", "ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "ncdu", "tealdeer", "hyperfine", "atuin", "zoxide", "starship", "chafa", "yq", "jq", "fd", "ripgrep", "fzf", "wl-clipboard", "just", "rate-mirrors", "cmake", "ninja", "git-delta", "ventoy", "eza", "bat", "wget", "curl", "uv"}

var migratedDesktopTerminalPackages = []string{
	"yazi", "sshfs", "glow", "ncdu", "tealdeer", "atuin", "zoxide", "starship",
	"chafa", "yq", "jq", "fd", "ripgrep", "fzf", "wl-clipboard", "rate-mirrors",
	"git-delta", "eza", "bat", "wget",
}

func lunarVimInstallStep() step {
	return terminalStep("Instalar LunarVim", "bash", "-c", lunarVimInstallCommand)
}

func terminalToolsJobFor(setupDir, home string, lookPath func(string) (string, error), device deviceProfile) job {
	_, lunarVimErr := lookPath("lvim")
	editors, aur, official := terminalToolPackages, terminalToolAurPackages, terminalToolPacmanPackages
	if device == deviceDesktop {
		editors = []string{"scc"}
		if lunarVimErr != nil {
			editors = append([]string{"neovim"}, editors...)
		}
		aur = []string{"usql-bin", "proton-vpn-cli"}
		official = slices.DeleteFunc(slices.Clone(official), func(name string) bool {
			return slices.Contains(migratedDesktopTerminalPackages, name)
		})
	}
	steps := ensureYayPkgs(editors...)
	steps = append(steps, ensureYayPkgs(aur...)...)
	steps = append(steps, ensurePkgs(official...)...)
	if _, err := lookPath("mprocs"); err != nil {
		steps = append(steps, terminalStep("Instalar mprocs via Dekit", "bash", "-c", mprocsInstallCommand))
	}
	if _, err := lookPath("posting"); err != nil {
		steps = append(steps, terminalStep("Instalar Posting via uv", "uv", "tool", "install", "--python", "3.13", "posting"))
	}
	if device == deviceNotebook {
		steps = append(steps, fastfetchConfigStep(setupDir, home))
	}
	if lunarVimErr != nil {
		steps = append(steps, lunarVimInstallStep())
	}
	return job{
		title: "Instalar ferramentas de terminal",
		steps: steps,
	}
}

func terminalToolsJob(setupDir string, device deviceProfile) job {
	home, _ := os.UserHomeDir()
	return terminalToolsJobFor(setupDir, home, exec.LookPath, device)
}
