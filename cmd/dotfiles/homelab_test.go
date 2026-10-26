package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func homelabSetup(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return setupDirectory(root, deviceServer)
}

func homelabMocks(t *testing.T) (string, string) {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "commands")
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	t.Setenv("COMMAND_LOG", log)
	t.Setenv("HOME", t.TempDir())
	writeTestExecutable(t, bin, "sudo", `if [ "$1" = -n ]; then shift; fi
if [ "$1" = -v ]; then exit 0; fi
"$@"`)
	for _, name := range []string{"apt-get", "install", "ufw", "clear"} {
		writeTestExecutable(t, bin, name, `printf '%s %s\n' "${0##*/}" "$*" >> "$COMMAND_LOG"
if [ -n "${FAIL_COMMAND:-}" ] && [ "${0##*/} $*" = "$FAIL_COMMAND" ]; then exit 9; fi`)
	}
	writeTestExecutable(t, bin, "sshd", `printf 'sshd %s\n' "$*" >> "$COMMAND_LOG"
if [ "${FAIL_SSHD:-}" = yes ]; then exit 8; fi
if [ "$1" = -T ]; then printf 'port 2222\npasswordauthentication yes\n'; fi`)
	writeTestExecutable(t, bin, "systemctl", `printf 'systemctl %s\n' "$*" >> "$COMMAND_LOG"
case "$1" in
  is-active|is-enabled) [ "${SOCKET_ACTIVE:-}" = yes ]; exit $? ;;
esac
if [ "${FAIL_RESTART:-}" = yes ] && [ "$1" = restart ]; then exit 7; fi`)
	return bin, log
}

func readHomelabFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestServerProfileHostnamePlatformAndMenu(t *testing.T) {
	for _, name := range []string{"mini-server", "home-MINI-SERVER-01"} {
		device, err := detectDevice(name)
		if err != nil || device != deviceServer {
			t.Fatalf("server detection: %v %v", device, err)
		}
	}
	for _, name := range []string{"server", "mini", "miniserver", "mini-server-desktop", "notebook-mini-server", "macbook-mini-server"} {
		if _, err := detectDevice(name); err == nil {
			t.Fatalf("accepted hostname %s", name)
		}
	}
	path := filepath.Join(t.TempDir(), "os-release")
	lookPath := func(string) (string, error) {
		t.Fatal("server must not inspect graphical stack")
		return "", errors.New("absent")
	}
	for _, id := range []string{"ubuntu", "cachyos", "debian"} {
		if err := os.WriteFile(path, []byte("ID=\""+id+"\"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		if err := checkPlatformForDevice(deviceServer, path, lookPath); (err == nil) != (id == "ubuntu") {
			t.Fatalf("platform %s: %v", id, err)
		}
	}
	if err := checkPlatformForDevice(deviceServer, path+"missing", lookPath); err == nil {
		t.Fatal("missing OS accepted")
	}
	if err := checkPlatformForDevice(deviceMacBook, path, lookPath); err == nil {
		t.Fatal("MacBook accepted")
	}
	m := newModelForDevice("/repo", deviceServer)
	if m.setupDir != "/repo/setup/homelab" {
		t.Fatal(m.setupDir)
	}
	for screen, titles := range map[screen][]string{
		screenMain:   {"Sistema", "CasaOS", "Vaultwarden HTTPS", "Liberar porta 53 (DNS)", "Sair"},
		screenSystem: {"SSH", "Firewall"},
		screenSSH:    {"Configurar SSH"}, screenFirewall: {"Configurar Firewall"},
		screenDesktop: nil, screenDevelopment: nil, screenDocker: nil, screenCleanup: nil,
	} {
		m.screen = screen
		var got []string
		for _, it := range m.items() {
			got = append(got, it.title)
		}
		if !slices.Equal(got, titles) {
			t.Fatalf("screen %v: %v", screen, got)
		}
	}
}

func TestServerViewIdentifiesUbuntu(t *testing.T) {
	m := newModelForDevice("/repo", deviceServer)
	view := m.View()
	if !strings.Contains(view, "Ubuntu") || strings.Contains(view, "CachyOS") || strings.Contains(view, "Noctalia Shell") {
		t.Fatalf("ambiente incorreto no cabeçalho: %s", view)
	}
}

func TestDebianPackagesAndProfileUnits(t *testing.T) {
	path := filepath.Join(t.TempDir(), "status")
	if err := os.WriteFile(path, []byte("Package: openssh-server\nStatus: install ok installed\n\nPackage: openssh-client\nStatus: deinstall ok config-files\n\nPackage: ufw\nStatus: install ok unpacked\n\nPackage: curl\nStatus: hold ok installed\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := missingDebianPkgs(path, "openssh-server", "openssh-client", "ufw", "curl"); !slices.Equal(got, []string{"openssh-client", "ufw"}) {
		t.Fatal(got)
	}
	if got := missingDebianPkgs(path+"missing", "ufw"); !slices.Equal(got, []string{"ufw"}) {
		t.Fatal(got)
	}
	for _, device := range []deviceProfile{deviceDesktop, deviceNotebook, deviceServer} {
		service, socket := sshUnits(device)
		prefix := "sshd"
		if device == deviceServer {
			prefix = "ssh"
		}
		if service != prefix+".service" || socket != prefix+".socket" {
			t.Fatal(service, socket)
		}
		card := (sshStatus{device: device, missing: []string{"openssh-server"}, service: unitState{"active", "enabled"}, cfg: map[string]string{"port": "22"}}).card(120)
		if !strings.Contains(card, prefix+".service") {
			t.Fatal(card)
		}
		if device == deviceServer && strings.Contains(card, "sshd.service") {
			t.Fatal(card)
		}
	}
}

func TestUbuntuSSHJobAuthorizesPersonalKeyAndKeepsFallback(t *testing.T) {
	_, log := homelabMocks(t)
	sshDir := filepath.Join(os.Getenv("HOME"), ".ssh")
	if err := os.Mkdir(sshDir, 0700); err != nil {
		t.Fatal(err)
	}
	publicKey := "ssh-ed25519 " + base64.StdEncoding.EncodeToString([]byte("personal-key")) + " pessoal@example\n"
	if err := os.WriteFile(filepath.Join(sshDir, "id_pessoal_desktop.pub"), []byte(publicKey), 0644); err != nil {
		t.Fatal(err)
	}
	j := serverSSHJob(homelabSetup(t))
	for _, current := range j.steps {
		if current.cmd != nil {
			if out, err := current.cmd().CombinedOutput(); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
		} else if _, err := current.run(); err != nil {
			t.Fatal(err)
		}
	}
	commands := readHomelabFile(t, log)
	for _, want := range []string{"apt-get update", "apt-get install --yes openssh-server openssh-client", "sshd -t", "systemctl enable --now ssh.service"} {
		if !strings.Contains(commands, want) {
			t.Fatalf("missing %s: %s", want, commands)
		}
	}
	if strings.Index(commands, "sshd -t") > strings.Index(commands, "systemctl enable") {
		t.Fatal(commands)
	}
	for _, forbidden := range []string{"pacman", "iptables-nft", "sshd.service", "disable", "reload"} {
		if strings.Contains(commands, forbidden) {
			t.Fatal(commands)
		}
	}
	authorized, err := os.ReadFile(filepath.Join(sshDir, "authorized_keys"))
	if err != nil || string(authorized) != publicKey {
		t.Fatalf("id_pessoal_desktop.pub não autorizada: %q, %v", authorized, err)
	}
}

func TestUbuntuSSHValidationFailureAndExistingSocket(t *testing.T) {
	for _, tc := range []struct{ name, fail, socket string }{{"invalid config", "yes", ""}, {"active socket", "", "yes"}} {
		t.Run(tc.name, func(t *testing.T) {
			_, log := homelabMocks(t)
			t.Setenv("FAIL_SSHD", tc.fail)
			t.Setenv("SOCKET_ACTIVE", tc.socket)
			j := serverSSHJob(homelabSetup(t))
			current := j.steps[len(j.steps)-1]
			_, err := current.run()
			if (err != nil) != (tc.fail == "yes") {
				t.Fatal(err)
			}
			commands := readHomelabFile(t, log)
			if tc.fail == "yes" && strings.Contains(commands, "systemctl") {
				t.Fatal(commands)
			}
			if tc.socket == "yes" && (!strings.Contains(commands, "enable --now ssh.socket") || strings.Contains(commands, "ssh.service")) {
				t.Fatal(commands)
			}
		})
	}
}

func TestServerFirewallRulesPrecedeEnablingAndPreserveAdmin(t *testing.T) {
	_, log := homelabMocks(t)
	j := serverFirewallJob(homelabSetup(t), "22", "192.168.10.0/24", "203.0.113.7 51000 192.168.10.5 2200")
	for _, current := range j.steps {
		if current.run != nil {
			if _, err := current.run(); err != nil {
				t.Fatal(err)
			}
		}
	}
	commands := readHomelabFile(t, log)
	for _, want := range []string{
		"ufw insert 1 allow from 192.168.10.0/24 to any port 22 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 2222 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 80 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 53 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 53 proto udp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 3001 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 3428 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 5000 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 5050 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 7575 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 8887 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 8888 proto tcp",
		"ufw insert 1 allow from 192.168.10.0/24 to any port 9001 proto tcp",
		"ufw insert 1 allow from 203.0.113.7 to any port 2200 proto tcp",
	} {
		if index := strings.Index(commands, want); index < 0 || index > strings.Index(commands, "ufw default deny incoming") {
			t.Fatalf("missing/late %s: %s", want, commands)
		}
	}
	if strings.Index(commands, "ufw default deny") > strings.Index(commands, "systemctl enable --now ufw.service") || strings.Index(commands, "systemctl enable --now ufw.service") > strings.Index(commands, "ufw --force enable") {
		t.Fatal(commands)
	}
	for _, forbidden := range []string{"delete", "reset", "port 10380", "iptables-nft", "pacman"} {
		if strings.Contains(commands, forbidden) {
			t.Fatal(commands)
		}
	}
}

func TestServerFirewallRejectsUnknownNetworkAndStopsOnRuleFailure(t *testing.T) {
	for _, tc := range []struct{ port, lan, connection string }{
		{"", "192.168.1.0/24", ""}, {"70000", "192.168.1.0/24", ""}, {"22", "", ""}, {"22", "0.0.0.0/0", ""}, {"22", "10.0.0.0/7", ""}, {"22", "192.168.1.0/24", "bad connection"}, {"22", "192.168.1.0/24", "1.1.1.1 0 192.168.1.1 22"},
	} {
		if err := validateServerFirewall(tc.port, tc.lan, tc.connection); err == nil {
			t.Fatalf("accepted %#v", tc)
		}
	}
	_, log := homelabMocks(t)
	t.Setenv("FAIL_COMMAND", "ufw insert 1 allow from 192.168.1.0/24 to any port 22 proto tcp comment SSH LAN")
	j := serverFirewallJob(homelabSetup(t), "22", "192.168.1.0/24", "")
	if _, err := j.steps[len(j.steps)-1].run(); err == nil {
		t.Fatal("rule failure ignored")
	}
	commands := readHomelabFile(t, log)
	if strings.Contains(commands, "default") || strings.Contains(commands, "enable") {
		t.Fatal(commands)
	}
}

func TestCasaOSInstallerPipelinePropagatesFailures(t *testing.T) {
	for _, tc := range []struct{ name, curl, sudo string }{
		{"curl failure", "exit 22", "bash"},
		{"installer failure", "printf 'exit 17\\n'", "bash"},
		{"sudo failure", "printf 'exit 0\\n'", "exit 19"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bin, _ := homelabMocks(t)
			writeTestExecutable(t, bin, "curl", tc.curl)
			writeTestExecutable(t, bin, "sudo", tc.sudo)
			cmd := exec.Command("bash", "-c", casaOSInstallScript)
			if err := cmd.Run(); err == nil {
				t.Fatal("pipeline failure ignored")
			}
		})
	}
	bin, _ := homelabMocks(t)
	writeTestExecutable(t, bin, "curl", `test "$*" = '-fsSL https://get.casaos.io' || exit 99
printf 'exit 0\n'`)
	if out, err := exec.Command("bash", "-c", casaOSInstallScript).CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	j := casaOSJob()
	want := []string{"bash", "-c", casaOSInstallScript}
	if got := commandTail(t, j.steps[len(j.steps)-1], len(want)); !slices.Equal(got, want) {
		t.Fatal(got)
	}
}

func TestVaultwardenHTTPSComposeTransformation(t *testing.T) {
	transformer := configPath(homelabSetup(t), "https", "compose_https.py")
	for _, tc := range []struct {
		name, kind string
		compose    map[string]any
		check      func(*testing.T, map[string]any)
	}{
		{
			name: "caddy", kind: "caddy",
			compose: map[string]any{
				"name": "big-bear-caddy", "services": map[string]any{"app": map[string]any{
					"image": "caddy:2.11.7", "environment": map[string]any{"CF_API_TOKEN": "must-not-survive"},
					"ports": []any{map[string]any{"target": float64(80), "published": "48100"}},
				}}, "x-casaos": map[string]any{"port_map": "48100"},
			},
			check: func(t *testing.T, got map[string]any) {
				service := got["services"].(map[string]any)["app"].(map[string]any)
				ports := service["ports"].([]any)
				port := ports[0].(map[string]any)
				if len(ports) != 1 || port["target"] != float64(443) || port["published"] != "443" || port["host_ip"] != "192.168.1.107" {
					t.Fatalf("portas Caddy inseguras: %#v", ports)
				}
				if service["entrypoint"].([]any)[0] != "/config/bin/caddy" {
					t.Fatal(service["entrypoint"])
				}
				if _, present := service["environment"].(map[string]any)["CF_API_TOKEN"]; present {
					t.Fatal("token Cloudflare permaneceu no Compose")
				}
				if got["x-casaos"].(map[string]any)["scheme"] != "https" {
					t.Fatal(got["x-casaos"])
				}
			},
		},
		{
			name: "vaultwarden", kind: "vaultwarden",
			compose: map[string]any{"services": map[string]any{"vaultwarden": map[string]any{
				"image": "vaultwarden/server:1.36.0", "environment": map[string]any{"SIGNUPS_ALLOWED": "true"},
				"ports": []any{map[string]any{"target": float64(80), "published": "10380"}},
			}}},
			check: func(t *testing.T, got map[string]any) {
				service := got["services"].(map[string]any)["vaultwarden"].(map[string]any)
				if service["environment"].(map[string]any)["DOMAIN"] != "https://homelab.reesearch64.com" {
					t.Fatal(service["environment"])
				}
				port := service["ports"].([]any)[0].(map[string]any)
				if port["host_ip"] != "127.0.0.1" || port["published"] != "10380" {
					t.Fatalf("Vaultwarden ainda exposto na LAN: %#v", port)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			source, destination := filepath.Join(t.TempDir(), "source.json"), filepath.Join(t.TempDir(), "destination.json")
			content, _ := json.Marshal(tc.compose)
			if err := os.WriteFile(source, content, 0600); err != nil {
				t.Fatal(err)
			}
			args := []string{transformer, tc.kind, source, destination, "--domain", "homelab.reesearch64.com", "--app-data", "/DATA/AppData/caddy"}
			if tc.kind == "caddy" {
				args = append(args, "--server-ip", "192.168.1.107")
			}
			if output, err := exec.Command("python3", args...).CombinedOutput(); err != nil {
				t.Fatalf("transformar Compose: %v: %s", err, output)
			}
			var got map[string]any
			result, err := os.ReadFile(destination)
			if err != nil || json.Unmarshal(result, &got) != nil {
				t.Fatalf("resultado inválido: %v: %s", err, result)
			}
			service := got["services"].(map[string]any)
			for _, raw := range service {
				networks := raw.(map[string]any)["networks"].(map[string]any)
				if _, ok := networks["homelab_proxy"]; !ok {
					t.Fatal("rede privada ausente")
				}
			}
			tc.check(t, got)
		})
	}
}

func TestVaultwardenHTTPSJobRunsInteractiveRootScript(t *testing.T) {
	bin, _ := homelabMocks(t)
	writeTestExecutable(t, bin, "sudo", `exit 1`)
	j := vaultwardenHTTPSJob("/repo/setup/homelab", "192.168.1.0/24")
	current := j.steps[len(j.steps)-1]
	want := []string{
		"sudo", "bash", "/repo/setup/homelab/https/configure.sh", "homelab.reesearch64.com", "192.168.1.0/24",
		localBackupRoot(), "/repo/setup/homelab/https/compose_https.py", "/repo/setup/homelab/https/Caddyfile",
	}
	if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
		t.Fatalf("comando HTTPS: %v", got)
	}
	script := readHomelabFile(t, configPath(homelabSetup(t), "https", "configure.sh"))
	for _, forbidden := range []string{"/dns_records", "-X POST", "-X PUT", "read -rsp"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("configuração permanente da Cloudflare dentro dos dotfiles: %s", forbidden)
		}
	}
	for _, required := range []string{"cloudflare.env", "/user/tokens/verify", "DNS deve apontar somente"} {
		if !strings.Contains(script, required) {
			t.Fatalf("pré-requisito externo não validado: %s", required)
		}
	}
}
