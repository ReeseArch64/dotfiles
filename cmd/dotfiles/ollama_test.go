package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func prepareOllamaAMD(t *testing.T, vendor, driver string) (string, string) {
	t.Helper()
	root := t.TempDir()
	sysDir, devDir := filepath.Join(root, "sys"), filepath.Join(root, "dev")
	device := filepath.Join(sysDir, "class", "drm", "renderD128", "device")
	driverDir := filepath.Join(sysDir, "bus", "pci", "drivers", driver)
	for _, dir := range []string{device, driverDir, devDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(device, "vendor"), []byte(vendor+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(driverDir, filepath.Join(device, "driver")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devDir, "kfd"), nil, 0600); err != nil {
		t.Fatal(err)
	}
	return sysDir, devDir
}

func TestValidateOllamaAMD(t *testing.T) {
	for _, tc := range []struct {
		name, vendor, driver string
		missingKFD, wantErr  bool
	}{
		{name: "AMD ROCm", vendor: "0x1002", driver: "amdgpu"},
		{name: "outra GPU", vendor: "0x10de", driver: "nvidia", wantErr: true},
		{name: "driver incorreto", vendor: "0x1002", driver: "radeon", wantErr: true},
		{name: "sem KFD", vendor: "0x1002", driver: "amdgpu", missingKFD: true, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sysDir, devDir := prepareOllamaAMD(t, tc.vendor, tc.driver)
			if tc.missingKFD {
				if err := os.Remove(filepath.Join(devDir, "kfd")); err != nil {
					t.Fatal(err)
				}
			}
			if err := validateOllamaAMD(sysDir, devDir); (err != nil) != tc.wantErr {
				t.Fatalf("erro inesperado: %v", err)
			}
		})
	}
	if err := validateOllamaAMD(t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("GPU ausente aceita")
	}
}

func TestOllamaROCmInstalled(t *testing.T) {
	root := t.TempDir()
	binary := filepath.Join(root, "bin", "ollama")
	if err := os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(binary, nil, 0755); err != nil {
		t.Fatal(err)
	}
	lookPath := func(string) (string, error) { return binary, nil }
	if ollamaROCmInstalled(lookPath) {
		t.Fatal("instalação somente CPU aceita")
	}
	library := filepath.Join(root, "lib", "ollama", "rocm", "libhipblas.so.3")
	if err := os.MkdirAll(filepath.Dir(library), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(library, nil, 0644); err != nil {
		t.Fatal(err)
	}
	if !ollamaROCmInstalled(lookPath) {
		t.Fatal("runtime ROCm não detectado")
	}
	link := filepath.Join(t.TempDir(), "ollama")
	if err := os.Symlink(binary, link); err != nil {
		t.Fatal(err)
	}
	if !ollamaROCmInstalled(func(string) (string, error) { return link, nil }) {
		t.Fatal("runtime de binário com link não detectado")
	}
	if ollamaROCmInstalled(func(string) (string, error) { return "", errors.New("ausente") }) {
		t.Fatal("binário ausente aceito")
	}
}

func TestOllamaInstallerPropagatesCurlFailure(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "curl"), []byte("#!/bin/sh\nexit 22\n"), 0755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", "-o", "pipefail", "-c", ollamaInstallScript)
	cmd.Env = append(os.Environ(), "PATH="+bin+":"+os.Getenv("PATH"))
	if err := cmd.Run(); err == nil {
		t.Fatal("falha do curl ignorada pelo pipeline")
	}
}

func TestOllamaJobCommandsAndMissingRuntime(t *testing.T) {
	configured := ollamaJobFor("/dotfiles", nil, func(string) (string, error) { return "", errors.New("ausente") })
	if configured.steps[1].skip() {
		t.Fatal("instalação ausente ignorada")
	}
	if _, err := configured.steps[2].run(); err == nil {
		t.Fatal("runtime ausente aceito")
	}
	want := [][]string{
		{"bash", "-o", "pipefail", "-c", ollamaInstallScript},
		{"sudo", "usermod", "-aG", "render,video", "ollama"},
		{"sudo", "install", "-Dm644", "/dotfiles/configs/ollama/amd.conf", "/etc/systemd/system/ollama.service.d/amd.conf"},
		{"sudo", "systemctl", "daemon-reload"},
		{"sudo", "systemctl", "enable", "ollama.service"},
		{"sudo", "systemctl", "restart", "ollama.service"},
		{"systemctl", "is-active", "--quiet", "ollama.service"},
		{"curl", "--fail", "--silent", "--show-error", "--retry", "10", "--retry-connrefused", "--retry-delay", "1", "--max-time", "5", "http://127.0.0.1:11434/api/version"},
	}
	var commands [][]string
	for _, current := range configured.steps {
		if current.cmd != nil {
			commands = append(commands, current.cmd().Args[4:])
		}
	}
	if len(commands) != len(want) {
		t.Fatalf("comandos inesperados: %v", commands)
	}
	for i := range want {
		if !slices.Equal(commands[i], want[i]) {
			t.Fatalf("comando %d: %v; esperado: %v", i, commands[i], want[i])
		}
	}
}

func TestOllamaServiceConfiguration(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "configs", "ollama", "amd.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"[Service]", "SupplementaryGroups=render video", "Environment=\"OLLAMA_HOST=127.0.0.1:11434\""} {
		if !strings.Contains(string(content), required) {
			t.Fatalf("configuração ausente: %s", required)
		}
	}
	if strings.Contains(string(content), "HSA_OVERRIDE_GFX_VERSION") {
		t.Fatal("override de arquitetura não deve ser forçado")
	}
}
