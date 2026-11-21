package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"
)

var errNoLaptopSSHPort = errors.New("sshd não informou nenhuma porta SSH")

// Aceita palavras com aspas, comentários e Key=value, sem expandir comandos.
func laptopSSHConfigFields(line string) ([]string, error) {
	var fields []string
	var word strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if word.Len() > 0 {
			fields = append(fields, word.String())
			word.Reset()
		}
	}
	for _, r := range line {
		if escaped {
			word.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		switch {
		case r == '#':
			flush()
			goto done
		case r == '\'' || r == '"':
			quote = r
		case unicode.IsSpace(r):
			flush()
		default:
			word.WriteRune(r)
		}
	}
	if quote != 0 || escaped {
		return nil, errors.New("aspas ou escape incompleto na configuração SSH")
	}
	flush()
done:
	if len(fields) == 0 {
		return nil, nil
	}
	if key, value, ok := strings.Cut(fields[0], "="); ok {
		fields[0] = key
		if value != "" {
			fields = append([]string{key, value}, fields[1:]...)
		}
	} else if len(fields) > 1 && strings.HasPrefix(fields[1], "=") {
		value := strings.TrimPrefix(fields[1], "=")
		if value == "" {
			fields = append(fields[:1], fields[2:]...)
		} else {
			fields[1] = value
		}
	}
	fields[0] = strings.ToLower(fields[0])
	return fields, nil
}

func normalizedLaptopSSHConfig(output string) string {
	var lines []string
	for _, line := range strings.Split(output, "\n") {
		fields, err := laptopSSHConfigFields(line)
		if err == nil && len(fields) > 0 {
			lines = append(lines, strings.Join(fields, " "))
		}
	}
	return strings.Join(lines, "\n")
}

// -G é a alternativa de impressão da configuração, quando -T não produz saída.
// Não substitui sshd -t nem dispensa a verificação da política candidata.
func dumpLaptopSSHConfig(commands laptopSSHCommands, prefix []string, extra ...string) (string, error) {
	var failures []error
	for _, mode := range []string{"-T", "-G"} {
		args := append(slices.Clone(prefix), mode)
		args = append(args, extra...)
		output, err := commands.capture("", args...)
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", mode, err))
			continue
		}
		normalized := normalizedLaptopSSHConfig(output)
		recognizable := false
		for _, line := range strings.Split(normalized, "\n") {
			for _, key := range []string{"port ", "permitrootlogin ", "passwordauthentication ", "authenticationmethods "} {
				if strings.HasPrefix(line, key) {
					recognizable = true
				}
			}
		}
		if recognizable {
			return normalized, nil
		}
		failures = append(failures, fmt.Errorf("sshd %s não retornou configuração reconhecível", mode))
	}
	return "", errors.Join(failures...)
}

// Lê apenas a porta e os Includes da configuração original. Só assume o padrão
// 22 se todos os arquivos puderem ser lidos e não houver Port explícito.
func configuredLaptopSSHPort(path string) (string, error) {
	base := filepath.Dir(path)
	stack := map[string]bool{}
	files := 0
	var ports []string
	var read func(string, int) error
	read = func(path string, depth int) error {
		path = filepath.Clean(path)
		files++
		if depth > 16 || files > 1024 || stack[path] {
			return errors.New("Include SSH cíclico ou excessivo")
		}
		stack[path] = true
		defer delete(stack, path)
		contents, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("ler configuração SSH %s: %w", path, err)
		}
		for number, line := range strings.Split(string(contents), "\n") {
			fields, err := laptopSSHConfigFields(line)
			if err != nil {
				return fmt.Errorf("%s:%d: %w", path, number+1, err)
			}
			if len(fields) == 0 {
				continue
			}
			switch fields[0] {
			case "port":
				ports = append(ports, strings.Join(fields, " "))
			case "include":
				if len(fields) < 2 {
					return fmt.Errorf("Include SSH sem caminho: %s:%d", path, number+1)
				}
				for _, pattern := range fields[1:] {
					if strings.ContainsAny(pattern, "$%~") {
						return fmt.Errorf("Include SSH exige revisão manual: %s", pattern)
					}
					if !filepath.IsAbs(pattern) {
						pattern = filepath.Join(base, pattern)
					}
					matches, err := filepath.Glob(pattern)
					if err != nil {
						return err
					}
					slices.Sort(matches)
					for _, include := range matches {
						if err := read(include, depth+1); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}
	if err := read(path, 0); err != nil {
		return "", err
	}
	if len(ports) == 0 {
		return "22", nil
	}
	return effectiveLaptopSSHPort(strings.Join(ports, "\n"))
}
