package main

// O fluxo interativo roda no próprio binário, fora da TUI. Somente as ferramentas
// do sistema (ssh-keygen, ssh, sudo e systemctl) são executadas como processos.
import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode"
)

var laptopSSHDevices = []string{"id_laptop", "id_desktop", "id_macbook", "id_iphone", "id_android"}

type laptopSSHCommands struct {
	capture     func(input string, argv ...string) (string, error)
	interactive func(argv ...string) error
}

func systemLaptopSSHCommands() laptopSSHCommands {
	return laptopSSHCommandsWithContext(context.Background())
}

func laptopSSHCommandsWithContext(ctx context.Context) laptopSSHCommands {
	return laptopSSHCommands{
		capture: func(input string, argv ...string) (string, error) {
			cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
			cmd.Stdin = strings.NewReader(input)
			output, err := cmd.CombinedOutput()
			if err != nil {
				return "", fmt.Errorf("%s: %w: %s", argv[0], err, strings.TrimSpace(string(output)))
			}
			return string(output), nil
		},
		interactive: func(argv ...string) error {
			cmd := exec.Command(argv[0], argv[1:]...)
			cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
			return cmd.Run()
		},
	}
}

func laptopSSHRegular(path string, missingOK bool) error {
	info, err := os.Lstat(path)
	if missingOK && os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("caminho não regular: %s", path)
	}
	return nil
}

func laptopSSHDirectory(home string) (string, error) {
	directory := filepath.Join(home, ".ssh")
	if err := os.Mkdir(directory, 0700); err != nil && !os.IsExist(err) {
		return "", err
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", errors.New("~/.ssh deve ser um diretório real")
	}
	if owner, ok := info.Sys().(*syscall.Stat_t); !ok || int(owner.Uid) != os.Getuid() {
		return "", errors.New("~/.ssh pertence a outro usuário")
	}
	return directory, os.Chmod(directory, 0700)
}

func laptopSSHLock(path string) (func(), error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = syscall.Close(fd)
		return nil, errors.New("outra operação SSH está em andamento")
	}
	return func() { _ = syscall.Close(fd) }, nil
}

func validateLaptopPublicKey(line string, commands laptopSSHCommands) (string, string, error) {
	if strings.IndexFunc(line, unicode.IsControl) >= 0 {
		return "", "", errors.New("cole uma chave pública em uma única linha")
	}
	fields := strings.Fields(line)
	algorithms := []string{"ssh-ed25519", "ssh-rsa", "ecdsa-sha2-nistp256", "ecdsa-sha2-nistp384", "ecdsa-sha2-nistp521", "sk-ssh-ed25519@openssh.com", "sk-ecdsa-sha2-nistp256@openssh.com"}
	if len(fields) < 2 || !slices.Contains(algorithms, fields[0]) {
		return "", "", errors.New("chave pública sem opções obrigatória; nunca cole a privada")
	}
	fingerprint, err := commands.capture(line+"\n", "ssh-keygen", "-lf", "/dev/stdin")
	if err != nil {
		return "", "", errors.New("chave pública inválida")
	}
	return fields[0] + " " + fields[1], strings.TrimSpace(fingerprint), nil
}

func readLaptopAuthorizedKeys(path string, commands laptopSSHCommands) ([]string, error) {
	if err := laptopSSHRegular(path, true); err != nil {
		return nil, err
	}
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var keys []string
	for _, line := range strings.Split(string(contents), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, err := validateLaptopPublicKey(line, commands); err != nil {
			return nil, err
		}
		keys = append(keys, line)
	}
	return keys, nil
}

func writeLaptopSSHFile(path string, contents []byte) error {
	if err := laptopSSHRegular(path, true); err != nil {
		return err
	}
	return writeAuthorizedKeys(path, contents)
}

func writeLaptopAuthorizedKeys(path string, keys []string) error {
	contents := ""
	if len(keys) != 0 {
		contents = strings.Join(keys, "\n") + "\n"
	}
	return writeLaptopSSHFile(path, []byte(contents))
}

type laptopSSHConsole struct {
	home, username string
	input          *bufio.Reader
	output         io.Writer
	commands       laptopSSHCommands
	probe          func(string) error
}

