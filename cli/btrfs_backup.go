package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const btrfsStreamMagic = "btrfs-stream\x00"

// Only the snapshot and send run as root. The destination is opened by Dotfiles.
const sendBtrfsRootScript = `set -eu
umask 077
directory=$(mktemp -d /.dotfiles-btrfs-XXXXXXXX)
snapshot=$directory/root
cleanup() {
    status=$?
    trap - EXIT
    if [ -d "$snapshot" ]; then
        btrfs subvolume delete -- "$snapshot" >&2 || exit 1
    fi
    rmdir -- "$directory" || exit 1
    exit "$status"
}
trap cleanup EXIT
trap 'exit 130' HUP INT TERM
btrfs subvolume snapshot -r / "$snapshot" >&2
btrfs send "$snapshot"
`

func sendBtrfsRoot(destination *os.File) error {
	cmd := exec.Command("sudo", "--", "sh", "-c", sendBtrfsRootScript)
	cmd.Stdout, cmd.Stderr = destination, os.Stderr
	return cmd.Run()
}

func backupBtrfsRoot(location backupLocation, home string, commands sshCommands, send func(*os.File) error) (string, error) {
	if err := checkBtrfsBackupSource(home, commands); err != nil {
		return "", err
	}
	if err := location.prepare(); err != nil {
		return "", err
	}
	file, err := os.CreateTemp(location.path, ".root-"+time.Now().UTC().Format("20060102T150405Z")+"-*")
	if err != nil {
		return "", err
	}
	staging := file.Name()
	defer os.Remove(staging)
	defer file.Close()
	if err := send(file); err != nil {
		return "", fmt.Errorf("gerar backup com btrfs send: %w", err)
	}
	var header [len(btrfsStreamMagic)]byte
	if _, err := file.ReadAt(header[:], 0); err != nil || string(header[:]) != btrfsStreamMagic {
		return "", errors.New("btrfs send não retornou um stream Btrfs válido")
	}
	if err := errors.Join(file.Sync(), file.Close()); err != nil {
		return "", err
	}
	destination := filepath.Join(location.path, strings.TrimPrefix(filepath.Base(staging), ".")+".btrfs")
	if err := os.Rename(staging, destination); err != nil {
		return "", err
	}
	return destination, nil
}
