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

func runBrowserBackup(ctx context.Context) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("localizar diretório pessoal: %w", err)
	}
	return backupBrowser(ctx, home, localBackupRoot())
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

	payload, err := reserveBackupAt(destination, "browser", home, time.Now())
	if err != nil {
		return err
	}
	if err := os.Mkdir(payload, 0700); err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(filepath.Dir(payload))
		}
	}()
	archive, err := os.CreateTemp(payload, ".zen-backup-*.tar")
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
	if err := os.Rename(archivePath, filepath.Join(payload, "zen-backup.tar")); err != nil {
		return fmt.Errorf("mover backup para %s: %w", destination, err)
	}
	complete = true
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
			return lipgloss.NewStyle().Foreground(colOK).Render("Backup salvo em " + filepath.Join(localBackupRoot(), "browser") + " (geração por origem/data).")
		},
	}
}

func browserBackupJob() job {
	setup := backupDestinationSetupSteps(localBackupRoot(), os.Getuid(), os.Getgid())
	return browserBackupJobFor(setup, func() error {
		return runBrowserBackup(context.Background())
	})
}

func runSSHKeysBackup() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("localizar diretório pessoal: %w", err)
	}
	return backupSSHKeys(filepath.Join(home, ".ssh"), localBackupRoot())
}

func backupSSHKeys(sshDir, root string) error {
	var missing []string
	for _, name := range requiredSSHIdentityFiles {
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

	for _, name := range requiredSSHIdentityFiles {
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

func sshKeysBackupJob() job {
	setup := backupDestinationSetupSteps(localBackupRoot(), os.Getuid(), os.Getgid())
	return sshKeysBackupJobFor(setup, runSSHKeysBackup)
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
