package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestTuringScreenAurUsesSystemEnvironment(t *testing.T) {
	want := []string{"env", "-u", "PYTHONHOME", "-u", "PYTHONPATH", "PATH=/usr/bin:/bin", "/usr/bin/yay", "-S", "turing-smart-screen-python"}
	if got := commandTail(t, turingScreenInstallStep(), len(want)); !slices.Equal(got, want) {
		t.Fatalf("ambiente AUR inesperado: %v", got)
	}
}

func TestTuringScreenJobKeepsInstallationStepsSeparateAndOrdered(t *testing.T) {
	home := "/home/test user"
	python := home + "/.local/share/turing-screen-venv/bin/python"
	configured := turingScreenJobFor("/dotfiles", home, "dev", []step{turingScreenInstallStep()})
	wantCommands := [][]string{
		{"mise", "install", "python@3.13"},
		{"mise", "exec", "python@3.13", "--", "python", "-m", "venv", home + "/.local/share/turing-screen-venv"},
		{python, "-m", "pip", "install", "-r", "/opt/turing-smart-screen-python/requirements.txt"},
		{"sudo", "usermod", "-aG", "turing-smart-screen", "dev"},
		{"sudo", "udevadm", "control", "--reload-rules"},
		{python, "-m", "pip", "install", "setuptools", "wheel", "Cython"},
		{python, "-m", "pip", "install", "--no-build-isolation", "--no-binary=pyamdgpuinfo", "pyamdgpuinfo"},
		{"sudo", "install", "-d", "/opt/turing-smart-screen-python/res/themes/frieren"},
		{"sudo", "cp", "-r", "/dotfiles/configs/turing-smart-screen-python/themes/frieren/.", "/opt/turing-smart-screen-python/res/themes/frieren/"},
		{"sudo", "install", "-m644", "/dotfiles/configs/turing-smart-screen-python/config.yaml", "/opt/turing-smart-screen-python/config.yaml"},
	}
	last := 1 // validação e instalação do AUR precedem os comandos acima
	for _, want := range wantCommands {
		found := -1
		for i := last + 1; i < len(configured.steps); i++ {
			if configured.steps[i].cmd == nil {
				continue
			}
			args := configured.steps[i].cmd().Args
			if len(args) >= len(want) && slices.Equal(args[len(args)-len(want):], want) {
				found = i
				break
			}
		}
		if found < 0 {
			t.Fatalf("comando ausente ou fora de ordem: %v", want)
		}
		last = found
	}
	for _, text := range []string{"reconecte", "sessão gráfica", "id", "/dev/ttyACM0"} {
		if !strings.Contains(configured.result(), text) {
			t.Fatalf("orientação USB ausente: %s", text)
		}
	}
}

func TestTuringScreenCopiesThemeAndConfigWithMockedPrivileges(t *testing.T) {
	dotfiles, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()
	root := t.TempDir()
	bin := t.TempDir()
	writeTestExecutable(t, bin, "mise", `exit 0`)
	// Redireciona somente os caminhos de destino para um diretório temporário.
	// Nenhum comando privilegiado, instalação ou acesso USB é executado.
	if err := os.WriteFile(filepath.Join(bin, "sudo"), []byte(`#!/bin/bash
args=()
for arg in "$@"; do
    args+=("${arg/#\/opt\/turing-smart-screen-python/$TURING_TEST_ROOT}")
done
case "${args[0]}" in
    install|cp) "${args[@]}" ;;
    *) exit 0 ;;
esac
`), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":/usr/bin:/bin")
	t.Setenv("TERM", "xterm")
	t.Setenv("TURING_TEST_ROOT", root)
	configured := turingScreenJobFor(dotfiles, home, "dev", nil)
	if _, err := configured.steps[0].run(); err != nil {
		t.Fatalf("pré-validação falhou: %v", err)
	}
	for _, s := range configured.steps {
		if s.cmd == nil || (!strings.HasPrefix(s.label, "Copiar") && s.label != "Criar diretório do tema Frieren") {
			continue
		}
		if out, err := s.cmd().CombinedOutput(); err != nil {
			t.Fatalf("%s: %v: %s", s.label, err, out)
		}
	}
	for _, relative := range []string{"config.yaml", "res/themes/frieren/theme.yaml", "res/themes/frieren/background.png", "res/themes/frieren/icons/gpu.png"} {
		if _, err := os.Stat(filepath.Join(root, relative)); err != nil {
			t.Fatalf("arquivo não copiado: %s: %v", relative, err)
		}
	}
	want, err := os.ReadFile(configPath(dotfiles, turingScreenPackage, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "config.yaml"))
	if err != nil || string(got) != string(want) {
		t.Fatalf("configuração copiada difere da origem: %v", err)
	}
}

func TestTuringScreenValidationRejectsMissingPrerequisites(t *testing.T) {
	for _, username := range []string{"", "root", "dev"} {
		configured := turingScreenJobFor(t.TempDir(), t.TempDir(), username, nil)
		if _, err := configured.steps[0].run(); err == nil {
			t.Fatalf("pré-validação deveria falhar para %q com arquivos ausentes", username)
		}
	}
}
