package main

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

func runBackupAction(p profile, action string) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute o backup pelo menu Desktop ou Laptop como usuário normal")
	}
	console := sshConsole{input: bufio.NewReader(os.Stdin), output: os.Stdout, commands: systemSSHCommands()}
	var description string
	var backup func() (string, error)
	switch action {
	case "snapshot":
		location := managedBackup("Snapshots", console.commands.interactive)
		description = "Gravar um backup Btrfs completo de / em " + location.path + "."
		backup = func() (string, error) {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			return backupBtrfsRoot(location, home, console.commands, sendBtrfsRoot)
		}
	case "projects":
		description = "Enviar commits de /mnt/workspaces aos remotos Git origin, ogle e backup."
		backup = func() (string, error) {
			count, err := backupProjectRepositories(workspacesRoot, console.commands)
			return fmt.Sprintf("%d projeto(s) enviado(s) aos três remotos.", count), err
		}
	case "gpg":
		location := managedBackup("GPG", console.commands.interactive)
		description = "Exportar as chaves públicas e privadas do GPG para " + location.path + "."
		backup = func() (string, error) {
			return backupGPGKeys(location, exportGPGKeys)
		}
	default:
		return fmt.Errorf("ação de backup inválida: %s", action)
	}

	printActionHeader()
	fmt.Fprintln(console.output, description)
	if err := console.confirm("Digite CONFIRMO para executar: ", "CONFIRMO"); err != nil {
		return err
	}
	unlock, err := lockOperation("workstation-backup-"+action, "backup")
	if err != nil {
		return err
	}
	result, err := backup()
	unlock()
	if err != nil {
		fmt.Fprintln(console.output, err)
	} else {
		fmt.Fprintln(console.output, result)
	}
	_, _ = console.prompt("\nEnter para voltar ao menu...")
	return err
}

func checkBtrfsBackupSource(home string, commands sshCommands) error {
	findFilesystem := func(path string) ([]string, error) {
		output, err := commands.capture("", "findmnt", "-rn", "-o", "FSTYPE,UUID", "-T", path)
		return strings.Fields(output), err
	}
	source, err := findFilesystem("/")
	if err != nil {
		return err
	}
	if len(source) != 2 || source[0] != "btrfs" {
		return errors.New("o backup exige um subvolume Btrfs em /")
	}

	// Check the user's SSH files against the containing subvolume, including linked directories.
	sshPath, err := filepath.EvalSymlinks(filepath.Join(home, ".ssh"))
	if errors.Is(err, os.ErrNotExist) {
		sshPath, err = filepath.EvalSymlinks(home)
	}
	if err != nil {
		return err
	}
	sshFilesystem, err := findFilesystem(sshPath)
	if err != nil {
		return err
	}
	if len(sshFilesystem) == 0 || (sshFilesystem[0] == "btrfs" && len(sshFilesystem) != 2) {
		return fmt.Errorf("não foi possível identificar o sistema de arquivos de %s", sshPath)
	}
	if sshFilesystem[0] == "btrfs" && sshFilesystem[1] == source[1] {
		rootID := func(path string) (uint64, error) {
			output, err := commands.capture("", "btrfs", "inspect-internal", "rootid", path)
			if err != nil {
				return 0, err
			}
			return strconv.ParseUint(strings.TrimSpace(output), 10, 64)
		}
		sourceID, err := rootID("/")
		if err != nil {
			return err
		}
		sshID, err := rootID(sshPath)
		if err != nil {
			return err
		}
		if sshID == sourceID {
			return errors.New("o backup incluiria ~/.ssh no subvolume raiz")
		}
	}
	return nil
}

func exportGPGKeys(private bool) ([]byte, error) {
	action, kind := "--export", "públicas"
	if private {
		action, kind = "--export-secret-keys", "privadas"
	}
	cmd := exec.Command("gpg", "--armor", action)
	cmd.Stdin, cmd.Stderr = os.Stdin, os.Stderr
	if term.IsTerminal(os.Stdin.Fd()) {
		if tty, err := os.Readlink("/proc/self/fd/0"); err == nil && strings.HasPrefix(tty, "/dev/") {
			cmd.Env = append(cmd.Environ(), "GPG_TTY="+tty)
		}
	}
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("exportar chaves %s: %w", kind, err)
	}
	return output, nil
}

func backupGPGKeys(location backupLocation, export func(bool) ([]byte, error)) (string, error) {
	public, err := export(false)
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(public, []byte("-----BEGIN PGP PUBLIC KEY BLOCK-----")) {
		return "", errors.New("nenhuma chave GPG pública encontrada para exportação")
	}
	private, err := export(true)
	defer clear(private)
	if err != nil {
		return "", err
	}
	if !bytes.HasPrefix(private, []byte("-----BEGIN PGP PRIVATE KEY BLOCK-----")) {
		return "", errors.New("nenhuma chave GPG privada encontrada para exportação")
	}
	if err := location.prepare(); err != nil {
		return "", err
	}
	staging, err := os.MkdirTemp(location.path, ".gpg-"+time.Now().UTC().Format("20060102T150405Z")+"-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	if err := writeConfigAtomic(filepath.Join(staging, "minha_chave_publica.asc"), public, 0644); err != nil {
		return "", err
	}
	if err := writeConfigAtomic(filepath.Join(staging, "minha_chave_privada.asc"), private, 0600); err != nil {
		return "", err
	}
	directory := filepath.Join(location.path, strings.TrimPrefix(filepath.Base(staging), "."))
	if err := os.Rename(staging, directory); err != nil {
		return "", err
	}
	return directory, nil
}
