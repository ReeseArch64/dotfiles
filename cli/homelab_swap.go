package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

const gibibyte = 1 << 30

func homelabSwapFstab(data []byte, path string) (entryPresent bool, err error) {
	for line := range strings.SplitSeq(string(data), "\n") {
		definition, _, _ := strings.Cut(line, "#")
		fields := strings.Fields(definition)
		if len(fields) == 0 || fields[0] != path {
			continue
		}
		if entryPresent || len(fields) != 6 || fields[1] != "none" || fields[2] != "swap" ||
			(fields[3] != "sw,nofail" && fields[3] != "nofail,sw") || fields[4] != "0" || fields[5] != "0" {
			return false, fmt.Errorf("entrada de %s no fstab incompatível ou duplicada", path)
		}
		entryPresent = true
	}
	return entryPresent, nil
}

func checkHomelabSwapFile(path string, size int64, commands sshCommands) (bool, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	stat := info.Sys().(*syscall.Stat_t)
	if !info.Mode().IsRegular() || info.Size() != size || stat.Nlink != 1 || int(stat.Uid) != os.Geteuid() {
		return false, fmt.Errorf("arquivo de swap existente incompatível: %s", path)
	}
	kind, err := commands.capture("", "blkid", "-p", "-s", "TYPE", "-o", "value", "--", path)
	if err != nil || strings.TrimSpace(kind) != "swap" {
		return false, fmt.Errorf("assinatura de swap inválida em %s", path)
	}
	return true, nil
}

func checkHomelabSwapSpace(directory string, size int64) error {
	var usage unix.Statfs_t
	if err := unix.Statfs(directory, &usage); err != nil {
		return err
	}
	if usage.Bavail*uint64(usage.Bsize) < uint64(size)+4*gibibyte {
		return errors.New("espaço insuficiente: é preciso manter 4 GiB livres após criar o arquivo de swap")
	}
	return nil
}

func createHomelabSwapFile(ctx context.Context, path string, size int64, commands sshCommands) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".swapfile-homelab-*")
	if err != nil {
		return err
	}
	defer file.Close()
	defer os.Remove(file.Name())
	// Fill the file on disk. The kernel refuses swap files with holes.
	block := make([]byte, 1<<20)
	for remaining := size; remaining > 0; remaining -= int64(len(block)) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := file.Write(block[:min(remaining, int64(len(block)))]); err != nil {
			return err
		}
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return err
	}
	if _, err := commands.capture("", "mkswap", "--", file.Name()); err != nil {
		return err
	}
	// Publish without overwriting another file that appeared during allocation.
	if err := os.Link(file.Name(), path); err != nil {
		return err
	}
	return os.Remove(file.Name())
}

func applyHomelabSwap(ctx context.Context, fstab, path string, size int64, commands, recovery sshCommands, save func(homelabBackup) (string, error)) error {
	original, mode, err := readHomelabSystemFile(fstab)
	if err != nil {
		return err
	}
	entryPresent, err := homelabSwapFstab(original, path)
	if err != nil {
		return err
	}
	filesystem, err := commands.capture("", "findmnt", "--noheadings", "--output", "FSTYPE", "--target", filepath.Dir(path))
	if err != nil {
		return err
	}
	switch strings.TrimSpace(filesystem) {
	case "ext4", "xfs":
	default:
		return fmt.Errorf("a swap do Homelab requer ext4 ou XFS, encontrado %q", strings.TrimSpace(filesystem))
	}
	exists, err := checkHomelabSwapFile(path, size, commands)
	if err != nil {
		return err
	}
	if !exists {
		if err := checkHomelabSwapSpace(filepath.Dir(path), size); err != nil {
			return err
		}
	}
	previouslyActive, err := commands.capture("", "swapon", "--show=NAME", "--noheadings", "--raw")
	if err != nil {
		return err
	}
	if !entryPresent {
		backup, err := save(homelabBackup{Contents: original})
		if err != nil {
			return err
		}
		fmt.Println("Backup do fstab:", backup)
	}
	if !exists {
		fmt.Println("Alocando o arquivo de swap do Homelab…")
		if err := createHomelabSwapFile(ctx, path, size, commands); err != nil {
			return err
		}
	}
	if err := os.Chmod(path, 0600); err != nil {
		return err
	}
	if !slices.Contains(strings.Split(previouslyActive, "\n"), path) {
		if _, err := commands.capture("", "swapon", "--", path); err != nil {
			return err
		}
	}
	if !entryPresent {
		updated := strings.TrimSuffix(string(original), "\n") + "\n" + path + " none swap sw,nofail 0 0\n"
		if err := writeConfigAtomic(fstab, []byte(updated), mode); err != nil {
			return fmt.Errorf("swap ativa, mas não foi possível gravar o fstab: %w", err)
		}
	}
	if _, err := commands.capture("", "systemctl", "daemon-reload"); err != nil {
		if entryPresent {
			return err
		}
		restoreErr := writeConfigAtomic(fstab, original, mode)
		_, reloadErr := recovery.capture("", "systemctl", "daemon-reload")
		return errors.Join(fmt.Errorf("swap ativa, mas a configuração para o próximo boot falhou: %w", err), restoreErr, reloadErr)
	}
	fmt.Printf("Swap Homelab ativa e registrada no fstab (%s).\n", path)
	return nil
}
