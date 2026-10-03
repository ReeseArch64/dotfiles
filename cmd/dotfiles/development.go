package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func developmentToolsInstallStep(dotfiles string) step {
	return terminalStep(
		"Instalar ferramentas configuradas via mise",
		"mise", "--cd", dotfiles, "install",
	)
}

func npmLoginStep(home string) step {
	path := filepath.Join(home, ".local", "share", "mise", "shims") + ":" + os.Getenv("PATH")
	return terminalStep("Autenticar no npm", "env", "PATH="+path, "npm", "login")
}

func rustupToolchainStep() step {
	return terminalStep("Instalar toolchain Rust estável", "rustup", "default", "stable")
}

func rustToolPath(home, name string) (string, error) {
	cargoPath := filepath.Join(home, ".cargo", "bin", name)
	if info, err := os.Stat(cargoPath); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0111 != 0 {
		return cargoPath, nil
	}
	path, err := exec.LookPath(name)
	if err != nil {
		return "", fmt.Errorf("%s não encontrado após instalar o Rust", name)
	}
	return path, nil
}

func verifyRustTools(home string) error {
	for _, name := range []string{"rustc", "cargo"} {
		path, err := rustToolPath(home, name)
		if err != nil {
			return err
		}
		if output, err := exec.Command(path, "--version").CombinedOutput(); err != nil {
			return fmt.Errorf("validar %s: %s: %w", name, string(output), err)
		}
	}
	return nil
}

func developmentEnvironmentSteps(dotfiles, home string, rustupSteps []step) []step {
	steps := miseSetupSteps(dotfiles)
	steps = append(steps, developmentToolsInstallStep(dotfiles))
	steps = append(steps, rustupSteps...)
	return append(steps,
		rustupToolchainStep(),
		nativeStep("Verificar rustc e cargo", func() error {
			return verifyRustTools(home)
		}),
		npmLoginStep(home),
	)
}

func developmentEnvironmentJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	steps := developmentEnvironmentSteps(dotfiles, home, ensurePkgs("rustup"))
	return job{
		title: "Configurar Ambiente de Desenvolvimento",
		steps: steps,
	}
}
