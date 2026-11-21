package main

import (
	"strings"
	"testing"
)

func TestSDDMAutologinConfig(t *testing.T) {
	for _, existing := range []string{
		"", "[Autologin]\n", "[Theme]\nCurrent=noctalia\n[Autologin]\nUser=old\nRelogin=true\n[General]\nDisplayServer=wayland\n",
		"[Autologin]\nUser=old\n[Autologin]\nSession=other.desktop\n",
	} {
		got, err := sddmAutologinConfig(existing, "alice")
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{"[Autologin]\nUser=alice\nSession=niri.desktop\nRelogin=false\n"} {
			if !strings.Contains(got, want) {
				t.Fatalf("configuração inesperada: %q", got)
			}
		}
		if strings.Count(got, "[Autologin]") != 1 {
			t.Fatalf("seção duplicada: %s", got)
		}
		if strings.Contains(existing, "[Theme]") && (!strings.Contains(got, "[Theme]\nCurrent=noctalia") || !strings.Contains(got, "[General]\nDisplayServer=wayland")) {
			t.Fatalf("outras configurações foram perdidas: %s", got)
		}
		again, err := sddmAutologinConfig(got, "alice")
		if err != nil || again != got {
			t.Fatalf("configuração não idempotente: %q, %v", again, err)
		}
	}
}

func TestSDDMAutologinRejectsUnsafeUser(t *testing.T) {
	for _, username := range []string{"", "root", " alice", "alice\nUser=root", "a\x00b", "[Autologin]"} {
		if _, err := sddmAutologinConfig("", username); err == nil {
			t.Fatalf("aceitou usuário %q", username)
		}
	}
}
