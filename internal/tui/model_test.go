package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jfmainville/revunk/internal/provider/github"
	"github.com/jfmainville/revunk/internal/review"
	"github.com/jfmainville/revunk/internal/store"
)

func newLoadedModel(t *testing.T) Model {
	t.Helper()
	svc := review.NewService(github.SampleSource{}, store.NewMemory())
	m := New(svc, review.PullRequest{Owner: "o", Repo: "r", Number: 1})
	updated, _ := m.Update(m.Init()())
	return updated.(Model)
}

func TestViewFitsTerminalAfterResize(t *testing.T) {
	m := newLoadedModel(t)
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 120, Height: 40}, {Width: 60, Height: 12}} {
		updated, _ := m.Update(size)
		m = updated.(Model)

		out := m.render()
		if h := lipgloss.Height(out); h > size.Height {
			t.Errorf("%dx%d: view height %d overflows terminal", size.Width, size.Height, h)
		}
		if w := lipgloss.Width(out); w > size.Width {
			t.Errorf("%dx%d: view width %d overflows terminal", size.Width, size.Height, w)
		}
	}
}

func TestNavigationShowsSelectedPatchAndClampsCursor(t *testing.T) {
	m := newLoadedModel(t)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = updated.(Model)

	for range 5 {
		updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
		m = updated.(Model)
	}

	if m.cursor != len(m.files)-1 {
		t.Fatalf("cursor = %d, want clamped to %d", m.cursor, len(m.files)-1)
	}
	if !strings.Contains(m.diff.View(), "Outdated notes") {
		t.Fatalf("diff view does not show the last file's patch:\n%s", m.diff.View())
	}
}

func TestQuitKeyReturnsQuitCommand(t *testing.T) {
	m := newLoadedModel(t)
	_, cmd := m.Update(tea.KeyPressMsg{Code: 'q', Text: "q"})
	if cmd == nil {
		t.Fatal("expected a quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q did not produce tea.QuitMsg")
	}
}
