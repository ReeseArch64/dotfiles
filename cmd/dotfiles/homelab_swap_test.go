package main

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestHomelabSwapMenuUsesIndependentTwoGiBScript(t *testing.T) {
	m := newModelForDevice("/repo", deviceServer)
	for _, it := range m.items() {
		if it.title != "Configurar Swap" {
			continue
		}
		configured := it.job()
		current := configured.steps[len(configured.steps)-1]
		want := []string{"sudo", "bash", "/repo/setup/homelab/swap/configure.sh", "2"}
		if got := commandTail(t, current, len(want)); !slices.Equal(got, want) {
			t.Fatalf("comando inesperado: %v", got)
		}
		return
	}
	t.Fatal("ação de swap ausente")
}

func TestHomelabSwapScriptSafety(t *testing.T) {
	path := configPath(homelabSetup(t), "swap", "configure.sh")
	script := readHomelabFile(t, path)
	for _, required := range []string{
		"size_gib=${1:-2}", "swap_file=/swapfile-homelab",
		"/run/lock/dotfiles-homelab-swap.lock", "flock -n 9",
		"ext4|xfs", "! -L /etc/fstab", "! -L \"$swap_file\"",
		"Arquivo existente incompatível", "bytes + 4 * 1024 * 1024 * 1024",
		"chmod 0600", "ln \"$temporary\" \"$swap_file\"",
		"swapon \"$swap_file\"", "if [[ -z \"$entry\" ]]",
		"/etc/fstab.homelab-swap.XXXXXXXX", "sw,nofail 0 0",
	} {
		if !strings.Contains(script, required) {
			t.Errorf("proteção/configuração ausente: %s", required)
		}
	}
	if strings.Contains(script, "homeprod") {
		t.Fatal("swap do Homelab não deve alterar recursos do HomeProd")
	}
	if strings.Index(script, "swapon \"$swap_file\"") > strings.Index(script, ">> /etc/fstab") {
		t.Fatal("persistência deve ocorrer somente após ativação bem-sucedida")
	}
	if out, err := exec.Command("bash", "-n", path).CombinedOutput(); err != nil {
		t.Fatalf("sintaxe inválida: %v: %s", err, out)
	}
	// Invalid sizes must fail before any privileged or disk operation.
	for _, size := range []string{"8", "12", "0", "invalid"} {
		out, err := exec.Command("bash", path, size).CombinedOutput()
		if err == nil || !strings.Contains(string(out), "Informe 2 GiB.") {
			t.Fatalf("tamanho inválido %q: %v: %s", size, err, out)
		}
	}
}
