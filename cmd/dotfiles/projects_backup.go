package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"github.com/charmbracelet/lipgloss"
)

const (
	projectsWorkspace = "/mnt/workspaces"
	projectsRoot      = "/mnt/workspaces/reesearch64"
)

type gitCommand func(context.Context, string, ...string) error

func ownedPrivateDirectory(path string, uid, gid int) bool {
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() || info.Mode().Perm() != 0700 {
		return false
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	return ok && int(stat.Uid) == uid && int(stat.Gid) == gid
}

func projectsWorkspaceSetupArgs(path string, uid, gid int) []string {
	return []string{"install", "-d", "-m", "0700", "-o", strconv.Itoa(uid), "-g", strconv.Itoa(gid), path}
}

func projectsWorkspaceSetupSteps(path string, uid, gid int) []step {
	if ownedPrivateDirectory(path, uid, gid) {
		return nil
	}
	return withSudo(sudoStep("Preparar "+path, false, projectsWorkspaceSetupArgs(path, uid, gid)...))
}

func ensureProjectsRoot(path string) error {
	if err := os.MkdirAll(path, 0700); err != nil {
		return fmt.Errorf("criar %s: %w", path, err)
	}
	if err := os.Chmod(path, 0700); err != nil {
		return fmt.Errorf("proteger %s: %w", path, err)
	}
	return nil
}

func ensureProjectsLink(home, target string) error {
	link := filepath.Join(home, ".projects")
	current, err := os.Readlink(link)
	if err == nil {
		if current == target {
			return nil
		}
		return fmt.Errorf("%s aponta para %s; esperado %s", link, current, target)
	}
	if _, statErr := os.Lstat(link); statErr == nil {
		return fmt.Errorf("%s já existe e não é um link simbólico", link)
	} else if !os.IsNotExist(statErr) {
		return fmt.Errorf("verificar %s: %w", link, statErr)
	}
	if err := os.Symlink(target, link); err != nil {
		return fmt.Errorf("criar %s: %w", link, err)
	}
	return nil
}

func findProjectRepositories(root string) ([]string, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("acessar %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s não é um diretório", root)
	}
	var repositories []string
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && entry.Name() == ".git" {
			repositories = append(repositories, filepath.Dir(path))
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("mapear projetos em %s: %w", root, err)
	}
	sort.Strings(repositories)
	return repositories, nil
}

func runProjectGit(ctx context.Context, repository string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, args...)...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		detail := strings.TrimSpace(output.String())
		if detail != "" {
			return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, detail)
		}
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

func backupProjectRepositories(ctx context.Context, root string, run gitCommand) (int, error) {
	repositories, err := findProjectRepositories(root)
	if err != nil {
		return 0, err
	}
	if len(repositories) == 0 {
		return 0, fmt.Errorf("nenhum projeto Git encontrado em %s", root)
	}
	var failures []error
	completed := 0
	for _, repository := range repositories {
		if err := run(ctx, repository, "push", "origin"); err != nil {
			failures = append(failures, fmt.Errorf("%s -> GitHub: %w", repository, err))
			continue
		}
		if err := run(ctx, repository, "push", "backup"); err != nil {
			failures = append(failures, fmt.Errorf("%s -> GitLab: %w", repository, err))
			continue
		}
		completed++
	}
	if len(failures) > 0 {
		return completed, fmt.Errorf("backup concluído em %d/%d projetos: %w", completed, len(repositories), errors.Join(failures...))
	}
	return completed, nil
}

func projectsBackupJobFor(setup []step, prepare, link func() error, backup func() (int, error)) job {
	count := 0
	steps := append([]step{}, setup...)
	steps = append(steps,
		nativeStep("Criar "+projectsRoot, prepare),
		nativeStep("Criar link ~/.projects", link),
		nativeStep("Enviar projetos ao GitHub e GitLab", func() error {
			var err error
			count, err = backup()
			return err
		}),
	)
	return job{
		title: "Backup de Projetos Locais",
		steps: steps,
		result: func() string {
			return lipgloss.NewStyle().Foreground(colOK).Render(fmt.Sprintf("%d projeto(s) enviado(s) ao GitHub e ao GitLab.", count))
		},
	}
}

func projectsBackupJob() job {
	home, _ := os.UserHomeDir()
	setup := projectsWorkspaceSetupSteps(projectsWorkspace, os.Getuid(), os.Getgid())
	return projectsBackupJobFor(
		setup,
		func() error { return ensureProjectsRoot(projectsRoot) },
		func() error { return ensureProjectsLink(home, projectsWorkspace) },
		func() (int, error) {
			return backupProjectRepositories(context.Background(), projectsRoot, runProjectGit)
		},
	)
}
