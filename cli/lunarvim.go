package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const lunarVimBranch = "release-1.4/neovim-0.9"
const lunarVimInstallerURL = "https://raw.githubusercontent.com/LunarVim/LunarVim/" + lunarVimBranch + "/utils/installer/install.sh"
const lunarVimNeovim = "aqua:neovim/neovim@0.9.5"

func installLunarVim(console *sshConsole, mise string) error {
	launcher := filepath.Join(console.home, ".local", "bin", "lvim")
	unlock, err := lockOperation(launcher, "instalação do LunarVim")
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := os.Lstat(launcher); err == nil {
		return fmt.Errorf("o caminho do LunarVim já existe: %s", launcher)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(config) {
		return fmt.Errorf("o diretório de configuração do LunarVim precisa ser absoluto")
	}
	prefix := []string{"env", "MISE_AUTO_INSTALL=0", mise, "--cd", "/"}
	nvim := miseLookup(console.commands, prefix, "nvim", lunarVimNeovim)
	if nvim == "" {
		if err := console.commands.interactive(mise, "--cd", "/", "install", lunarVimNeovim); err != nil {
			return fmt.Errorf("instalar o Neovim do LunarVim: %w", err)
		}
		nvim = miseLookup(console.commands, prefix, "nvim", lunarVimNeovim)
		if nvim == "" {
			return errors.New("o Mise não retornou o executável do Neovim 0.9.5")
		}
	}

	parent := filepath.Join(console.home, ".local", "share", "dotfiles")
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	work, err := os.MkdirTemp(parent, "lunarvim-*")
	if err != nil {
		return err
	}
	installed := false
	defer func() {
		if !installed {
			_ = os.RemoveAll(work)
		}
	}()
	script := filepath.Join(work, "install.sh")
	if err := console.commands.interactive("curl", "-fsSL", "--output", script, lunarVimInstallerURL); err != nil {
		return err
	}
	runtime := filepath.Join(work, "runtime")
	binary := filepath.Join(work, "bin", "lvim")
	nvimDir := filepath.Dir(nvim)
	if err := console.commands.interactive(
		"env", "PATH="+nvimDir+":"+os.Getenv("PATH"),
		"LV_BRANCH="+lunarVimBranch, "LV_REMOTE=LunarVim/LunarVim.git", "INSTALL_PREFIX="+work,
		"NVIM_APPNAME=lvim", "LUNARVIM_CONFIG_DIR="+filepath.Join(config, "lvim"),
		"LUNARVIM_RUNTIME_DIR="+runtime, "LUNARVIM_BASE_DIR="+filepath.Join(runtime, "lvim"),
		"LUNARVIM_CACHE_DIR="+filepath.Join(work, "cache"),
		"bash", script, "--no-install-dependencies",
	); err != nil {
		return fmt.Errorf("instalar LunarVim: %w", err)
	}
	if info, err := os.Stat(binary); err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("o instalador do LunarVim não disponibilizou %s", binary)
	}
	if err := os.Remove(script); err != nil {
		return err
	}
	quote := func(path string) string { return "'" + strings.ReplaceAll(path, "'", "'\"'\"'") + "'" }
	wrapper := filepath.Join(work, "launch")
	contents := fmt.Sprintf("#!/bin/sh\nPATH=%s:\"$PATH\"\nexport PATH\nexec %s \"$@\"\n", quote(nvimDir), quote(binary))
	if err := writeConfigAtomic(wrapper, []byte(contents), 0700); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(launcher), 0755); err != nil {
		return err
	}
	if err := os.Symlink(wrapper, launcher); err != nil {
		return err
	}
	installed = true
	fmt.Fprintln(console.output, "LunarVim instalado em", launcher)
	return nil
}
