package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
)

const turingPackage = "turing-smart-screen-python"
const turingRoot = "/opt/" + turingPackage
const turingGroup = "turing-smart-screen"
const turingPython = "python@3.13"

type turingAsset struct {
	source, relative string
}

func (installer packageInstaller) setupTuring() error {
	c := installer.console
	mise := findMise(c.home, installer.which)
	if mise == "" {
		return errors.New("configure o Mise em Programação → Mise antes de instalar a Turing Smart Screen")
	}
	account, err := user.Current()
	if err != nil {
		return err
	}
	source := cachyosAsset(c.home, turingPackage, "config.yaml")
	var assets []turingAsset
	theme := filepath.Join("res", "themes", "frieren")
	for _, name := range []string{
		"theme.yaml", "background.png", "preview.png", "theme_example.png", "icons/background.png",
		"icons/cpu.png", "icons/disk.png", "icons/gpu.png", "icons/ram.png",
	} {
		assets = append(assets, turingAsset{
			filepath.Join(filepath.Dir(source), "themes", "frieren", name), filepath.Join(theme, name),
		})
	}
	assets = append(assets, turingAsset{source, "config.yaml"})
	for _, asset := range assets {
		if err := sshRegular(asset.source, false); err != nil {
			return err
		}
	}
	fmt.Fprintln(c.output, "Instalar Turing Smart Screen com Python 3.13, suporte AMD e tema Frieren.")
	if err := c.confirm("Digite CONFIRMO para configurar: ", "CONFIRMO"); err != nil {
		return err
	}
	venv := filepath.Join(c.home, ".local", "share", "turing-screen-venv")
	unlock, err := lockOperation(venv, "configuração da Turing Smart Screen")
	if err != nil {
		return err
	}
	defer unlock()
	if err := c.ensurePackages("base-devel", "libdrm", "git", "tk"); err != nil {
		return err
	}
	if !installer.installed(turingPackage) {
		if err := installer.ensureYay(); err != nil {
			return err
		}
		yay, err := installer.which("yay")
		if err != nil {
			return err
		}
		if err := c.commands.interactive("env", "-u", "PYTHONHOME", "-u", "PYTHONPATH", "-u", "VIRTUAL_ENV",
			"PATH=/usr/bin:/bin", yay, "-S", "--needed", "--", turingPackage); err != nil {
			return err
		}
	}
	if err := configureTuringPython(c.commands, mise, venv, filepath.Join(turingRoot, "requirements.txt")); err != nil {
		return err
	}
	if err := c.commands.interactive("sudo", "usermod", "-aG", turingGroup, "--", account.Username); err != nil {
		return err
	}
	if err := c.commands.interactive("sudo", "udevadm", "control", "--reload-rules"); err != nil {
		return err
	}
	backup, err := applyTuringAssets(c.commands, turingRoot, assets, managedBackup("Turing", c.commands.interactive))
	if backup != "" {
		fmt.Fprintln(c.output, "Backup da Turing Smart Screen em", backup)
	}
	if err == nil {
		fmt.Fprintln(c.output, "Reconecte a tela e abra uma nova sessão para usar o grupo", turingGroup)
		fmt.Fprintln(c.output, "Para iniciar: env PATH=\"$HOME/.local/share/turing-screen-venv/bin:$PATH\" /usr/bin/turing-smart-screen main")
	}
	return err
}

func configureTuringPython(commands sshCommands, mise, venv, requirements string) error {
	if err := sshRegular(requirements, false); err != nil {
		return err
	}
	python := filepath.Join(venv, "bin", "python")
	switch _, err := os.Lstat(python); {
	case errors.Is(err, os.ErrNotExist):
		prefix := []string{"env", "MISE_AUTO_INSTALL=0", mise, "--cd", "/"}
		if miseLookup(commands, prefix, "python", turingPython) == "" {
			if err := commands.interactive(mise, "--cd", "/", "install", turingPython); err != nil {
				return fmt.Errorf("instalar Python 3.13: %w", err)
			}
		}
		if err := commands.interactive(mise, "--cd", "/", "exec", turingPython, "--",
			"env", "-u", "PYTHONHOME", "-u", "PYTHONPATH", "python", "-m", "venv", venv); err != nil {
			return fmt.Errorf("criar ambiente virtual em %s: %w", venv, err)
		}
	case err != nil:
		return err
	}
	prefix := []string{"env", "-u", "PYTHONHOME", "-u", "PYTHONPATH", python}
	if _, err := commands.capture("", append(prefix, "-c",
		"import sys; sys.exit(0 if sys.version_info[:2] == (3, 13) and sys.prefix != sys.base_prefix else 1)")...); err != nil {
		return fmt.Errorf("o ambiente %s precisa conter um Python virtual 3.13: %w", venv, err)
	}
	for _, args := range [][]string{
		{"-r", requirements},
		{"setuptools", "wheel", "Cython"},
		{"--no-build-isolation", "--no-binary=pyamdgpuinfo", "pyamdgpuinfo"},
	} {
		if err := commands.interactive(append(append(prefix, "-m", "pip", "install"), args...)...); err != nil {
			return fmt.Errorf("instalar dependências do ambiente Turing: %w", err)
		}
	}
	if _, err := commands.capture("", append(prefix, "-c", "import pyamdgpuinfo")...); err != nil {
		return fmt.Errorf("carregar suporte à GPU AMD: %w", err)
	}
	return nil
}

func applyTuringAssets(commands sshCommands, root string, assets []turingAsset, location backupLocation) (string, error) {
	var changed []turingAsset
	originals := make(map[string][]byte)
	for _, asset := range assets {
		destination := filepath.Join(root, asset.relative)
		if err := validateBackupPath(filepath.Dir(destination)); err != nil {
			return "", err
		}
		if err := sshRegular(destination, true); err != nil {
			return "", err
		}
		mode := os.FileMode(0644)
		if asset.relative == "config.yaml" {
			mode = 0664
		}
		previous, err := os.ReadFile(destination)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if err == nil {
			info, err := os.Lstat(destination)
			if err != nil {
				return "", err
			}
			newContents, err := os.ReadFile(asset.source)
			if err != nil {
				return "", err
			}
			if bytes.Equal(previous, newContents) && info.Mode().Perm() == mode {
				continue
			}
			originals[asset.relative] = previous
		}
		changed = append(changed, asset)
	}
	var backup string
	if len(originals) > 0 {
		if err := location.prepare(); err != nil {
			return "", err
		}
		var err error
		backup, err = os.MkdirTemp(location.path, "config-*")
		if err != nil {
			return "", err
		}
		for name, data := range originals {
			path := filepath.Join(backup, name)
			if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				return "", errors.Join(err, os.RemoveAll(backup))
			}
			if err := writeConfigAtomic(path, data, 0600); err != nil {
				return "", errors.Join(err, os.RemoveAll(backup))
			}
		}
	}
	for _, asset := range changed {
		destination := filepath.Join(root, asset.relative)
		directory := filepath.Dir(destination)
		if _, err := os.Stat(directory); errors.Is(err, os.ErrNotExist) {
			if err := commands.interactive("sudo", "install", "-d", "-m", "0755", "--", directory); err != nil {
				return backup, err
			}
		} else if err != nil {
			return backup, err
		}
		mode, group := "0644", "root"
		if asset.relative == "config.yaml" {
			mode, group = "0664", turingGroup
		}
		if err := commands.interactive("sudo", "--", "sh", "-c", installRootConfigAtomically, "dotfiles-turing",
			asset.source, destination, mode, group); err != nil {
			return backup, err
		}
	}
	return backup, nil
}
