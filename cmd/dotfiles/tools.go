package main

import "os/exec"

const lunarVimInstallCommand = "LV_BRANCH='release-1.4/neovim-0.9' bash <(curl -s https://raw.githubusercontent.com/LunarVim/LunarVim/release-1.4/neovim-0.9/utils/installer/install.sh)"

var terminalToolPackages = []string{"neovim", "wget", "curl", "bat", "eza"}

func lunarVimInstallStep() step {
	return terminalStep("Instalar LunarVim", "bash", "-c", lunarVimInstallCommand)
}

func terminalToolsJobFor(lookPath func(string) (string, error)) job {
	steps := ensureShellyPkgs("standard", terminalToolPackages...)
	if _, err := lookPath("lvim"); err != nil {
		steps = append(steps, lunarVimInstallStep())
	}
	return job{
		title: "Instalar ferramentas de terminal",
		steps: steps,
	}
}

func terminalToolsJob() job {
	return terminalToolsJobFor(exec.LookPath)
}
