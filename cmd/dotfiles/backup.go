package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"
)

const backupDestination = "/mnt/backups"

func backupDestinationReady(destination string, uid, gid int) bool {
	info, err := os.Stat(destination)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == uid && int(stat.Gid) == gid
}

func backupDestinationSetupArgs(destination string, uid, gid int) []string {
	return []string{"install", "-d", "-m", "0700", "-o", strconv.Itoa(uid), "-g", strconv.Itoa(gid), destination}
}

func backupDestinationSetupSteps(destination string, uid, gid int) []step {
	if backupDestinationReady(destination, uid, gid) {
		return nil
	}
	return withSudo(sudoStep("Preparar "+destination, false, backupDestinationSetupArgs(destination, uid, gid)...))
}

func runBrowserBackup(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("localizar diretório pessoal: %w", err)
	}
	return backupBrowser(ctx, home, backupDestination)
}

func backupBrowser(ctx context.Context, home, destination string) error {
	return backupBrowserAt(ctx, home, destination, "/proc")
}

func backupBrowserAt(ctx context.Context, home, destination, procRoot string) error {
	open, err := zenIsOpen(procRoot)
	if err != nil {
		return fmt.Errorf("verificar se o Zen está aberto: %w", err)
	}
	if open {
		return fmt.Errorf("feche o Zen Browser antes de iniciar o backup")
	}

	info, err := os.Stat(destination)
	if err != nil {
		return fmt.Errorf("acessar %s: %w", destination, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s não é um diretório", destination)
	}

	archive, err := os.CreateTemp(destination, ".zen-backup-*.tar")
	if err != nil {
		return fmt.Errorf("criar arquivo temporário em %s: %w", destination, err)
	}
	archivePath := archive.Name()
	if err := archive.Close(); err != nil {
		return fmt.Errorf("fechar arquivo temporário: %w", err)
	}
	defer os.Remove(archivePath)

	args := append([]string{"-cvf", archivePath, "-C", home}, zenBackupRoots...)
	cmd := exec.CommandContext(ctx, "tar", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return fmt.Errorf("criar backup: %w: %s", err, detail)
		}
		return fmt.Errorf("criar backup: %w", err)
	}
	if err := os.Chmod(archivePath, 0600); err != nil {
		return fmt.Errorf("proteger arquivo de backup: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("backup cancelado: %w", err)
	}
	open, err = zenIsOpen(procRoot)
	if err != nil {
		return fmt.Errorf("verificar se o Zen está aberto: %w", err)
	}
	if open {
		return fmt.Errorf("o Zen Browser foi aberto durante o backup; feche-o e tente novamente")
	}
	if err := os.Rename(archivePath, filepath.Join(destination, "zen-backup.tar")); err != nil {
		return fmt.Errorf("mover backup para %s: %w", destination, err)
	}
	return nil
}

func zenIsOpen(procRoot string) (bool, error) {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		pidPath := filepath.Join(procRoot, entry.Name())
		comm, err := os.ReadFile(filepath.Join(pidPath, "comm"))
		if err == nil && isZenName(strings.TrimSpace(string(comm))) {
			return true, nil
		}
		exe, err := os.Readlink(filepath.Join(pidPath, "exe"))
		if err == nil && isZenName(filepath.Base(strings.TrimSuffix(exe, " (deleted)"))) {
			return true, nil
		}
	}
	return false, nil
}

func isZenName(name string) bool {
	switch strings.ToLower(name) {
	case "zen", "zen-bin", "zen-browser", "zen-browser-bin":
		return true
	}
	return false
}

func browserBackupJobFor(setup []step, run func() error) job {
	steps := append([]step{}, setup...)
	steps = append(steps, nativeStep("Criar zen-backup.tar", run))
	return job{
		title: "Backup do Navegador",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Backup salvo em /mnt/backups/zen-backup.tar.")
		},
	}
}

func browserBackupJob() job {
	setup := backupDestinationSetupSteps(backupDestination, os.Getuid(), os.Getgid())
	return browserBackupJobFor(setup, func() error {
		return runBrowserBackup(context.Background())
	})
}
