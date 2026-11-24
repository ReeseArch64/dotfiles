package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unicode"
)

type sshCommands struct {
	capture     func(input string, argv ...string) (string, error)
	interactive func(argv ...string) error
	terminal    func(*exec.Cmd) error
}

func runInTerminal(cmd *exec.Cmd) error {
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func systemSSHCommands() sshCommands {
	return sshCommandsWithContext(context.Background())
}

func sshCommandsWithContext(ctx context.Context) sshCommands {
	return sshCommands{
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
			return runInTerminal(exec.Command(argv[0], argv[1:]...))
		},
		terminal: runInTerminal,
	}
}

func sshRegular(path string, missingOK bool) error {
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

func sshDirectory(home string) (string, error) {
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

func sshLock(key string) (func(), error) {
	return lockOperation(key, "SSH")
}

func validatePublicKey(line string, commands sshCommands) (string, string, error) {
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

type authorizedKeys struct {
	lines   []string
	indices []int
}

func readAuthorizedKeys(path string, commands sshCommands) (authorizedKeys, error) {
	var result authorizedKeys
	if err := sshRegular(path, true); err != nil {
		return result, err
	}
	contents, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return result, nil
	}
	if err != nil {
		return result, err
	}
	if len(contents) > 0 {
		result.lines = strings.Split(strings.TrimSuffix(string(contents), "\n"), "\n")
	}
	for i, line := range result.lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if _, _, err := validatePublicKey(line, commands); err != nil {
			return result, fmt.Errorf("authorized_keys, linha %d: %w", i+1, err)
		}
		result.indices = append(result.indices, i)
	}
	return result, nil
}

func writeAuthorizedKeys(path string, lines []string) error {
	var content string
	if len(lines) > 0 {
		content = strings.Join(lines, "\n") + "\n"
	}
	return writeConfigAtomic(path, []byte(content), 0600)
}

type promptKind int

const (
	promptText promptKind = iota
	promptConfirm
	promptContinue
)

var errActionCanceled = errors.New("operação cancelada")

type sshConsole struct {
	home, keyName string
	input         *bufio.Reader
	output        io.Writer
	commands      sshCommands
	ask           func(promptKind, string) (string, error)
	withTerminal  func(func() error) error
}

func terminalConsole() sshConsole {
	return sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
}

func (c *sshConsole) prompt(label string) (string, error) {
	if c.ask != nil {
		return c.ask(promptText, label)
	}
	fmt.Fprint(c.output, label)
	line, err := c.input.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

func (c *sshConsole) confirm(label string) error {
	if c.ask != nil {
		_, err := c.ask(promptConfirm, label)
		return err
	}
	answer, err := c.prompt(label + " [s/N] ")
	if err != nil {
		return err
	}
	if !strings.EqualFold(answer, "s") {
		return errActionCanceled
	}
	return nil
}

func (c *sshConsole) proceed(label string) error {
	if c.ask != nil {
		_, err := c.ask(promptContinue, label)
		return err
	}
	_, err := c.prompt(label + " Pressione Enter para continuar… ")
	return err
}

func (c *sshConsole) display(keys authorizedKeys) error {
	if len(keys.indices) == 0 {
		fmt.Fprintln(c.output, "Nenhuma chave autorizada.")
	}
	for i, index := range keys.indices {
		key := strings.TrimSpace(keys.lines[index])
		_, fingerprint, err := validatePublicKey(key, c.commands)
		if err != nil {
			return err
		}
		fmt.Fprintf(c.output, "%d. %s\n   %s\n", i+1, key, fingerprint)
	}
	return nil
}

func (c *sshConsole) manageKeys(action string) error {
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
	keys, err := readAuthorizedKeys(path, c.commands)
	if err != nil {
		return err
	}
	switch action {
	case "list":
		return c.display(keys)
	case "add":
		pasted, err := c.prompt("Cole a chave pública (uma linha): ")
		if err != nil {
			return err
		}
		identity, fingerprint, err := validatePublicKey(pasted, c.commands)
		if err != nil {
			return err
		}
		for _, index := range keys.indices {
			fields := strings.Fields(keys.lines[index])
			existing := strings.Join(fields[:2], " ")
			if existing == identity {
				return errors.New("esta chave já está autorizada")
			}
		}
		fmt.Fprintln(c.output, fingerprint)
		if err := writeAuthorizedKeys(path, append(keys.lines, pasted)); err != nil {
			return err
		}
		fmt.Fprintln(c.output, "Chave autorizada. Teste o acesso antes de encerrar sua sessão atual.")
	case "remove":
		if err := c.display(keys); err != nil {
			return err
		}
		if len(keys.indices) == 0 {
			return nil
		}
		if len(keys.indices) == 1 && os.Getenv("SSH_CONNECTION") != "" {
			return errors.New("não é permitido remover a última chave; adicione e teste outra primeiro")
		}
		answer, err := c.prompt("Número da chave a remover: ")
		if err != nil {
			return err
		}
		number, err := strconv.Atoi(answer)
		if err != nil || number < 1 || number > len(keys.indices) {
			return errors.New("número inválido")
		}
		_, fingerprint, _ := validatePublicKey(strings.TrimSpace(keys.lines[keys.indices[number-1]]), c.commands)
		fmt.Fprintln(c.output, fingerprint+"\nMantenha a sessão atual aberta e confirme acesso por outra chave.")
		if err := c.confirm("Remover esta chave autorizada?"); err != nil {
			return err
		}
		index := keys.indices[number-1]
		keys.lines = slices.Delete(keys.lines, index, index+1)
		if err := writeAuthorizedKeys(path, keys.lines); err != nil {
			return err
		}
		fmt.Fprintln(c.output, "Chave removida.")
	default:
		return errors.New("ação SSH inválida")
	}
	return nil
}
