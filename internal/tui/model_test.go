package tui

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jfmainville/revunk/internal/provider/github"
	"github.com/jfmainville/revunk/internal/review"
	"github.com/jfmainville/revunk/internal/store"
)

type fakeSource struct {
	files []review.File
	err   error
}

func (f fakeSource) Files(context.Context, review.PullRequest) ([]review.File, error) {
	return f.files, f.err
}

func loadedModel(t *testing.T, source review.Source) Model {
	t.Helper()
	m := New(review.NewService(source, store.NewMemory()), review.PullRequest{Owner: "o", Repo: "r", Number: 1})
	m = update(t, m, m.Init()())
	return update(t, m, tea.WindowSizeMsg{Width: 100, Height: 30})
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func press(t *testing.T, m Model, k string) (Model, tea.Cmd) {
	t.Helper()
	msg := tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	if k == "ctrl+c" {
		msg = tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
	}
	updated, cmd := m.Update(msg)
	return updated.(Model), cmd
}

func TestViewFitsTerminalAfterResize(t *testing.T) {
	m := loadedModel(t, github.SampleSource{})
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 120, Height: 40}, {Width: 60, Height: 12}} {
		m = update(t, m, size)

		out := m.render()
		if h := lipgloss.Height(out); h > size.Height {
			t.Errorf("%dx%d: view height %d overflows terminal", size.Width, size.Height, h)
		}
		if w := lipgloss.Width(out); w > size.Width {
			t.Errorf("%dx%d: view width %d overflows terminal", size.Width, size.Height, w)
		}
	}
}

func TestRenderIsEmptyBeforeFirstWindowSize(t *testing.T) {
	m := New(review.NewService(github.SampleSource{}, store.NewMemory()), review.PullRequest{})
	if out := m.render(); out != "" {
		t.Fatalf("expected empty render before sizing, got %q", out)
	}
}

func TestViewUsesAltScreen(t *testing.T) {
	if !loadedModel(t, github.SampleSource{}).View().AltScreen {
		t.Fatal("expected the view to use the alternate screen")
	}
}

func TestStatusShowsPullRequest(t *testing.T) {
	if out := loadedModel(t, github.SampleSource{}).render(); !strings.Contains(out, "o/r#1") {
		t.Fatalf("status line missing pull request:\n%s", out)
	}
}

func TestNavigationShowsSelectedPatchAndClampsCursor(t *testing.T) {
	m := loadedModel(t, github.SampleSource{})

	m, _ = press(t, m, "k")
	if m.cursor != 0 {
		t.Fatalf("cursor moved above the first file to %d", m.cursor)
	}
	for range 5 {
		m, _ = press(t, m, "j")
	}

	if m.cursor != len(m.files)-1 {
		t.Fatalf("cursor = %d, want clamped to %d", m.cursor, len(m.files)-1)
	}
	if !strings.Contains(m.diff.View(), "Outdated notes") {
		t.Fatalf("diff view does not show the last file's patch:\n%s", m.diff.View())
	}
}

func TestPageKeysScrollDiffAndSelectionResetsScroll(t *testing.T) {
	long := strings.Repeat("+line\n", 200)
	m := loadedModel(t, fakeSource{files: []review.File{{Path: "a.go", Patch: long}, {Path: "b.go", Patch: "+b"}}})

	m, _ = press(t, m, "f")
	if m.diff.YOffset() == 0 {
		t.Fatal("page down did not scroll the diff")
	}
	m, _ = press(t, m, "b")
	if m.diff.YOffset() != 0 {
		t.Fatalf("page up left offset at %d", m.diff.YOffset())
	}

	m, _ = press(t, m, "f")
	m, _ = press(t, m, "j")
	if m.diff.YOffset() != 0 {
		t.Fatal("selecting another file did not reset the diff scroll")
	}
}

func TestToggleViewedMarksSelectedFile(t *testing.T) {
	m := loadedModel(t, github.SampleSource{})

	for _, want := range []bool{true, false} {
		var cmd tea.Cmd
		m, cmd = press(t, m, "v")
		if cmd == nil {
			t.Fatal("v did not return a command")
		}
		m = update(t, m, cmd())

		marked := strings.Contains(m.renderFiles(), "✓ cmd/revunk/main.go")
		if marked != want {
			t.Fatalf("viewed mark = %v, want %v:\n%s", marked, want, m.renderFiles())
		}
	}
}

func TestToggleViewedErrorIsShownWithoutMarking(t *testing.T) {
	m := loadedModel(t, github.SampleSource{})
	m = update(t, m, viewedToggledMsg{path: "README.md", viewed: true, err: errors.New("store down")})

	if !strings.Contains(m.render(), "store down") {
		t.Fatal("toggle error not shown in status line")
	}
	if m.viewed["README.md"] {
		t.Fatal("file marked viewed despite the error")
	}
}

func TestLoadShowsPreviouslyViewedFiles(t *testing.T) {
	pr := review.PullRequest{Owner: "o", Repo: "r", Number: 1}
	mem := store.NewMemory()
	if err := mem.SetViewed(context.Background(), pr, "README.md", true); err != nil {
		t.Fatal(err)
	}
	m := New(review.NewService(github.SampleSource{}, mem), pr)
	m = update(t, m, m.Init()())

	if !strings.Contains(m.renderFiles(), "✓ README.md") {
		t.Fatalf("stored viewed state not shown on load:\n%s", m.renderFiles())
	}
}

func TestLoadErrorIsShownInStatus(t *testing.T) {
	m := loadedModel(t, fakeSource{err: errors.New("rate limited")})
	if !strings.Contains(m.render(), "rate limited") {
		t.Fatalf("load error not shown:\n%s", m.render())
	}
}

func TestEmptyFileListShowsPlaceholderAndIgnoresKeys(t *testing.T) {
	m := loadedModel(t, fakeSource{})

	m, _ = press(t, m, "j")
	m, cmd := press(t, m, "v")

	if cmd != nil {
		t.Fatal("toggling viewed with no files should do nothing")
	}
	if m.cursor != 0 {
		t.Fatalf("cursor moved to %d with no files", m.cursor)
	}
	if !strings.Contains(m.diff.View(), "No files to review.") {
		t.Fatalf("missing empty placeholder:\n%s", m.diff.View())
	}
}

func TestQuitKeysReturnQuitCommand(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		_, cmd := press(t, loadedModel(t, github.SampleSource{}), k)
		if cmd == nil {
			t.Fatalf("%s returned no command", k)
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s did not produce tea.QuitMsg", k)
		}
	}
}

func TestHelpListsEveryKeyBinding(t *testing.T) {
	short := keys.ShortHelp()
	full := keys.FullHelp()
	if len(full) != 1 || len(full[0]) != len(short) {
		t.Fatalf("FullHelp should contain the ShortHelp bindings, got %v", full)
	}
	if len(short) != 6 {
		t.Fatalf("expected 6 bindings in help, got %d", len(short))
	}
	for _, b := range short {
		if b.Help().Key == "" || b.Help().Desc == "" {
			t.Errorf("binding %v has no help text", b.Keys())
		}
	}
}
