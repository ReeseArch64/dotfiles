package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const firefoxBackupDir = backupMount + "/Firefox"

var errFirefoxOpen = errors.New("feche completamente o Firefox antes de fazer o backup")

func runFirefoxBackup(p profile) error {
	if p.sshKeyName() == "" || os.Geteuid() == 0 {
		return errors.New("execute o backup pelo menu Desktop ou Laptop como usuário normal")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	config, err := os.UserConfigDir()
	if err != nil {
		return err
	}
	printActionHeader()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	fmt.Println("Verificando se o Firefox está fechado e preparando o backup…")
	location := backupLocation{path: firefoxBackupDir, base: backupMount, sudo: systemSSHCommands().interactive}
	path, err := backupFirefox(ctx, home, config, location, time.Now(), firefoxClosed)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	} else {
		fmt.Println("Backup salvo em", path)
	}
	fmt.Print("\nEnter para voltar ao menu...")
	_, _ = bufio.NewReader(os.Stdin).ReadString('\n')
	return err
}

func firefoxClosed() error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return fmt.Errorf("verificar se o Firefox está fechado: %w", err)
	}
	uid := strconv.Itoa(os.Getuid())
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil || !entry.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "stat"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if errors.Is(err, os.ErrPermission) {
			if info, infoErr := entry.Info(); infoErr == nil {
				if owner, ok := info.Sys().(*syscall.Stat_t); ok && int(owner.Uid) != os.Getuid() {
					continue
				}
			}
		}
		if err != nil {
			return fmt.Errorf("verificar processos: %w", err)
		}
		line := string(data)
		start, end := strings.Index(line, "("), strings.LastIndex(line, ") ")
		if start < 0 || end <= start || len(line) <= end+2 || line[end+2] == 'Z' {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(line[start+1 : end]))
		if name != "firefox" && !strings.HasPrefix(name, "firefox-") {
			continue
		}
		status, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "status"))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("verificar processo do Firefox: %w", err)
		}
		for _, line := range strings.Split(string(status), "\n") {
			ids, ok := strings.CutPrefix(line, "Uid:")
			if fields := strings.Fields(ids); ok && len(fields) > 0 && fields[0] == uid {
				return errFirefoxOpen
			}
		}
	}
	return nil
}

type firefoxBackupPlan struct {
	paths, profiles []string
}

func discoverFirefox(home, config string) (firefoxBackupPlan, error) {
	var plan firefoxBackupPlan
	var roots []string
	flatpak := filepath.Join(home, ".var", "app", "org.mozilla.firefox")
	snap := filepath.Join(home, "snap", "firefox", "common")
	for _, root := range []string{
		filepath.Join(config, "mozilla", "firefox"),
		filepath.Join(home, ".mozilla", "firefox"),
		filepath.Join(flatpak, ".mozilla", "firefox"),
		filepath.Join(flatpak, "config", "mozilla", "firefox"),
		filepath.Join(snap, ".mozilla", "firefox"),
		filepath.Join(snap, ".config", "mozilla", "firefox"),
	} {
		if _, err := os.Lstat(root); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return plan, err
		}
		physical, err := firefoxDirectory(root)
		if err != nil {
			return plan, err
		}
		for _, path := range []string{root, physical} {
			if !slices.Contains(plan.paths, path) {
				plan.paths = append(plan.paths, path)
			}
		}
		if !slices.Contains(roots, physical) {
			roots = append(roots, physical)
		}
	}
	if len(roots) == 0 {
		return plan, errors.New("nenhuma pasta de dados do Firefox encontrada")
	}
	for _, root := range roots {
		profiles, err := readFirefoxProfiles(root)
		if err != nil {
			return plan, err
		}
		for _, path := range profiles {
			physical, err := firefoxDirectory(path)
			if err != nil {
				return plan, fmt.Errorf("localizar perfil do Firefox: %w", err)
			}
			if slices.Contains(plan.profiles, physical) {
				continue
			}
			if !slices.ContainsFunc(roots, func(base string) bool { return pathInside(base, physical) }) {
				if !looksLikeFirefoxProfile(physical) {
					return plan, fmt.Errorf("perfil externo do Firefox não reconhecido: %s", path)
				}
				plan.paths = append(plan.paths, physical)
			}
			plan.profiles = append(plan.profiles, physical)
		}
	}
	return plan, nil
}

func firefoxDirectory(path string) (string, error) {
	physical, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(physical)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("pasta do Firefox inválida: %s", path)
	}
	return physical, nil
}

func pathInside(parent, path string) bool {
	relative, err := filepath.Rel(parent, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, "../")
}

