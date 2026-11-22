package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

func runNetworkAction(device profile, args []string) error {
	if device.sshKeyName() == "" {
		return errors.New("SSH e firewall disponíveis apenas nos perfis Desktop e Laptop")
	}
	if len(args) == 2 {
		switch args[0] {
		case "apply-server":
			return applySSHServer(args[1])
		case "apply-firewall":
			return applyFirewall(args[1])
		}
	}
	if len(args) != 1 || os.Geteuid() == 0 {
		return errors.New("execute a ação pelo menu como usuário normal")
	}
	account, err := user.Current()
	if err != nil {
		return err
	}
	console := sshConsole{
		home: account.HomeDir, keyName: device.sshKeyName(),
		input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands(),
	}
	printActionHeader()
	switch args[0] {
	case "configure":
		err = console.configure()
	case "generate":
		err = console.generate()
	case "add", "list", "remove":
		err = console.ensurePackages("openssh")
		if err == nil {
			err = console.manageKeys(args[0])
		}
	case "firewall":
		err = console.firewall()
	default:
		return errors.New("ação inválida")
	}
	if err != nil {
		fmt.Fprintln(console.output, err)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func (c *sshConsole) ensurePackages(packages ...string) error {
	var missing []string
	for _, name := range packages {
		if _, err := c.commands.capture("", "pacman", "-Q", name); err != nil {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return c.commands.interactive(append([]string{"sudo", "pacman", "-S", "--needed", "--"}, missing...)...)
}

func (c *sshConsole) privileged(action string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return c.commands.interactive("sudo", "--", executable, "--network", action, os.Getenv("SSH_CONNECTION"))
}

func (c *sshConsole) generate() error {
	if err := c.ensurePackages("openssh"); err != nil {
		return err
	}
	return c.generateIdentity()
}

func (c *sshConsole) generateIdentity() error {
	files, err := prepareSSHKey(c.home, c.keyName)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.output, "Defina a passphrase da chave no prompt do OpenSSH.")
	command := files.command()
	result := files.finish(c.commands.interactive(command.Args...))
	if result.err == nil {
		fmt.Fprintf(c.output, "Gerada ~/.ssh/%s. Compartilhe apenas ~/.ssh/%s.pub.\n", result.keyName, result.keyName)
	}
	return result.err
}

func clientSSHConfig(keyName string) string {
	return "# Gerenciado pelo Dotfiles.\nHost *\n" +
		"    IdentityFile ~/.ssh/" + keyName + "\n" +
		"    IdentitiesOnly yes\n    IdentityAgent /run/user/%i/ssh-agent.socket\n" +
		"    AddKeysToAgent 8h\n" +
		"    PreferredAuthentications publickey\n    ForwardAgent no\n"
}

func (c *sshConsole) enableSSHAgent() error {
	const socket = "ssh-agent.socket"
	if _, err := c.commands.capture("", "systemctl", "--user", "enable", "--now", socket); err != nil {
		return fmt.Errorf("habilitar o agente SSH do usuário: %w", err)
	}
	if _, err := c.commands.capture("", "systemctl", "--user", "is-active", "--quiet", socket); err != nil {
		return fmt.Errorf("o socket do agente SSH não está ativo: %w", err)
	}
	return nil
}

func (c *sshConsole) configure() error {
	fmt.Fprintln(c.output, "Esta ação configura uma identidade no cliente e habilita o sshd somente por chave para seu usuário. Os arquivos anteriores recebem backup.")
	if err := c.confirm("Digite CONFIRMO para configurar: ", "CONFIRMO"); err != nil {
		return err
	}
	clientBackup := managedBackup("SSH", c.commands.interactive)
	for _, location := range []backupLocation{clientBackup, managedBackup("SSHD", c.commands.interactive)} {
		if err := location.prepare(); err != nil {
			return err
		}
	}
	if err := c.ensurePackages("openssh"); err != nil {
		return err
	}
	directory, err := sshDirectory(c.home)
	if err != nil {
		return err
	}
	unlock, err := sshLock(filepath.Join(directory, ".dotfiles-keys.lock"))
	if err != nil {
		return err
	}
	defer unlock()

	authorized := filepath.Join(directory, "authorized_keys")
	keys, err := readAuthorizedKeys(authorized, c.commands)
	if err != nil {
		return err
	}
	if len(keys.indices) == 0 && os.Getenv("SSH_CONNECTION") != "" {
		return errors.New("sem chaves autorizadas, configure pelo console local")
	}
	client := filepath.Join(directory, "config")
	if err := sshRegular(client, true); err != nil {
		return err
	}
	if err := c.ensureIdentity(directory); err != nil {
		return err
	}
	if err := writeAuthorizedKeys(authorized, keys.lines); err != nil {
		return err
	}
	applyClient, err := c.prepareClient(client)
	if err != nil {
		return err
	}
	defer applyClient.cleanup()
	if err := c.enableSSHAgent(); err != nil {
		return err
	}
	if err := c.privileged("apply-server"); err != nil {
		return err
	}
	backup, err := applyClient.install(clientBackup)
	if backup != "" {
		fmt.Fprintln(c.output, "Backup do cliente SSH:", backup)
	}
	return err
}

func (c *sshConsole) ensureIdentity(directory string) error {
	private := filepath.Join(directory, c.keyName)
	public := private + ".pub"
	if err := sshRegular(private, false); errors.Is(err, os.ErrNotExist) {
		return c.generateIdentity()
	} else if err != nil {
		return err
	}
	if err := sshRegular(public, false); err != nil {
		return fmt.Errorf("a chave %s existente foi preservada; verifique sua pública: %w", c.keyName, err)
	}
	data, err := os.ReadFile(public)
	if err != nil {
		return err
	}
	if _, _, err := validatePublicKey(strings.TrimSpace(string(data)), c.commands); err != nil {
		return err
	}
	return os.Chmod(private, 0600)
}

type preparedClient struct {
	directory, candidate, target string
}

func (c *sshConsole) prepareClient(path string) (preparedClient, error) {
	dir, err := os.MkdirTemp(filepath.Dir(path), ".dotfiles-client-")
	if err != nil {
		return preparedClient{}, err
	}
	p := preparedClient{directory: dir, candidate: filepath.Join(dir, "config"), target: path}
	if err := os.WriteFile(p.candidate, []byte(clientSSHConfig(c.keyName)), 0600); err != nil {
		p.cleanup()
		return preparedClient{}, err
	}
	if _, err := c.commands.capture("", "ssh", "-G", "-F", p.candidate, "localhost"); err != nil {
		p.cleanup()
		return preparedClient{}, err
	}
	return p, nil
}

func (p preparedClient) cleanup() { _ = os.RemoveAll(p.directory) }

func (p preparedClient) install(location backupLocation) (string, error) {
	if err := sshRegular(p.target, true); err != nil {
		return "", err
	}
	var backup string
	original, err := os.ReadFile(p.target)
	if err == nil {
		candidate, err := os.ReadFile(p.candidate)
		if err != nil || string(original) == string(candidate) {
			return "", err
		}
		backup, err = location.saveFile("config.backup-*", original)
		if err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := sshRegular(p.target, true); err != nil {
		return backup, err
	}
	return backup, os.Rename(p.candidate, p.target)
}

func (c *sshConsole) firewall() error {
	lan, err := detectLAN()
	if err != nil {
		return err
	}
	if err := lan.checkConnection(os.Getenv("SSH_CONNECTION")); err != nil {
		return err
	}
	fmt.Fprintf(c.output, "O UFW permitirá SSH de entrada apenas pela LAN %s (%s) e manterá outras regras.\n", lan.subnet, lan.device)
	if err := c.confirm("Digite CONFIRMO para aplicar o firewall: ", "CONFIRMO"); err != nil {
		return err
	}
	// Validate SSH before installing or modifying the firewall.
	if err := c.commands.interactive("sudo", "-v"); err != nil {
		return err
	}
	if err := c.commands.interactive("sudo", "systemctl", "is-active", "--quiet", "sshd.service"); err != nil {
		return errors.New("configure o SSH antes do firewall")
	}
	if err := c.ensurePackages("ufw", "iptables-nft"); err != nil {
		return err
	}
	return c.privileged("apply-firewall")
}

func printActionHeader() {
	if title := os.Getenv("DOTFILES_ACTION_TITLE"); title != "" {
		fmt.Printf("\x1b[2J\x1b[H\n  \x1b[1;35m━━ %s ━━\x1b[0m\n\n", title)
	}
}

func (p profile) menuAction(action string) string {
	if p.prefix == "homelab" {
		switch action {
		case "configure":
			return "homelab-ssh"
		case "firewall":
			return "homelab-firewall"
		}
	}
	return action
}

func actionCommand(action, title string) (*exec.Cmd, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	args := []string{"--network", action}
	switch action {
	case "homelab-ssh", "homelab-firewall", "homelab-casaos", "homelab-dns", "homelab-swap":
		args = []string{"--homelab", strings.TrimPrefix(action, "homelab-")}
	case "cursor":
		args = []string{"--cursor"}
	case "firefox-backup":
		args = []string{"--firefox-backup"}
	case "sdk-links":
		args = []string{"--sdk-links"}
	case "setup-git":
		args = []string{"--setup-git"}
	case "setup-mise":
		args = []string{"--setup-mise"}
	case "setup-fish", "setup-fastfetch":
		args = []string{"--terminal-config", strings.TrimPrefix(action, "setup-")}
	case "essential-tools", "install-yay", "terminal-tools", "install-kitty", "drivers", "proton-pass":
		args = []string{"--packages", action}
	}
	command := exec.Command(executable, args...)
	command.Env = append(command.Environ(), "DOTFILES_ACTION_TITLE="+title)
	return command, nil
}
