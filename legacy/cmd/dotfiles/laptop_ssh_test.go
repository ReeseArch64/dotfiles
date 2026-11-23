package main

import (
	"bufio"
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func newLaptopSSHConsoleTest(t *testing.T) (*laptopSSHConsole, string, string) {
	t.Helper()
	testBackupStore(t)
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen necessário")
	}
	home := t.TempDir()
	directory, err := laptopSSHDirectory(home)
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for _, name := range []string{"id_laptop", "other"} {
		path := filepath.Join(directory, name)
		output, err := exec.Command("ssh-keygen", "-q", "-t", "ed25519", "-N", "", "-C", "test@example.com", "-f", path).CombinedOutput()
		if err != nil {
			t.Fatalf("ssh-keygen: %v: %s", err, output)
		}
		public, err := os.ReadFile(path + ".pub")
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, strings.TrimSpace(string(public)))
	}
	c := &laptopSSHConsole{home: home, username: "alan", input: bufio.NewReader(strings.NewReader("")), output: &bytes.Buffer{}, commands: systemLaptopSSHCommands(), probe: func(string) error { return nil }}
	return c, keys[0], keys[1]
}

func laptopSSHAnswers(c *laptopSSHConsole, answers ...string) {
	c.input = bufio.NewReader(strings.NewReader(strings.Join(answers, "\n") + "\n"))
}

