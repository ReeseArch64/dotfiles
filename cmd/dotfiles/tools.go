package main

import (
	"os"
	"os/exec"
)

const lunarVimInstallCommand = "LV_BRANCH='release-1.4/neovim-0.9' bash <(curl -s https://raw.githubusercontent.com/LunarVim/LunarVim/release-1.4/neovim-0.9/utils/installer/install.sh)"
const mprocsInstallCommand = `curl -fsSL https://dekit.run/install.sh | sh && printf '%s\n' '#!/bin/sh' 'exec "$(dirname "$0")/dekit" mprocs "$@"' > "$HOME/.local/bin/mprocs" && chmod 755 "$HOME/.local/bin/mprocs"`

var terminalToolPackages = []string{"neovim", "scc"}
var terminalToolAurPackages = []string{"viddy", "usql-bin", "proton-pass-cli-bin"}
var terminalToolPacmanPackages = []string{"yazi", "hurl", "glow", "ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "ncdu", "tealdeer", "hyperfine", "atuin", "zoxide", "starship", "btop", "chafa", "yq", "jq", "fd", "ripgrep", "fzf", "wl-clipboard", "just", "rate-mirrors", "cmake", "git-delta", "ventoy", "eza", "bat", "wget", "curl", "uv"}

func lunarVimInstallStep() step {
	return terminalStep("Instalar LunarVim", "bash", "-c", lunarVimInstallCommand)
}

func terminalToolsJobFor(dotfiles, home string, lookPath func(string) (string, error)) job {
	steps := ensureShellyPkgs("standard", terminalToolPackages...)
	steps = append(steps, ensureShellyPkgs("aur", terminalToolAurPackages...)...)
	steps = append(steps, ensurePkgs(terminalToolPacmanPackages...)...)
	if _, err := lookPath("mprocs"); err != nil {
		steps = append(steps, terminalStep("Instalar mprocs via Dekit", "bash", "-c", mprocsInstallCommand))
	}
	if _, err := lookPath("posting"); err != nil {
		steps = append(steps, terminalStep("Instalar Posting via uv", "uv", "tool", "install", "--python", "3.13", "posting"))
	}
	steps = append(steps, fastfetchConfigStep(dotfiles, home))
	steps = append(steps, btopConfigStep(dotfiles, home))
	if _, err := lookPath("lvim"); err != nil {
		steps = append(steps, lunarVimInstallStep())
	}
	return job{
		title: "Instalar ferramentas de terminal",
		steps: steps,
	}
}

func terminalToolsJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	return terminalToolsJobFor(dotfiles, home, exec.LookPath)
}
