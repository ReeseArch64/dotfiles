package main

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestHomeProdProfileDetectionPlatformAndMenu(t *testing.T) {
	for _, hostname := range []string{"homeprod", "vps-HOMEPROD-01"} {
		device, err := detectDevice(hostname)
		if err != nil || device != deviceHomeProd {
			t.Fatalf("detecção de %q: dispositivo=%v erro=%v", hostname, device, err)
		}
	}

	osRelease := filepath.Join(t.TempDir(), "os-release")
	lookPath := func(string) (string, error) {
		t.Fatal("HomeProd não deve inspecionar a pilha gráfica")
		return "", errors.New("ausente")
	}
	for _, id := range []string{"ubuntu", "debian"} {
		if err := os.WriteFile(osRelease, []byte("ID="+id+"\n"), 0600); err != nil {
			t.Fatal(err)
		}
		err := checkPlatformForDevice(deviceHomeProd, osRelease, lookPath)
		if (err == nil) != (id == "ubuntu") {
			t.Fatalf("plataforma %s: %v", id, err)
		}
	}

	m := newModelForDevice("/repo", deviceHomeProd)
	if m.setupDir != "/repo/setup/homeprod" {
		t.Fatal(m.setupDir)
	}
	if titles := itemTitles(m.items()); !slices.Equal(titles, []string{"Preparação inicial", "Configurar SSH", "Configurar Swap", "Sair"}) {
		t.Fatalf("menu inesperado: %v", titles)
	}
}

func TestHomeProdBootstrapOrderAndPasswordHandling(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	setupDir := setupDirectory(root, deviceHomeProd)
	configured := homeProdBootstrapJob(setupDir)
	if labels := jobStepLabels(configured); !slices.Equal(labels, []string{"Atualizar Ubuntu, criar administrador alsgy e instalar CasaOS"}) {
		t.Fatalf("passos inesperados: %v", labels)
	}
	args := configured.steps[0].cmd().Args
	if !slices.Contains(args, configPath(setupDir, "bootstrap.sh")) || !slices.Contains(args, "alsgy") {
		t.Fatalf("comando inesperado: %v", args)
	}

	scriptPath := configPath(setupDir, "bootstrap.sh")
	script := readHomelabFile(t, scriptPath)
	ordered := []string{"apt-get update", "apt-get upgrade --yes", "apt-get install --yes sudo", "adduser --gecos", "usermod --append --groups sudo", "apt-get install --yes curl ca-certificates", "curl -fsSL https://get.casaos.io | sudo bash"}
	previous := -1
	for _, command := range ordered {
		current := strings.Index(script, command)
		if current <= previous {
			t.Fatalf("comando ausente ou fora de ordem: %s", command)
		}
		previous = current
	}
	for _, forbidden := range []string{"chpasswd", "useradd -p", "--password"} {
		if strings.Contains(script, forbidden) {
			t.Fatalf("senha não deve ser fornecida pelo script: %s", forbidden)
		}
	}
	info, err := os.Stat(scriptPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Fatalf("permissão do bootstrap: %o", info.Mode().Perm())
	}
}
