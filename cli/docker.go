package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

const dockerService = "docker.service"
const dockerDaemonConfig = `{
  "data-root": "/var/lib/docker",
  "storage-driver": "overlay2"
}
`

var dockerTools = []string{"docker", "docker-compose", "docker-buildx", "docker-machine", "lazydocker"}

func runDockerSetup(device profile) error {
	if device.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute a configuração do Docker pelo menu Desktop ou Laptop como usuário normal")
	}
	account, err := user.Current()
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	console := sshConsole{home: account.HomeDir, input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	printActionHeader()
	err = configureDocker(&console, account.Username, executable, exec.LookPath, managedBackup("Docker", console.commands.interactive))
	if err != nil {
		fmt.Fprintln(console.output, err)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func configureDocker(c *sshConsole, username, executable string, lookup func(string) (string, error), location backupLocation) error {
	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" {
		configDir = filepath.Join(c.home, ".docker")
	}
	hasCredentials := func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return dockerHasCredentials(configDir, sshCommandsWithContext(ctx))
	}
	if !hasCredentials() {
		if _, err := lookup("firefox"); err != nil {
			return errors.New("instale o Firefox antes de configurar o login no Docker Hub")
		}
	}

	packages := append(slices.Clone(dockerTools), "util-linux", "xdg-utils")
	fmt.Fprintln(c.output, "Pacotes:", strings.Join(packages, ", "))
	fmt.Fprintln(c.output, "Aplicar /etc/docker/daemon.json com data-root=/var/lib/docker e storage-driver=overlay2, guardando uma cópia do arquivo anterior.")
	fmt.Fprintln(c.output, "Habilitar docker.service, adicionar", username, "ao grupo docker e definir Firefox como navegador padrão.")
	fmt.Fprintln(c.output, "Iniciar login no Docker Hub se ainda não houver credenciais locais.")
	if err := c.confirm("Digite CONFIRMO para configurar: ", "CONFIRMO"); err != nil {
		return err
	}
	if err := c.ensurePackages(packages...); err != nil {
		return fmt.Errorf("instalar pacotes Docker: %w", err)
	}
	if err := location.prepare(); err != nil {
		return err
	}
	if err := c.commands.interactive("sudo", "--", executable, "--apply-docker"); err != nil {
		return fmt.Errorf("configurar docker.service: %w", err)
	}
	if err := c.commands.interactive("sudo", "usermod", "-aG", "docker", "--", username); err != nil {
		return fmt.Errorf("adicionar %s ao grupo docker: %w", username, err)
	}
	fmt.Fprintln(c.output, "Abra uma nova sessão para atualizar o grupo docker nos demais terminais.")
	if err := setFirefoxDefault(c.commands); err != nil {
		return err
	}
	if !hasCredentials() {
		if err := c.commands.interactive("newgrp", "docker", "-c", "docker login"); err != nil {
			return fmt.Errorf("autenticar no Docker Hub: %w", err)
		}
	}
	fmt.Fprintln(c.output, "Docker configurado.")
	return nil
}

func dockerHasCredentials(configDir string, commands sshCommands) bool {
	content, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if err != nil {
		return false
	}
	var config struct {
		Auths map[string]struct {
			Auth          string `json:"auth"`
			IdentityToken string `json:"identitytoken"`
		} `json:"auths"`
		CredsStore  string            `json:"credsStore"`
		CredHelpers map[string]string `json:"credHelpers"`
	}
	if json.Unmarshal(content, &config) != nil {
		return false
	}
	registries := []string{"https://index.docker.io/v1/", "docker.io", "https://registry-1.docker.io"}
	helpers := make(map[string]bool)
	for _, registry := range registries {
		auth := config.Auths[registry]
		if auth.Auth != "" || auth.IdentityToken != "" {
			return true
		}
		helper := config.CredHelpers[registry]
		if helper == "" {
			helper = config.CredsStore
		}
		if helper != "" {
			helpers[helper] = true
		}
	}
	for helper := range helpers {
		output, err := commands.capture("", "docker-credential-"+helper, "list")
		var credentials map[string]string
		if err != nil || json.Unmarshal([]byte(output), &credentials) != nil {
			continue
		}
		for _, registry := range registries {
			if _, ok := credentials[registry]; ok {
				return true
			}
		}
	}
	return false
}

func runPrivilegedDocker(device profile) error {
	if device.sshKeyName() == "" {
		return errors.New("ação disponível nos perfis Desktop e Laptop")
	}
	account, err := sudoAccount()
	if err != nil {
		return err
	}
	unlock, err := lockOperation("system-docker", "Docker")
	if err != nil {
		return err
	}
	defer unlock()
	save := func(data []byte) (string, error) {
		return saveBackupAsUser(context.Background(), account, data, "--docker-backup")
	}
	return applyDockerDaemon("/etc/docker", systemSSHCommands(), os.Stdout, save)
}

func writeDockerBackup(p profile) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute o backup do Docker pelo menu Desktop ou Laptop como usuário normal")
	}
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}
	location := backupLocation{path: filepath.Join(backupRoot, "Docker")}
	path, err := location.saveFile("daemon.json.backup-*", data)
	if err != nil {
		return err
	}
	fmt.Println(path)
	return nil
}

func applyDockerDaemon(directory string, commands sshCommands, output io.Writer, save func([]byte) (string, error)) error {
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s precisa ser um diretório real", directory)
	}
	target := filepath.Join(directory, "daemon.json")
	var previous []byte
	var mode os.FileMode
	info, err = os.Lstat(target)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return err
	case !info.Mode().IsRegular():
		return fmt.Errorf("%s precisa ser um arquivo regular", target)
	default:
		mode = info.Mode().Perm()
		previous, err = os.ReadFile(target)
		if err != nil {
			return err
		}
	}

	contents := []byte(dockerDaemonConfig)
	changed := !bytes.Equal(previous, contents)
	active := isActive(dockerService, commands)
	if changed {
		work, err := os.MkdirTemp(directory, ".dotfiles-docker-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(work)
		candidate := filepath.Join(work, "daemon.json")
		if err := writeConfigAtomic(candidate, contents, 0644); err != nil {
			return err
		}
		if _, err := commands.capture("", "dockerd", "--validate", "--config-file", candidate); err != nil {
			return fmt.Errorf("validar daemon.json: %w", err)
		}
		if previous != nil {
			path, err := save(previous)
			if err != nil {
				return err
			}
			fmt.Fprintln(output, "Backup da configuração anterior:", path)
		}
		if err := os.Rename(candidate, target); err != nil {
			return err
		}
	}

	if _, err := commands.capture("", "systemctl", "start", dockerService); err != nil {
		if changed {
			if previous == nil {
				return errors.Join(err, os.Remove(target))
			}
			return errors.Join(err, writeConfigAtomic(target, previous, mode))
		}
		return err
	}
	if active && changed {
		fmt.Fprintln(output, "O Docker já estava ativo. O novo daemon.json será usado no próximo reinício do serviço.")
	}
	_, err = commands.capture("", "systemctl", "enable", dockerService)
	return err
}
