package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestApplyObsidianPreservesOtherFiles(t *testing.T) {
	home, _, backups := sdkTestRoots(t)
	destination := filepath.Join(home, ".obsidian")
	sdkTestFile(t, filepath.Join(destination, "appearance.json"), "previous appearance")
	sdkTestFile(t, filepath.Join(destination, "workspace.json"), "keep workspace")
	source := filepath.Join("..", "cachyos", "obsidian")
	generation, err := applyObsidian(source, destination, backupLocation{path: backups})
	if err != nil || generation == "" {
		t.Fatalf("backup do Obsidian: %q, %v", generation, err)
	}
	sdkTestContent(t, filepath.Join(destination, "workspace.json"), "keep workspace")
	sdkTestContent(t, filepath.Join(generation, "appearance.json"), "previous appearance")
	for _, name := range []string{"appearance.json", "snippets/noctalia.css"} {
		contents, err := os.ReadFile(filepath.Join(source, name))
		if err != nil {
			t.Fatal(err)
		}
		sdkTestContent(t, filepath.Join(destination, name), string(contents))
	}
	if backup, err := applyObsidian(source, destination, backupLocation{path: backups}); err != nil || backup != "" {
		t.Fatalf("reaplicação do Obsidian: %q, %v", backup, err)
	}
}

func TestConfigureTuringPythonReusesExistingMisePython(t *testing.T) {
	home := t.TempDir()
	venv := filepath.Join(home, "venv")
	requirements := filepath.Join(home, "requirements.txt")
	sdkTestFile(t, requirements, "fixture")
	const mise = "/opt/mise/bin/mise"
	var ran [][]string
	commands := sshCommands{
		capture: func(_ string, args ...string) (string, error) {
			if slices.Equal(args, []string{"env", "MISE_AUTO_INSTALL=0", mise, "--cd", "/", "--quiet", "which", "python", "--tool", turingPython}) {
				return "/opt/mise/python/bin/python3.13", nil
			}
			if !slices.Contains(args, filepath.Join(venv, "bin", "python")) {
				t.Fatalf("validação inesperada: %v", args)
			}
			return "", nil
		},
		interactive: func(args ...string) error {
			ran = append(ran, args)
			return nil
		},
	}
	if err := configureTuringPython(commands, mise, venv, requirements); err != nil {
		t.Fatal(err)
	}
	if len(ran) != 4 || !slices.Equal(ran[0][0:5], []string{mise, "--cd", "/", "exec", turingPython}) ||
		!strings.HasSuffix(strings.Join(ran[3], " "), "pip install --no-build-isolation --no-binary=pyamdgpuinfo pyamdgpuinfo") {
		t.Fatalf("comandos de instalação Turing: %v", ran)
	}
}

func TestApplyTuringAssetsBacksUpConfigBeforeInstalling(t *testing.T) {
	source, target, backups := sdkTestRoots(t)
	config := filepath.Join(source, "config.yaml")
	sdkTestFile(t, config, "new configuration")
	sdkTestFile(t, filepath.Join(target, "config.yaml"), "previous configuration")
	commands := sshCommands{interactive: func(args ...string) error {
		if !slices.Equal(args[0:6], []string{"sudo", "--", "sh", "-c", turingInstallFile, "dotfiles-turing"}) ||
			!slices.Equal(args[6:], []string{config, filepath.Join(target, "config.yaml"), "0664", turingGroup}) {
			t.Fatalf("instalação privilegiada inesperada: %v", args)
		}
		entries, err := os.ReadDir(backups)
		if err != nil || len(entries) != 1 {
			t.Fatalf("backup ausente antes de instalar: %v, %v", entries, err)
		}
		sdkTestContent(t, filepath.Join(backups, entries[0].Name(), "config.yaml"), "previous configuration")
		return nil
	}}
	path, err := applyTuringAssets(commands, target, []turingAsset{{config, "config.yaml"}}, backupLocation{path: backups})
	if err != nil || path == "" {
		t.Fatalf("backup da configuração Turing: %q, %v", path, err)
	}
}
