package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupJobsRejectSymlinkStoreBeforePrivilegedSetup(t *testing.T) {
	for _, kind := range []string{"destination", "ancestor"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			referent := filepath.Join(root, "referent")
			if err := os.Mkdir(referent, 0755); err != nil {
				t.Fatal(err)
			}
			link := filepath.Join(root, "link")
			if err := os.Symlink(referent, link); err != nil {
				t.Fatal(err)
			}
			destination := link
			if kind == "ancestor" {
				destination = filepath.Join(link, "backups")
			}
			t.Setenv("DOTFILES_BACKUP_DIR", destination)
			bin := t.TempDir()
			marker := filepath.Join(root, "sudo-called")
			t.Setenv("SUDO_MARKER", marker)
			writeTestExecutable(t, bin, "sudo", `touch "$SUDO_MARKER"; exit 99`)
			t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
			if backupDestinationReady(destination, os.Getuid(), os.Getgid()) {
				t.Fatal("symlink aceito como diretório de backup pronto")
			}
			for _, configured := range []job{browserBackupJob(), sshKeysBackupJob(deviceDesktop), gpgBackupJob()} {
				if len(configured.steps) == 0 || configured.steps[0].run == nil {
					t.Fatalf("%s não valida o caminho antes dos comandos privilegiados", configured.title)
				}
				if _, err := configured.steps[0].run(); err == nil {
					t.Fatalf("%s aceitou caminho inseguro", configured.title)
				}
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatal("sudo foi chamado antes de rejeitar o caminho")
			}
			info, err := os.Stat(referent)
			if err != nil || info.Mode().Perm() != 0755 {
				t.Fatalf("destino externo foi alterado: %v %v", info, err)
			}
		})
	}
}

func TestBackupSetupRevalidatesPathBeforeInstall(t *testing.T) {
	root := t.TempDir()
	destination := filepath.Join(root, "backups")
	if err := os.Mkdir(destination, 0755); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marker := filepath.Join(root, "sudo-install-called")
	t.Setenv("SUDO_MARKER", marker)
	writeTestExecutable(t, bin, "sudo", `if [ "$*" = '-n true' ]; then exit 0; fi; touch "$SUDO_MARKER"; exit 99`)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	steps := backupDestinationSetupSteps(destination, os.Getuid(), os.Getgid())
	if len(steps) != 1 {
		t.Fatalf("passos inesperados: %v", jobStepLabels(job{steps: steps}))
	}
	if err := os.Remove(destination); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), destination); err != nil {
		t.Fatal(err)
	}
	if _, err := steps[0].run(); err == nil {
		t.Fatal("symlink criado após o agendamento foi aceito")
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("sudo install foi chamado sobre o symlink")
	}
}