func (c *laptopSSHConsole) prompt(label string) (string, error) {
	fmt.Fprint(c.output, label)
	line, err := c.input.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (c *laptopSSHConsole) confirm(label, expected string) error {
	answer, err := c.prompt(label)
	if err != nil {
		return err
	}
	if answer != expected {
		return errors.New("operação cancelada")
	}
	return nil
}

func (c *laptopSSHConsole) display(keys []string) error {
	if len(keys) == 0 {
		fmt.Fprintln(c.output, "Nenhuma chave autorizada.")
	}
	for i, key := range keys {
		_, fingerprint, err := validateLaptopPublicKey(key, c.commands)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.output, "%d. %s\n   %s\n", i+1, key, fingerprint)
	}
	return nil
}

func (c *laptopSSHConsole) manageKeys(action string) error {
	directory, err := laptopSSHDirectory(c.home)
	if err != nil {
		return err
	}
	unlock, err := laptopSSHLock(directory)
	if err != nil {
		return err
	}
	defer unlock()
	path := filepath.Join(directory, "authorized_keys")
	keys, err := readLaptopAuthorizedKeys(path, c.commands)
	if err != nil {
		return err
	}
	switch action {
	case "list":
		return c.display(keys)
	case "add":
		fmt.Fprintln(c.output, "Dispositivos: "+strings.Join(laptopSSHDevices, ", "))
		device, err := c.prompt("Nome do dispositivo: ")
		if err != nil {
			return err
		}
		if !slices.Contains(laptopSSHDevices, device) {
			return errors.New("escolha um dos dispositivos listados")
		}
		pasted, err := c.prompt("Cole a chave pública (uma linha): ")
		if err != nil {
			return err
		}
		identity, fingerprint, err := validateLaptopPublicKey(pasted, c.commands)
		if err != nil {
			return err
		}
		for _, key := range keys {
			existing, _, _ := validateLaptopPublicKey(key, c.commands)
			if existing == identity {
				return errors.New("esta chave já está autorizada")
			}
		}
		fmt.Fprintln(c.output, fingerprint)
		if err := c.confirm("Digite ADICIONAR para autorizar: ", "ADICIONAR"); err != nil {
			return err
		}
		if err := writeLaptopAuthorizedKeys(path, append(keys, identity+" "+device)); err != nil {
			return err
		}
		fmt.Fprintln(c.output, "Chave autorizada. Teste o acesso antes de encerrar sua sessão atual.")
	case "remove":
		if err := c.display(keys); err != nil {
			return err
		}
		if len(keys) == 0 {
			return nil
		}
		if len(keys) == 1 {
			return errors.New("não é permitido remover a última chave; adicione e teste outra primeiro")
		}
		answer, err := c.prompt("Número da chave a remover: ")
		if err != nil {
			return err
		}
		number, err := strconv.Atoi(answer)
		if err != nil || number < 1 || number > len(keys) {
			return errors.New("número inválido")
		}
		_, fingerprint, _ := validateLaptopPublicKey(keys[number-1], c.commands)
		fmt.Fprintln(c.output, fingerprint+"\nMantenha a sessão atual aberta e confirme acesso por outra chave.")
		if err := c.confirm("Digite REMOVER para confirmar: ", "REMOVER"); err != nil {
			return err
		}
		keys = append(keys[:number-1], keys[number:]...)
		if err := writeLaptopAuthorizedKeys(path, keys); err != nil {
			return err
		}
		fmt.Fprintln(c.output, "Chave removida.")
	default:
		return errors.New("ação SSH inválida")
	}
	return nil
}

func configureLaptopSSHClient(directory string) error {
	path := filepath.Join(directory, "config")
	if err := laptopSSHRegular(path, true); err != nil {
		return err
	}
	original, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	previous := string(original)
	const start, end = "# BEGIN DOTFILES LAPTOP", "# END DOTFILES LAPTOP"
	if strings.Contains(previous, start) || strings.Contains(previous, end) {
		if strings.Count(previous, start) != 1 || strings.Count(previous, end) != 1 {
			return errors.New("bloco SSH gerenciado inválido")
		}
		before, rest, _ := strings.Cut(previous, start)
		_, after, ok := strings.Cut(rest, end)
		if !ok {
			return errors.New("bloco SSH gerenciado inválido")
		}
		previous = before + strings.TrimLeft(after, "\n")
	}
	if err == nil {
		backup, err := reserveBackupAt(localBackupRoot(), "Legacy-SSH", path, time.Now())
		if err != nil {
			return err
		}
		if err := writeLaptopSSHFile(backup, original); err != nil {
			return err
		}
	}
	block := start + "\nHost *\n    IdentityFile ~/.ssh/id_laptop\n    IdentitiesOnly yes\n    AddKeysToAgent yes\n" + end + "\n"
	return writeLaptopSSHFile(path, []byte(block+previous))
}

