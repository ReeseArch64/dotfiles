package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

func lockOperation(key, operation string) (func(), error) {
	uid := strconv.Itoa(os.Geteuid())
	runtime := filepath.Join("/run/user", uid)
	if os.Geteuid() == 0 {
		runtime = "/run"
	}
	root := filepath.Join(runtime, "dotfiles")
	if _, err := os.Stat(runtime); errors.Is(err, os.ErrNotExist) {
		root = filepath.Join("/tmp", "dotfiles-runtime-"+uid)
	}
	if err := os.Mkdir(root, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode().Perm() != 0700 || info.Sys().(*syscall.Stat_t).Uid != uint32(os.Geteuid()) {
		return nil, fmt.Errorf("diretório de execução inválido: %s", root)
	}

	// Keep one directory inode per key so concurrent calls always lock the same inode.
	path := filepath.Join(root, fmt.Sprintf("%x", sha256.Sum256([]byte(key))))
	if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, os.ErrExist) {
		return nil, err
	}
	return lockDirectory(path, operation)
}

func lockDirectory(path, operation string) (func(), error) {
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_DIRECTORY|syscall.O_NOFOLLOW|syscall.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = syscall.Close(fd)
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("outra operação de %s está em andamento", operation)
		}
		return nil, err
	}
	return func() { _ = syscall.Close(fd) }, nil
}
