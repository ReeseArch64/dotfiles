package main

// Leitura nativa do estado do sistema: nada aqui precisa de root.

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// ---- pacman ----

// missingPkgs devolve os pacotes que não estão instalados nem são providos por
// outro pacote (ex.: iptables provê iptables-nft), como `pacman -T`.
func missingPkgs(names ...string) []string {
	have := map[string]bool{}
	entries, _ := os.ReadDir("/var/lib/pacman/local")
	for _, e := range entries {
		f, err := os.Open(filepath.Join("/var/lib/pacman/local", e.Name(), "desc"))
		if err != nil {
			continue
		}
		section := ""
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			switch {
			case strings.HasPrefix(line, "%"):
				section = line
			case line != "" && (section == "%NAME%" || section == "%PROVIDES%"):
				name, _, _ := strings.Cut(line, "=")
				have[name] = true
			}
		}
		f.Close()
	}
	var missing []string
	for _, n := range names {
		if !have[n] {
			missing = append(missing, n)
		}
	}
	return missing
}
