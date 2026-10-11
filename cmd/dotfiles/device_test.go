package main

import "testing"

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
	for _, hostname := range []string{"servidor", "desktop-notebook"} {
		if _, err := detectDevice(hostname); err == nil {
			t.Fatalf("hostname %q deveria ser rejeitado", hostname)
		}
	}
}
