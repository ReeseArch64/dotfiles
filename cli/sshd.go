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

// OpenSSH pode listar a mesma porta mais de uma vez após expandir Includes.
func effectiveSSHPort(effective string) (string, error) {
	var ports []string
	for _, line := range strings.Split(effective, "\n") {
		fields, err := sshConfigFields(line)
		if err != nil {
			return "", err
		}
		if len(fields) == 0 || fields[0] != "port" {
			continue
		}
		if len(fields) != 2 {
			return "", fmt.Errorf("diretiva de porta SSH inválida: %q", line)
		}
		number, err := strconv.Atoi(fields[1])
		if err != nil || number < 1 || number > 65535 {
			return "", fmt.Errorf("porta SSH inválida: %q", fields[1])
		}
		port := strconv.Itoa(number)
		if !slices.Contains(ports, port) {
			ports = append(ports, port)
		}
	}
	if len(ports) == 0 {
		return "", errNoSSHPort
	}
	if len(ports) != 1 {
		return "", fmt.Errorf("foram encontradas portas SSH distintas (%s); revise a configuração antes de continuar", strings.Join(ports, ", "))
	}
	return ports[0], nil
}

func sshPolicy(username, port string) string {
	return "# Gerenciado pelo Dotfiles.\nPort " + port + "\n" +
		"PermitRootLogin no\nPubkeyAuthentication yes\nAuthenticationMethods publickey\n" +
		"PasswordAuthentication no\nKbdInteractiveAuthentication no\nHostbasedAuthentication no\n" +
		"UsePAM yes\nStrictModes yes\nAllowUsers " + username + "\nAuthorizedKeysFile .ssh/authorized_keys\n" +
		"AuthorizedKeysCommand none\nTrustedUserCAKeys none\nSubsystem sftp internal-sftp\n"
}

func verifySSHPolicy(effective, username string) error {
	lines := strings.Split(normalizedSSHConfig(effective), "\n")
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

func writeSSHConfig(path string, contents []byte, mode os.FileMode) error {
	if err := sshRegular(path, true); err != nil {
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

type sshServer struct {
	username, home, config, lockPath string
	connection                       string
	commands                         sshCommands
	recoveryCommands                 sshCommands
	probe                            func(string) error
}

// Não executa shell: valida candidato e política efetiva antes da substituição.
// Em erro após a substituição, restaura o arquivo e o estado de habilitação.
func (s sshServer) apply() (backupPath string, result error) {
	unlock, err := sshLock(s.lockPath)
	if err != nil {
		return "", err
	}
	defer unlock()
	directory, err := os.Lstat(filepath.Join(s.home, ".ssh"))
	if err != nil || !directory.IsDir() {
		return "", errors.New("diretório SSH inválido")
	}
	keyPath := filepath.Join(s.home, ".ssh", "authorized_keys")
	if err := sshRegular(keyPath, false); err != nil {
		return "", err
	}
	keys, err := readAuthorizedKeys(keyPath, s.commands)
	if err != nil {
		return "", err
	}
	if len(keys.indices) == 0 && s.connection != "" {
		return "", errors.New("sem chaves autorizadas, configure pelo console local")
	}
	if err := sshRegular(s.config, false); err != nil {
		return "", err
	}
	for _, state := range []string{"is-active", "is-enabled"} {
		if _, err := s.commands.capture("", "systemctl", state, "--quiet", "sshd.socket"); err == nil {
			return "", errors.New("sshd.socket ativo/habilitado; revise antes de configurar sshd.service")
		}
	}
	_, activeErr := s.commands.capture("", "systemctl", "is-active", "--quiet", "sshd.service")
	wasActive := activeErr == nil
	if _, err := s.commands.capture("", "sshd", "-t"); err != nil {
		return "", err
	}
	effective, dumpErr := dumpSSHConfig(s.commands, []string{"sshd"})
	port, portErr := effectiveSSHPort(effective)
	if dumpErr != nil || errors.Is(portErr, errNoSSHPort) {
		port, err = configuredSSHPort(s.config)
		if err != nil {
			return "", errors.Join(dumpErr, portErr, err)
		}
		fmt.Printf("Porta SSH %s obtida da configuração local (saída efetiva sem porta disponível).\n", port)
	} else if portErr != nil {
		return "", portErr
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
	backup, err := os.MkdirTemp(filepath.Dir(s.config), "dotfiles-ssh-backup.*")
	if err != nil {
		return "", err
	}
	backupPath = backup
	if err := os.WriteFile(filepath.Join(backup, "sshd_config"), original, 0600); err != nil {
		return backup, err
	}
	candidate := filepath.Join(backup, "candidate")
	policy := []byte(sshPolicy(s.username, port))
	if err := os.WriteFile(candidate, policy, 0600); err != nil {
		return backup, err
	}
	if _, err := s.commands.capture("", "sshd", "-t", "-f", candidate); err != nil {
		return backup, err
	}
	address := "127.0.0.1"
	if fields := strings.Fields(s.connection); len(fields) > 0 {
		address = fields[0]
	}
	for _, username := range []string{s.username, "root"} {
		effective, err := dumpSSHConfig(s.commands, []string{"sshd"}, "-f", candidate, "-C", "user="+username+",host=localhost,addr="+address)
		if err != nil {
			return backup, err
		}
		if err := verifySSHPolicy(effective, s.username); err != nil {
			return backup, err
		}
	}
	installed := false
	defer func() {
		if result == nil || !installed {
			return
		}
		restoreErr := writeSSHConfig(s.config, original, originalInfo.Mode().Perm())
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
		action := "stop"
		if wasActive {
			action = "restart"
		}
		_, restartErr := recovery.capture("", "systemctl", action, "sshd.service")
		result = errors.Join(result, restoreErr, restartErr, fmt.Errorf("tentativa de restaurar política anterior; backup: %s", backup))
	}()
	if err := writeSSHConfig(s.config, policy, 0644); err != nil {
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
	probe := s.probe
	if probe == nil {
		probe = waitSSHPort
	}
	if err := probe(port); err != nil {
		return backup, err
	}
	return backup, nil
}

func applySSHServer(connection string) error {
	account, err := sudoAccount()
	if err != nil {
		return err
	}
	username := account.Username
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	server := sshServer{username: username, home: account.HomeDir, connection: connection, config: "/etc/ssh/sshd_config", lockPath: "/run/dotfiles-ssh.lock", commands: sshCommandsWithContext(ctx), recoveryCommands: systemSSHCommands()}
	if _, err := server.commands.capture("", "ssh-keygen", "-A"); err != nil {
		return err
	}
	backup, err := server.apply()
	if err != nil {
		return err
	}
	fmt.Printf("SSH somente por chaves, sem root, habilitado para %s. Backup: %s\nTeste uma NOVA conexão antes de fechar a sessão atual. O firewall não foi alterado.\n", username, backup)
	return nil
}

func sudoAccount() (*user.User, error) {
	username := os.Getenv("SUDO_USER")
	if os.Geteuid() != 0 || username == "root" || !regexp.MustCompile(`^[a-z_][a-z0-9_-]*\$?$`).MatchString(username) {
		return nil, errors.New("execute pelo menu SSH como usuário normal")
	}
	account, err := user.Lookup(username)
	if err != nil {
		return nil, err
	}
	if account.Uid == "0" {
		return nil, errors.New("não é permitido habilitar root")
	}
	return account, nil
}
