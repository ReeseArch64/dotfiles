package main

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
)

var gitPackages = []string{"git", "gnupg", "lazygit", "git-delta", "git-lfs", "neovim", "openssh"}

func runGitSetup(p profile) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("configure o Git pelo menu Desktop ou Laptop como usuário normal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	if !filepath.IsAbs(home) || !filepath.IsAbs(config) {
		return errors.New("as pastas pessoal e de configuração precisam ter caminhos absolutos")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	assets := filepath.Join(home, ".dotfiles", "cachyos", "git")
	if binary, err := os.Executable(); err == nil {
		local := filepath.Join(filepath.Dir(filepath.Dir(binary)), "cachyos", "git")
		if _, err := os.Stat(filepath.Join(local, ".gitconfig")); err == nil {
			assets = local
		}
	}

	printActionHeader()
	console := sshConsole{home: home, input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	fmt.Fprintln(console.output, "Instalar os pacotes do Git e aplicar os quatro arquivos globais de configuração.")
	fmt.Fprintln(console.output, "Pacotes:", strings.Join(append(slices.Clone(gitPackages), "git-ignore-bin"), ", "))
	fmt.Fprintln(console.output, "Os arquivos substituídos recebem backup em", filepath.Join(backupRoot, "Git"))
	err = console.confirm("Digite CONFIRMO para configurar o Git: ", "CONFIRMO")
	if err == nil {
		err = configureGit(console, config, assets, hostname)
	}
	if err != nil {
		fmt.Fprintln(console.output, err)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func configureGit(console sshConsole, config, assets, hostname string) error {
	files, err := gitFiles(console.home, config, assets)
	if err != nil {
		return err
	}
	unlock, err := lockOperation(filepath.Join(console.home, ".dotfiles-git.lock"), "configuração do Git")
	if err != nil {
		return err
	}
	defer unlock()
	if err := console.ensurePackages(gitPackages...); err != nil {
		return err
	}
	installer := packageInstaller{console: &console, which: exec.LookPath, build: buildYay}
	if !installer.installed("git-ignore-bin") {
		if err := installer.ensureYay(); err != nil {
			return err
		}
		if err := console.commands.interactive("yay", "-S", "--needed", "--", "git-ignore-bin"); err != nil {
			return err
		}
	}

	capture := console.commands.capture
	template := filepath.Join(assets, ".gitconfig")
	readConfig := func(path, key string) (string, error) {
		value, err := capture("", "git", "config", "--no-includes", "--file", path, "--get", key)
		return strings.TrimSpace(value), err
	}
	name, err := readConfig(template, "user.name")
	if err != nil {
		return err
	}
	email, err := readConfig(template, "user.email")
	if err != nil {
		return err
	}
	for _, value := range []string{name, email, hostname} {
		if value == "" || strings.ContainsAny(value, "\r\n\x00<>()") {
			return errors.New("identidade Git inválida para a chave GPG")
		}
	}

	var previous string
	if info, err := os.Stat(files[0].path); err == nil && info.Mode().IsRegular() {
		previous, _ = readConfig(files[0].path, "user.signingkey")
	}
	fingerprint, err := gitSigningKey(console, name, email, hostname, previous)
	if err != nil {
		return err
	}
	staging, err := os.MkdirTemp("", "dotfiles-git-config-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	candidate := filepath.Join(staging, ".gitconfig")
	if err := os.WriteFile(candidate, files[0].contents, 0600); err != nil {
		return err
	}
	if _, err := capture("", "git", "config", "--file", candidate, "--replace-all", "user.signingkey", fingerprint); err != nil {
		return err
	}
	files[0].contents, err = os.ReadFile(candidate)
	if err != nil {
		return err
	}
	backup, err := applyGitFiles(files, managedBackup("Git", console.commands.interactive))
	if backup != "" {
		fmt.Fprintln(console.output, "Backup do Git:", backup)
	}
	if err == nil {
		fmt.Fprintln(console.output, "Git configurado com a chave GPG", fingerprint)
	}
	return err
}

type gitFile struct {
	path     string
	contents []byte
	mode     os.FileMode
}

func gitFiles(home, config, assets string) ([]gitFile, error) {
	var files []gitFile
	for _, name := range []string{".gitconfig", ".gitattributes", ".gitignore", "config"} {
		destination, mode := filepath.Join(home, name), os.FileMode(0644)
		if name == "config" {
			destination = filepath.Join(config, "git", name)
		}
		if name == "config" || name == ".gitconfig" {
			mode = 0600
		}
		content, err := os.ReadFile(filepath.Join(assets, name))
		if err != nil {
			return nil, fmt.Errorf("ler arquivo Git do Dotfiles: %w", err)
		}
		files = append(files, gitFile{destination, content, mode})
	}
	return files, nil
}

type stagedGitFile struct {
	target, staged, original string
	previous, replacement    os.FileInfo
}

func applyGitFiles(files []gitFile, backup backupLocation) (backupDir string, result error) {
	var staged []stagedGitFile
	for _, file := range files {
		info, err := os.Lstat(file.path)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			return "", err
		case info.Mode().IsRegular():
			current, err := os.ReadFile(file.path)
			if err != nil {
				return "", err
			}
			if bytes.Equal(current, file.contents) && info.Mode().Perm() == file.mode {
				continue
			}
		case info.Mode()&os.ModeSymlink != 0:
			// Replace the legacy link itself and preserve it in the backup.
		default:
			return "", fmt.Errorf("destino Git não é arquivo regular nem link: %s", file.path)
		}
		dir := filepath.Dir(file.path)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return "", err
		}
		work, err := os.MkdirTemp(dir, ".dotfiles-git-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(work)
		candidate := filepath.Join(work, "candidate")
		if err := writeConfigAtomic(candidate, file.contents, file.mode); err != nil {
			return "", err
		}
		replacement, err := os.Stat(candidate)
		if err != nil {
			return "", err
		}
		staged = append(staged, stagedGitFile{target: file.path, staged: candidate, previous: info, replacement: replacement})
	}

	for i := range staged {
		file := &staged[i]
		if file.previous == nil {
			continue
		}
		if backupDir == "" {
			if err := backup.prepare(); err != nil {
				return "", err
			}
			backupDir, result = os.MkdirTemp(backup.path, "git-*")
			if result != nil {
				return "", result
			}
		}
		file.original = filepath.Join(backupDir, filepath.Base(file.target))
		if err := copySDKTree(context.Background(), file.target, file.original); err != nil {
			return backupDir, err
		}
	}

	applied := 0
	defer func() {
		if result != nil {
			for _, file := range slices.Backward(staged[:applied]) {
				result = errors.Join(result, file.restore())
			}
		}
	}()
	for _, file := range staged {
		current, err := os.Lstat(file.target)
		switch {
		case file.previous == nil && errors.Is(err, os.ErrNotExist):
		case err == nil && file.previous != nil && os.SameFile(file.previous, current) &&
			file.previous.Mode() == current.Mode() && file.previous.Size() == current.Size() && file.previous.ModTime().Equal(current.ModTime()):
		default:
			return backupDir, fmt.Errorf("arquivo Git mudou durante a preparação: %s", file.target)
		}
		if err := os.Rename(file.staged, file.target); err != nil {
			return backupDir, err
		}
		applied++
	}
	return backupDir, nil
}

func (file stagedGitFile) restore() error {
	current, err := os.Lstat(file.target)
	if err != nil || !os.SameFile(file.replacement, current) {
		return fmt.Errorf("arquivo Git mudou durante a restauração: %s", file.target)
	}
	if file.original == "" {
		return os.Remove(file.target)
	}
	if err := copySDKTree(context.Background(), file.original, file.staged); err != nil {
		return err
	}
	return os.Rename(file.staged, file.target)
}
