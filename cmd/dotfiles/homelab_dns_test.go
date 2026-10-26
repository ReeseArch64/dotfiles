package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type dnsFixture struct{ config, resolver, stub, upstream, log, backups string }

func prepareDNS(t *testing.T, content, resolverKind string) dnsFixture {
	t.Helper()
	bin, log := homelabMocks(t)
	root := t.TempDir()
	f := dnsFixture{filepath.Join(root, "resolved.conf"), filepath.Join(root, "resolv.conf"), filepath.Join(root, "stub-resolv.conf"), filepath.Join(root, "upstream-resolv.conf"), log, t.TempDir()}
	for path, content := range map[string]string{f.config: content, f.stub: "nameserver 127.0.0.53\n", f.upstream: "nameserver 192.168.1.1\n"} {
		if err := os.WriteFile(path, []byte(content), 0640); err != nil {
			t.Fatal(err)
		}
	}
	switch resolverKind {
	case "stub":
		// A relative symlink must be recognized as well as an absolute one.
		if err := os.Symlink(filepath.Base(f.stub), f.resolver); err != nil {
			t.Fatal(err)
		}
	case "external":
		if err := os.Symlink(f.upstream, f.resolver); err != nil {
			t.Fatal(err)
		}
	case "regular stub":
		if err := os.WriteFile(f.resolver, []byte("nameserver 127.0.0.53\n"), 0644); err != nil {
			t.Fatal(err)
		}
	default:
		if err := os.WriteFile(f.resolver, []byte("nameserver 10.1.1.1\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DNS_CONFIG", f.config)
	t.Setenv("DNS_RESOLVER", f.resolver)
	t.Setenv("DNS_UPSTREAM", f.upstream)
	writeTestExecutable(t, bin, "systemctl", `printf 'systemctl %s\n' "$*" >> "$COMMAND_LOG"
[ "$*" = 'restart systemd-resolved' ] || exit 90
grep -q '^DNSStubListener=no$' "$DNS_CONFIG" || exit 91
! grep -q '^nameserver 127.0.0.53$' "$DNS_RESOLVER" || exit 92
if [ "${FAIL_RESTART:-}" = yes ]; then exit 7; fi`)
	return f
}

func (f dnsFixture) run(t *testing.T) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", configPath(homelabSetup(t), "dns", "disable-stub.sh"), f.config, f.resolver, f.stub, f.upstream, f.backups, filepath.Join(filepath.Dir(f.config), "resolved.conf.d"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func TestServerDNSSectionAwareBackupResolverAndIdempotence(t *testing.T) {
	for _, kind := range []string{"stub", "regular", "external"} {
		t.Run(kind, func(t *testing.T) {
			original := "#DNSStubListener=yes\n[Other]\nDNSStubListener=yes\n[Resolve]\n#DNSStubListener=yes\nDNS=192.168.1.1\nDNSStubListener=yes\n[OtherAgain]\nDNSStubListener=yes\n"
			f := prepareDNS(t, original, kind)
			if out, err := f.run(t); err != nil {
				t.Fatalf("%v: %s", err, out)
			}
			got := readHomelabFile(t, f.config)
			want := "#DNSStubListener=yes\n[Other]\nDNSStubListener=yes\n[Resolve]\nDNSStubListener=no\nDNS=192.168.1.1\n[OtherAgain]\nDNSStubListener=yes\n"
			if got != want {
				t.Fatalf("section-aware rewrite: %q", got)
			}
			if backup := readHomelabFile(t, testBackupAt(t, f.backups, "dns", f.config)); backup != original {
				t.Fatal(backup)
			}
			if info, err := os.Stat(f.config); err != nil || info.Mode().Perm() != 0640 {
				t.Fatalf("mode lost: %v %v", info, err)
			}
			if kind == "stub" {
				if target, err := os.Readlink(f.resolver); err != nil || target != f.upstream {
					t.Fatalf("resolver not migrated: %s %v", target, err)
				}
				if target, err := os.Readlink(testBackupAt(t, f.backups, "dns", f.resolver)); err != nil || target != filepath.Base(f.stub) {
					t.Fatalf("symlink backup lost: %s %v", target, err)
				}
			} else if paths, err := testBackupPaths(f.backups, "dns", f.resolver); err != nil || len(paths) != 0 {
				t.Fatal("externally managed resolver was modified")
			}
			if out, err := f.run(t); err != nil {
				t.Fatalf("second run: %v %s", err, out)
			}
			if second := readHomelabFile(t, f.config); second != got {
				t.Fatal("not idempotent", second)
			}
			if backup := readHomelabFile(t, testBackupAt(t, f.backups, "dns", f.config)); backup != original {
				t.Fatal("backup overwritten")
			}
			if commands := readHomelabFile(t, f.log); commands != strings.Repeat("systemctl restart systemd-resolved\n", 2) {
				t.Fatal(commands)
			}
		})
	}
}

func TestServerDNSAddsResolveOrMissingDirective(t *testing.T) {
	for _, content := range []string{"# config\n[Other]\nDNSStubListener=yes\n", "[Resolve]\nDNS=1.1.1.1\n", "[Resolve]\n  # DNSStubListener = yes\n[Resolve]\nDNSStubListener=yes\n"} {
		f := prepareDNS(t, content, "regular")
		if out, err := f.run(t); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		got := readHomelabFile(t, f.config)
		if !strings.Contains(got, "[Resolve]\nDNSStubListener=no\n") {
			t.Fatal(got)
		}
		if out, err := f.run(t); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		if second := readHomelabFile(t, f.config); got != second {
			t.Fatal("not idempotent", second)
		}
	}
}

func TestServerDNSRefusesUnsafeResolutionAndPropagatesRestartFailure(t *testing.T) {
	for _, kind := range []string{"regular stub", "missing upstream", "stub upstream", "missing config", "linked config"} {
		t.Run(kind, func(t *testing.T) {
			resolverKind := "stub"
			if kind == "regular stub" {
				resolverKind = kind
			}
			original := "[Resolve]\n#DNSStubListener=yes\n"
			f := prepareDNS(t, original, resolverKind)
			switch kind {
			case "missing upstream":
				if err := os.Remove(f.upstream); err != nil {
					t.Fatal(err)
				}
			case "stub upstream":
				if err := os.WriteFile(f.upstream, []byte("nameserver 127.0.0.53\n"), 0644); err != nil {
					t.Fatal(err)
				}
			case "missing config":
				if err := os.Remove(f.config); err != nil {
					t.Fatal(err)
				}
			case "linked config":
				if err := os.Rename(f.config, f.config+".external"); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(f.config+".external", f.config); err != nil {
					t.Fatal(err)
				}
			}
			if out, err := f.run(t); err == nil {
				t.Fatal("unsafe resolver accepted", out)
			}
			if _, err := os.Stat(f.log); !os.IsNotExist(err) {
				t.Fatal("restart happened after validation failure")
			}
			if paths, err := testBackupPaths(f.backups, "dns", f.config); err != nil || len(paths) != 0 {
				t.Fatal("config changed before validation")
			}
			if kind != "missing config" && readHomelabFile(t, f.config) != original {
				t.Fatal("config changed")
			}
		})
	}
	f := prepareDNS(t, "[Resolve]\n#DNSStubListener=yes\n", "stub")
	t.Setenv("FAIL_RESTART", "yes")
	if _, err := f.run(t); err == nil {
		t.Fatal("restart failure ignored")
	}
	if target, err := os.Readlink(f.resolver); err != nil || target != f.upstream {
		t.Fatal("resolution must survive restart failure", target, err)
	}
}

func TestServerDNSRejectsConflictingDropInsBeforeChanges(t *testing.T) {
	for _, directive := range []string{"yes", "udp", "tcp", ""} {
		t.Run(directive, func(t *testing.T) {
			original := "[Resolve]\n#DNSStubListener=yes\n"
			f := prepareDNS(t, original, "stub")
			directory := filepath.Join(filepath.Dir(f.config), "resolved.conf.d")
			if err := os.MkdirAll(directory, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(directory, "override.conf"), []byte("[Resolve]\nDNSStubListener="+directive+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			out, err := f.run(t)
			if err == nil || !strings.Contains(out, "Drop-in DNSStubListener conflitante") {
				t.Fatalf("drop-in conflitante aceito: %v %s", err, out)
			}
			if readHomelabFile(t, f.config) != original {
				t.Fatal("configuração alterada antes de validar drop-ins")
			}
			if target, err := os.Readlink(f.resolver); err != nil || target != filepath.Base(f.stub) {
				t.Fatal("resolver alterado antes de validar drop-ins", target, err)
			}
			if _, err := os.Stat(f.log); !os.IsNotExist(err) {
				t.Fatal("serviço reiniciado apesar do drop-in conflitante")
			}
		})
	}
}

func TestServerDNSAllowsNonconflictingDropIns(t *testing.T) {
	f := prepareDNS(t, "[Resolve]\n#DNSStubListener=yes\n", "stub")
	directory := filepath.Join(filepath.Dir(f.config), "resolved.conf.d")
	if err := os.MkdirAll(directory, 0755); err != nil {
		t.Fatal(err)
	}
	content := "[Other]\nDNSStubListener=yes\n[Resolve]\n#DNSStubListener=yes\nDNSStubListener=no\n"
	if err := os.WriteFile(filepath.Join(directory, "override.conf"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	if out, err := f.run(t); err != nil {
		t.Fatalf("drop-in sem conflito rejeitado: %v %s", err, out)
	}
}

func TestServerDNSJobUsesSudoAndIndependentAction(t *testing.T) {
	bin, _ := homelabMocks(t)
	writeTestExecutable(t, bin, "sudo", `printf '%s\n' "$*" >> "$COMMAND_LOG"`)
	j := serverDNSJob("/repo/setup/homelab")
	if len(j.steps) != 1 {
		t.Fatal(jobStepLabels(j))
	}
	if _, err := j.steps[0].run(); err != nil {
		t.Fatal(err)
	}
	commands := readHomelabFile(t, os.Getenv("COMMAND_LOG"))
	if !strings.Contains(commands, "-n bash /repo/setup/homelab/dns/disable-stub.sh /etc/systemd/resolved.conf /etc/resolv.conf /run/systemd/resolve/stub-resolv.conf /run/systemd/resolve/resolv.conf") {
		t.Fatal(commands)
	}
	if strings.Contains(commands, "curl") {
		t.Fatal("DNS must be independently selectable")
	}
}

func TestServerDNSCreatesMissingBackupRootOnDemand(t *testing.T) {
	f := prepareDNS(t, "[Resolve]\nDNSStubListener=yes\n", "stub")
	f.backups = filepath.Join(t.TempDir(), "mnt", "backups")
	if out, err := f.run(t); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	info, err := os.Stat(f.backups)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("backup root não foi criado privado: %v %v", info, err)
	}
	if got := readHomelabFile(t, testBackupAt(t, f.backups, "dns", f.config)); got != "[Resolve]\nDNSStubListener=yes\n" {
		t.Fatal("backup original não preservado")
	}
}

func TestServerDNSPreservesExistingBackupRootPermissions(t *testing.T) {
	f := prepareDNS(t, "[Resolve]\nDNSStubListener=yes\n", "stub")
	if err := os.Chmod(f.backups, 0750); err != nil {
		t.Fatal(err)
	}
	if out, err := f.run(t); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	info, err := os.Stat(f.backups)
	if err != nil || info.Mode().Perm() != 0750 {
		t.Fatalf("permissões alteradas: %v %v", info, err)
	}
}

func TestServerDNSRejectsSymlinkBackupParentBeforeCreatingRoot(t *testing.T) {
	original := "[Resolve]\nDNSStubListener=yes\n"
	f := prepareDNS(t, original, "stub")
	parent, target := t.TempDir(), t.TempDir()
	link := filepath.Join(parent, "linked")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	f.backups = filepath.Join(link, "backups")
	if out, err := f.run(t); err == nil {
		t.Fatalf("symlink aceito: %s", out)
	}
	if got := readHomelabFile(t, f.config); got != original {
		t.Fatal("DNS alterado sem backup seguro")
	}
	if _, err := os.Stat(filepath.Join(target, "backups")); !os.IsNotExist(err) {
		t.Fatal("diretório criado através de symlink")
	}
}
