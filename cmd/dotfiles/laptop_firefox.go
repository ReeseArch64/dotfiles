package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

const laptopFirefoxRestorePath = "/mnt/backups/Restore Firefox"

func validateLaptopFirefoxHome(home string, uid int) error {
	if uid <= 0 {
		return fmt.Errorf("execute como usuário normal, não com sudo")
	}
	path := filepath.Join(home, ".config", "mozilla")
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s já deve existir antes de configurar Firefox: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s deve ser um diretório", path)
	}
	return nil
}

func prepareLaptopFirefoxRestore(path string, uid, gid int, run func(...string) error) error {
	if uid <= 0 || gid < 0 {
		return fmt.Errorf("usuário/grupo inválidos para restauração do Firefox")
	}
	// Não permita que sudo atribua permissões a um alvo redirecionado por link.
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("verificar %s: %w", current, err)
		}
		if err == nil && !info.IsDir() {
			return fmt.Errorf("%s deve ser diretório real, não arquivo ou link simbólico", current)
		}
		if parent := filepath.Dir(current); parent == current {
			break
		}
	}
	// install -d cria os pais ausentes e aplica dono/grupo/modo somente ao alvo.
	// Nunca usar chown/chmod recursivo: outros backups permanecem intactos.
	if err := run("sudo", "-n", "install", "-d", "-m", "0700", "-o", strconv.Itoa(uid), "-g", strconv.Itoa(gid), "--", path); err != nil {
		return fmt.Errorf("criar pasta de restauração do Firefox: %w", err)
	}
	// Confirme acesso pelo próprio usuário, não apenas pelo processo sudo.
	probe, err := os.CreateTemp(path, ".dotfiles-write-test-*")
	if err != nil {
		return fmt.Errorf("usuário sem acesso de escrita em %s; confira também as permissões dos diretórios pais: %w", path, err)
	}
	closeErr := probe.Close()
	removeErr := os.Remove(probe.Name())
	if closeErr != nil {
		return closeErr
	}
	return removeErr
}

func laptopFirefoxJobFor(home string, uid, gid int, packageSteps []step, run func(...string) error) job {
	steps := []step{nativeStep("Validar ~/.config/mozilla existente", func() error { return validateLaptopFirefoxHome(home, uid) })}
	setup := append([]step{}, packageSteps...)
	setup = append(setup, nativeStep("Preparar /mnt/backups/Restore Firefox para o usuário", func() error {
		return prepareLaptopFirefoxRestore(laptopFirefoxRestorePath, uid, gid, run)
	}), terminalStep("Definir Firefox como navegador padrão", "env", "-u", "BROWSER", "xdg-settings", "set", "default-web-browser", "firefox.desktop"))
	steps = append(steps, withSudo(setup...)...)
	return job{title: "Configurar Firefox", steps: steps}
}

func laptopFirefoxJob() job {
	home, err := os.UserHomeDir()
	if err != nil {
		return job{title: "Configurar Firefox", steps: []step{nativeStep("Identificar pasta pessoal", func() error { return err })}}
	}
	return laptopFirefoxJobFor(home, os.Getuid(), os.Getgid(), ensurePkgs("firefox", "xdg-utils"), func(argv ...string) error {
		output, err := exec.Command(argv[0], argv[1:]...).CombinedOutput()
		if err != nil {
			return fmt.Errorf("%w: %s", err, output)
		}
		return nil
	})
}
