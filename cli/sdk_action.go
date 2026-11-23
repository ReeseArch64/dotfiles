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
	"sync"
	"syscall"

	"golang.org/x/sys/unix"
)

const (
	sdkRoot     = "/mnt/sdk"
	storageRoot = "/mnt/storage"
)

var storageDirectoryMappings = [][2]string{
	{"Documents", "Documents"}, {"Downloads", "Downloads"}, {"Pictures", "Pictures"},
	{"Videos", "Videos"}, {"Music", "Music"},
}

func (p profile) canMigrateSDK(osID string) bool {
	return p.sshKeyName() != "" && osID == "cachyos"
}

func runSDKMigration(p profile) error {
	return runDirectoryMigration(p, sdkRoot, "SDK", sdkDirectoryMappings, false)
}

func runStorageMigration(p profile) error {
	return runDirectoryMigration(p, storageRoot, "Storage", storageDirectoryMappings, true)
}

func runDirectoryMigration(p profile, target, category string, mappings [][2]string, archiveHomeOnly bool) error {
	if !p.canMigrateSDK(systemOSID()) || os.Geteuid() == 0 {
		return fmt.Errorf("a migração para %s exige CachyOS e deve ser executada pelo menu Desktop ou Laptop como usuário normal", target)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	printActionHeader()
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	location := managedBackup(category, console.commands.interactive)
	for _, mapping := range mappings {
		fmt.Printf("  ~/%s → %s\n", mapping[0], filepath.Join(target, mapping[1]))
	}
	if archiveHomeOnly {
		fmt.Printf("\nAs pastas locais com conteúdo serão arquivadas em %s antes de criar os links. Feche os aplicativos que usam essas pastas.\n", location.path)
	} else {
		fmt.Printf("\nOs diretórios anteriores recebem backup em %s. Feche os aplicativos que usam essas pastas.\n", location.path)
	}
	if err := console.confirm("Digite CONFIRMO para iniciar a migração: ", "CONFIRMO"); err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	err = migrateDirectories(ctx, home, target, location, mappings, archiveHomeOnly, console.commands.interactive)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		fmt.Println("Links configurados para", target)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func migrateDirectories(ctx context.Context, home, target string, location backupLocation, mappings [][2]string, archiveHomeOnly bool, run func(...string) error) error {
	if !filepath.IsAbs(home) {
		return errors.New("a pasta pessoal precisa ter um caminho absoluto")
	}
	if err := sdkRealDirectoryPath(home); err != nil {
		return err
	}
	if err := validateBackupPath(target); err != nil {
		return err
	}
	if pathInside(home, target) || pathInside(target, home) ||
		pathInside(location.path, home) || pathInside(home, location.path) ||
		pathInside(location.path, target) || pathInside(target, location.path) {
		return errors.New("a pasta pessoal e os destinos da migração não podem se sobrepor")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := ensureDirectoryRoot(target, 0755, run); err != nil {
		return err
	}
	unlock, err := lockDirectory(target, "migração para "+target)
	if err != nil {
		return err
	}
	defer unlock()
	ops := systemSDKOps(ctx, location.path)
	ops.archiveHomeOnly = archiveHomeOnly
	ops.prepareBackup = sync.OnceValue(location.prepare)
	if !archiveHomeOnly {
		if err := ops.prepareBackup(); err != nil {
			return err
		}
		fmt.Println("Preparando as cópias e os backups…")
	} else {
		ops.reportArchive = func(source, backup string) {
			fmt.Printf("Arquivando %s em %s…\n", source, backup)
		}
	}
	ops.repairPermissions = func(path string) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		fmt.Printf("Corrigindo acesso do usuário em %s antes de migrar…\n", path)
		return repairMigrationPermissions(path, os.Getuid(), os.Getgid(), run)
	}
	return prepareDirectoryLinks(home, target, mappings, ops)
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

func ensureDirectoryRoot(path string, mode os.FileMode, run func(...string) error) error {
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
