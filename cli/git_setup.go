package main

import (
	"bufio"
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

	previous, _ := capture("", "git", "config", "--global", "--includes", "--get", "user.signingkey")
	signingKey, err := gitSigningKey(console, name, email, hostname, strings.TrimSpace(previous))
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
	if _, err := capture("", "git", "config", "--file", candidate, "--replace-all", "user.signingkey", signingKey); err != nil {
		return err
	}
	files[0].contents, err = os.ReadFile(candidate)
	if err != nil {
		return err
	}
	backup, err := applyConfigFiles(files, managedBackup("Git", console.commands.interactive), "git")
	if backup != "" {
		fmt.Fprintln(console.output, "Backup do Git:", backup)
	}
	if err == nil {
		fmt.Fprintln(console.output, "Git configurado com a chave GPG", signingKey)
	}
	return err
}

func gitFiles(home, config, assets string) ([]configFile, error) {
	var files []configFile
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
		files = append(files, configFile{destination, content, mode})
	}
	return files, nil
}
