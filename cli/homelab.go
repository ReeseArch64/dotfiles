package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
)

const casaOSInstallerURL = "https://get.casaos.io"

func ubuntuRelease(data string) bool {
	return osReleaseID(data) == "ubuntu"
}

func runHomelabAction(device profile, args []string, console sshConsole) error {
	if device.prefix != "homelab" {
		return errors.New("ação disponível apenas no perfil Homelab")
	}
	release, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return err
	}
	if !ubuntuRelease(string(release)) {
		return errors.New("as ações do Homelab exigem Ubuntu (ID=ubuntu em /etc/os-release)")
	}
	if len(args) == 2 {
		if args[0] == "save-backup" {
			return writeHomelabBackup(args[1])
		}
		return runPrivilegedHomelab(args[0], args[1])
	}
	if len(args) != 1 || os.Geteuid() == 0 {
		return errors.New("execute a ação pelo menu Homelab como usuário administrador sem sudo")
	}
	account, err := user.Current()
	if err != nil {
		return err
	}
	console.home = account.HomeDir
	switch args[0] {
	case "ssh":
		err = configureHomelabSSH(&console, account.Username)
	case "firewall":
		err = configureHomelabFirewall(&console)
	case "casaos":
		err = installCasaOS(&console, account.Username)
	case "dns":
		err = configureHomelabSystem(&console, "DNS", "apply-dns", "Desativar o stub DNS do systemd-resolved e preservar os servidores DNS.")
	case "swap":
		err = configureHomelabSystem(&console, "Swap", "apply-swap", "Ativar 2 GiB de swap em /swapfile-homelab e registrar no /etc/fstab.")
	case "fish", "fastfetch":
		err = configureHomelabTerminal(&console, account.Username, args[0] == "fish")
	case "tools":
		err = installHomelabTools(&console, exec.LookPath)
	default:
		return errors.New("ação Homelab inválida")
	}
	return err
}

func runPrivilegedHomelab(action, connection string) error {
	account, err := sudoAccount()
	if err != nil {
		return err
	}
	lock, operation := "system-ssh", "SSH e firewall"
	switch action {
	case "apply-dns":
		lock, operation = "system-dns", "DNS"
	case "apply-swap":
		lock, operation = "system-swap", "swap"
	}
	unlock, err := lockOperation(lock, operation)
	if err != nil {
		return err
	}
	defer unlock()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()
	commands := sshCommandsWithContext(ctx)
	switch action {
	case "prepare-ssh":
		return prepareHomelabSSH(commands)
	case "harden-ssh":
		backup := func(data []byte) (string, error) { return backupSSHDAsUser(ctx, account, data) }
		return hardenHomelabSSH("/etc/ssh/sshd_config", account.Username, connection, commands, systemSSHCommands(), backup)
	case "apply-dns":
		return homelabDNSPaths().apply(commands, systemSSHCommands(), homelabBackupAsUser(ctx, account, "DNS"))
	case "apply-swap":
		return applyHomelabSwap(ctx, "/etc/fstab", "/swapfile-homelab", 2*gibibyte, commands, systemSSHCommands(), homelabBackupAsUser(ctx, account, "Swap"))
	case "apply-firewall":
		lan, err := detectLAN()
		if err != nil {
			return err
		}
		f := firewall{"/etc/ufw", "/etc/default/ufw", commands, systemSSHCommands()}
		if err := f.applyHomelab(lan, connection); err != nil {
			return err
		}
		fmt.Printf("UFW ativo. LAN %s liberada pela interface %s.\n", lan.subnet, lan.device)
		return nil
	default:
		return errors.New("ação administrativa Homelab inválida")
	}
}

func configureHomelabSystem(c *sshConsole, category, action, description string) error {
	location := managedBackup(category, c.commands.interactive)
	fmt.Fprintln(c.output, description)
	fmt.Fprintln(c.output, "Arquivos substituídos recebem backup em", location.path)
	if action == "apply-dns" {
		if err := c.confirm("Alterar a resolução DNS do servidor?"); err != nil {
			return err
		}
	}
	if err := location.prepare(); err != nil {
		return err
	}
	return c.homelabPrivileged(action)
}

func (c *sshConsole) homelabPrivileged(action string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return c.commands.interactive("sudo", "--", executable, "--homelab", action, os.Getenv("SSH_CONNECTION"))
}

func (c *sshConsole) ensureUbuntuPackages(packages ...string) error {
	var missing []string
	for _, name := range packages {
		status, err := c.commands.capture("", "dpkg-query", "--show", "--showformat=${Status}", "--", name)
		if err != nil || !strings.HasSuffix(strings.TrimSpace(status), " ok installed") {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := c.commands.interactive("sudo", "apt-get", "update"); err != nil {
		return err
	}
	return c.commands.interactive(append([]string{"sudo", "apt-get", "install", "--yes", "--"}, missing...)...)
}

func installCasaOS(c *sshConsole, username string) error {
	active := isActive("casaos.service", c.commands)
	if active {
		fmt.Fprintln(c.output, "CasaOS já está ativo. A ação prepara o acesso ao grupo docker para", username)
	} else {
		fmt.Fprintln(c.output, "Instalar CasaOS com o script oficial de", casaOSInstallerURL, "e adicionar", username, "ao grupo docker.")
	}
	if !active {
		if err := c.ensureUbuntuPackages("curl", "ca-certificates"); err != nil {
			return err
		}
		work, err := os.MkdirTemp("", "dotfiles-casaos-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(work)
		path := filepath.Join(work, "install.sh")
		if err := c.commands.interactive("curl", "-fsSL", "--proto", "=https", "--proto-redir", "=https", "--output", path, casaOSInstallerURL); err != nil {
			return fmt.Errorf("baixar instalador do CasaOS: %w", err)
		}
		if info, err := os.Stat(path); err != nil || info.Size() == 0 {
			return errors.New("o download do instalador do CasaOS não produziu um arquivo válido")
		}
		if _, err := c.commands.capture("", "bash", "-n", "--", path); err != nil {
			return fmt.Errorf("validar instalador do CasaOS: %w", err)
		}
		if err := c.commands.interactive("sudo", "bash", "--", path); err != nil {
			return fmt.Errorf("instalar CasaOS: %w", err)
		}
	}
	if err := c.commands.interactive("sudo", "usermod", "-aG", "docker", "--", username); err != nil {
		return fmt.Errorf("adicionar %s ao grupo docker: %w", username, err)
	}
	fmt.Fprintln(c.output, "CasaOS disponível na LAN. Abra uma nova sessão para usar o Docker com o grupo atualizado.")
	return nil
}
