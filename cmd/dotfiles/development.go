package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var developmentPacmanPackages = []string{"fish", "rustup", "tk"}

const betterStackInstallCommand = "curl -fsSL https://raw.githubusercontent.com/sounak98/betterstack-cli/main/install.sh | sh"

func developmentToolsInstallStep(dotfiles string) step {
	return terminalStep(
		"Instalar ferramentas configuradas via mise",
		"mise", "--cd", dotfiles, "install",
	)
}

func miseToolLoginStep(home, label, command string, args ...string) step {
	path := filepath.Join(home, ".local", "share", "mise", "shims") + ":" + os.Getenv("PATH")
	argv := []string{"env", "PATH=" + path, command}
	return terminalStep(label, append(argv, args...)...)
}

func cloudLoginSteps(home string) []step {
	return []step{
		miseToolLoginStep(home, "Autenticar na AWS", "aws", "login"),
		miseToolLoginStep(home, "Autenticar no Google Cloud", "gcloud", "auth", "login"),
		miseToolLoginStep(home, "Autenticar no Railway", "railway", "login"),
		miseToolLoginStep(home, "Autenticar no Firebase", "firebase", "login"),
		miseToolLoginStep(home, "Autenticar no Azure", "az", "login"),
	}
}

func npmLoginStep(home string) step {
	return miseToolLoginStep(home, "Autenticar no npm", "npm", "login")
}

func rustupToolchainStep() step {
	return terminalStep("Instalar toolchain Rust estável", "rustup", "default", "stable")
}

func betterStackInstallStep() step {
	return terminalStep("Instalar Better Stack CLI", "sh", "-c", betterStackInstallCommand)
}

func betterStackLoginStep(home string) step {
	path := filepath.Join(home, ".local", "bin") + ":" + os.Getenv("PATH")
	return terminalStep("Autenticar no Better Stack", "env", "PATH="+path, "bs", "auth", "init")
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

func developmentEnvironmentSteps(dotfiles, home string, packageSteps []step) []step {
	steps := miseSetupSteps(dotfiles)
	steps = append(steps, developmentToolsInstallStep(dotfiles))
	steps = append(steps, cloudLoginSteps(home)...)
	steps = append(steps, packageSteps...)
	steps = append(steps, fishConfigStep(dotfiles, home))
	return append(steps,
		rustupToolchainStep(),
		nativeStep("Verificar rustc e cargo", func() error {
			return verifyRustTools(home)
		}),
		betterStackInstallStep(),
		betterStackLoginStep(home),
		npmLoginStep(home),
	)
}

func developmentEnvironmentJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	steps := developmentEnvironmentSteps(dotfiles, home, ensurePkgs(developmentPacmanPackages...))
	return job{
		title: "Configurar Ambiente de Desenvolvimento",
		steps: steps,
	}
}
