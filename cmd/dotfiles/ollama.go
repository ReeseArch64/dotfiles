package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var ollamaDependencies = []string{"curl", "pciutils", "tar", "zstd"}

const ollamaInstallScript = "curl -fsSL https://ollama.com/install.sh | sh"

func validateOllamaAMD(sysDir, devDir string) error {
	devices, err := filepath.Glob(filepath.Join(sysDir, "class", "drm", "renderD*", "device"))
	if err != nil {
		return err
	}
	found := false
	for _, device := range devices {
		vendor, err := os.ReadFile(filepath.Join(device, "vendor"))
		if err != nil || strings.TrimSpace(string(vendor)) != "0x1002" {
			continue
		}
		driver, err := filepath.EvalSymlinks(filepath.Join(device, "driver"))
		if err == nil && filepath.Base(driver) == "amdgpu" {
			found = true
			break
		}
	}
	if !found {
		return errors.New("GPU AMD com driver amdgpu não detectada; configure o driver antes de instalar Ollama")
	}
	if _, err := os.Stat(filepath.Join(devDir, "kfd")); err != nil {
		return fmt.Errorf("ROCm exige /dev/kfd: %w", err)
	}
	return nil
}

func ollamaROCmInstalled(lookPath func(string) (string, error)) bool {
	binary, err := lookPath("ollama")
	if err != nil {
		return false
	}
	binary, err = filepath.EvalSymlinks(binary)
	if err != nil {
		return false
	}
	libraries, _ := filepath.Glob(filepath.Join(filepath.Dir(filepath.Dir(binary)), "lib", "ollama", "rocm", "libhipblas.so*"))
	for _, library := range libraries {
		if info, err := os.Stat(library); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func ollamaJobFor(dotfiles string, packageSteps []step, lookPath func(string) (string, error)) job {
	steps := []step{nativeStep("Validar GPU AMD e dispositivo ROCm", func() error {
		return validateOllamaAMD("/sys", "/dev")
	})}
	steps = append(steps, packageSteps...)
	steps = append(steps,
		skipWhen(terminalStep("Instalar Ollama e ROCm via curl", "bash", "-o", "pipefail", "-c", ollamaInstallScript), func() bool {
			return ollamaROCmInstalled(lookPath) && commandSucceeds(nil, "systemctl", "cat", "ollama.service")
		}),
		nativeStep("Verificar instalação do runtime ROCm", func() error {
			if !ollamaROCmInstalled(lookPath) {
				return errors.New("runtime ROCm do Ollama ausente; confira a saída do instalador oficial")
			}
			return nil
		}),
		terminalStep("Permitir acesso do serviço à GPU AMD", "sudo", "usermod", "-aG", "render,video", "ollama"),
		terminalStep("Configurar serviço Ollama para GPU AMD", "sudo", "install", "-Dm644",
			configPath(dotfiles, "ollama", "amd.conf"), "/etc/systemd/system/ollama.service.d/amd.conf"),
		terminalStep("Recarregar unidades systemd", "sudo", "systemctl", "daemon-reload"),
		terminalStep("Habilitar Ollama no boot", "sudo", "systemctl", "enable", "ollama.service"),
		terminalStep("Reiniciar Ollama com acesso à GPU", "sudo", "systemctl", "restart", "ollama.service"),
		terminalStep("Verificar serviço Ollama", "systemctl", "is-active", "--quiet", "ollama.service"),
		terminalStep("Verificar API local do Ollama", "curl", "--fail", "--silent", "--show-error", "--retry", "10",
			"--retry-connrefused", "--retry-delay", "1", "--max-time", "5", "http://127.0.0.1:11434/api/version"),
	)
	return job{
		title: "Configurar Ollama (AMD)",
		steps: steps,
		result: func() string {
			return "Ollama configurado com runtime ROCm e API local em 127.0.0.1:11434.\nExecute um modelo e use ollama ps para conferir o uso da GPU."
		},
	}
}

func ollamaJob(dotfiles string) job {
	return ollamaJobFor(dotfiles, ensurePkgs(ollamaDependencies...), exec.LookPath)
}
