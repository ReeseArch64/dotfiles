package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestPrepareMountDirectoriesCreatesOwnedDirectoriesAndIsIdempotent(t *testing.T) {
	root := t.TempDir()
	uid, gid := os.Getuid(), os.Getgid()
	var calls [][]string
	run := func(args ...string) error {
		calls = append(calls, args)
		mode, err := strconv.ParseUint(args[4], 8, 32)
		if err != nil {
			return err
		}
		path := args[len(args)-1]
		if err := os.MkdirAll(path, os.FileMode(mode)); err != nil {
			return err
		}
		return os.Chmod(path, os.FileMode(mode))
	}
	if err := prepareMountDirectories(root, uid, gid, run); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 {
		t.Fatalf("esperava quatro diretórios, comandos: %v", calls)
	}
	for i, name := range []string{"workspaces", "sdk", "storage", "backups"} {
		mode := "0755"
		if name == "workspaces" || name == "backups" {
			mode = "0700"
		}
		want := []string{"sudo", "install", "-d", "-m", mode, "-o", strconv.Itoa(uid), "-g", strconv.Itoa(gid), filepath.Join(root, name)}
		if !slices.Equal(calls[i], want) {
			t.Fatalf("comando incorreto: %v, esperado %v", calls[i], want)
		}
	}
	if err := prepareMountDirectories(root, uid, gid, func(...string) error {
		t.Fatal("sudo solicitado para diretórios já corretos")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareMountDirectoriesRepairsPermissionsWithoutChangingContents(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "backups")
	if err := os.MkdirAll(path, 0755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(path, "private-key")
	if err := os.WriteFile(file, []byte("preservar"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := prepareMountDirectories(root, os.Getuid(), os.Getgid(), func(args ...string) error {
		if slices.Contains(args, "-R") {
			t.Fatal("alteração recursiva não permitida")
		}
		mode, err := strconv.ParseUint(args[4], 8, 32)
		if err != nil {
			return err
		}
		path := args[len(args)-1]
		if err := os.MkdirAll(path, os.FileMode(mode)); err != nil {
			return err
		}
		return os.Chmod(path, os.FileMode(mode))
	}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("permissões de backups incorretas: %v, %v", info, err)
	}
	content, err := os.ReadFile(file)
	if err != nil || string(content) != "preservar" {
		t.Fatalf("conteúdo alterado: %q %v", content, err)
	}
	info, err = os.Stat(file)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("permissões internas alteradas: %v %v", info, err)
	}
}

func TestPrepareMountDirectoriesRejectsFilesAndSymlinksBeforeChanges(t *testing.T) {
	for _, kind := range []string{"file", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, "storage")
			var err error
			if kind == "file" {
				err = os.WriteFile(path, nil, 0600)
			} else {
				err = os.Symlink(t.TempDir(), path)
			}
			if err != nil {
				t.Fatal(err)
			}
			err = prepareMountDirectories(root, os.Getuid(), os.Getgid(), func(...string) error {
				t.Fatal("não deve modificar nada antes de validar todos os caminhos")
				return nil
			})
			if err == nil || !strings.Contains(err.Error(), path) {
				t.Fatalf("caminho inválido aceito: %v", err)
			}
		})
	}
}

func TestPrepareMountDirectoriesRepairsOwnerAndGroup(t *testing.T) {
	root := t.TempDir()
	for _, directory := range mountDirectories {
		path := filepath.Join(root, directory.name)
		if err := os.MkdirAll(path, directory.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, directory.mode); err != nil {
			t.Fatal(err)
		}
	}
	for _, owner := range [][2]int{{os.Getuid() + 1, os.Getgid()}, {os.Getuid(), os.Getgid() + 1}} {
		calls := 0
		if err := prepareMountDirectories(root, owner[0], owner[1], func(args ...string) error {
			calls++
			if args[6] != strconv.Itoa(owner[0]) || args[8] != strconv.Itoa(owner[1]) {
				t.Fatalf("proprietário/grupo incorretos: %v", args)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if calls != 4 {
			t.Fatalf("proprietário/grupo divergente ignorado: %d comandos", calls)
		}
	}
}

func TestPrepareMountDirectoriesPropagatesSudoFailure(t *testing.T) {
	failure := errors.New("sudo recusado")
	calls := 0
	err := prepareMountDirectories(t.TempDir(), 1000, 1001, func(args ...string) error {
		calls++
		if !slices.Contains(args, "1000") || !slices.Contains(args, "1001") {
			t.Fatal("usuário/grupo incorretos", args)
		}
		return failure
	})
	if !errors.Is(err, failure) || calls != 1 {
		t.Fatalf("falha ignorada: %v, chamadas=%d", err, calls)
	}
}
