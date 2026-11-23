package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const sandboxName = "sandbox"
const sandboxImage = "docker.io/library/archlinux:latest"

func runDistroboxSetup(device profile) error {
	if device.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("configure o Ambiente Sandbox pelo menu Desktop ou Laptop como usuário normal")
	}
	account, err := user.Current()
	if err != nil {
		return err
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(configHome) {
		configHome = filepath.Join(account.HomeDir, ".config")
	}
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	printActionHeader()
	fmt.Fprintln(console.output, "Instalar Docker e Distrobox quando necessário e criar o contêiner sandbox com Arch Linux.")
	fmt.Fprintln(console.output, "Se o Docker local precisar de configuração, iniciar docker.service e liberar o acesso pelo grupo docker.")
	if err := console.confirm("Digite CONFIRMO para configurar: ", "CONFIRMO"); err != nil {
		return err
	}
	unblock, err := lockOperation("distrobox-sandbox", "configuração do Distrobox")
	if err != nil {
		return err
	}
	err = configureDistrobox(&console, account, configHome, managedBackup("Distrobox", console.commands.interactive))
	unblock()
	if err != nil {
		fmt.Fprintln(console.output, err)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func configureDistrobox(c *sshConsole, account *user.User, configHome string, backup backupLocation) error {
	if err := c.ensurePackages("docker", "distrobox"); err != nil {
		return fmt.Errorf("instalar Docker e Distrobox: %w", err)
	}
	commands, err := sandboxDockerAccess(c, account.Username)
	if err != nil {
		return err
	}
	config, err := distroboxDockerConfig(account.HomeDir, configHome)
	if err != nil {
		return err
	}
	list, err := commands.capture("", "docker", "container", "ls", "--all", "--filter", "name="+sandboxName,
		"--format", `{{.Names}} {{.Label "manager"}}`)
	if err != nil {
		return fmt.Errorf("listar contêineres Docker: %w", err)
	}
	found := false
	for line := range strings.SplitSeq(list, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != sandboxName {
			continue
		}
		if len(fields) != 2 || fields[1] != "distrobox" {
			return errors.New("já existe um contêiner Docker chamado sandbox que não pertence ao Distrobox")
		}
		found = true
		break
	}
	if !found {
		if err := commands.interactive("env", "DBX_CONTAINER_MANAGER=docker", "distrobox", "create", "--yes", "--name", sandboxName, "--image", sandboxImage); err != nil {
			return fmt.Errorf("criar Distrobox sandbox: %w", err)
		}
	}
	previous, err := applyConfigFiles([]configFile{config}, backup, "distrobox")
	if previous != "" {
		fmt.Fprintln(c.output, "Configuração anterior em", previous)
	}
	if err != nil {
		return err
	}
	fmt.Fprintln(c.output, "Sandbox disponível com DBX_CONTAINER_MANAGER=docker distrobox enter sandbox")
	return nil
}

func sandboxDockerAccess(c *sshConsole, username string) (sshCommands, error) {
	commands := c.commands
	info := []string{"docker", "info", "--format", "{{.ServerVersion}}"}
	if _, err := commands.capture("", info...); err == nil {
		return commands, nil
	}
	if err := commands.interactive("sudo", "systemctl", "enable", "--now", dockerService); err != nil {
		return sshCommands{}, fmt.Errorf("ativar %s: %w", dockerService, err)
	}
	if _, err := commands.capture("", info...); err == nil {
		return commands, nil
	}
	if err := commands.interactive("sudo", "usermod", "-aG", "docker", "--", username); err != nil {
		return sshCommands{}, fmt.Errorf("adicionar %s ao grupo docker: %w", username, err)
	}
	groupArgs := func(args []string) []string {
		quoted := make([]string, 0, len(args))
		for _, arg := range args {
			quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", "'\\''")+"'")
		}
		return []string{"newgrp", "docker", "-c", strings.Join(quoted, " ")}
	}
	if _, err := commands.capture("", groupArgs(info)...); err != nil {
		return sshCommands{}, fmt.Errorf("acessar o Docker com o grupo docker: %w", err)
	}
	fmt.Fprintln(c.output, "Abra uma nova sessão para atualizar o grupo docker nos demais terminais.")
	return sshCommands{
		capture: func(input string, args ...string) (string, error) {
			return commands.capture(input, groupArgs(args)...)
		},
		interactive: func(args ...string) error {
			return commands.interactive(groupArgs(args)...)
		},
	}, nil
}

func distroboxDockerConfig(home, configHome string) (configFile, error) {
	path := filepath.Join(home, ".distroboxrc")
	if _, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
		path = filepath.Join(configHome, "distrobox", "distrobox.conf")
	} else if err != nil {
		return configFile{}, err
	}
	mode := os.FileMode(0600)
	var contents []byte
	if info, err := os.Stat(path); err == nil {
		if !info.Mode().IsRegular() {
			return configFile{}, fmt.Errorf("configuração do Distrobox precisa ser um arquivo regular: %s", path)
		}
		mode = info.Mode().Perm()
		contents, err = os.ReadFile(path)
		if err != nil {
			return configFile{}, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return configFile{}, err
	}
	const dockerSetting = `container_manager="docker"`
	var lines []string
	var replaced bool
	if len(contents) > 0 {
		for line := range strings.SplitSeq(strings.TrimSuffix(string(contents), "\n"), "\n") {
			setting := strings.TrimSpace(line)
			if strings.HasPrefix(setting, "export ") || strings.HasPrefix(setting, "export\t") {
				setting = strings.TrimSpace(setting[len("export"):])
			}
			if strings.HasPrefix(setting, "container_manager=") {
				if !replaced {
					lines = append(lines, dockerSetting)
					replaced = true
				}
				continue
			}
			lines = append(lines, line)
		}
	}
	if !replaced {
		lines = append(lines, dockerSetting)
	}
	return configFile{path, []byte(strings.Join(lines, "\n") + "\n"), mode}, nil
}
