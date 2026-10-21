package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
)

type mountDirectory struct {
	name string
	mode os.FileMode
}

var mountDirectories = []mountDirectory{
	{"workspaces", 0700},
	{"sdk", 0755},
	{"storage", 0755},
	{"backups", 0700},
}

func prepareMountDirectories(root string, uid, gid int, run func(...string) error) error {
	var pending []mountDirectory
	// Validate all paths before changing any of them. Never follow symlinks or
	// recursively chown mounted data or service-owned subdirectories.
	for _, directory := range mountDirectories {
		path := filepath.Join(root, directory.name)
		info, err := os.Lstat(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("verificar %s: %w", path, err)
		}
		if err == nil {
			if !info.IsDir() {
				return fmt.Errorf("%s deve ser um diretório real, não um arquivo ou link simbólico", path)
			}
			owner, ok := info.Sys().(*syscall.Stat_t)
			if ok && int(owner.Uid) == uid && int(owner.Gid) == gid && info.Mode().Perm() == directory.mode && info.Mode()&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) == 0 {
				continue
			}
		}
		pending = append(pending, directory)
	}
	for _, directory := range pending {
		path := filepath.Join(root, directory.name)
		args := []string{"sudo", "install", "-d", "-m", fmt.Sprintf("%04o", directory.mode), "-o", strconv.Itoa(uid), "-g", strconv.Itoa(gid), path}
		if err := run(args...); err != nil {
			return fmt.Errorf("preparar %s: %w", path, err)
		}
	}
	return nil
}

func prepareSystemMountDirectories() error {
	return prepareMountDirectories("/mnt", os.Getuid(), os.Getgid(), func(args ...string) error {
		fmt.Fprintln(os.Stdout, "Preparando", args[len(args)-1]+"...")
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	})
}
