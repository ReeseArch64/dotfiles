package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ollamaInstallScript = "curl -fsSL https://ollama.com/install.sh | sh"
	ollamaStorage       = "/mnt/storage"
	ollamaModels        = ollamaStorage + "/ollama/models"
	ollamaDropIn        = "/etc/systemd/system/ollama.service.d/amd.conf"
)

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

func ollamaROCmInstalled(binary string) bool {
	binary, err := filepath.EvalSymlinks(binary)
	if err != nil {
		return false
	}
	runtime := filepath.Join(filepath.Dir(filepath.Dir(binary)), "lib", "ollama", "rocm*")
	for _, pattern := range []string{"libggml-hip.so", "libhipblas.so*"} {
		libraries, _ := filepath.Glob(filepath.Join(runtime, pattern))
		for _, library := range libraries {
			if info, err := os.Stat(library); err == nil && info.Mode().IsRegular() {
				return true
			}
		}
	}
	return false
}

func (installer packageInstaller) setupOllama() error {
	c := installer.console
	fmt.Fprintln(c.output, "Configurar Ollama com ROCm para GPU AMD.")
	fmt.Fprintln(c.output, "Armazenar modelos em", ollamaModels, "e disponibilizar a API em 127.0.0.1:11434.")
	unlock, err := lockOperation(ollamaDropIn, "configuração do Ollama")
	if err != nil {
		return err
	}
	defer unlock()
	if err := validateOllamaAMD("/sys", "/dev"); err != nil {
		return err
	}
	commands := c.commands
	if err := commands.interactive("mountpoint", "-q", ollamaStorage); err != nil {
		return fmt.Errorf("monte %s antes de configurar Ollama: %w", ollamaStorage, err)
	}
	source := cachyosAsset(c.home, "ollama", "amd.conf")
	if err := sshRegular(source, false); err != nil {
		return fmt.Errorf("localizar configuração do Ollama: %w", err)
	}
	if err := c.ensurePackages("curl"); err != nil {
		return err
	}
	if err := installer.ensureOllamaRuntime(); err != nil {
		return err
	}
	for _, args := range [][]string{
		{"install", "-d", "-o", "ollama", "-g", "ollama", "-m", "0750", "--", filepath.Dir(ollamaModels), ollamaModels},
		{"usermod", "-aG", "render,video", "--", "ollama"},
	} {
		if err := commands.interactive(append([]string{"sudo"}, args...)...); err != nil {
			return fmt.Errorf("preparar serviço do Ollama (%s): %w", args[0], err)
		}
	}
	location := managedBackup("Ollama", commands.interactive)
	if err := applyOllamaDropIn(c, source, ollamaDropIn, location); err != nil {
		return err
	}
	if err := restartOllamaService(commands); err != nil {
		return err
	}
	fmt.Fprintln(c.output, "Ollama configurado. Após carregar um modelo, use ollama ps para conferir o uso da GPU.")
	return nil
}

func (installer packageInstaller) ensureOllamaRuntime() error {
	c := installer.console
	commands := c.commands
	hasVendorRuntime := func() bool {
		for _, path := range []string{"/usr/local/bin/ollama", "/usr/bin/ollama"} {
			if binary, err := installer.which(path); err == nil && ollamaROCmInstalled(binary) {
				return true
			}
		}
		return false
	}

	if installer.installed("ollama") {
		if err := c.ensurePackages("ollama-rocm"); err != nil {
			return fmt.Errorf("instalar ollama-rocm: %w", err)
		}
	} else {
		_, serviceErr := commands.capture("", "systemctl", "cat", "ollama.service")
		if serviceErr != nil || !hasVendorRuntime() {
			if err := c.ensurePackages("pciutils", "tar", "zstd", "util-linux"); err != nil {
				return err
			}
			if err := commands.interactive("bash", "-o", "pipefail", "-c", ollamaInstallScript); err != nil {
				return fmt.Errorf("instalar Ollama com ROCm: %w", err)
			}
		}
	}
	if !hasVendorRuntime() {
		return errors.New("bibliotecas ROCm do Ollama ausentes após a instalação")
	}
	return nil
}

func applyOllamaDropIn(c *sshConsole, source, destination string, location backupLocation) error {
	if err := validateBackupPath(filepath.Dir(destination)); err != nil {
		return err
	}
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	info, err := os.Lstat(destination)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("configuração do Ollama requer arquivo regular: %s", destination)
		}
		previous, err := os.ReadFile(destination)
		if err != nil {
			return err
		}
		if bytes.Equal(previous, data) && info.Mode().Perm() == 0644 {
			return nil
		}
		backup, err := location.saveFile("amd.conf.backup-*", previous)
		if err != nil {
			return err
		}
		fmt.Fprintln(c.output, "Backup do Ollama em", backup)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := c.commands.interactive("sudo", "install", "-d", "-m", "0755", "--", filepath.Dir(destination)); err != nil {
		return err
	}
	return c.commands.interactive("sudo", "--", "sh", "-c", installRootConfigAtomically, "dotfiles-ollama",
		source, destination, "0644", "root")
}

func restartOllamaService(commands sshCommands) error {
	for _, args := range [][]string{
		{"sudo", "systemctl", "daemon-reload"},
		{"sudo", "systemctl", "enable", "ollama.service"},
		{"sudo", "systemctl", "restart", "ollama.service"},
		{"systemctl", "is-active", "--quiet", "ollama.service"},
		{"curl", "--fail", "--silent", "--show-error", "--retry", "10", "--retry-connrefused",
			"--retry-delay", "1", "--max-time", "5", "http://127.0.0.1:11434/api/version"},
	} {
		if err := commands.interactive(args...); err != nil {
			return fmt.Errorf("configurar ou verificar o serviço do Ollama (%s): %w", strings.Join(args, " "), err)
		}
	}
	return nil
}
