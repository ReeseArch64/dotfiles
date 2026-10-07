package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
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

func TestOllamaROCmVersionedRuntime(t *testing.T) {
	for _, directory := range []string{"rocm", "rocm_v6", "rocm_v7_2"} {
		t.Run(directory, func(t *testing.T) {
			root := t.TempDir()
			binary := filepath.Join(root, "bin", "ollama")
			library := filepath.Join(root, "lib", "ollama", directory, "libhipblas.so.3.2.70201")
			for _, dir := range []string{filepath.Dir(binary), filepath.Dir(library)} {
				if err := os.MkdirAll(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(binary, nil, 0755); err != nil {
				t.Fatal(err)
			}
			lookPath := func(string) (string, error) { return binary, nil }
			configured := ollamaJobFor("/dotfiles", nil, lookPath)
			if _, err := configured.steps[2].run(); err == nil {
				t.Fatal("diretório ROCm vazio aceito")
			}
			if err := os.WriteFile(library, nil, 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := configured.steps[2].run(); err != nil {
				t.Fatalf("runtime instalado em %s rejeitado: %v", directory, err)
			}
			if err := os.Remove(library); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("missing-library", library); err != nil {
				t.Fatal(err)
			}
			if ollamaROCmInstalled(lookPath) {
				t.Fatal("link de biblioteca quebrado aceito")
			}
		})
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
		{"mountpoint", "-q", "/mnt/storage"},
		{"sudo", "install", "-d", "-o", "ollama", "-g", "ollama", "-m", "0750", "/mnt/storage/ollama", "/mnt/storage/ollama/models"},
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

func TestOllamaModelsDirectoriesPreserveDataAndCorrectPermissions(t *testing.T) {
	configured := ollamaJobFor("/dotfiles", nil, exec.LookPath)
	var args []string
	for _, current := range configured.steps {
		if current.cmd == nil {
			continue
		}
		command := current.cmd().Args[4:]
		if len(command) > 2 && slices.Equal(command[:3], []string{"sudo", "install", "-d"}) {
			args = append([]string{}, command[2:]...)
			break
		}
	}
	if len(args) != 9 {
		t.Fatalf("criação dos diretórios não encontrada: %v", args)
	}
	root := t.TempDir()
	parent := filepath.Join(root, "ollama")
	models := filepath.Join(parent, "models")
	args[2], args[4] = strconv.Itoa(os.Getuid()), strconv.Itoa(os.Getgid())
	args[7], args[8] = parent, models
	create := func() {
		t.Helper()
		if out, err := exec.Command("install", args...).CombinedOutput(); err != nil {
			t.Fatalf("criar diretórios: %v: %s", err, out)
		}
		for _, path := range []string{parent, models} {
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if !info.IsDir() || info.Mode().Perm() != 0750 {
				t.Fatalf("permissões incorretas em %s: %v", path, info.Mode())
			}
		}
	}
	create()
	model := filepath.Join(models, "existing-model")
	if err := os.WriteFile(model, []byte("model data"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{parent, models} {
		if err := os.Chmod(path, 0777); err != nil {
			t.Fatal(err)
		}
	}
	create()
	content, err := os.ReadFile(model)
	if err != nil || string(content) != "model data" {
		t.Fatalf("modelo existente alterado: %q, %v", content, err)
	}
	info, err := os.Stat(model)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissões do modelo existente alteradas: %v, %v", info, err)
	}
}

func TestOllamaMissingStorageMountStopsJob(t *testing.T) {
	bin := t.TempDir()
	writeTestExecutable(t, bin, "mountpoint", "exit 1")
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	configured := ollamaJobFor("/dotfiles", nil, exec.LookPath)
	for i, current := range configured.steps {
		if current.cmd == nil || current.cmd().Args[4] != "mountpoint" {
			continue
		}
		command := current.cmd().Args[4:]
		err := exec.Command(command[0], command[1:]...).Run()
		if err == nil {
			t.Fatal("montagem ausente aceita")
		}
		run := newJobRun(configured)
		run.cur = i
		run.states[i] = stepRunning
		if next := run.handle(stepDoneMsg{err: err}); next != nil || !run.failed || !run.done {
			t.Fatal("configuração continuou sem o disco de modelos")
		}
		if run.states[i+1] != stepPending {
			t.Fatal("diretórios criados apesar da montagem ausente")
		}
		return
	}
	t.Fatal("verificação da montagem não encontrada")
}

func TestOllamaServiceConfiguration(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "configs", "ollama", "amd.conf"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"[Unit]", "RequiresMountsFor=/mnt/storage/ollama/models", "ConditionPathIsMountPoint=/mnt/storage", "[Service]", "SupplementaryGroups=render video", "Environment=\"OLLAMA_HOST=127.0.0.1:11434\"", "Environment=\"OLLAMA_MODELS=/mnt/storage/ollama/models\""} {
		if !strings.Contains(string(content), required) {
			t.Fatalf("configuração ausente: %s", required)
		}
	}
	if strings.Contains(string(content), "HSA_OVERRIDE_GFX_VERSION") {
		t.Fatal("override de arquitetura não deve ser forçado")
	}
}
