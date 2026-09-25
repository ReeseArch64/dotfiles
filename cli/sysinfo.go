package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type infoRow struct{ label, value string }

func systemInfo(dotfiles string) []infoRow {
	host, _ := os.Hostname()
	return []infoRow{
		{"Usuário", os.Getenv("USER")},
		{"Host", host},
		{"Sistema", osName()},
		{"Kernel", readTrim("/proc/sys/kernel/osrelease")},
		{"Uptime", uptime()},
		{"Shell", filepath.Base(os.Getenv("SHELL"))},
		{"Terminal", os.Getenv("TERM")},
		{"Dotfiles", dotfiles},
		{"Git", gitHead(dotfiles)},
	}
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return "?"
	}
	return strings.TrimSpace(string(b))
}

func osName() string {
	f, err := os.Open("/etc/os-release")
	if err != nil {
		return "?"
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return "?"
}

func uptime() string {
	fields := strings.Fields(readTrim("/proc/uptime"))
	if len(fields) == 0 {
		return "?"
	}
	secs, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return "?"
	}
	d := time.Duration(secs) * time.Second
	days := int(d.Hours()) / 24
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dmin", days, int(d.Hours())%24, int(d.Minutes())%60)
	}
	return fmt.Sprintf("%dh %dmin", int(d.Hours()), int(d.Minutes())%60)
}

func gitHead(dir string) string {
	out, err := exec.Command("git", "-C", dir, "log", "-1", "--format=%h · %s").Output()
	if err != nil {
		return "—"
	}
	branch, _ := exec.Command("git", "-C", dir, "branch", "--show-current").Output()
	return strings.TrimSpace(string(branch)) + " @ " + strings.TrimSpace(string(out))
}

type script struct{ name, desc, path string }

// listScripts lista scripts/*.sh usando a primeira linha de comentário como descrição.
func listScripts(dotfiles string) []script {
	paths, _ := filepath.Glob(filepath.Join(dotfiles, "scripts", "*.sh"))
	var out []script
	for _, p := range paths {
		out = append(out, script{
			name: strings.TrimSuffix(filepath.Base(p), ".sh"),
			desc: scriptDesc(p),
			path: p,
		})
	}
	return out
}

func scriptDesc(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#!") {
			continue
		}
		if v, ok := strings.CutPrefix(line, "# "); ok {
			return v
		}
		break
	}
	return ""
}
