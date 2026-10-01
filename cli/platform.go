package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

func checkPlatform(osReleasePath string, lookPath func(string) (string, error)) error {
	id, err := osReleaseID(osReleasePath)
	var missing []string
	if err != nil || id != "cachyos" {
		missing = append(missing, "CachyOS")
	}
	if _, err := lookPath("niri"); err != nil {
		missing = append(missing, "Niri")
	}
	if _, err := lookPath("noctalia"); err != nil {
		missing = append(missing, "Noctalia Shell")
	}
	if len(missing) > 0 {
		return fmt.Errorf("ambiente incompatível: requer CachyOS -> Niri -> Noctalia Shell; ausente: %s", strings.Join(missing, ", "))
	}
	return nil
}

func osReleaseID(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok && key == "ID" {
			return strings.Trim(value, `"'`), nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	return "", errors.New("ID ausente em os-release")
}
