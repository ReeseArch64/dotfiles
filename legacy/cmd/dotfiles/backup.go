package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/lipgloss"
)

const (
	backupDestination = "/mnt/backups"
)

func backupDestinationReady(destination string, uid, gid int) bool {
	if err := validateBackupDirectoryPath(destination); err != nil {
		return false
	}
	info, err := os.Lstat(destination)
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
	if err := validateBackupDirectoryPath(destination); err != nil {
		return []step{nativeStep("Validar diretório de backup", func() error { return err })}
	}
	if backupDestinationReady(destination, uid, gid) {
		return nil
	}
	prepare := step{label: "Preparar " + destination, run: func() (string, error) {
		if err := validateBackupDirectoryPath(destination); err != nil {
			return "", err
		}
		return sudoRun(backupDestinationSetupArgs(destination, uid, gid)...)
	}}
	return withSudo(prepare)
}

func restorePointPath(at time.Time) string {
	return filepath.Join(localBackupRoot(), "snapshots", fmt.Sprintf("%x", sha256.Sum256([]byte("/"))), at.UTC().Format("20060102T150405.000000000Z")+"-"+rand.Text(), "original")
}

const restorePointScript = `set -euo pipefail
destination=$1
root=$2
[[ $root = /* && -d $root ]] || { echo "Diretório de backup deve ser absoluto e existente: $root" >&2; exit 1; }
# Snapshotting across Btrfs subvolumes is supported; comparing st_dev is not.
read -r source_type source_uuid < <(findmnt -n -o FSTYPE,UUID -T /) || { echo 'Não foi possível identificar o filesystem Btrfs de /.' >&2; exit 1; }
read -r target_type target_uuid < <(findmnt -n -o FSTYPE,UUID -T "$root") || { echo "Não foi possível identificar o filesystem de $root." >&2; exit 1; }
if [[ $source_type != btrfs || $target_type != btrfs || -z $source_uuid || $source_uuid != "$target_uuid" ]]; then
    echo "Snapshot Btrfs requer / e $root no mesmo filesystem Btrfs (UUID). Nenhum fallback fora de $root." >&2
    exit 1
fi
btrfs subvolume show / >/dev/null || { echo 'A raiz / deve ser um subvolume Btrfs para criar o snapshot.' >&2; exit 1; }
parent=$(dirname -- "$destination")
category=$root/snapshots
path=$root
while [[ $path != / ]]; do
    [[ ! -L $path ]] || { echo "Diretório de backup não pode ser symlink: $path" >&2; exit 1; }
    path=$(dirname -- "$path")
done
umask 077
for path in "$category" "$(dirname -- "$parent")"; do
    if [[ -e $path || -L $path ]]; then
        [[ -d $path && ! -L $path && $(stat -c %u "$path") = "$EUID" && $(stat -c %a "$path") = 700 ]] || { echo "Diretório de snapshot deve ser privado e pertencer ao usuário atual: $path" >&2; exit 1; }
    else
        mkdir -- "$path"
    fi
done
# Reserve a generation exclusively; never reuse even an incomplete record.
mkdir -- "$parent" || { echo "Registro de snapshot já existe ou inacessível: $parent" >&2; exit 1; }
printf '{"origin":"/","created_utc":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%S.%NZ)" > "$parent/origin.json"
btrfs subvolume snapshot / "$destination"
`

func restorePointSnapshotArgs(destination string) []string {
	return []string{"bash", "-c", restorePointScript, "dotfiles-restore-point", destination, localBackupRoot()}
}

func restorePointJobFor(destination string, steps []step) job {
	return job{
		title: "Ponto de Restauração",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Ponto de restauração criado em " + destination + ".")
		},
	}
}

func restorePointJob() job {
	destination := restorePointPath(time.Now())
	steps := withSudo(
		sudoStep("Criar snapshot Btrfs", false, restorePointSnapshotArgs(destination)...),
	)
	return restorePointJobFor(destination, steps)
}

func runSSHKeysBackup(device deviceProfile) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("localizar diretório pessoal: %w", err)
	}
	return backupSSHKeys(filepath.Join(home, ".ssh"), localBackupRoot(), requiredSSHIdentityFiles(device))
}

