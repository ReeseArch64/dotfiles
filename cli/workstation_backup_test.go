package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestWorkstationBackupMenuActions(t *testing.T) {
	menu := newMenu("desktop")
	next, _ := menu.handleKey("8")
	page := next.(model)
	if page.screen != screenBackup {
		t.Fatal("atalho 8 deve abrir o menu Backup")
	}
	items := page.items()
	want := []string{"snapshot", "projects", "gpg"}
	if len(items) != len(want) {
		t.Fatalf("ações de backup: %v", items)
	}
	for i, item := range items {
		command, err := actionCommand(item.action, item.title)
		if err != nil || item.disabled || !slices.Equal(command.Args[1:], []string{"--backup", want[i]}) {
			t.Fatalf("ação %s: %v, %v", item.action, command, err)
		}
	}
}

func TestBackupRootSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name, sshID, targetFS, wantError string
		snapshotError                    bool
	}{
		{name: "separate-home", sshID: "259", targetFS: "btrfs root-uuid"},
		{name: "ssh-inside-root", sshID: "256", targetFS: "btrfs root-uuid", wantError: "incluiria ~/.ssh"},
		{name: "different-filesystem", sshID: "259", targetFS: "btrfs other-uuid", wantError: "mesmo sistema"},
		{name: "snapshot-fails", sshID: "259", targetFS: "btrfs root-uuid", wantError: "falha simulada", snapshotError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			sshPath := filepath.Join(home, ".ssh")
			sdkTestFile(t, filepath.Join(sshPath, "fixture"), "ssh")
			_, _, backups := sdkTestRoots(t)
			location := backupLocation{path: backups}
			commands := sshCommands{
				capture: func(_ string, args ...string) (string, error) {
					path := args[len(args)-1]
					switch {
					case args[0] == "findmnt" && path == location.path:
						return tc.targetFS, nil
					case args[0] == "findmnt" && (path == "/" || path == sshPath):
						return "btrfs root-uuid", nil
					case args[0] == "btrfs" && path == "/":
						return "256", nil
					case args[0] == "btrfs" && path == sshPath:
						return tc.sshID, nil
					default:
						t.Fatalf("comando inesperado: %v", args)
						return "", nil
					}
				},
				interactive: func(args ...string) error {
					if !slices.Equal(args[:5], []string{"sudo", "btrfs", "subvolume", "snapshot", "/"}) || !pathInside(location.path, args[5]) {
						t.Fatalf("snapshot inesperado: %v", args)
					}
					if tc.snapshotError {
						return errors.New(tc.wantError)
					}
					if tc.wantError != "" {
						t.Fatal("tentou criar snapshot sem passar pelas verificações")
					}
					return os.Mkdir(args[5], 0700)
				},
			}
			destination, err := backupRootSnapshot(location, home, commands)
			if tc.wantError == "" {
				if err != nil || filepath.Base(destination) != "original" {
					t.Fatalf("snapshot = %q, %v", destination, err)
				}
				if _, err := os.Stat(destination); err != nil {
					t.Fatal(err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("esperava %q, recebi %v", tc.wantError, err)
			}
			if entries, err := os.ReadDir(location.path); err != nil || len(entries) != 0 {
				t.Fatalf("geração incompleta: %v, %v", entries, err)
			}
		})
	}
}

func TestBackupGPGPublishesCompleteGenerations(t *testing.T) {
	const publicKey = "-----BEGIN PGP PUBLIC KEY BLOCK-----\nfixture"
	const privateKey = "-----BEGIN PGP PRIVATE KEY BLOCK-----\nfixture"
	_, _, backups := sdkTestRoots(t)
	location := backupLocation{path: backups}
	export := func(private bool) ([]byte, error) {
		if private {
			return []byte(privateKey), nil
		}
		return []byte(publicKey), nil
	}
	directory, err := backupGPGKeys(location, export)
	if err != nil {
		t.Fatal(err)
	}
	sdkTestContent(t, filepath.Join(directory, "minha_chave_publica.asc"), publicKey)
	privatePath := filepath.Join(directory, "minha_chave_privada.asc")
	sdkTestContent(t, privatePath, privateKey)
	if info, err := os.Stat(privatePath); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissão da chave privada: %v, %v", info, err)
	}
	_, err = backupGPGKeys(location, func(private bool) ([]byte, error) {
		if private {
			return []byte("saída sem armor"), nil
		}
		return export(false)
	})
	if err == nil {
		t.Fatal("aceitou uma exportação privada inválida")
	}
	if entries, err := os.ReadDir(location.path); err != nil || len(entries) != 1 || entries[0].Name() != filepath.Base(directory) {
		t.Fatalf("gerações GPG: %v, %v", entries, err)
	}
}

func TestProjectBackupChecksRemotesAndContinuesAfterFailures(t *testing.T) {
	root := t.TempDir()
	for _, project := range []string{"a-missing", "b-github", "c-gitlab"} {
		sdkTestFile(t, filepath.Join(root, project, ".git", "config"), "fixture")
	}
	sdkTestFile(t, filepath.Join(root, "d-worktree", ".git"), "gitdir: /some/repository")
	var pushes []string
	commands := sshCommands{
		capture: func(_ string, args ...string) (string, error) {
			if filepath.Base(args[2]) == "a-missing" {
				return "origin\n", nil
			}
			return "origin\nogle\nbackup\n", nil
		},
		interactive: func(args ...string) error {
			push := filepath.Base(args[2]) + "/" + args[4]
			pushes = append(pushes, push)
			if push == "b-github/origin" || push == "c-gitlab/ogle" {
				return errors.New("falha simulada")
			}
			return nil
		},
	}
	count, err := backupProjectRepositories(root, commands)
	if count != 1 || err == nil || !strings.Contains(err.Error(), "1/4 projetos") {
		t.Fatalf("backup = %d, %v", count, err)
	}
	want := []string{"b-github/origin", "c-gitlab/origin", "c-gitlab/ogle", "c-gitlab/backup", "d-worktree/origin", "d-worktree/ogle", "d-worktree/backup"}
	if !slices.Equal(pushes, want) {
		t.Fatalf("envios = %v, esperava %v", pushes, want)
	}
}
