package main

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"strings"
)

// Preserve other SDDM settings, replacing all autologin sections. The main
// configuration overrides drop-ins in /etc/sddm.conf.d.
func sddmAutologinConfig(existing, username string) (string, error) {
	if username == "" || username == "root" || strings.ContainsAny(username, "\r\n\x00=[]") || strings.TrimSpace(username) != username {
		return "", fmt.Errorf("usuário inválido para login automático: %q", username)
	}
	var lines []string
	autologin := false
	for _, line := range strings.Split(existing, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			autologin = trimmed == "[Autologin]"
		}
		if !autologin {
			lines = append(lines, line)
		}
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n\n[Autologin]\nUser=" + username + "\nSession=niri.desktop\nRelogin=false\n", nil
}

func autologinJob() job {
	var config string
	return job{
		title: "Ativar login automático",
		steps: []step{
			nativeStep("Preparar login automático no SDDM para o usuário atual", func() error {
				dm, err := os.Readlink("/etc/systemd/system/display-manager.service")
				if err != nil || !strings.HasSuffix(dm, "/sddm.service") {
					return fmt.Errorf("login automático exige SDDM como gerenciador de login")
				}
				if _, err := os.Stat("/usr/share/wayland-sessions/niri.desktop"); err != nil {
					return err
				}
				current, err := user.Current()
				if err != nil {
					return err
				}
				if os.Geteuid() == 0 {
					return fmt.Errorf("execute a CLI como usuário normal, sem sudo")
				}
				existing, err := os.ReadFile("/etc/sddm.conf")
				if err != nil && !os.IsNotExist(err) {
					return err
				}
				config, err = sddmAutologinConfig(string(existing), current.Username)
				return err
			}),
			{
				label: "Salvar backup e ativar login automático (sudo)",
				cmd: func() *exec.Cmd {
					// Keep the first backup, and replace the config atomically without
					// restarting SDDM (which would terminate the current graphical session).
					command := exec.Command("sudo", "sh", "-c", `set -eu
if [ -e /etc/sddm.conf ]; then
 cp -pn /etc/sddm.conf /etc/sddm.conf.before-autologin
fi
tmp=$(mktemp /etc/sddm.conf.XXXXXX)
trap 'rm -f "$tmp"' EXIT
cat > "$tmp"
chmod 644 "$tmp"
mv -f "$tmp" /etc/sddm.conf
`)
					command.Stdin = strings.NewReader(config)
					return command
				},
			},
		},
		result: func() string {
			return "Login automático no Niri ativado para a próxima inicialização.\nA senha da conta, o sudo e o bloqueio de tela permanecem inalterados."
		},
	}
}
