package main

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func newSSHConsoleTest(t *testing.T) (*sshConsole, string, string) {
	t.Helper()
	if _, err := exec.LookPath("ssh-keygen"); err != nil {
		t.Skip("ssh-keygen necessário")
	}
	home := t.TempDir()
	directory, err := sshDirectory(home)
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
	c := &sshConsole{home: home, keyName: "id_laptop", input: bufio.NewReader(strings.NewReader("")), output: &bytes.Buffer{}, commands: systemSSHCommands()}
	return c, keys[0], keys[1]
}

func sshAnswers(c *sshConsole, answers ...string) {
	c.input = bufio.NewReader(strings.NewReader(strings.Join(answers, "\n") + "\n"))
}

func TestAuthorizedKeysKeepCommentsAndAllowNewDevices(t *testing.T) {
	c, first, second := newSSHConsoleTest(t)
	t.Setenv("SSH_CONNECTION", "")
	path := filepath.Join(c.home, ".ssh", "authorized_keys")
	if err := writeAuthorizedKeys(path, []string{"# pessoal", first, ""}); err != nil {
		t.Fatal(err)
	}

	sshAnswers(c, second+" device-futuro", "ADICIONAR")
	if err := c.manageKeys("add"); err != nil {
		t.Fatal(err)
	}
	sshAnswers(c, second, "ADICIONAR")
	if err := c.manageKeys("add"); err == nil {
		t.Fatal("aceitou uma chave duplicada")
	}
	sshAnswers(c, "1", "REMOVER")
	if err := c.manageKeys("remove"); err != nil {
		t.Fatal(err)
	}
	contents, err := os.ReadFile(path)
	if err != nil || string(contents) != "# pessoal\n\n"+second+" device-futuro\n" {
		t.Fatalf("authorized_keys não preservado: %q, %v", contents, err)
	}
	t.Setenv("SSH_CONNECTION", "192.168.1.9 12345 192.168.1.8 22")
	if err := c.manageKeys("remove"); err == nil {
		t.Fatal("permitiu remover a última chave em sessão SSH")
	}
}

func TestConfigureSSHClientUsesOneIdentityAndBacksUpLegacy(t *testing.T) {
	c, _, _ := newSSHConsoleTest(t)
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh necessário")
	}
	c.keyName = "id_desktop"
	location := backupLocation{path: filepath.Join(t.TempDir(), "SSH")}
	path := filepath.Join(c.home, ".ssh", "config")
	old := "Host peer\n    HostName 192.168.1.90\n    IdentityFile ~/.ssh/id_legacy\n"
	if err := os.WriteFile(path, []byte(old), 0600); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		client, err := c.prepareClient(path)
		if err != nil {
			t.Fatal(err)
		}
		defer client.cleanup()
		if _, err := client.install(location); err != nil {
			t.Fatal(err)
		}
	}
	effective, err := c.commands.capture("", "ssh", "-G", "-F", path, "homeprod")
	if err != nil {
		t.Fatal(err)
	}
	var identities []string
	for _, line := range strings.Split(effective, "\n") {
		if value, ok := strings.CutPrefix(line, "identityfile "); ok {
			identities = append(identities, value)
		}
	}
	if len(identities) != 1 || identities[0] != "~/.ssh/id_desktop" {
		t.Fatalf("identidades efetivas: %v", identities)
	}
	socket := "identityagent /run/user/" + strconv.Itoa(os.Getuid()) + "/ssh-agent.socket"
	if !slices.Contains(strings.Split(effective, "\n"), socket) ||
		!slices.Contains(strings.Split(effective, "\n"), "addkeystoagent 28800") {
		t.Fatal("o cliente deve usar o agente do usuário e guardar a chave por oito horas")
	}
	backups, err := filepath.Glob(filepath.Join(location.path, "config.backup-*"))
	if err != nil || len(backups) != 1 {
		t.Fatalf("backups: %v, %v", backups, err)
	}
	if contents, err := os.ReadFile(backups[0]); err != nil || string(contents) != old {
		t.Fatalf("backup incorreto: %q, %v", contents, err)
	}
}
