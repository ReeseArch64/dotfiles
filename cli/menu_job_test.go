package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestJobConfirmationDefaultsToCancel(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys []tea.KeyType
		want error
	}{
		{"Enter cancela por padrão", []tea.KeyType{tea.KeyEnter}, errActionCanceled},
		{"seta e Enter confirmam", []tea.KeyType{tea.KeyRight, tea.KeyEnter}, nil},
		{"Esc cancela mesmo após escolher confirmar", []tea.KeyType{tea.KeyRight, tea.KeyEsc}, errActionCanceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newMenu("desktop")
			m.screen, m.stack = screenJob, []screen{screenPreinstalled}
			job, start := startLocalJob(m.profile, "Remover aplicativos", func(_ profile, c sshConsole) error {
				fmt.Fprintln(c.output, "Apagar dados do Alacritty")
				return c.confirm("Remover estes arquivos?")
			})
			m.job = job
			next, _ := m.Update(start())
			view := ansi.Strip(next.View())
			for _, text := range []string{"Apagar dados do Alacritty", "Remover estes arquivos?", "[ Cancelar ]", "Confirmar"} {
				if !strings.Contains(view, text) {
					t.Fatalf("texto %q ausente no diálogo: %s", text, view)
				}
			}
			var cmd tea.Cmd
			for _, key := range tc.keys {
				next, cmd = next.Update(tea.KeyMsg{Type: key})
			}
			if cmd == nil {
				t.Fatal("o diálogo não retomou a ação")
			}
			next, _ = next.Update(cmd())
			result := next.(model).job.result
			if !errors.Is(result.err, tc.want) || !strings.Contains(result.details, "Apagar dados") {
				t.Fatalf("resultado: %+v", result)
			}
			next, _ = next.Update(tea.KeyMsg{Type: tea.KeyEsc})
			if next.(model).screen != screenPreinstalled {
				t.Fatal("não voltou ao submenu de origem")
			}
		})
	}
}

func TestJobTextAndContinueStayInMenu(t *testing.T) {
	m := newMenu("desktop")
	m.screen = screenJob
	job, start := startLocalJob(m.profile, "Testar entrada", func(_ profile, c sshConsole) error {
		text, err := c.prompt("Cole um texto")
		if err != nil {
			return err
		}
		fmt.Fprint(c.output, text)
		return c.proceed("Volte quando estiver pronto")
	})
	m.job = job
	next, _ := m.Update(start())
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ignorar\nesta colagem"), Paste: true})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("entrada válida!"), Paste: true})
	next, _ = next.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	next, cmd := next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = next.Update(cmd())
	view := ansi.Strip(next.View())
	if !strings.Contains(view, "[ Continuar ]") || !strings.Contains(view, "entrada válida") || strings.Contains(view, "ignorar") {
		t.Fatalf("diálogo de continuação incorreto: %s", view)
	}
	next, cmd = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = next.Update(cmd())
	if result := next.(model).job.result; result.err != nil || result.details != "entrada válida" {
		t.Fatalf("resultado: %+v", result)
	}
}

func TestNoctaliaRequirementsAndRefresh(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_STATE_HOME", home)
	t.Setenv("NOCTALIA_STATE_HOME", "")
	plugins := filepath.Join(noctaliaDirectory(home), "plugins", "materialized")
	install := func(name string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Join(plugins, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	install("official/world_clock")
	m := newMenu("laptop")
	m.screen = screenSystem
	next, cmd := m.handleKey("7")
	next, _ = next.Update(cmd())
	page := next.(model)
	if page.screen != screenNoctalia || !strings.Contains(ansi.Strip(page.View()), "Plugins pendentes") {
		t.Fatalf("requisito ausente: %s", page.View())
	}
	if blocked, cmd := page.handleKey("2"); cmd != nil || blocked.(model).screen != screenNoctalia {
		t.Fatal("atalho ignorou os requisitos")
	}
	next, cmd = page.handleKey("1")
	next, _ = next.Update(cmd())
	result := next.(model).job.result
	if result.err != nil || !strings.Contains(result.details, "Instalado · official/world_clock") || !strings.Contains(result.details, "Pendente · community/androidconnect") {
		t.Fatalf("listagem inesperada: %+v", result)
	}
	for _, name := range noctaliaPlugins {
		install(name)
	}
	next, cmd = next.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next, _ = next.Update(cmd())
	if next.(model).items()[1].disabled {
		t.Fatal("aplicação continuou bloqueada após instalar os plugins")
	}
}
