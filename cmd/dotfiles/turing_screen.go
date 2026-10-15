package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
)

const turingScreenPackage = "turing-smart-screen-python"
const turingScreenRoot = "/opt/turing-smart-screen-python"

func turingScreenJobFor(dotfiles, home, username string, packageSteps []step) job {
	python := filepath.Join(home, ".local", "share", "turing-screen-venv", "bin", "python")
	steps := []step{nativeStep("Validar mise, usuário e arquivos do Turing Smart Screen", func() error {
		if username == "" || username == "root" {
			return fmt.Errorf("execute a CLI com um usuário comum, não como root")
		}
		if _, err := exec.LookPath("mise"); err != nil {
			return fmt.Errorf("instale o mise em Desenvolvimento > Ambiente de Desenvolvimento: %w", err)
		}
		for _, entry := range []struct {
			path string
			dir  bool
		}{
			{configPath(dotfiles, turingScreenPackage, "config.yaml"), false},
			{configPath(dotfiles, turingScreenPackage, "themes", "frieren"), true},
		} {
			info, err := os.Stat(entry.path)
			if err != nil {
				return err
			}
			if (entry.dir && !info.IsDir()) || (!entry.dir && !info.Mode().IsRegular()) {
				return fmt.Errorf("arquivo ou diretório inválido: %s", entry.path)
			}
		}
		return nil
	})}
	steps = append(steps, packageSteps...)
	steps = append(steps,
		terminalStep("Instalar Python 3.13 via mise", "mise", "install", "python@3.13"),
		terminalStep("Criar ambiente virtual do Turing com Python 3.13", "mise", "exec", "python@3.13", "--", "python", "-m", "venv", filepath.Dir(filepath.Dir(python))),
		terminalStep("Instalar dependências Python do Turing", python, "-m", "pip", "install", "-r", turingScreenRoot+"/requirements.txt"),
		terminalStep("Adicionar usuário ao grupo turing-smart-screen", "sudo", "usermod", "-aG", "turing-smart-screen", username),
		terminalStep("Recarregar regras USB do udev", "sudo", "udevadm", "control", "--reload-rules"),
	)
	steps = append(steps, ensurePkgs("base-devel", "libdrm")...)
	steps = append(steps,
		terminalStep("Instalar ferramentas Python para GPU AMD", python, "-m", "pip", "install", "setuptools", "wheel", "Cython"),
		terminalStep("Compilar pyamdgpuinfo no ambiente Python 3.13", python, "-m", "pip", "install", "--no-build-isolation", "--no-binary=pyamdgpuinfo", "pyamdgpuinfo"),
		terminalStep("Criar diretório do tema Frieren", "sudo", "install", "-d", turingScreenRoot+"/res/themes/frieren"),
		terminalStep("Copiar tema Frieren para o Turing", "sudo", "cp", "-r", configPath(dotfiles, turingScreenPackage, "themes", "frieren")+"/.", turingScreenRoot+"/res/themes/frieren/"),
		terminalStep("Copiar config.yaml para o Turing", "sudo", "install", "-m644", configPath(dotfiles, turingScreenPackage, "config.yaml"), turingScreenRoot+"/config.yaml"),
	)
	return job{
		title: "Configurar Turing Smart Screen",
		steps: steps,
		result: func() string {
			return "Turing Smart Screen configurado com Python 3.13, GPU AMD e tema Frieren.\n" +
				"Desconecte e reconecte a tela USB; saia da sessão gráfica e entre novamente.\n" +
				"Confira o grupo turing-smart-screen com id e /usr/bin/ls -l /dev/ttyACM0.\n" +
				`Para iniciar: env PATH="$HOME/.local/share/turing-screen-venv/bin:$PATH" /usr/bin/turing-smart-screen main`
		},
	}
}

func turingScreenInstallStep() step {
	return terminalStep("Instalar Turing pelo AUR com Python do sistema", "env", "-u", "PYTHONHOME", "-u", "PYTHONPATH", "PATH=/usr/bin:/bin", "/usr/bin/yay", "-S", turingScreenPackage)
}

func turingScreenJob(dotfiles string) job {
	home, _ := os.UserHomeDir()
	current, err := user.Current()
	if err != nil {
		return job{title: "Configurar Turing Smart Screen", steps: []step{nativeStep("Identificar usuário atual", func() error { return err })}}
	}
	var steps []step
	if len(missingPkgs(turingScreenPackage)) > 0 {
		steps = yaySetupSteps()
		steps = append(steps,
			pacmanSyncStep(),
			turingScreenInstallStep(),
		)
	}
	return turingScreenJobFor(dotfiles, home, current.Username, steps)
}
