package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

func laptopSSHPolicy(username, port string) string {
	return "# Gerenciado pelo perfil Laptop Omarchy do DOTFILES.\nPort " + port + "\n" +
		"PermitRootLogin no\nPubkeyAuthentication yes\nAuthenticationMethods publickey\n" +
		"PasswordAuthentication no\nKbdInteractiveAuthentication no\nHostbasedAuthentication no\n" +
		"UsePAM yes\nStrictModes yes\nAllowUsers " + username + "\nAuthorizedKeysFile .ssh/authorized_keys\n" +
		"AuthorizedKeysCommand none\nTrustedUserCAKeys none\nSubsystem sftp internal-sftp\n"
}

func verifyLaptopSSHPolicy(effective, username string) error {
	lines := strings.Split(effective, "\n")
	for _, expected := range []string{
		"permitrootlogin no", "pubkeyauthentication yes", "authenticationmethods publickey",
		"passwordauthentication no", "kbdinteractiveauthentication no", "allowusers " + username,
		"authorizedkeysfile .ssh/authorized_keys", "authorizedkeyscommand none", "trustedusercakeys none",
	} {
		if !slices.Contains(lines, expected) {
			return fmt.Errorf("política SSH efetiva pendente: %s", expected)
		}
	}
	return nil
}

func writeLaptopServerConfig(path string, contents []byte, mode os.FileMode) error {
	if err := laptopSSHRegular(path, true); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".sshd-config-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if _, err = temporary.Write(contents); err == nil {
		err = temporary.Chmod(mode)
	}
	if err == nil {
		err = temporary.Sync()
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporary.Name(), path)
}

type laptopSSHServer struct {
	username, home, config, lockPath string
	commands                         laptopSSHCommands
	recoveryCommands                 laptopSSHCommands
}

