package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSSHKeyFilesPreserveExistingFiles(t *testing.T) {
	for _, name := range []string{"id_desktop", "id_laptop"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			files, err := prepareSSHKey(home, name)
			if err != nil {
				t.Fatal(err)
			}
			staged := filepath.Join(files.tempDir, name)
			for _, file := range []string{staged, staged + ".pub"} {
				if err := os.WriteFile(file, []byte("fixture"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if result := files.finish(nil); result.err != nil || result.keyName != name {
				t.Fatalf("resultado: %+v", result)
			}
			for _, file := range []string{files.destination, files.destination + ".pub"} {
				if contents, err := os.ReadFile(file); err != nil || string(contents) != "fixture" {
					t.Fatalf("conteúdo de %s = %q, erro %v", file, contents, err)
				}
			}
			if _, err := prepareSSHKey(home, name); err == nil {
				t.Fatal("uma nova execução deve preservar o par existente")
			}
			if _, err := os.Stat(files.tempDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("diretório temporário não removido: %v", err)
			}
		})
	}
}

func TestSSHKeyFilesHandlePublicKeyCollision(t *testing.T) {
	files, err := prepareSSHKey(t.TempDir(), "id_desktop")
	if err != nil {
		t.Fatal(err)
	}
	staged := filepath.Join(files.tempDir, "id_desktop")
	for _, path := range []string{staged, staged + ".pub", files.destination + ".pub"} {
		if err := os.WriteFile(path, []byte(path), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if result := files.finish(nil); result.err == nil {
		t.Fatal("deveria falhar ao tentar publicar a chave pública")
	}
	if _, err := os.Stat(files.destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a chave privada recém-criada deveria ser removida: %v", err)
	}
	path := files.destination + ".pub"
	if read, err := os.ReadFile(path); err != nil || string(read) != path {
		t.Fatalf("chave existente alterada: %q, %v", read, err)
	}
}
