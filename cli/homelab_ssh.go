package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	homelabSSHBegin = "# BEGIN Dotfiles Homelab SSH\n"
	homelabSSHEnd   = "# END Dotfiles Homelab SSH\n"
)

func configureHomelabSSH(c *sshConsole, username string) error {
	fmt.Fprintln(c.output, "Instalar OpenSSH, autorizar uma chave Ed25519 para teste e restringir o login após confirmação.")
	if err := c.confirm("Digite CONFIRMO para preparar o SSH: ", "CONFIRMO"); err != nil {
		return err
	}
	if err := c.ensureUbuntuPackages("openssh-server", "openssh-client"); err != nil {
		return err
	}
	key, err := c.prompt("Cole a chave pública Ed25519 do seu computador em uma linha: ")
	if err != nil {
		return err
	}
	identity, fingerprint, err := validatePublicKey(key, c.commands)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(identity, "ssh-ed25519 ") {
		return errors.New("o Homelab exige uma chave pública Ed25519")
	}
	fmt.Fprintln(c.output, fingerprint)
	directory, err := sshDirectory(c.home)
	if err != nil {
		return err
	}
	unlock, err := lockDirectory(directory, "SSH")
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(directory, "authorized_keys")
	if err := sshRegular(path, true); err != nil {
		return err
	}
	original, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err == nil && !bytes.Equal(original, []byte(key+"\n")) {
		backup, err := managedBackup("SSH", c.commands.interactive).saveFile("authorized_keys.backup-*", original)
		if err != nil {
			return err
		}
		fmt.Fprintln(c.output, "Backup das chaves autorizadas:", backup)
	}
	if err := c.homelabPrivileged("prepare-ssh"); err != nil {
		return err
	}
	tested := original
	if !slices.Contains(strings.Split(string(original), "\n"), key) {
		if len(tested) > 0 && tested[len(tested)-1] != '\n' {
			tested = append(tested, '\n')
		}
		tested = append(tested, key+"\n"...)
	}
	if err := writeConfigAtomic(path, tested, 0600); err != nil {
		return err
	}
	fmt.Fprintf(c.output, "Mantenha esta sessão aberta. Em outro terminal, conecte como %s com a chave privada correspondente e execute sudo -v.\n", username)
	fmt.Fprintln(c.output, "No teste use IdentitiesOnly=yes, IdentityAgent=none, PasswordAuthentication=no e KbdInteractiveAuthentication=no.")
	if err := c.confirm("Após testar, digite CONFIRMO para manter só esta chave e exigir autenticação por chave: ", "CONFIRMO"); err != nil {
		return fmt.Errorf("a chave foi adicionada para teste: %w", err)
	}
	if err := managedBackup("SSHD", c.commands.interactive).prepare(); err != nil {
		return err
	}
	if err := writeAuthorizedKeys(path, []string{key}); err != nil {
		return err
	}
	if err := c.homelabPrivileged("harden-ssh"); err != nil {
		return errors.Join(err, writeConfigAtomic(path, tested, 0600))
	}
	return nil
}

func prepareHomelabSSH(commands sshCommands) error {
	if _, err := commands.capture("", "install", "-d", "-m", "0755", "--", "/run/sshd"); err != nil {
		return err
	}
	if _, err := commands.capture("", "sshd", "-t"); err != nil {
		return err
	}
	unit := "ssh.service"
	if isActive("ssh.socket", commands) || isEnabled("ssh.socket", commands) {
		unit = "ssh.socket"
	}
	_, err := commands.capture("", "systemctl", "enable", "--now", unit)
	return err
}

func homelabSSHConfig(original []byte, username string) ([]byte, error) {
	remaining := string(original)
	if after, ok := strings.CutPrefix(remaining, homelabSSHBegin); ok {
		_, rest, found := strings.Cut(after, homelabSSHEnd)
		if !found {
			return nil, errors.New("bloco SSH do Dotfiles incompleto no sshd_config")
		}
		remaining = rest
	}
	policy := "PermitRootLogin no\nPubkeyAuthentication yes\nAuthenticationMethods publickey\n" +
		"PasswordAuthentication no\nKbdInteractiveAuthentication no\nAllowUsers " + username + "\n" +
		"AuthorizedKeysFile .ssh/authorized_keys\nAuthorizedKeysCommand none\nTrustedUserCAKeys none\n"
	return []byte(homelabSSHBegin + policy + homelabSSHEnd + remaining), nil
}

func verifyHomelabSSH(effective, username string) error {
	var policy []string
	allowed := false
	for line := range strings.SplitSeq(normalizedSSHConfig(effective), "\n") {
		if usernames, ok := strings.CutPrefix(line, "allowusers "); ok {
			for _, name := range strings.Fields(usernames) {
				if name != username {
					return errors.New("AllowUsers existente autoriza outro usuário")
				}
				allowed = true
			}
			continue
		}
		policy = append(policy, line)
	}
	if allowed {
		policy = append(policy, "allowusers "+username)
	}
	return verifySSHPolicy(strings.Join(policy, "\n"), username)
}

func hardenHomelabSSH(path, username, connection string, commands, recovery sshCommands, saveBackup func([]byte) (string, error)) error {
	remote, err := parseHomelabConnection(connection)
	if err != nil {
		return err
	}
	if err := sshRegular(path, false); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	original, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	policy, err := homelabSSHConfig(original, username)
	if err != nil {
		return err
	}
	staging, err := os.MkdirTemp(filepath.Dir(path), ".dotfiles-sshd-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging)
	candidate := filepath.Join(staging, "candidate")
	if err := os.WriteFile(candidate, policy, 0600); err != nil {
		return err
	}
	if _, err := commands.capture("", "sshd", "-t", "-f", candidate); err != nil {
		return err
	}
	for _, user := range []string{username, "root"} {
		spec := "user=" + user + ",host=localhost,addr=127.0.0.1"
		if remote.client != "" {
			spec = "user=" + user + ",host=homelab,addr=" + remote.client + ",laddr=" + remote.server + ",lport=" + remote.port
		}
		effective, err := dumpSSHConfig(commands, []string{"sshd"}, "-f", candidate, "-C", spec)
		if err != nil {
			return err
		}
		if err := verifyHomelabSSH(effective, username); err != nil {
			return err
		}
	}
	backup, err := saveBackup(original)
	if err != nil {
		return err
	}
	if err := writeConfigAtomic(path, policy, info.Mode().Perm()); err != nil {
		return err
	}
	for _, action := range []string{"start", "reload"} {
		if _, err := commands.capture("", "systemctl", action, "ssh.service"); err != nil {
			restoreErr := writeConfigAtomic(path, original, info.Mode().Perm())
			_, reloadErr := recovery.capture("", "systemctl", "reload", "ssh.service")
			return errors.Join(err, restoreErr, reloadErr, fmt.Errorf("configuração SSH anterior em %s", backup))
		}
	}
	fmt.Printf("SSH por chave configurado. Backup em %s. Teste uma nova conexão antes de fechar esta sessão.\n", backup)
	return nil
}
