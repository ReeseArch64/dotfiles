package main

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

func validateMigrationTree(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Special files (sockets, FIFOs, devices) são artefatos de runtime; o
		// cp -a os recria sem conteúdo, então apenas ignoramos aqui.
		if !entry.IsDir() && !entry.Type().IsRegular() && entry.Type()&os.ModeSymlink == 0 {
			return nil
		}
		// WalkDir checks directory access, but not whether files can be copied.
		if entry.Type().IsRegular() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			return file.Close()
		}
		return nil
	})
}

func repairMigrationPermissions(path string, uid, gid int, run func(...string) error) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("%s deve ser um diretório real, sem links simbólicos", path)
	}
	// GNU chown/chmod do not traverse symlinks encountered during recursion.
	// Keep existing modes, adding access only for the owner; never use 777.
	owner := strconv.Itoa(uid) + ":" + strconv.Itoa(gid)
	if err := run("sudo", "chown", "-R", "-P", "--no-dereference", "--", owner, path); err != nil {
		return err
	}
	return run("sudo", "chmod", "-R", "u+rwX", "--", path)
}

func repairSystemMigrationPermissions(path string) error {
	fmt.Fprintln(os.Stdout, "Corrigindo permissões antes de migrar", path+"...")
	return repairMigrationPermissions(path, os.Getuid(), os.Getgid(), func(args ...string) error {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		return cmd.Run()
	})
}
