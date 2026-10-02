package main

import (
	"os"
	"path/filepath"
)

func javascriptInstallStep(dotfiles string) step {
	return terminalStep(
		"Instalar Node.js, npm, Yarn, Bun, Deno e pnpm via mise",
		"mise", "--cd", dotfiles, "install", "node", "yarn", "bun", "deno", "pnpm",
	)
}

func npmLoginStep(home string) step {
	path := filepath.Join(home, ".local", "share", "mise", "shims") + ":" + os.Getenv("PATH")
	return terminalStep("Autenticar no npm", "env", "PATH="+path, "npm", "login")
}

func javascriptJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	steps := miseSetupSteps(dotfiles)
	steps = append(steps, javascriptInstallStep(dotfiles), npmLoginStep(home))
	return job{
		title: "Instalar ambiente JavaScript",
		steps: steps,
	}
}
