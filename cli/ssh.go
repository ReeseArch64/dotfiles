package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

type sshKeyResult struct {
	keyName string
	err     error
}

type sshKeyFiles struct {
	destination string
	tempDir     string
}

func prepareSSHKey(home, keyName string) (sshKeyFiles, error) {
	directory, err := sshDirectory(home)
	if err != nil {
		return sshKeyFiles{}, err
	}

	destination := filepath.Join(directory, keyName)
	for _, path := range []string{destination, destination + ".pub"} {
		if _, err := os.Lstat(path); err == nil {
			return sshKeyFiles{}, fmt.Errorf("~/.ssh/%s já existe", filepath.Base(path))
		} else if !errors.Is(err, os.ErrNotExist) {
			return sshKeyFiles{}, fmt.Errorf("verificar chave: %w", err)
		}
	}

	tempDir, err := os.MkdirTemp(directory, ".dotfiles-key-")
	if err != nil {
		return sshKeyFiles{}, fmt.Errorf("preparar geração da chave: %w", err)
	}
	return sshKeyFiles{destination: destination, tempDir: tempDir}, nil
}

func (f sshKeyFiles) command() *exec.Cmd {
	path := filepath.Join(f.tempDir, filepath.Base(f.destination))
	return exec.Command("ssh-keygen", "-t", "ed25519", "-f", path)
}

func (f sshKeyFiles) finish(commandErr error) sshKeyResult {
	defer os.RemoveAll(f.tempDir)
	if commandErr != nil {
		return sshKeyResult{err: fmt.Errorf("gerar chave SSH: %w", commandErr)}
	}

	path := filepath.Join(f.tempDir, filepath.Base(f.destination))
	if err := os.Link(path, f.destination); err != nil {
		return sshKeyResult{err: fmt.Errorf("salvar chave privada: %w", err)}
	}
	if err := os.Link(path+".pub", f.destination+".pub"); err != nil {
		rollbackErr := os.Remove(f.destination)
		return sshKeyResult{err: errors.Join(fmt.Errorf("salvar chave pública: %w", err), rollbackErr)}
	}
	return sshKeyResult{keyName: filepath.Base(f.destination)}
}