func (c *laptopSSHConsole) configureIdentity() error {
	directory, err := laptopSSHDirectory(c.home)
	if err != nil {
		return err
	}
	unlock, err := laptopSSHLock(directory)
	if err != nil {
		return err
	}
	defer unlock()
	private := filepath.Join(directory, "id_laptop")
	public := private + ".pub"
	for _, path := range []string{private, public} {
		if err := laptopSSHRegular(path, true); err != nil {
			return err
		}
	}
	if _, err := os.Stat(private); err == nil {
		contents, err := os.ReadFile(public)
		if err != nil {
			return errors.New("id_laptop existe sem .pub; recupere a pública sem sobrescrever a privada")
		}
		if _, _, err := validateLaptopPublicKey(strings.TrimSpace(string(contents)), c.commands); err != nil {
			return err
		}
		if err := os.Chmod(private, 0600); err != nil {
			return err
		}
		fmt.Fprintln(c.output, "id_laptop já existe; chave preservada.")
	} else if !os.IsNotExist(err) {
		return err
	} else {
		if _, err := os.Stat(public); !os.IsNotExist(err) {
			return errors.New("id_laptop.pub já existe ou não pôde ser lida; nada será sobrescrito")
		}
		email, err := c.prompt("E-mail para identificar a chave: ")
		if err != nil {
			return err
		}
		address, err := mail.ParseAddress(email)
		if err != nil || address.Address != email || address.Name != "" || strings.IndexFunc(email, unicode.IsControl) >= 0 {
			return errors.New("e-mail inválido")
		}
		fmt.Fprintln(c.output, "Defina e confirme a passphrase no prompt do ssh-keygen.")
		if err := c.commands.interactive("ssh-keygen", "-t", "ed25519", "-C", email, "-f", private); err != nil {
			return err
		}
	}
	if err := configureLaptopSSHClient(directory); err != nil {
		return err
	}
	publicContents, err := os.ReadFile(public)
	if err != nil {
		return err
	}
	fmt.Fprintln(c.output, "Chave pública deste laptop:\n"+strings.TrimSpace(string(publicContents)))
	keys, err := readLaptopAuthorizedKeys(filepath.Join(directory, "authorized_keys"), c.commands)
	if err != nil {
		return err
	}
	path := filepath.Join(directory, "authorized_keys")
	if len(keys) == 0 {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return writeLaptopAuthorizedKeys(path, nil)
		}
	}
	return os.Chmod(path, 0600)
}

func (c *laptopSSHConsole) configure() error {
	root := localBackupRoot()
	if err := validateBackupDirectoryPath(root); err != nil {
		return err
	}
	if !backupDestinationReady(root, os.Getuid(), os.Getgid()) {
		args := backupDestinationSetupArgs(root, os.Getuid(), os.Getgid())
		if err := c.commands.interactive(append([]string{"sudo"}, args...)...); err != nil {
			return err
		}
	}
	if err := c.configureIdentity(); err != nil {
		return err
	}
	keys, err := readLaptopAuthorizedKeys(filepath.Join(c.home, ".ssh", "authorized_keys"), c.commands)
	if err != nil {
		return err
	}
	if os.Getenv("SSH_CONNECTION") != "" && len(keys) == 0 {
		return errors.New("sem chaves autorizadas, configure pelo console local para não perder acesso")
	}
	fmt.Fprintln(c.output, "O SSH será configurado e iniciado somente por chave, sem senha e sem root. O firewall não será alterado.")
	if len(keys) == 0 {
		fmt.Fprintln(c.output, "Nenhuma chave autorizada: a porta ficará escutando, mas ninguém poderá autenticar até adicionar uma pública.")
	}
	fmt.Fprintln(c.output, "A configuração substitui /etc/ssh/sshd_config, com backup. Tenha console/recuperação disponível; não é obrigatório testar login remoto.")
	if err := c.confirm("Digite CONFIRMO para configurar: ", "CONFIRMO"); err != nil {
		return err
	}
	if err := c.commands.interactive("sudo", "ssh-keygen", "-A"); err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return c.commands.interactive("sudo", executable, "--laptop-ssh", "apply-server")
}