func looksLikeFirefoxProfile(path string) bool {
	for _, name := range []string{"prefs.js", "key4.db", "compatibility.ini", "times.json"} {
		if info, err := os.Stat(filepath.Join(path, name)); err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

func readFirefoxProfiles(root string) ([]string, error) {
	var profiles []string
	path := filepath.Join(root, "profiles.ini")
	file, err := os.Open(path)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err == nil {
		defer file.Close()
		var inProfile bool
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if strings.HasPrefix(line, "[") {
				inProfile = strings.HasPrefix(line, "[Profile") && strings.HasSuffix(line, "]")
				continue
			}
			name, value, ok := strings.Cut(line, "=")
			if !inProfile || !ok || strings.TrimSpace(name) != "Path" || value == "" {
				continue
			}
			if !filepath.IsAbs(value) {
				value = filepath.Join(root, value)
			}
			profiles = append(profiles, value)
		}
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("ler %s: %w", path, err)
		}
	}
	// Mozilla normally lists every profile in profiles.ini. Also find local
	// profile directories without an entry, for example after a manual copy.
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		dir := filepath.Join(root, entry.Name())
		if looksLikeFirefoxProfile(dir) && !slices.Contains(profiles, dir) {
			profiles = append(profiles, dir)
		}
	}
	return profiles, nil
}

func lockFirefoxProfile(profile string) (*os.File, error) {
	path := filepath.Join(profile, ".parentlock")
	fd, err := syscall.Open(path, syscall.O_CREAT|syscall.O_RDWR|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("lock do perfil inválido: %s", profile)
	}
	lock := syscall.Flock_t{Type: syscall.F_WRLCK}
	if err := syscall.FcntlFlock(file.Fd(), syscall.F_SETLK, &lock); err != nil {
		file.Close()
		if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EACCES) {
			return nil, errFirefoxOpen
		}
		return nil, fmt.Errorf("bloquear perfil do Firefox: %w", err)
	}
	return file, nil
}

func tarLocation(home, path string) (base, member string) {
	if pathInside(home, path) {
		relative, _ := filepath.Rel(home, path)
		return home, "./" + filepath.ToSlash(relative)
	}
	return "/", "." + filepath.ToSlash(path)
}

func (plan firefoxBackupPlan) tarArgs(home string) []string {
	args := []string{"--create", "--file=-", "--anchored", "--no-wildcards"}
	for _, profile := range plan.profiles {
		for _, lock := range []string{".parentlock", "lock"} {
			_, member := tarLocation(home, filepath.Join(profile, lock))
			args = append(args, "--exclude="+member)
		}
	}
	for _, dir := range plan.paths {
		base, member := tarLocation(home, dir)
		args = append(args, "--directory", base, member)
	}
	return args
}

func backupFirefox(ctx context.Context, home, config string, location backupLocation, now time.Time, checkClosed func() error) (string, error) {
	dest := location.path
	if err := checkClosed(); err != nil {
		return "", err
	}
	plan, err := discoverFirefox(home, config)
	if err != nil {
		return "", err
	}
	if slices.ContainsFunc(plan.paths, func(source string) bool { return pathInside(source, dest) }) {
		return "", errors.New("a pasta de destino não pode ficar dentro dos dados do Firefox")
	}
	for _, profile := range plan.profiles {
		lock, err := lockFirefoxProfile(profile)
		if err != nil {
			return "", err
		}
		defer lock.Close()
	}
	if err := location.prepare(); err != nil {
		return "", err
	}
	if err := checkClosed(); err != nil {
		return "", err
	}
	temp, err := os.CreateTemp(dest, ".firefox-*.part")
	if err != nil {
		return "", err
	}
	defer os.Remove(temp.Name())
	defer temp.Close()
	info, err := temp.Stat()
	if err != nil {
		return "", err
	}
	if info.Mode().Perm() != 0600 {
		return "", errors.New("o armazenamento não permite criar um arquivo de backup privado (0600)")
	}

	if err := archiveClosedFirefox(ctx, temp, plan.tarArgs(home), checkClosed); err != nil {
		return "", err
	}
	if err := temp.Sync(); err != nil {
		return "", err
	}
	if err := temp.Close(); err != nil {
		return "", err
	}
	if err := checkClosed(); err != nil {
		return "", err
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	filename := filepath.Join(dest, "firefox-"+now.Format("2006-01-02-15-04-05")+".tar")
	if err := os.Link(temp.Name(), filename); err != nil {
		return "", fmt.Errorf("publicar backup sem sobrescrever: %w", err)
	}
	return filename, nil
}

func archiveClosedFirefox(ctx context.Context, output *os.File, args []string, checkClosed func() error) error {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	watchDone := make(chan struct{})
	go func() {
		defer close(watchDone)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := checkClosed(); err != nil {
					cancel(err)
					return
				}
			}
		}
	}()

	command := exec.CommandContext(ctx, "tar", args...)
	command.Env = append(command.Environ(), "TAR_OPTIONS=")
	command.Stdout, command.Stderr = output, os.Stderr
	err := command.Run()
	cause := context.Cause(ctx)
	cancel(nil)
	<-watchDone
	if cause != nil {
		return cause
	}
	if err != nil {
		return fmt.Errorf("criar arquivo tar do Firefox: %w", err)
	}
	return nil
}
