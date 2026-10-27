package main

import "testing"

func assertNoDeviceOptions(t *testing.T, device deviceProfile) {
	t.Helper()
	m := newModelForDevice("/repo", device)
	if m.setupDir != "" {
		t.Fatalf("perfil inválido recebeu diretório: %q", m.setupDir)
	}
	for screen := screenMain; screen <= screenJob; screen++ {
		m.screen = screen
		if items := m.items(); len(items) != 0 {
			t.Fatalf("perfil inválido recebeu opções na tela %v: %v", screen, itemTitles(items))
		}
	}
}

func TestUnknownDeviceHasNoOptions(t *testing.T) {
	var device deviceProfile
	assertNoDeviceOptions(t, device)
	assertNoDeviceOptions(t, deviceProfile(-1))
	assertNoDeviceOptions(t, deviceProfile(999))
}

func TestDetectDeviceByHostnameMarker(t *testing.T) {
	tests := []struct {
		hostname string
		want     deviceProfile
	}{
		{"desktop-alan", deviceDesktop},
		{"meu-NOTEBOOK", deviceNotebook},
		{"MacBook-Pro", deviceMacBook},
	}
	for _, test := range tests {
		got, err := detectDevice(test.hostname)
		if err != nil {
			t.Fatalf("detectar %q: %v", test.hostname, err)
		}
		if got != test.want {
			t.Fatalf("perfil de %q: obtido %s, esperado %s", test.hostname, got, test.want)
		}
	}
}

func TestDetectDeviceRejectsUnknownOrAmbiguousHostname(t *testing.T) {
	for _, hostname := range []string{"", "servidor", "mini-server", "desktop-notebook", "homelab-homeprod"} {
		device, err := detectDevice(hostname)
		if err == nil || device != deviceUnknown {
			t.Fatalf("hostname %q deveria retornar perfil desconhecido e erro", hostname)
		}
		assertNoDeviceOptions(t, device)
	}
}