func (c *laptopSSHConsole) verify() error {
	directory := filepath.Join(c.home, ".ssh")
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0077 != 0 {
		return errors.New("diretório SSH inválido ou com permissões inseguras")
	}
	if owner, ok := info.Sys().(*syscall.Stat_t); !ok || int(owner.Uid) != os.Getuid() {
		return errors.New("proprietário do diretório SSH incorreto")
	}
	for _, name := range []string{"id_laptop", "id_laptop.pub", "authorized_keys", "config"} {
		path := filepath.Join(directory, name)
		if err := laptopSSHRegular(path, false); err != nil {
			return err
		}
		info, err := os.Stat(path)
		if err != nil {
			return err
		}
		if owner, ok := info.Sys().(*syscall.Stat_t); !ok || int(owner.Uid) != os.Getuid() {
			return errors.New("proprietário de arquivo SSH incorreto")
		}
		if (name == "id_laptop" || name == "authorized_keys") && info.Mode().Perm()&0077 != 0 {
			return fmt.Errorf("permissões SSH inseguras: %s", name)
		}
	}
	public, err := os.ReadFile(filepath.Join(directory, "id_laptop.pub"))
	if err != nil {
		return err
	}
	if _, _, err := validateLaptopPublicKey(strings.TrimSpace(string(public)), c.commands); err != nil {
		return err
	}
	if _, err := readLaptopAuthorizedKeys(filepath.Join(directory, "authorized_keys"), c.commands); err != nil {
		return err
	}
	client, err := c.commands.capture("", "ssh", "-G", "-F", filepath.Join(directory, "config"), "localhost")
	if err != nil {
		return err
	}
	if !slices.Contains(strings.Split(client, "\n"), "identityfile ~/.ssh/id_laptop") {
		return errors.New("cliente SSH não usa id_laptop")
	}
	address := "127.0.0.1"
	if fields := strings.Fields(os.Getenv("SSH_CONNECTION")); len(fields) > 0 {
		address = fields[0]
	}
	if _, err := c.commands.capture("", "sudo", "-n", "sshd", "-t"); err != nil {
		return err
	}
	effective, err := dumpLaptopSSHConfig(c.commands, []string{"sudo", "-n", "sshd"}, "-C", "user="+c.username+",host=localhost,addr="+address)
	if err != nil {
		return err
	}
	if err := verifyLaptopSSHPolicy(effective, c.username); err != nil {
		return err
	}
	for _, state := range []string{"is-active", "is-enabled"} {
		if _, err := c.commands.capture("", "systemctl", state, "--quiet", "sshd.service"); err != nil {
			return err
		}
		if _, err := c.commands.capture("", "systemctl", state, "--quiet", "sshd.socket"); err == nil {
			return errors.New("sshd.socket deve estar inativo e desabilitado")
		}
	}
	port, err := effectiveLaptopSSHPort(effective)
	if errors.Is(err, errNoLaptopSSHPort) {
		port, err = configuredLaptopSSHPort("/etc/ssh/sshd_config")
	}
	if err != nil {
		return err
	}
	probe := c.probe
	if probe == nil {
		probe = probeLaptopSSHPort
	}
	if err := probe(port); err != nil {
		return err
	}
	fmt.Fprintln(c.output, "Pré-requisitos SSH verificados; porta escutando.")
	return nil
}

func laptopSSHProcessStep(title, action string) step {
	executable, err := os.Executable()
	if err != nil {
		return nativeStep(title, func() error { return err })
	}
	return step{label: title, cmd: func() *exec.Cmd {
		return exec.Command(executable, "--laptop-ssh", action)
	}}
}

func runLaptopSSHCommand(action string) error {
	if action == "apply-server" {
		return applyLaptopSSHServer()
	}
	if os.Geteuid() == 0 {
		return errors.New("execute como usuário normal, não com sudo")
	}
	hostname, err := os.Hostname()
	if err != nil {
		return err
	}
	device, err := detectDevice(hostname)
	if err != nil || device != deviceLaptop {
		return errors.New("ação exclusiva do laptop Omarchy")
	}
	if err := checkPlatformForDevice(device, "/etc/os-release", exec.LookPath); err != nil {
		return err
	}
	account, err := user.Current()
	if err != nil {
		return err
	}
	c := &laptopSSHConsole{home: account.HomeDir, username: account.Username, input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemLaptopSSHCommands()}
	switch action {
	case "save-backup":
		return writeLaptopSSHDBackup()
	case "configure":
		err = c.configure()
	case "add", "list", "remove":
		err = c.manageKeys(action)
	case "verify":
		err = c.verify()
		if err == nil {
			return nil
		}
	default:
		return errors.New("ação SSH inválida")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "SSH:", err)
	}
	_, _ = c.prompt("Enter para voltar...")
	return err
}
