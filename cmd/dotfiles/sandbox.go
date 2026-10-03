package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var sandboxPacmanPackages = []string{"podman", "podman-compose", "distrobox"}

const sandboxAurPackage = "podman-tui-bin"
const distroboxPodmanSetting = `container_manager="podman"`

func configureDistroboxForPodman(home string) error {
	destination := filepath.Join(home, ".config", "distrobox", "distrobox.conf")
	content, err := os.ReadFile(destination)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("ler %s: %w", destination, err)
	}
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
	updated := make([]string, 0, len(lines)+1)
	found := false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "container_manager=") {
			if !found {
				updated = append(updated, distroboxPodmanSetting)
				found = true
			}
			continue
		}
		if line != "" || len(content) != 0 {
			updated = append(updated, line)
		}
	}
	if !found {
		updated = append(updated, distroboxPodmanSetting)
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0755); err != nil {
		return fmt.Errorf("criar %s: %w", filepath.Dir(destination), err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".distrobox-conf-")
	if err != nil {
		return fmt.Errorf("criar arquivo temporário: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(strings.Join(updated, "\n") + "\n"); err != nil {
		temporary.Close()
		return fmt.Errorf("gravar %s: %w", temporaryPath, err)
	}
	if err := temporary.Chmod(0644); err != nil {
		temporary.Close()
		return fmt.Errorf("ajustar permissões de %s: %w", temporaryPath, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("fechar %s: %w", temporaryPath, err)
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return fmt.Errorf("substituir %s: %w", destination, err)
	}
	return nil
}

func createSandboxStep() step {
	return terminalStep("Criar Distrobox sandbox com Arch Linux", "distrobox", "create", "--name", "sandbox", "--image", "archlinux")
}

func sandboxEnvironmentJobFor(home string, pacmanSteps, aurSteps []step) job {
	steps := append([]step{}, pacmanSteps...)
	steps = append(steps, aurSteps...)
	steps = append(steps,
		nativeStep("Configurar Distrobox para usar Podman", func() error {
			return configureDistroboxForPodman(home)
		}),
		createSandboxStep(),
	)
	return job{
		title: "Configurar Ambiente Sandbox",
		steps: steps,
	}
}

func sandboxEnvironmentJob() job {
	home, _ := os.UserHomeDir()
	return sandboxEnvironmentJobFor(
		home,
		ensurePkgs(sandboxPacmanPackages...),
		ensureShellyPkgs("aur", sandboxAurPackage),
	)
}
