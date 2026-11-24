package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/x/term"
)

func gitSigningKey(console sshConsole, name, email, hostname, previous string) (string, error) {
	list := []string{"gpg", "--batch", "--with-colons", "--with-fingerprint", "--with-subkey-fingerprint", "--list-secret-keys"}
	if previous != "" {
		selector, force := strings.CutSuffix(previous, "!")
		var exactKey string
		if force {
			exactKey = strings.TrimPrefix(strings.ToUpper(selector), "0X")
		}
		if output, err := console.commands.capture("", append(list, "--", selector)...); err == nil &&
			(!force || exactKey != "") && len(signingFingerprints(output, "", exactKey)) == 1 {
			return previous, nil
		}
	}
	output, err := console.commands.capture("", list...)
	if err != nil {
		return "", err
	}
	keys := signingFingerprints(output, email, "")
	if len(keys) == 0 {
		keys = signingFingerprints(output, "", "")
	}
	switch len(keys) {
	case 0:
		fmt.Fprintf(console.output, "Nenhuma chave GPG de assinatura encontrada. Será criada uma RSA de 4096 bits para %s, válida por dois anos.\n", email)
		var fingerprint string
		generate := func() error {
			var err error
			fingerprint, err = generateGitSigningKey(os.Stdout, name, email, hostname)
			return err
		}
		if console.withTerminal != nil {
			err = console.withTerminal(generate)
		} else {
			err = generate()
		}
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

func signingFingerprints(listing, email, exactKey string) []string {
	var fingerprints []string
	var fingerprint string
	var active, signing, matchesEmail, primary, eligible bool
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
			signing, matchesEmail, fingerprint, primary = false, email == "", "", true
			eligible = active && localSigningRecord(fields)
		case "ssb":
			primary = false
			eligible = active && localSigningRecord(fields)
		case "fpr":
			if len(fields) > 9 {
				if primary && fingerprint == "" {
					fingerprint = fields[9]
				}
				signing = signing || (eligible && strings.HasSuffix(fields[9], exactKey))
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

func generateGitSigningKey(output io.Writer, name, email, hostname string) (string, error) {
	fd := os.Stdin.Fd()
	if !term.IsTerminal(fd) {
		return "", errors.New("a geração da chave GPG precisa de um terminal interativo")
	}
	fmt.Fprint(output, "Senha da nova chave GPG: ")
	password, err := term.ReadPassword(fd)
	fmt.Fprintln(output)
	defer clear(password)
	if err != nil {
		return "", err
	}
	if len(bytes.TrimSpace(password)) == 0 {
		return "", errors.New("a senha da chave GPG não pode ficar vazia")
	}
	fmt.Fprint(output, "Confirme a senha: ")
	confirmation, err := term.ReadPassword(fd)
	fmt.Fprintln(output)
	defer clear(confirmation)
	if err != nil {
		return "", err
	}
	if !bytes.Equal(password, confirmation) {
		return "", errors.New("as senhas não coincidem")
	}
	return createGitSigningKey(name, email, hostname, password)
}

func createGitSigningKey(name, email, hostname string, password []byte) (string, error) {
	input := make([]byte, len(password)+1)
	copy(input, password)
	input[len(password)] = '\n'
	defer clear(input)
	reader, writer, err := os.Pipe()
	if err != nil {
		return "", err
	}
	defer reader.Close()
	defer writer.Close()
	command := exec.Command("gpg", "--batch", "--pinentry-mode", "loopback", "--passphrase-fd", "3",
		"--status-fd", "1", "--default-new-key-algo", "rsa4096", "--gen-key")
	// Batch generation needs an explicit Key-Length to override GnuPG's batch default.
	command.Stdin = strings.NewReader(fmt.Sprintf(`Key-Type: RSA
Key-Length: 4096
Key-Usage: sign
Name-Real: %s
Name-Comment: %s
Name-Email: %s
Expire-Date: 2y
%%commit
`, name, hostname, email))
	command.ExtraFiles = []*os.File{reader}
	command.Stderr = os.Stderr
	var output bytes.Buffer
	command.Stdout = &output
	if err := command.Start(); err != nil {
		return "", fmt.Errorf("gerar chave GPG: %w", err)
	}
	_ = reader.Close()
	_, writeErr := writer.Write(input)
	closeErr := writer.Close()
	if err := errors.Join(command.Wait(), writeErr, closeErr); err != nil {
		return "", fmt.Errorf("gerar chave GPG: %w", err)
	}
	for _, line := range strings.Split(output.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 4 && fields[0] == "[GNUPG:]" && fields[1] == "KEY_CREATED" && validSigningFingerprint(fields[3]) {
			return fields[3], nil
		}
	}
	return "", errors.New("fingerprint da chave gerada não encontrado")
}
