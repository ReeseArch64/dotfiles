package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestConfigureGitUsesLocalSigningKey(t *testing.T) {
	for _, executable := range []string{"git", "gpg", "gpgconf"} {
		if _, err := exec.LookPath(executable); err != nil {
			t.Skip(executable + " necessário")
		}
	}
	home, keyring := t.TempDir(), t.TempDir()
	config := filepath.Join(home, ".config")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", config)
	t.Setenv("GNUPGHOME", keyring)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_COUNT", "0")
	t.Cleanup(func() { _ = exec.Command("gpgconf", "--kill", "gpg-agent").Run() })
	run := func(name string, args ...string) string {
		t.Helper()
		output, err := exec.Command(name, args...).CombinedOutput()
		if err != nil {
			t.Fatalf("%s %v: %v\n%s", name, args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	run("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase", "",
		"--quick-generate-key", "Git Test <dev@reesearch64.com>", "ed25519", "sign", "1d")

	var packages []string
	commands := systemSSHCommands()
	capture := commands.capture
	commands.capture = func(input string, args ...string) (string, error) {
		if args[0] == "pacman" {
			packages = append(packages, args[2])
			return args[2] + " 1", nil
		}
		return capture(input, args...)
	}
	commands.interactive = func(args ...string) error {
		t.Fatalf("instalação inesperada: %v", args)
		return nil
	}
	console := sshConsole{home: home, output: &bytes.Buffer{}, commands: commands}
	assets := filepath.Join("..", "cachyos", "git")
	if err := configureGit(console, config, assets, "desktop-test"); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(packages, []string{"git", "gnupg", "lazygit", "git-delta", "git-lfs", "neovim", "openssh", "git-ignore-bin"}) {
		t.Fatalf("pacotes: %v", packages)
	}
	if key := run("git", "config", "--file", filepath.Join(home, ".gitconfig"), "--get", "user.signingkey"); !validSigningFingerprint(key) {
		t.Fatalf("fingerprint inválido: %s", key)
	}
	for _, path := range []string{
		filepath.Join(home, ".gitconfig"), filepath.Join(home, ".gitignore"),
		filepath.Join(home, ".gitattributes"), filepath.Join(config, "git", "config"),
	} {
		if info, err := os.Lstat(path); err != nil || !info.Mode().IsRegular() {
			t.Fatalf("destino inválido %s: %v", path, err)
		}
	}
	repo := t.TempDir()
	run("git", "-C", repo, "init")
	run("git", "-C", repo, "commit", "--allow-empty", "-m", "test")
	run("git", "-C", repo, "tag", "-a", "v0.0.1", "-m", "test")
	run("git", "-C", repo, "verify-commit", "HEAD")
	run("git", "-C", repo, "verify-tag", "v0.0.1")
	if err := configureGit(console, config, assets, "desktop-test"); err != nil {
		t.Fatalf("reaplicação: %v", err)
	}
}

func TestGitFilesBackUpLegacyLinkAndExistingConfig(t *testing.T) {
	home, central := t.TempDir(), filepath.Join(t.TempDir(), "Git")
	linked := filepath.Join(home, "legacy-ignore")
	sdkTestFile(t, linked, "old ignore")
	ignore := filepath.Join(home, ".gitignore")
	if err := os.Symlink("legacy-ignore", ignore); err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(home, ".gitconfig")
	sdkTestFile(t, config, "old config")
	files := []configFile{{ignore, []byte("new ignore"), 0644}, {config, []byte("new config"), 0600}}
	backup, err := applyConfigFiles(files, backupLocation{path: central}, "git")
	if err != nil || !pathInside(central, backup) {
		t.Fatalf("backup: %s, %v", backup, err)
	}
	if target, err := os.Readlink(filepath.Join(backup, ".gitignore")); err != nil || target != "legacy-ignore" {
		t.Fatalf("link de backup: %s, %v", target, err)
	}
	if info, err := os.Lstat(ignore); err != nil || !info.Mode().IsRegular() {
		t.Fatalf("arquivo instalado inválido: %v", err)
	}
	sdkTestContent(t, linked, "old ignore")
	sdkTestContent(t, ignore, "new ignore")
	sdkTestContent(t, filepath.Join(backup, ".gitconfig"), "old config")
	sdkTestContent(t, config, "new config")
}
