package main

import (
	"bytes"
	"errors"
	"os"
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

func TestOllamaReusesBothROCmInstallations(t *testing.T) {
	for _, tt := range []struct {
		name, library string
		packaged      bool
	}{
		{"legacy", "libhipblas.so.3", false},
		{"system package", "libggml-hip.so", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			installer, commands := newPackageInstallerTest(t, "")
			binary := filepath.Join(installer.console.home, "bin", "ollama")
			sdkTestFile(t, binary, "")
			library := filepath.Join(installer.console.home, "lib", "ollama", "rocm_v7_2", tt.library)
			sdkTestFile(t, library, "")
			installer.which = func(string) (string, error) { return binary, nil }
			installer.console.commands.capture = func(_ string, args ...string) (string, error) {
				switch {
				case slices.Equal(args, []string{"systemctl", "cat", "ollama.service"}) && !tt.packaged:
					return "[Service]", nil
				case len(args) == 3 && args[0] == "pacman" && args[1] == "-Q":
					if tt.packaged {
						return args[2], nil
					}
					return "", errors.New("pacote ausente")
				default:
					t.Fatalf("comando inesperado: %v", args)
					return "", nil
				}
			}
			if err := installer.ensureOllamaRuntime(); err != nil || len(*commands) != 0 {
				t.Fatalf("runtime existente executou %v: %v", *commands, err)
			}
			if err := os.Remove(library); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("biblioteca-ausente", library); err != nil {
				t.Fatal(err)
			}
			if ollamaROCmInstalled(binary) {
				t.Fatal("link de biblioteca quebrado aceito")
			}
		})
	}
}

func TestOllamaAssetAndDropInBackup(t *testing.T) {
	source := filepath.Join("..", "cachyos", "ollama", "amd.conf")
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range []string{
		"RequiresMountsFor=" + ollamaModels,
		"ConditionPathIsMountPoint=" + ollamaStorage,
		"SupplementaryGroups=render video",
		`Environment="OLLAMA_HOST=127.0.0.1:11434"`,
		`Environment="OLLAMA_MODELS=` + ollamaModels + `"`,
	} {
		if !bytes.Contains(data, []byte(line)) {
			t.Fatalf("configuração ausente: %s", line)
		}
	}

	installer, commands := newPackageInstallerTest(t, "")
	destination := filepath.Join(installer.console.home, "ollama.service.d", "amd.conf")
	location := backupLocation{path: filepath.Join(installer.console.home, "backups")}
	sdkTestFile(t, destination, "configuração anterior")
	if err := applyOllamaDropIn(installer.console, source, destination, location); err != nil {
		t.Fatal(err)
	}
	backups, err := os.ReadDir(location.path)
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups = %v, erro = %v", backups, err)
	}
	sdkTestContent(t, filepath.Join(location.path, backups[0].Name()), "configuração anterior")
	if len(*commands) != 2 || !strings.HasSuffix((*commands)[1], destination+" 0644 root") {
		t.Fatalf("comandos de instalação do drop-in = %q", *commands)
	}
}