func TestLaptopSSHAddAndDuplicate(t *testing.T) {
	c, key, _ := newLaptopSSHConsoleTest(t)
	laptopSSHAnswers(c, "id_desktop", key, "ADICIONAR")
	if err := c.manageKeys("add"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(c.home, ".ssh", "authorized_keys")
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	identity, _, _ := validateLaptopPublicKey(key, c.commands)
	if string(contents) != identity+" id_desktop\n" {
		t.Fatalf("conteúdo inesperado: %s", contents)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0600 {
		t.Fatal("permissões incorretas")
	}
	laptopSSHAnswers(c, "id_iphone", key)
	if err := c.manageKeys("add"); err == nil {
		t.Fatal("duplicata aceita")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(contents, after) {
		t.Fatal("duplicata alterou o arquivo")
	}
}

func TestLaptopSSHRejectInvalidPublicInput(t *testing.T) {
	c, key, _ := newLaptopSSHConsoleTest(t)
	private, _ := os.ReadFile(filepath.Join(c.home, ".ssh", "id_laptop"))
	for _, input := range []string{string(private), "ssh-ed25519 YWJj", "command=evil " + key, key + "\n" + key, key + "\x1b[31m"} {
		if _, _, err := validateLaptopPublicKey(input, c.commands); err == nil {
			t.Fatalf("entrada inválida aceita: %.30s", input)
		}
	}
}

func TestLaptopSSHCancelAndInvalidDevice(t *testing.T) {
	for _, device := range []string{"id_android", "outro"} {
		t.Run(device, func(t *testing.T) {
			c, key, _ := newLaptopSSHConsoleTest(t)
			laptopSSHAnswers(c, device, key, "cancelar")
			if err := c.manageKeys("add"); err == nil {
				t.Fatal("operação deveria ser recusada")
			}
			if _, err := os.Stat(filepath.Join(c.home, ".ssh", "authorized_keys")); !os.IsNotExist(err) {
				t.Fatal("arquivo foi criado")
			}
		})
	}
}

func TestLaptopSSHListAndRemove(t *testing.T) {
	c, first, second := newLaptopSSHConsoleTest(t)
	path := filepath.Join(c.home, ".ssh", "authorized_keys")
	if err := writeLaptopAuthorizedKeys(path, []string{first, second}); err != nil {
		t.Fatal(err)
	}
	if err := c.manageKeys("list"); err != nil {
		t.Fatal(err)
	}
	output := c.output.(*bytes.Buffer).String()
	if !strings.Contains(output, "SHA256:") || !strings.Contains(output, second) {
		t.Fatal("listagem sem públicas e fingerprints")
	}
	private, _ := os.ReadFile(filepath.Join(c.home, ".ssh", "id_laptop"))
	if strings.Contains(output, string(private)) {
		t.Fatal("privada foi exibida")
	}
	laptopSSHAnswers(c, "1", "cancelar")
	if err := c.manageKeys("remove"); err == nil {
		t.Fatal("remoção sem confirmação")
	}
	keys, _ := readLaptopAuthorizedKeys(path, c.commands)
	if len(keys) != 2 {
		t.Fatal("cancelamento removeu chave")
	}
	laptopSSHAnswers(c, "1", "REMOVER")
	if err := c.manageKeys("remove"); err != nil {
		t.Fatal(err)
	}
	keys, _ = readLaptopAuthorizedKeys(path, c.commands)
	if !slices.Equal(keys, []string{second}) {
		t.Fatalf("chave incorreta removida: %v", keys)
	}
	if err := c.manageKeys("remove"); err == nil {
		t.Fatal("última chave removida")
	}
}

func TestLaptopSSHRefuseSymlinks(t *testing.T) {
	c, _, _ := newLaptopSSHConsoleTest(t)
	target := filepath.Join(c.home, "target")
	if err := os.WriteFile(target, []byte("unchanged"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(c.home, ".ssh", "authorized_keys")); err != nil {
		t.Fatal(err)
	}
	if err := c.manageKeys("add"); err == nil {
		t.Fatal("symlink aceito")
	}
	contents, _ := os.ReadFile(target)
	if string(contents) != "unchanged" {
		t.Fatal("alvo alterado")
	}
	other := t.TempDir()
	if err := os.Symlink(filepath.Join(c.home, ".ssh"), filepath.Join(other, ".ssh")); err != nil {
		t.Fatal(err)
	}
	if _, err := laptopSSHDirectory(other); err == nil {
		t.Fatal("diretório simbólico aceito")
	}
}

func TestLaptopSSHLockPreventsConcurrentEdit(t *testing.T) {
	path := t.TempDir()
	unlock, err := laptopSSHLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	if second, err := laptopSSHLock(path); err == nil {
		second()
		t.Fatal("lock concorrente permitido")
	}
}

func TestLaptopSSHClientConfigIdempotent(t *testing.T) {
	testBackupStore(t)
	directory := t.TempDir()
	path := filepath.Join(directory, "config")
	if err := os.WriteFile(path, []byte("Host test\n    HostName test.example.com\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := configureLaptopSSHClient(directory); err != nil {
			t.Fatal(err)
		}
	}
	contents, _ := os.ReadFile(path)
	if strings.Count(string(contents), "IdentityFile ~/.ssh/id_laptop") != 1 || !strings.Contains(string(contents), "HostName test.example.com") {
		t.Fatalf("configuração incorreta: %s", contents)
	}
	backups, _ := testBackupPaths(localBackupRoot(), "Legacy-SSH", path)
	if len(backups) != 2 {
		t.Fatal("backups não preservados")
	}
}

func TestLaptopSSHExistingIdentityPreserved(t *testing.T) {
	c, _, _ := newLaptopSSHConsoleTest(t)
	path := filepath.Join(c.home, ".ssh", "id_laptop")
	before, _ := os.ReadFile(path)
	if err := c.configureIdentity(); err != nil {
		t.Fatalf("deve permitir authorized_keys vazio: %v", err)
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("privada sobrescrita")
	}
	if err := os.Remove(path + ".pub"); err != nil {
		t.Fatal(err)
	}
	if err := c.configureIdentity(); err == nil {
		t.Fatal("privada sem pública aceita")
	}
	after, _ = os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("privada sem .pub foi alterada")
	}
}

func TestLaptopSSHGenerateIdentityNativePrompts(t *testing.T) {
	c, _, _ := newLaptopSSHConsoleTest(t)
	path := filepath.Join(c.home, ".ssh", "id_laptop")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path + ".pub"); err != nil {
		t.Fatal(err)
	}
	laptopSSHAnswers(c, "invalid")
	if err := c.configureIdentity(); err == nil {
		t.Fatal("email inválido aceito")
	}
	var generated []string
	c.commands.interactive = func(argv ...string) error {
		generated = slices.Clone(argv)
		// Somente o teste evita a interação de passphrase com -N vazio.
		args := append(slices.Clone(argv[1:]), "-N", "", "-q")
		return exec.Command(argv[0], args...).Run()
	}
	laptopSSHAnswers(c, "laptop@example.com")
	if err := c.configureIdentity(); err != nil {
		t.Fatalf("resultado inesperado: %v", err)
	}
	if !slices.Equal(generated, []string{"ssh-keygen", "-t", "ed25519", "-C", "laptop@example.com", "-f", path}) {
		t.Fatalf("senha não deve aparecer nos argumentos: %v", generated)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("chave não gerada")
	}
}

func TestLaptopSSHConfigureRequiresPolicyConfirmation(t *testing.T) {
	c, first, _ := newLaptopSSHConsoleTest(t)
	if err := writeLaptopAuthorizedKeys(filepath.Join(c.home, ".ssh", "authorized_keys"), []string{first}); err != nil {
		t.Fatal(err)
	}
	var commands [][]string
	c.commands.interactive = func(argv ...string) error { commands = append(commands, slices.Clone(argv)); return nil }
	laptopSSHAnswers(c, "cancelar")
	if err := c.configure(); err == nil {
		t.Fatalf("confirmação não exigida: %v", err)
	}
	for _, args := range commands {
		if slices.Contains(args, "apply-server") {
			t.Fatal("servidor endurecido sem confirmação")
		}
	}
	laptopSSHAnswers(c, "CONFIRMO")
	if err := c.configure(); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(commands[len(commands)-1], "apply-server") {
		t.Fatal("política não executada após confirmação")
	}
}

func TestLaptopSSHConfigureWithoutAuthorizedKeys(t *testing.T) {
	c, _, _ := newLaptopSSHConsoleTest(t)
	t.Setenv("SSH_CONNECTION", "")
	var commands [][]string
	c.commands.interactive = func(argv ...string) error { commands = append(commands, slices.Clone(argv)); return nil }
	laptopSSHAnswers(c, "CONFIRMO")
	if err := c.configure(); err != nil {
		t.Fatalf("SSH deve configurar sem públicas: %v", err)
	}
	contents, err := os.ReadFile(filepath.Join(c.home, ".ssh", "authorized_keys"))
	if err != nil || len(contents) != 0 {
		t.Fatal("authorized_keys vazio não foi criado")
	}
	if !slices.Contains(commands[len(commands)-1], "apply-server") {
		t.Fatal("política não aplicada")
	}
	if strings.Contains(c.output.(*bytes.Buffer).String(), "De OUTRO dispositivo") {
		t.Fatal("teste remoto não deve ser obrigatório")
	}
	commands = nil
	t.Setenv("SSH_CONNECTION", "192.168.1.2 12345 192.168.1.3 22")
	if err := c.configure(); err == nil {
		t.Fatal("configuração remota sem chaves deve exigir console local")
	}
	if len(commands) != 0 {
		t.Fatal("servidor alterado sem meio de acesso remoto")
	}
}

func TestLaptopSSHVerifyEffectivePolicy(t *testing.T) {
	c, first, _ := newLaptopSSHConsoleTest(t)
	if err := writeLaptopAuthorizedKeys(filepath.Join(c.home, ".ssh", "authorized_keys"), []string{first}); err != nil {
		t.Fatal(err)
	}
	if err := configureLaptopSSHClient(filepath.Join(c.home, ".ssh")); err != nil {
		t.Fatal(err)
	}
	realCapture := c.commands.capture
	policy := strings.ToLower(laptopSSHPolicy("alan", "22"))
	c.commands.capture = func(input string, args ...string) (string, error) {
		switch args[0] {
		case "sudo":
			return policy, nil
		case "systemctl":
			if args[len(args)-1] == "sshd.socket" {
				return "", errors.New("inactive")
			}
			return "", nil
		default:
			return realCapture(input, args...)
		}
	}
	if err := c.verify(); err != nil {
		t.Fatal(err)
	}
	if err := writeLaptopAuthorizedKeys(filepath.Join(c.home, ".ssh", "authorized_keys"), nil); err != nil {
		t.Fatal(err)
	}
	if err := c.verify(); err != nil {
		t.Fatalf("verificação deve aceitar zero chaves: %v", err)
	}
	c.probe = func(string) error { return errors.New("porta fechada") }
	if err := c.verify(); err == nil {
		t.Fatal("porta fechada aceita")
	}
	c.probe = func(string) error { return nil }
	policy = strings.ReplaceAll(policy, "passwordauthentication no", "passwordauthentication yes")
	if err := c.verify(); err == nil {
		t.Fatal("senha SSH permitida na política")
	}
	if err := os.Chmod(filepath.Join(c.home, ".ssh", "id_laptop"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := c.verify(); err == nil {
		t.Fatal("permissões inseguras aceitas")
	}
}
