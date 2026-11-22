package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
)

func gitSigningKey(console sshConsole, name, email, hostname, previous string) (string, error) {
	output, err := console.commands.capture("", "gpg", "--batch", "--with-colons", "--with-fingerprint", "--list-secret-keys")
	if err != nil {
		return "", err
	}
	keys := signingFingerprints(output, email)
	previous = strings.TrimSuffix(strings.TrimPrefix(strings.ToUpper(previous), "0X"), "!")
	if (len(previous) == 16 || validSigningFingerprint(previous)) && len(keys) > 0 {
		var matches []string
		for _, key := range keys {
			if strings.HasSuffix(key, previous) {
				matches = append(matches, key)
			}
		}
		if len(matches) == 1 {
			return matches[0], nil
		}
	}
	switch len(keys) {
	case 0:
		fmt.Fprintf(console.output, "Nenhuma chave GPG de assinatura encontrada para %s. Será criada uma Ed25519 válida por dois anos.\n", email)
		fingerprint, err := generateGitSigningKey(console, fmt.Sprintf("%s (%s) <%s>", name, hostname, email))
		if err != nil {
			return "", err
		}
		publicKey, err := console.commands.capture("", "gpg", "--batch", "--armor", "--export", fingerprint)
		if err != nil {
			return "", err
		}
		if !strings.Contains(publicKey, "-----BEGIN PGP PUBLIC KEY BLOCK-----") {
			return "", errors.New("GnuPG não retornou a chave pública")
		}
		fmt.Fprintln(console.output, "Chave pública para copiar e cadastrar no serviço Git:")
		fmt.Fprintln(console.output, publicKey)
		return fingerprint, nil
	case 1:
		return keys[0], nil
	default:
		for index, key := range keys {
			fmt.Fprintf(console.output, "  %d. %s\n", index+1, key)
		}
		choice, err := console.prompt("Escolha a chave GPG de assinatura pelo número: ")
		if err != nil {
			return "", err
		}
		number, err := strconv.Atoi(choice)
		if err != nil || number < 1 || number > len(keys) {
			return "", errors.New("chave GPG selecionada inválida")
		}
		return keys[number-1], nil
	}
}

func validSigningFingerprint(fingerprint string) bool {
	if len(fingerprint) != 40 && len(fingerprint) != 64 {
		return false
	}
	_, err := hex.DecodeString(fingerprint)
	return err == nil
}

func signingFingerprints(listing, email string) []string {
	var fingerprints []string
	var fingerprint string
	var active, signing, matchesEmail, primary bool
	finish := func() {
		if active && signing && matchesEmail && validSigningFingerprint(fingerprint) && !slices.Contains(fingerprints, fingerprint) {
			fingerprints = append(fingerprints, fingerprint)
		}
	}
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Split(line, ":")
		switch fields[0] {
		case "sec":
			finish()
			active = len(fields) > 11 && validGPGRecord(fields[1]) && !strings.Contains(fields[11], "D")
			signing, matchesEmail, fingerprint, primary = false, false, "", true
			signing = active && localSigningRecord(fields)
		case "ssb":
			primary = false
			signing = signing || (active && localSigningRecord(fields))
		case "fpr":
			if primary && len(fields) > 9 && fingerprint == "" {
				fingerprint = fields[9]
			}
		case "uid":
			if len(fields) > 9 && validGPGRecord(fields[1]) {
				id := strings.ToLower(strings.TrimSpace(fields[9]))
				mail := strings.ToLower(email)
				matchesEmail = matchesEmail || id == mail || strings.HasSuffix(id, "<"+mail+">")
			}
		}
	}
	finish()
	return fingerprints
}

func localSigningRecord(fields []string) bool {
	return len(fields) > 14 && fields[14] != "" && fields[14] != "#" &&
		validGPGRecord(fields[1]) && strings.Contains(fields[11], "s")
}

func validGPGRecord(validity string) bool {
	switch validity {
	case "e", "r", "i", "d":
		return false
	default:
		return true
	}
}

func generateGitSigningKey(console sshConsole, identity string) (string, error) {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return "", errors.New("a geração da chave GPG precisa de um terminal interativo")
	}
	fmt.Fprint(console.output, "Senha da nova chave GPG: ")
	password, err := term.ReadPassword(fd)
	fmt.Fprintln(console.output)
	defer clear(password)
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(password)) == 0 {
		return "", errors.New("a senha da chave GPG não pode ficar vazia")
	}
	fmt.Fprint(console.output, "Confirme a senha: ")
	confirmation, err := term.ReadPassword(fd)
	fmt.Fprintln(console.output)
	defer clear(confirmation)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(password, confirmation) {
		return "", errors.New("as senhas não coincidem")
	}
	input := make([]byte, len(password)+1)
	copy(input, password)
	input[len(password)] = '\n'
	defer clear(input)
	command := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase-fd", "0",
		"--status-fd", "1", "--quick-generate-key", identity, "ed25519", "sign", "2y")
	command.Stdin = bytes.NewReader(input)
	command.Stderr = os.Stderr
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("gerar chave GPG: %w", err)
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "[GNUPG:]" && fields[1] == "KEY_CREATED" && validSigningFingerprint(fields[3]) {
			return fields[3], nil
		}
	}
	return "", errors.New("fingerprint da chave gerada não encontrado")
}
