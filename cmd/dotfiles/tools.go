package main

import (
	"os"
	"os/exec"
)

const lunarVimInstallCommand = "LV_BRANCH='release-1.4/neovim-0.9' bash <(curl -s https://raw.githubusercontent.com/LunarVim/LunarVim/release-1.4/neovim-0.9/utils/installer/install.sh)"

var terminalToolPackages = []string{"neovim", "scc"}
var terminalToolAurPackages = []string{"viddy", "mprocs", "posting", "usql-bin", "proton-pass-cli-bin"}
var terminalToolPacmanPackages = []string{"yazi", "hurl", "glow", "ffmpeg", "mpv", "yt-dlp", "scrcpy", "android-tools", "ncdu", "tealdeer", "hyperfine", "atuin", "zoxide", "starship", "btop", "yq", "jq", "fd", "ripgrep", "fzf", "wl-clipboard", "just", "rate-mirrors", "cmake", "git-delta", "ventoy", "eza", "bat", "wget", "curl"}

func lunarVimInstallStep() step {
	return terminalStep("Instalar LunarVim", "bash", "-c", lunarVimInstallCommand)
}

func optionalMprocsSteps() []step {
	if len(missingPkgs("mprocs")) == 0 {
		return nil
	}
	return []step{
		optionalTerminalStep("Atualizar bases do Pacman", "sudo", "pacman", "-Syy"),
		optionalTerminalStep("Instalar mprocs via Shelly", "bash", "-c", shellyFallbackScript, "dotfiles-shelly-fallback", "aur", "mprocs"),
	}
}

func terminalToolsJobFor(dotfiles, home string, lookPath func(string) (string, error)) job {
	steps := ensureShellyPkgs("standard", terminalToolPackages...)
	for _, pkg := range terminalToolAurPackages {
		if pkg == "mprocs" {
			steps = append(steps, optionalMprocsSteps()...)
		} else {
			steps = append(steps, ensureShellyPkgs("aur", pkg)...)
		}
	}
	steps = append(steps, ensurePkgs(terminalToolPacmanPackages...)...)
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