// Não executa shell: valida candidato e política efetiva antes da substituição.
// Em erro após a substituição, restaura o arquivo e o estado de habilitação.
func (s laptopSSHServer) apply() (backupPath string, result error) {
	unlock, err := laptopSSHLock(s.lockPath)
	if err != nil {
		return "", err
	}
	defer unlock()
	directory, err := os.Lstat(filepath.Join(s.home, ".ssh"))
	if err != nil || !directory.IsDir() {
		return "", errors.New("diretório SSH inválido")
	}
	keys, err := readLaptopAuthorizedKeys(filepath.Join(s.home, ".ssh", "authorized_keys"), s.commands)
	if err != nil {
		return "", err
	}
	if len(keys) == 0 {
		return "", errors.New("adicione uma chave autorizada primeiro")
	}
	if err := laptopSSHRegular(s.config, false); err != nil {
		return "", err
	}
	for _, state := range []string{"is-active", "is-enabled"} {
		if _, err := s.commands.capture("", "systemctl", state, "--quiet", "sshd.socket"); err == nil {
			return "", errors.New("sshd.socket ativo/habilitado; revise antes de configurar sshd.service")
		}
	}
	if _, err := s.commands.capture("", "systemctl", "is-active", "--quiet", "sshd.service"); err != nil {
		return "", errors.New("inicie e teste sshd.service antes de aplicar a política")
	}
	if _, err := s.commands.capture("", "sshd", "-t"); err != nil {
		return "", err
	}
	effective, err := s.commands.capture("", "sshd", "-T")
	if err != nil {
		return "", err
	}
	var ports []string
	for _, line := range strings.Split(effective, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "port" {
			ports = append(ports, fields[1])
		}
	}
	if len(ports) != 1 {
		return "", errors.New("é necessária uma única porta SSH")
	}
	port, err := strconv.Atoi(ports[0])
	if err != nil || port < 1 || port > 65535 {
		return "", errors.New("porta SSH inválida")
	}
	original, err := os.ReadFile(s.config)
	if err != nil {
		return "", err
	}
	originalInfo, err := os.Stat(s.config)
	if err != nil {
		return "", err
	}
	enabledOutput, enabledErr := s.commands.capture("", "systemctl", "is-enabled", "sshd.service")
	enabledState := strings.TrimSpace(enabledOutput)
	if enabledErr != nil {
		enabledState = "disabled"
	}
	backup, err := os.MkdirTemp(filepath.Dir(s.config), "laptop-backup.*")
	if err != nil {
		return "", err
	}
	backupPath = backup
	if err := os.WriteFile(filepath.Join(backup, "sshd_config"), original, 0600); err != nil {
		return backup, err
	}
	candidate := filepath.Join(backup, "candidate")
	policy := []byte(laptopSSHPolicy(s.username, ports[0]))
	if err := os.WriteFile(candidate, policy, 0600); err != nil {
		return backup, err
	}
	if _, err := s.commands.capture("", "sshd", "-t", "-f", candidate); err != nil {
		return backup, err
	}
	address := "127.0.0.1"
	if fields := strings.Fields(os.Getenv("SSH_CONNECTION")); len(fields) > 0 {
		address = fields[0]
	}
	for _, username := range []string{s.username, "root"} {
		effective, err := s.commands.capture("", "sshd", "-T", "-f", candidate, "-C", "user="+username+",host=localhost,addr="+address)
		if err != nil {
			return backup, err
		}
		if err := verifyLaptopSSHPolicy(effective, s.username); err != nil {
			return backup, err
		}
	}
	installed := false
	defer func() {
		if result == nil || !installed {
			return
		}
		restoreErr := writeLaptopServerConfig(s.config, original, originalInfo.Mode().Perm())
		recovery := s.recoveryCommands
		if recovery.capture == nil {
			recovery = s.commands
		}
		// Preserve enabled-runtime as well as persistent enablement on rollback.
		if enabledState != "enabled" {
			_, err := recovery.capture("", "systemctl", "disable", "sshd.service")
			restoreErr = errors.Join(restoreErr, err)
			if enabledState == "enabled-runtime" {
				_, err := recovery.capture("", "systemctl", "enable", "--runtime", "sshd.service")
				restoreErr = errors.Join(restoreErr, err)
			}
		}
		_, restartErr := recovery.capture("", "systemctl", "restart", "sshd.service")
		result = errors.Join(result, restoreErr, restartErr, fmt.Errorf("tentativa de restaurar política anterior; backup: %s", backup))
	}()
	if err := writeLaptopServerConfig(s.config, policy, 0644); err != nil {
		return backup, err
	}
	installed = true
	for _, args := range [][]string{
		{"sshd", "-t"}, {"systemctl", "enable", "sshd.service"},
		{"systemctl", "restart", "sshd.service"}, {"systemctl", "is-active", "--quiet", "sshd.service"},
	} {
		if _, err := s.commands.capture("", args...); err != nil {
			return backup, err
		}
	}
	return backup, nil
}

func applyLaptopSSHServer() error {
	username := os.Getenv("SUDO_USER")
	if os.Geteuid() != 0 || username == "root" || !regexp.MustCompile(`^[a-z_][a-z0-9_-]*\$?$`).MatchString(username) {
		return errors.New("execute pelo menu SSH como usuário normal")
	}
	account, err := user.Lookup(username)
	if err != nil {
		return err
	}
	if account.Uid == "0" {
		return errors.New("não é permitido habilitar root")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	device, err := detectDevice(hostname)
	if err != nil || device != deviceLaptop {
		return errors.New("ação exclusiva do laptop Omarchy")
	}
	if err := checkPlatformForDevice(device, "/etc/os-release", nil); err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	server := laptopSSHServer{username: username, home: account.HomeDir, config: "/etc/ssh/sshd_config", lockPath: "/run/dotfiles-laptop-ssh.lock", commands: laptopSSHCommandsWithContext(ctx), recoveryCommands: systemLaptopSSHCommands()}
	backup, err := server.apply()
	if err != nil {
		return err
	}
	fmt.Printf("SSH somente por chaves, sem root, habilitado para %s. Backup: %s\nTeste uma NOVA conexão antes de fechar a sessão atual. O firewall não foi alterado.\n", username, backup)
	return nil
}