func backupSSHKeys(sshDir, root string, names []string) error {
	var missing []string
	for _, name := range names {
		info, err := os.Stat(filepath.Join(sshDir, name))
		if err != nil || !info.Mode().IsRegular() {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return errors.New("chaves ausentes em " + sshDir + ": " + strings.Join(missing, ", "))
	}
	destination, err := reserveBackupAt(root, "ssh", sshDir, time.Now())
	if err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(filepath.Dir(destination))
		}
	}()

	for _, name := range names {
		if err := copyFileAtomic(filepath.Join(sshDir, name), filepath.Join(destination, name)); err != nil {
			return fmt.Errorf("copiar %s: %w", name, err)
		}
	}
	complete = true
	return nil
}

func sshKeysBackupJobFor(setup []step, run func() error) job {
	steps := append([]step{}, setup...)
	steps = append(steps, nativeStep("Copiar chaves SSH", run))
	return job{
		title: "Backup das Chaves SSH",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Chaves SSH salvas em " + filepath.Join(localBackupRoot(), "ssh") + " (geração por origem/data).")
		},
	}
}

func sshKeysBackupJob(device deviceProfile) job {
	setup := backupDestinationSetupSteps(localBackupRoot(), os.Getuid(), os.Getgid())
	return sshKeysBackupJobFor(setup, func() error { return runSSHKeysBackup(device) })
}

type gpgExporter func(context.Context, bool) ([]byte, error)

func exportGPGKey(ctx context.Context, secret bool) ([]byte, error) {
	args := []string{"--armor", "--export"}
	kind := "pública"
	if secret {
		args = []string{"--armor", "--export-secret-keys"}
		kind = "privada"
	}
	cmd := exec.CommandContext(ctx, "gpg", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail != "" {
			return nil, fmt.Errorf("exportar chave %s: %w: %s", kind, err, detail)
		}
		return nil, fmt.Errorf("exportar chave %s: %w", kind, err)
	}
	header := "-----BEGIN PGP PUBLIC KEY BLOCK-----"
	if secret {
		header = "-----BEGIN PGP PRIVATE KEY BLOCK-----"
	}
	if !bytes.Contains(output, []byte(header)) {
		return nil, fmt.Errorf("nenhuma chave %s encontrada", kind)
	}
	return output, nil
}

func writeBackupFileAtomic(path string, content []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".gpg-backup-*")
	if err != nil {
		return fmt.Errorf("criar arquivo temporário: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err = temporary.Write(content); err == nil {
		err = temporary.Chmod(mode)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("gravar %s: %w", path, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("substituir %s: %w", path, err)
	}
	return nil
}

func backupGPGKeys(ctx context.Context, root string, export gpgExporter) error {
	publicKey, err := export(ctx, false)
	if err != nil {
		return err
	}
	privateKey, err := export(ctx, true)
	if err != nil {
		return err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	destination, err := reserveBackupAt(root, "gpg", filepath.Join(home, ".gnupg"), time.Now())
	if err != nil {
		return err
	}
	if err := os.Mkdir(destination, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(filepath.Dir(destination))
		}
	}()

	if err := writeBackupFileAtomic(filepath.Join(destination, "minha_chave_publica.asc"), publicKey, 0644); err != nil {
		return err
	}
	if err := writeBackupFileAtomic(filepath.Join(destination, "minha_chave_privada.asc"), privateKey, 0600); err != nil {
		return err
	}
	complete = true
	return nil
}

func gpgBackupJobFor(setup []step, run func() error) job {
	steps := append([]step{}, setup...)
	steps = append(steps, nativeStep("Exportar chaves GPG", run))
	return job{
		title: "Backup das Chaves GPG",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render("Chaves GPG salvas em " + filepath.Join(localBackupRoot(), "gpg") + " (geração por origem/data).")
		},
	}
}

func gpgBackupJob() job {
	setup := backupDestinationSetupSteps(localBackupRoot(), os.Getuid(), os.Getgid())
	return gpgBackupJobFor(setup, func() error {
		return backupGPGKeys(context.Background(), localBackupRoot(), exportGPGKey)
	})
}
