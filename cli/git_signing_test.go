package main

import (
	"io"
	"os/exec"
	"strings"
	"testing"
)

func TestCreateGitSigningKeyUsesRSA4096AndPassphrase(t *testing.T) {
	isolatedGitKeyring(t)
	password := []byte("only-for-testing-rsa4096")
	key, err := createGitSigningKey("Dotfiles Test", "dev@example.invalid", "desktop-test", password)
	if err != nil || !validSigningFingerprint(key) {
		t.Fatalf("chave gerada: %q, %v", key, err)
	}
	listing := testGPG(t, "--batch", "--with-colons", "--list-secret-keys", "--", key)
	if !strings.Contains(listing, "sec:u:4096:1:") || !strings.Contains(listing, "Dotfiles Test (desktop-test) <dev@example.invalid>") {
		t.Fatalf("tipo ou identidade da chave incorretos: %s", listing)
	}
	if err := exec.Command("gpgconf", "--kill", "gpg-agent").Run(); err != nil {
		t.Fatal(err)
	}
	sign := func(passphrase string) error {
		command := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase-fd", "0", "--local-user", key, "--detach-sign")
		command.Stdin = strings.NewReader(passphrase + "\nconteúdo para assinar\n")
		return command.Run()
	}
	if sign("senha incorreta") == nil {
		t.Fatal("GnuPG aceitou uma senha incorreta")
	}
	if err := sign(string(password)); err != nil {
		t.Fatalf("GnuPG não assinou com a senha correta: %v", err)
	}
}

func TestGitSigningKeyPreservesConfiguredSubkey(t *testing.T) {
	isolatedGitKeyring(t)
	options := []string{"--batch", "--pinentry-mode", "loopback", "--passphrase", ""}
	testGPG(t, append(options, "--quick-generate-key", "Chave anterior <anterior@example.invalid>", "ed25519", "cert", "1d")...)
	listing := testGPG(t, "--batch", "--with-colons", "--list-secret-keys")
	var primary string
	for line := range strings.SplitSeq(listing, "\n") {
		if strings.HasPrefix(line, "fpr:") {
			primary = strings.Split(line, ":")[9]
			break
		}
	}
	if !validSigningFingerprint(primary) {
		t.Fatalf("fingerprint primário ausente: %s", listing)
	}
	testGPG(t, append(options, "--quick-add-key", primary, "ed25519", "sign", "1d")...)
	listing = testGPG(t, "--batch", "--with-colons", "--with-subkey-fingerprint", "--list-secret-keys", primary)
	var selector string
	for line := range strings.SplitSeq(listing, "\n") {
		if strings.HasPrefix(line, "fpr:") {
			fingerprint := strings.Split(line, ":")[9]
			if fingerprint != primary {
				selector = fingerprint + "!"
				break
			}
		}
	}
	if selector == "" {
		t.Fatalf("fingerprint da subchave ausente: %s", listing)
	}
	console := sshConsole{output: io.Discard, commands: systemSSHCommands()}
	const email = "novo@example.invalid"
	key, err := gitSigningKey(console, "Dotfiles Test", email, "desktop-test", selector)
	if err != nil || key != selector {
		t.Fatalf("chave configurada = %q, esperava %q: %v", key, selector, err)
	}
	key, err = gitSigningKey(console, "Dotfiles Test", email, "desktop-test", "")
	if err != nil || key != primary {
		t.Fatalf("chave do chaveiro = %q, esperava %q: %v", key, primary, err)
	}
}

func isolatedGitKeyring(t *testing.T) {
	t.Helper()
	for _, executable := range []string{"gpg", "gpgconf"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skip(executable + " necessário")
		}
	}
	t.Setenv("GNUPGHOME", t.TempDir())
	t.Cleanup(func() { _ = exec.Command("gpgconf", "--kill", "gpg-agent").Run() })
}

func testGPG(t *testing.T, args ...string) string {
	t.Helper()
	output, err := exec.Command("gpg", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("GnuPG %v: %v\n%s", args, err, output)
	}
	return string(output)
}
