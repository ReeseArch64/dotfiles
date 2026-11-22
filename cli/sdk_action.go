package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	sdkRoot       = "/mnt/sdk"
	sdkBackupRoot = backupRoot + "/SDK"
)

func runSDKMigration(p profile) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute a migração pelo menu Desktop ou Laptop como usuário normal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	printActionHeader()
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	for _, mapping := range sdkDirectoryMappings {
		fmt.Printf("  ~/%s → %s\n", mapping[0], filepath.Join(sdkRoot, mapping[1]))
	}
	fmt.Printf("\nOs diretórios anteriores recebem backup em %s. Feche aplicativos que usam essas pastas, especialmente a pasta de cache.\n", sdkBackupRoot)
	if err := console.confirm("Digite CONFIRMO para iniciar a migração: ", "CONFIRMO"); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	err = migrateSDK(ctx, home, console.commands.interactive)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		fmt.Println("Links SDK configurados. Backups em", sdkBackupRoot)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func migrateSDK(ctx context.Context, home string, run func(...string) error) error {
	if !filepath.IsAbs(home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	if err := sdkRealDirectoryPath(home); err != nil {
		return err
	}
	for _, root := range []string{sdkRoot, sdkBackupRoot} {
		if err := validateBackupPath(root); err != nil {
			return err
		}
	}
	if pathInside(home, sdkRoot) || pathInside(sdkRoot, home) ||
		pathInside(sdkBackupRoot, home) || pathInside(home, sdkBackupRoot) {
		return errors.New("a pasta pessoal e os destinos da migração não podem se sobrepor")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureSDKRoot(sdkRoot, 0755, run); err != nil {
		return err
	}
	if err := (backupLocation{path: sdkBackupRoot, sudo: run}).prepare(); err != nil {
		return err
	}

	unlock, err := lockOperation(filepath.Join(sdkRoot, ".dotfiles-sdk.lock"), "migração SDK")
	if err != nil {
		return err
	}
	defer unlock()
	ops := systemSDKOps(ctx, sdkBackupRoot)
	ops.repairPermissions = func(path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Printf("Corrigindo acesso do usuário em %s antes de migrar…\n", path)
		return repairMigrationPermissions(path, os.Getuid(), os.Getgid(), run)
	}
	fmt.Println("Preparando as cópias e os backups…")
	return prepareSDKDirectories(home, sdkRoot, ops)
}

func systemSDKOps(ctx context.Context, backups string) sdkMigrationOps {
	return sdkMigrationOps{
		ctx:        ctx,
		backupRoot: backups,
		copyTree: func(source, target string) error {
			return copySDKTree(ctx, source, target)
		},
		recoveryCopy: func(source, target string) error {
			return copySDKTree(context.Background(), source, target)
		},
		rename: func(source, target string) error {
			err := unix.Renameat2(unix.AT_FDCWD, source, unix.AT_FDCWD, target, unix.RENAME_NOREPLACE)
			if err != nil {
				return &os.LinkError{Op: "mover sem sobrescrever", Old: source, New: target, Err: err}
			}
			return nil
		},
		symlink: os.Symlink,
	}
}

func ensureSDKRoot(path string, mode os.FileMode, run func(...string) error) error {
	uid, gid := os.Getuid(), os.Getgid()
	isReady := func() (bool, error) {
		info, err := os.Lstat(path)
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if !info.IsDir() {
			return false, fmt.Errorf("%s precisa ser um diretório real", path)
		}
		owner, ok := info.Sys().(*syscall.Stat_t)
		return ok && int(owner.Uid) == uid && int(owner.Gid) == gid &&
			info.Mode()&(os.ModePerm|os.ModeSticky|os.ModeSetgid|os.ModeSetuid) == mode, nil
	}

	if ready, err := isReady(); err != nil || ready {
		return err
	}
	if err := run("sudo", "install", "-d", "-m", fmt.Sprintf("%04o", mode),
		"-o", strconv.Itoa(uid), "-g", strconv.Itoa(gid), "--", path); err != nil {
		return fmt.Errorf("preparar %s: %w", path, err)
	}
	if ready, err := isReady(); err != nil {
		return err
	} else if !ready {
		return fmt.Errorf("não foi possível aplicar dono, grupo ou permissão em %s", path)
	}
	return nil
}
