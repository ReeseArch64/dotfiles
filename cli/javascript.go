package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func javascriptInstallStep(dotfiles string) step {
	return terminalStep(
		"Instalar Node.js, npm, Yarn, Bun, Deno e pnpm via mise",
		"mise", "--cd", dotfiles, "install", "node", "yarn", "bun", "deno", "pnpm",
	)
}

func installYarnConfig(dotfiles, home string) error {
	source, err := filepath.Abs(filepath.Join(dotfiles, ".yarnrc"))
	if err != nil {
		return fmt.Errorf("resolver .yarnrc: %w", err)
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("ler %s: %w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%s não é um arquivo regular", source)
	}
	destination := filepath.Join(home, ".yarnrc")
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remover %s: %w", destination, err)
	}
	if err := os.Symlink(source, destination); err != nil {
		return fmt.Errorf("criar %s: %w", destination, err)
	}
	return nil
}

func npmLoginStep(home string) step {
	path := filepath.Join(home, ".local", "share", "mise", "shims") + ":" + os.Getenv("PATH")
	return terminalStep("Autenticar no npm", "env", "PATH="+path, "npm", "login")
}

func javascriptJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	steps := miseSetupSteps(dotfiles)
	steps = append(steps,
		javascriptInstallStep(dotfiles),
		nativeStep("Symlink ~/.yarnrc", func() error { return installYarnConfig(dotfiles, home) }),
		npmLoginStep(home),
	)
	return job{
		title: "Instalar ambiente JavaScript",
		steps: steps,
	}
}
