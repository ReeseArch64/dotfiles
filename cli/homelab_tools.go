package main

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

type homelabToolAsset struct {
	name, sha256 string
}

const (
	fastfetchReleaseURL  = "https://github.com/fastfetch-cli/fastfetch/releases/download/2.70.0/"
	lazydockerReleaseURL = "https://github.com/jesseduffield/lazydocker/releases/download/v0.25.2/"
)

var homelabFastfetchAssets = map[string]homelabToolAsset{
	"amd64": {"fastfetch-linux-amd64.deb", "06e9a1be9a863dc815f8a31a6b1abec7b29967be30084ffcc29c1bca072d801b"},
	"arm64": {"fastfetch-linux-aarch64.deb", "25053651486d97b8731dd6bc59e725944e39adcdcd903741334c1c3282a7281d"},
}

var homelabLazydockerAssets = map[string]homelabToolAsset{
	"amd64": {"lazydocker_0.25.2_Linux_x86_64.tar.gz", "0d9dbfc26068b218e7ed84b104748cadc6e3cf733c0afd35465306fb39b9523c"},
	"arm64": {"lazydocker_0.25.2_Linux_arm64.tar.gz", "005c38b685aaa557e7d646d83a3dadb5024340eeed8c6a2e1949eee6f530de23"},
}

func homelabToolForArch(commands sshCommands, assets map[string]homelabToolAsset) (homelabToolAsset, error) {
	output, err := commands.capture("", "dpkg", "--print-architecture")
	if err != nil {
		return homelabToolAsset{}, err
	}
	arch := strings.TrimSpace(output)
	if asset, ok := assets[arch]; ok {
		return asset, nil
	}
	return homelabToolAsset{}, fmt.Errorf("arquitetura Ubuntu sem pacote disponível: %s", arch)
}

func downloadHomelabTool(commands sshCommands, directory, release string, asset homelabToolAsset) (string, error) {
	path := filepath.Join(directory, asset.name)
	if err := commands.interactive("curl", "-fsSL", "--proto", "=https", "--proto-redir", "=https", "--tlsv1.2", "--output", path, release+asset.name); err != nil {
		return "", fmt.Errorf("baixar %s: %w", asset.name, err)
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, err = io.Copy(hash, file)
	if err := errors.Join(err, file.Close()); err != nil {
		return "", err
	}
	if actual := fmt.Sprintf("%x", hash.Sum(nil)); actual != asset.sha256 {
		return "", fmt.Errorf("SHA-256 incorreto para %s", asset.name)
	}
	return path, nil
}

func installHomelabFastfetch(c *sshConsole) error {
	if _, err := c.commands.capture("", "fastfetch", "--version"); err == nil {
		return nil
	}
	asset, err := homelabToolForArch(c.commands, homelabFastfetchAssets)
	if err != nil {
		return err
	}
	if err := c.ensureUbuntuPackages("curl", "ca-certificates"); err != nil {
		return err
	}
	work, err := os.MkdirTemp("", "dotfiles-fastfetch-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	path, err := downloadHomelabTool(c.commands, work, fastfetchReleaseURL, asset)
	if err != nil {
		return err
	}
	// Allow APT's sandbox user to read the verified local package.
	if err := os.Chmod(path, 0644); err != nil {
		return err
	}
	if err := os.Chmod(work, 0755); err != nil {
		return err
	}
	if err := c.commands.interactive("sudo", "apt-get", "update"); err != nil {
		return err
	}
	if err := c.commands.interactive("sudo", "apt-get", "install", "--yes", "--", path); err != nil {
		return err
	}
	_, err = c.commands.capture("", "fastfetch", "--version")
	return err
}

func installHomelabTools(c *sshConsole, lookup func(string) (string, error)) error {
	fmt.Fprintln(c.output, "Instalar LazyDocker, jq, ripgrep, fd, fzf, bat, Git e ncdu no Ubuntu.")
	if err := c.confirm("Digite CONFIRMO para instalar: ", "CONFIRMO"); err != nil {
		return err
	}
	_, lazydockerErr := c.commands.capture("", "lazydocker", "--version")
	var asset homelabToolAsset
	if lazydockerErr != nil {
		var err error
		asset, err = homelabToolForArch(c.commands, homelabLazydockerAssets)
		if err != nil {
			return err
		}
	}
	if err := c.ensureUbuntuPackages("jq", "ripgrep", "fd-find", "fzf", "bat", "git", "ncdu", "curl", "ca-certificates"); err != nil {
		return err
	}
	if lazydockerErr != nil {
		if err := installLazydockerRelease(c.commands, asset); err != nil {
			return err
		}
	}
	for _, names := range [][2]string{{"fd", "fdfind"}, {"bat", "batcat"}} {
		if _, err := lookup(names[0]); err == nil {
			continue
		}
		source := filepath.Join("/usr/bin", names[1])
		target := filepath.Join("/usr/local/bin", names[0])
		if _, err := lookup(source); err != nil {
			return fmt.Errorf("executável Ubuntu %s indisponível: %w", source, err)
		}
		if err := c.commands.interactive("sudo", "ln", "-sT", "--", source, target); err != nil {
			return fmt.Errorf("criar link %s: %w", target, err)
		}
	}
	for _, tool := range []string{"lazydocker", "jq", "rg", "fd", "fzf", "bat", "git", "ncdu"} {
		if _, err := lookup(tool); err != nil {
			return fmt.Errorf("%s indisponível após a instalação: %w", tool, err)
		}
	}
	fmt.Fprintln(c.output, "Ferramentas instaladas. O LazyDocker usa o Docker existente.")
	return nil
}

func installLazydockerRelease(commands sshCommands, asset homelabToolAsset) error {
	work, err := os.MkdirTemp("", "dotfiles-lazydocker-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	path, err := downloadHomelabTool(commands, work, lazydockerReleaseURL, asset)
	if err != nil {
		return err
	}
	binary := filepath.Join(work, "lazydocker")
	if err := unpackLazydocker(path, binary); err != nil {
		return err
	}
	return commands.interactive("sudo", "install", "-m", "0755", "--", binary, "/usr/local/bin/lazydocker")
}

func unpackLazydocker(archive, destination string) error {
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	compressed, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer compressed.Close()
	reader := tar.NewReader(compressed)
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return errors.New("executável lazydocker ausente no pacote")
		}
		if err != nil {
			return err
		}
		if header.Name != "lazydocker" && header.Name != "./lazydocker" {
			continue
		}
		if header.Typeflag != tar.TypeReg || header.Size <= 0 {
			return errors.New("executável lazydocker inválido no pacote")
		}
		output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = io.Copy(output, reader)
		return errors.Join(err, output.Close())
	}
}
