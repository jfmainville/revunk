package tui

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jfmainville/revunk/internal/review"
)

type fakeSource struct {
	commits []review.Commit
	files   map[string][]review.File
	err     error
}

func (f fakeSource) Commits(context.Context) ([]review.Commit, error) { return f.commits, f.err }
func (f fakeSource) Files(_ context.Context, commit string) ([]review.File, error) {
	return f.files[commit], f.err
}

type fakeStore struct {
	viewed   map[string]string
	comments []review.Comment
	err      error
	next     int
}

func (s *fakeStore) SetViewed(_ context.Context, _, path, hash string) error {
	if s.err != nil {
		return s.err
	}
	if hash == "" {
		delete(s.viewed, path)
	} else {
		s.viewed[path] = hash
	}
	return nil
}
func (s *fakeStore) Viewed(context.Context, string) (map[string]string, error) {
	return s.viewed, nil
}
func (s *fakeStore) AddComment(_ context.Context, _ string, c review.Comment) error {
	if s.err != nil {
		return s.err
	}
	s.next++
	c.ID = fmt.Sprintf("id%d", s.next)
	s.comments = append(s.comments, c)
	return nil
}
func (s *fakeStore) Comments(context.Context, string) ([]review.Comment, error) {
	return slices.Clone(s.comments), nil
}
func (s *fakeStore) UpdateComments(_ context.Context, _ string, ids []string, update func(*review.Comment)) error {
	if s.err != nil {
		return s.err
	}
	for i := range s.comments {
		if slices.Contains(ids, s.comments[i].ID) {
			update(&s.comments[i])
		}
	}
	return nil
}
func (s *fakeStore) DeleteComments(_ context.Context, _ string, ids []string) error {
	if s.err != nil {
		return s.err
	}
	s.comments = slices.DeleteFunc(s.comments, func(c review.Comment) bool { return slices.Contains(ids, c.ID) })
	return nil
}

var sample = fakeSource{
	commits: []review.Commit{{SHA: "1111111aaaa", Subject: "add readme"}, {SHA: "2222222bbbb", Subject: "rework main"}},
	files: map[string][]review.File{
		"": {
			{Path: "README.md", Status: review.StatusAdded, Additions: 2, Patch: "@@ -0,0 +1,2 @@\n+# Title\n+text"},
			{Path: "a/main.go", Status: review.StatusModified, Additions: 2, Deletions: 1, Patch: "@@ -1,3 +1,4 @@\n package main\n-func old() {}\n+func a() {}\n+func b() {}\n // end"},
			{Path: "img.png", Status: review.StatusModified},
		},
		"2222222bbbb": {
			{Path: "a/main.go", Status: review.StatusModified, Additions: 1, Patch: "@@ -1 +1,2 @@\n package main\n+func only() {}"},
		},
	},
}

func newModel(t *testing.T, source review.Source, st *fakeStore) Model {
	t.Helper()
	m := New(review.NewService(source, st, nil, "feature"), "", "main")
	m = update(t, m, m.Init()())
	return update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
}

func loaded(t *testing.T) (Model, *fakeStore) {
	t.Helper()
	st := &fakeStore{viewed: map[string]string{}}
	return newModel(t, sample, st), st
}

func update(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

var specialKeys = map[string]tea.KeyPressMsg{
	"enter":     {Code: tea.KeyEnter},
	"esc":       {Code: tea.KeyEscape},
	"tab":       {Code: tea.KeyTab},
	"shift+tab": {Code: tea.KeyTab, Mod: tea.ModShift},
	"ctrl+w":    {Code: 'w', Mod: tea.ModCtrl},
	"down":      {Code: tea.KeyDown},
	"up":        {Code: tea.KeyUp},
	"pgdown":    {Code: tea.KeyPgDown},
	"pgup":      {Code: tea.KeyPgUp},
	"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
	"ctrl+d":    {Code: 'd', Mod: tea.ModCtrl},
	"ctrl+u":    {Code: 'u', Mod: tea.ModCtrl},
	"ctrl+s":    {Code: 's', Mod: tea.ModCtrl},
	"ctrl+n":    {Code: 'n', Mod: tea.ModCtrl},
	"ctrl+p":    {Code: 'p', Mod: tea.ModCtrl},
	"backspace": {Code: tea.KeyBackspace},
	"space":     {Code: tea.KeySpace, Text: " "},
}

func press(t *testing.T, m Model, keys ...string) (Model, tea.Cmd) {
	t.Helper()
	var cmd tea.Cmd
	for _, k := range keys {
		msg, ok := specialKeys[k]
		if !ok {
			msg = tea.KeyPressMsg{Code: []rune(k)[0], Text: k}
		}
		var updated tea.Model
		updated, cmd = m.Update(msg)
		m = updated.(Model)
	}
	return m, cmd
}

func typeText(t *testing.T, m Model, text string) Model {
	t.Helper()
	for _, r := range text {
		m = update(t, m, tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return m
}

func run(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		t.Fatal("expected a command")
	}
	return update(t, m, cmd())
}

func screen(m Model) string {
	return ansi.Strip(m.View().Content)
}

func rowOf(t *testing.T, m Model, kind rowKind, file, line int) int {
	t.Helper()
	for i, r := range m.rows {
		if r.kind == kind && r.file == file && (kind == rowFile || r.line == line) {
			return i
		}
	}
	t.Fatalf("no row kind=%d file=%d line=%d", kind, file, line)
	return -1
}

func TestLoadedScreenMatchesDesignLayout(t *testing.T) {
	m, _ := loaded(t)
	out := screen(m)

	for _, want := range []string{"Agent", "commits", "all changes", "1111111 add readme", "files", "a/", "M main.go", "continuous diff", "vs main", "A  README.md", "+2 -0", "unseen", "@@ -1,3 +1,4 @@", "no textual changes", "NORMAL", " feature", "0 comments · 0/3 viewed"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen missing %q", want)
		}
	}
	if strings.Contains(out, "local changes on") {
		t.Fatal("the top bar should only show the tab")
	}
	if !strings.Contains(out, "│     1    package main") || !strings.Contains(out, "     2 -  func old() {}") || !strings.Contains(out, "     2 +  func a() {}") || strings.Contains(out, "1    1") {
		t.Fatalf("diff lines missing numbers or markers:\n%s", out)
	}
}

func TestViewFitsTerminalAfterResize(t *testing.T) {
	m, _ := loaded(t)
	for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 160, Height: 50}, {Width: 50, Height: 12}} {
		m = update(t, m, size)
		out := m.render()
		if h := lipgloss.Height(out); h != size.Height {
			t.Errorf("%dx%d: view height %d", size.Width, size.Height, h)
		}
		if w := lipgloss.Width(out); w > size.Width {
			t.Errorf("%dx%d: view width %d overflows", size.Width, size.Height, w)
		}
	}
}

func TestRenderIsEmptyBeforeFirstWindowSizeAndUsesAltScreen(t *testing.T) {
	m := New(review.NewService(sample, &fakeStore{viewed: map[string]string{}}, nil, "feature"), "", "main")
	if out := m.render(); out != "" {
		t.Fatalf("expected empty render before sizing, got %q", out)
	}
	if !m.View().AltScreen {
		t.Fatal("expected the alternate screen")
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 80, Height: 20})
	if !strings.Contains(screen(m), "loading…") {
		t.Fatal("missing loading state before data arrives")
	}
}

func TestLoadErrorAndEmptyBranchAreShown(t *testing.T) {
	m := newModel(t, fakeSource{err: errors.New("bad revision")}, &fakeStore{viewed: map[string]string{}})
	if !strings.Contains(screen(m), "bad revision") {
		t.Fatalf("load error not shown:\n%s", screen(m))
	}
	m = newModel(t, fakeSource{}, &fakeStore{viewed: map[string]string{}})
	if out := screen(m); !strings.Contains(out, "no changes between main and feature") || !strings.Contains(out, "no changes") {
		t.Fatalf("empty state missing:\n%s", out)
	}
	m, cmd := press(t, m, "-")
	if cmd != nil {
		t.Fatal("r with no files should do nothing")
	}
	if _, cmd := press(t, m, "C"); cmd != nil {
		t.Fatal("C with no files should do nothing")
	}
}

func TestTabCyclesFocusAndHLJump(t *testing.T) {
	m, _ := loaded(t)
	if m.focus != focusFiles {
		t.Fatalf("initial focus = %d, want files", m.focus)
	}
	for _, want := range []focusArea{focusDiff, focusCommits, focusFiles} {
		m, _ = press(t, m, "tab")
		if m.focus != want {
			t.Fatalf("focus = %d, want %d", m.focus, want)
		}
	}
	m, _ = press(t, m, "shift+tab")
	if m.focus != focusCommits {
		t.Fatalf("shift+tab focus = %d", m.focus)
	}
	m, _ = press(t, m, "l")
	if m.focus != focusDiff {
		t.Fatal("l did not focus the diff")
	}
	m, _ = press(t, m, "tab", "tab")
	if m.focus != focusFiles {
		t.Fatal("tab did not cycle back to the files")
	}
	m, _ = press(t, m, "enter")
	if m.focus != focusDiff {
		t.Fatal("enter on a file did not focus the diff")
	}
}

func TestSelectingFileScrollsContinuousDiffToItsHeader(t *testing.T) {
	m, _ := loaded(t)

	m, _ = press(t, m, "j")

	header := rowOf(t, m, rowFile, 1, 0)
	if m.fileCursor != 1 || m.cursor != header || m.offset != header {
		t.Fatalf("fileCursor=%d cursor=%d offset=%d, want 1/%d/%d", m.fileCursor, m.cursor, m.offset, header, header)
	}
	if !strings.Contains(screen(m), "2/3") {
		t.Fatal("files counter not updated")
	}
	m, _ = press(t, m, "j", "j", "j")
	if m.fileCursor != 2 {
		t.Fatalf("fileCursor = %d, want clamped to 2", m.fileCursor)
	}
	m, _ = press(t, m, "k", "k", "k", "up")
	if m.fileCursor != 0 {
		t.Fatalf("fileCursor = %d, want clamped to 0", m.fileCursor)
	}
}

func TestDiffNavigationSkipsTextRowsAndTracksFile(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "l")

	m, _ = press(t, m, "]", "f")
	if m.cursor != rowOf(t, m, rowFile, 1, 0) || m.fileCursor != 1 {
		t.Fatalf("]f moved to row %d (file %d)", m.cursor, m.fileCursor)
	}
	m, _ = press(t, m, "]", "f")
	m, _ = press(t, m, "j", "j", "down")
	if m.cursor != rowOf(t, m, rowFile, 2, 0) {
		t.Fatalf("cursor landed on a text row: %d", m.cursor)
	}
	m, _ = press(t, m, "[", "f")
	if m.fileCursor != 1 {
		t.Fatalf("[f went to file %d", m.fileCursor)
	}
	m, _ = press(t, m, "{")
	if m.cursor != rowOf(t, m, rowLine, 0, 1) {
		t.Fatalf("{ did not reach the previous change: %+v", m.rows[m.cursor])
	}
	m, _ = press(t, m, "}")
	if m.cursor != rowOf(t, m, rowLine, 1, 2) {
		t.Fatalf("} did not reach the next change")
	}
	m, _ = press(t, m, "G")
	if m.cursor != rowOf(t, m, rowFile, 2, 0) {
		t.Fatalf("G stopped at %d", m.cursor)
	}
	m, _ = press(t, m, "g", "g")
	if m.cursor != 0 || m.offset != 0 {
		t.Fatalf("gg left cursor=%d offset=%d", m.cursor, m.offset)
	}
	m, _ = press(t, m, "ctrl+d")
	if m.cursor == 0 {
		t.Fatal("ctrl+d did not move")
	}
	m, _ = press(t, m, "ctrl+u", "pgdown", "pgup", "k")
	if m.cursor != 0 {
		t.Fatalf("cursor = %d after paging back up", m.cursor)
	}
}

func TestSmallWindowScrollsDiffToKeepCursorVisible(t *testing.T) {
	m, _ := loaded(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 9})
	m, _ = press(t, m, "l", "G")

	if m.offset == 0 || m.cursor-m.offset >= m.diffHeight() {
		t.Fatalf("cursor %d not visible from offset %d (height %d)", m.cursor, m.offset, m.diffHeight())
	}
	if !strings.Contains(screen(m), "img.png") {
		t.Fatalf("last file not on screen:\n%s", screen(m))
	}
}

func TestPastingFillsTheCommentEditor(t *testing.T) {
	m, _ := loaded(t)
	if m = update(t, m, tea.PasteMsg{Content: "c"}); m.editing {
		t.Fatal("a paste should not run keys")
	}
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "c")
	m = update(t, m, tea.PasteMsg{Content: "one\ntwo\nthree\nfour"})
	if m.editor.area.Value() != "one\ntwo\nthree\nfour" || m.editor.area.Height() != 4 {
		t.Fatalf("pasted value = %q, height = %d", m.editor.area.Value(), m.editor.area.Height())
	}
}

func TestCommentOnLineIsSavedAndShownUnderIt(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)

	m, _ = press(t, m, "c")
	if !m.editing || !strings.Contains(screen(m), "INSERT") || !strings.Contains(screen(m), "comment ─") || !strings.Contains(screen(m), " L2 ─") {
		t.Fatalf("editor not open on L2:\n%s", screen(m))
	}
	m = typeText(t, m, "Rename `a`.")
	m, _ = press(t, m, "tab")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)

	want := review.Comment{ID: "id1", Kind: "note", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "Rename `a`."}
	if len(st.comments) != 1 || !reflect.DeepEqual(st.comments[0], want) {
		t.Fatalf("stored %+v, want %+v", st.comments, want)
	}
	out := screen(m)
	lineAt := strings.Index(out, "func a() {}")
	commentAt := strings.Index(out, " note")
	if m.editing || commentAt < lineAt || !strings.Contains(out, "L2 · draft") || !strings.Contains(out, "Rename") || !strings.Contains(out, "1 comments") {
		t.Fatalf("comment not rendered under its line:\n%s", out)
	}
}

func TestVisualRangeCommentsOnSelectedLines(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 1)

	m, _ = press(t, m, "V")
	if m.anchor < 0 || !strings.Contains(screen(m), "VISUAL") {
		t.Fatal("V did not start visual mode")
	}
	m, _ = press(t, m, "j", "j", "j", "c")
	m = typeText(t, m, "Split this.")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)

	got := st.comments[0]
	if got.Side != review.SideRight || got.Start != 1 || got.End != 3 {
		t.Fatalf("range comment = %+v, want right L1-3", got)
	}
	if !strings.Contains(screen(m), "L1-3") {
		t.Fatal("range label not shown")
	}
}

func TestVisualToggleAndEscapeCancel(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "l")
	m, _ = press(t, m, "v")
	if m.anchor >= 0 {
		t.Fatal("visual started on a file header")
	}
	m.cursor = rowOf(t, m, rowLine, 0, 1)
	m, _ = press(t, m, "v")
	m, _ = press(t, m, "v")
	if m.anchor >= 0 {
		t.Fatal("second v did not end visual mode")
	}
	m, _ = press(t, m, "V", "esc")
	if m.anchor >= 0 {
		t.Fatal("esc did not end visual mode")
	}
}

func TestCommentOnRemovedLineUsesLeftSide(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 2)

	m, _ = press(t, m, "c")
	m = typeText(t, m, "why?")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)

	if got := st.comments[0]; got.Side != review.SideLeft || got.End != 2 {
		t.Fatalf("comment = %+v, want left L2", got)
	}
	if !strings.Contains(screen(m), "why?") {
		t.Fatal("left-side comment not shown")
	}
}

func TestFileCommentFromFilesPaneAndHeader(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "j", "C")
	if !m.editing || !m.editor.target.FileLevel() || m.editor.target.Path != "a/main.go" {
		t.Fatalf("C did not open a file comment: %+v", m.editor.target)
	}
	m = typeText(t, m, "Whole file looks good.")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)

	out := screen(m)
	if !strings.Contains(out, " comment") || !strings.Contains(out, "file · draft") || strings.Index(out, "Whole file") > strings.Index(out, "package main") {
		t.Fatalf("file comment not shown below its header:\n%s", out)
	}

	m, _ = press(t, m, "c")
	if !m.editing || !m.editor.target.FileLevel() {
		t.Fatal("c in the diff on a file header should open a file comment")
	}
	m, _ = press(t, m, "esc", "esc")
	m.cursor = rowOf(t, m, rowHunk, 0, 0)
	m, cmd = press(t, m, "c")
	if m.editing || cmd != nil || !strings.Contains(screen(m), "move to a line") {
		t.Fatal("c on a hunk header should explain instead of opening the editor")
	}
	if len(st.comments) != 1 {
		t.Fatalf("stored comments = %+v", st.comments)
	}
}

func TestEditorNewlineSuggestionAndValidation(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "V", "j", "c")
	if strings.Contains(screen(m), "```suggestion") {
		t.Fatal("an empty comment should not show a suggestion block")
	}

	m, _ = press(t, m, "ctrl+s")
	if !m.editing || !strings.Contains(screen(m), "write something") {
		t.Fatal("blank comment should keep the editor open with a notice")
	}
	m.editor.area.SetValue("Use this:")
	m, _ = press(t, m, "tab", "tab", "tab")
	if want := "Use this:\n\n```suggestion\nfunc a() {}\nfunc b() {}\n```"; m.editor.area.Value() != want {
		t.Fatalf("suggestion block = %q, want %q", m.editor.area.Value(), want)
	}
	m.editor.area.SetValue("first")
	m.editor.area.CursorEnd()
	m, _ = press(t, m, "enter")
	m = typeText(t, m, "second")
	m, _ = press(t, m, "enter")
	if m.editor.area.Value() != "first\nsecond\n" {
		t.Fatalf("newlines not inserted: %q", m.editor.area.Value())
	}
	m, _ = press(t, m, "tab", "tab", "tab", "tab")
	if strings.Count(m.editor.area.Value(), "```suggestion") != 1 {
		t.Fatalf("suggestion injected twice: %q", m.editor.area.Value())
	}
	m, _ = press(t, m, "shift+tab")
	if commentKinds[m.editor.kind] != "issue" {
		t.Fatalf("shift+tab kind = %s", commentKinds[m.editor.kind])
	}
	if m.editor.area.Value() != "first\nsecond" {
		t.Fatalf("leaving suggestion should remove the block: %q", m.editor.area.Value())
	}
	if m, _ = press(t, m, "esc"); !m.editing || !m.editor.normal || !strings.Contains(screen(m), "NORMAL") || !strings.Contains(screen(m), "i insert") {
		t.Fatal("esc while typing should only switch to normal mode")
	}
	m, _ = press(t, m, "esc")
	if m.editing || len(st.comments) != 0 {
		t.Fatal("esc in normal mode should close the editor without saving")
	}

	m.cursor = rowOf(t, m, rowLine, 1, 2)
	m, _ = press(t, m, "c", "tab", "tab", "tab")
	if m.editor.area.Value() != "" || !strings.Contains(screen(m), "suggestions apply") {
		t.Fatal("suggestions on removed lines should be refused")
	}
	if _, cmd := press(t, m, "ctrl+c"); cmd == nil {
		t.Fatal("ctrl+c should quit from the editor")
	}
}

func TestSuggestionRendersAsDiffAgainstOriginalLines(t *testing.T) {
	got := expandSuggestions("Try:\n```suggestion\nfunc a(ctx context.Context) {}\n```\nthanks", []string{"func a() {}"})
	for _, want := range []string{"**Suggested change**", "- func a() {}", "+ func a(ctx context.Context) {}", "thanks"} {
		if !strings.Contains(got, want) {
			t.Errorf("expanded body missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "```suggestion") {
		t.Fatal("suggestion fence left in place")
	}
	if got := expandSuggestions("```suggestion\n```", []string{"x"}); !strings.Contains(got, "- x") || strings.Contains(got, "+ ") {
		t.Fatalf("empty suggestion should only remove lines: %q", got)
	}
	if got := expandSuggestions("plain", nil); got != "plain" {
		t.Fatalf("plain body changed: %q", got)
	}
}

func TestDeleteCommentUnderCursor(t *testing.T) {
	m, st := loaded(t)
	st.comments = []review.Comment{
		{ID: "a", Kind: "nit", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "keep me"},
		{ID: "b", Kind: "issue", Path: "a/main.go", Body: "file note"},
	}
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "l")

	m.cursor = rowOf(t, m, rowFile, 1, 0)
	m, cmd := press(t, m, "d", "d")
	m = run(t, m, cmd)
	if len(st.comments) != 1 || st.comments[0].Body != "keep me" || strings.Contains(screen(m), "file note") {
		t.Fatalf("dd on header removed the wrong comment: %+v", st.comments)
	}

	m.cursor = rowOf(t, m, rowLine, 1, 4)
	m, cmd = press(t, m, "d", "d")
	if cmd != nil || !strings.Contains(screen(m), "no comment here") {
		t.Fatal("dd with nothing to delete should explain")
	}
}

func TestResolveHidesCommentFromDiffButKeepsItInThePane(t *testing.T) {
	m, st := loaded(t)
	st.comments = []review.Comment{{ID: "a", Kind: "nit", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "rename this"}}
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "l")

	m.cursor = rowOf(t, m, rowLine, 1, 4)
	if m, cmd := press(t, m, "x"); cmd != nil || !strings.Contains(screen(m), "no comment here to resolve") {
		t.Fatal("x with no comment under the cursor should explain")
	}

	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, cmd := press(t, m, "x")
	m = run(t, m, cmd)
	if !st.comments[0].Resolved || len(st.comments) != 1 {
		t.Fatalf("x should resolve without deleting: %+v", st.comments)
	}
	if out := screen(m); strings.Contains(out, "rename this") || !strings.Contains(out, "resolved the comment") {
		t.Fatalf("resolved comment should leave the diff:\n%s", out)
	}
	if again, cmd := press(t, m, "S"); cmd != nil || !strings.Contains(screen(again), "no unsent comments to send") {
		t.Fatal("resolved comments should not be sent to the agent")
	}

	m, _ = press(t, m, "t")
	if out := screen(m); !strings.Contains(out, " nit") || !strings.Contains(out, "resolved  rename this") {
		t.Fatalf("comments pane should keep the resolved comment:\n%s", out)
	}

	m, cmd = press(t, m, "x")
	m = run(t, m, cmd)
	if out := screen(m); st.comments[0].Resolved || !strings.Contains(out, "reopened the comment") || !strings.Contains(out, " nit") || !strings.Contains(out, "L2 · draft") {
		t.Fatalf("x in the pane should reopen the comment in the diff:\n%s", out)
	}

	st.err = errors.New("store locked")
	m, cmd = press(t, m, "x")
	if m = run(t, m, cmd); !strings.Contains(screen(m), "store locked") {
		t.Fatal("resolve error not shown")
	}
}

func TestReplyAnswersTheAgentUnderTheComment(t *testing.T) {
	m, st := loaded(t)
	st.comments = []review.Comment{{ID: "a", Kind: "nit", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "rename this", Sent: true}}
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	if m, cmd := press(t, m, "r"); cmd != nil || m.editing || !strings.Contains(screen(m), "you can reply once the agent has answered") {
		t.Fatal("r before the agent answers should explain")
	}
	m.cursor = rowOf(t, m, rowLine, 1, 4)
	if m, _ := press(t, m, "r"); m.editing || !strings.Contains(screen(m), "no comment here to reply to") {
		t.Fatal("r with no comment under the cursor should explain")
	}

	st.comments[0].Replies = []string{"renamed to b"}
	m = update(t, m, m.Init()())
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "r")
	if out := screen(m); !m.editing || !strings.Contains(out, "reply to the agent") || !strings.Contains(out, "L2 · reply") || strings.Contains(out, "suggestion") {
		t.Fatalf("r should open a reply editor under the comment:\n%s", out)
	}
	m, _ = press(t, m, "tab")
	if m.editor.kind != 0 {
		t.Fatal("tab should not change the type of a reply")
	}
	m = typeText(t, m, "why b?")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)
	if c := st.comments[0]; c.Sent || !reflect.DeepEqual(c.Replies, []string{"renamed to b", review.YourReply + "why b?"}) || len(st.comments) != 1 {
		t.Fatalf("reply not saved as a follow-up: %+v", st.comments)
	}
	if out := screen(m); !strings.Contains(out, " your reply") || !strings.Contains(out, "why b?") || !strings.Contains(out, "L2 · draft") || strings.Contains(out, "**you**") {
		t.Fatalf("follow-up should show under the comment as a draft:\n%s", out)
	}
	if m, _ = press(t, m, "r"); m.editing {
		t.Fatal("r should wait for the agent's next answer")
	}

	st.comments[0].Replies = append(st.comments[0].Replies, "short for b")
	st.comments[0].Sent, st.comments[0].Commit = true, "2222222bbbb"
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "t")
	if m, _ = press(t, m, "r"); m.editing || !strings.Contains(screen(m), "to go to the comment first") {
		t.Fatal("r from the pane on a comment outside this view should explain")
	}
}

func TestOutdatedCommentIsShownUnderFileHeader(t *testing.T) {
	st := &fakeStore{viewed: map[string]string{}, comments: []review.Comment{{Kind: "question", Path: "a/main.go", Side: review.SideRight, Start: 40, End: 40, Body: "still relevant?"}}}
	m := newModel(t, sample, st)
	out := screen(m)
	if !strings.Contains(out, "still relevant?") || strings.Index(out, "still relevant?") > strings.Index(out, "package main") {
		t.Fatalf("outdated comment missing or misplaced:\n%s", out)
	}
}

func TestMarkViewedCollapsesFileUntilItChanges(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "j")

	m, cmd := press(t, m, "-")
	m = run(t, m, cmd)

	out := screen(m)
	if !strings.Contains(out, "") || !strings.Contains(out, "viewed") || strings.Contains(out, "func a() {}") || !strings.Contains(out, "1/3 viewed") {
		t.Fatalf("viewed file not collapsed and marked:\n%s", out)
	}
	if st.viewed["a/main.go"] == "" {
		t.Fatal("viewed state not stored")
	}

	m, _ = press(t, m, "j")
	if m.fileCursor != 1 {
		t.Fatalf("j should reach the reviewed file, got %d", m.fileCursor)
	}
	m, cmd = press(t, m, "-")
	m = run(t, m, cmd)
	if !strings.Contains(screen(m), "func a() {}") {
		t.Fatal("unmarking did not expand the file")
	}

	failing := newModel(t, sample, &fakeStore{viewed: map[string]string{}, err: errors.New("read-only")})
	failing, cmd = press(t, failing, "-")
	failing = run(t, failing, cmd)
	if !strings.Contains(screen(failing), "read-only") {
		t.Fatal("store error not shown")
	}
}

func TestCommitSelectionLoadsCommitFiles(t *testing.T) {
	st := &fakeStore{viewed: map[string]string{}, comments: []review.Comment{{Kind: "nit", Path: "a/main.go", Body: "branch note"}}}
	m := newModel(t, sample, st)
	m, _ = press(t, m, "tab", "tab")

	m, cmd := press(t, m, "j")
	stale := cmd()
	m, cmd = press(t, m, "j")
	m = run(t, m, cmd)
	m = update(t, m, stale)

	out := screen(m)
	if len(m.files) != 1 || !strings.Contains(out, "func only() {}") || !strings.Contains(out, "2222222") {
		t.Fatalf("commit diff not shown:\n%s", out)
	}
	if strings.Contains(out, "branch note") {
		t.Fatal("branch comment shown on a single commit")
	}
	m, _ = press(t, m, "j")
	if m.commit != 2 {
		t.Fatal("commit selection moved past the last commit")
	}
	m, cmd = press(t, m, "k", "k", "up")
	if m.commit != 0 || cmd != nil {
		t.Fatalf("commit = %d after moving up", m.commit)
	}

	failing := newModel(t, sample, &fakeStore{viewed: map[string]string{}})
	failing, _ = press(t, failing, "tab", "tab", "j")
	failing = update(t, failing, filesMsg{commit: 1, err: errors.New("bad object")})
	if !strings.Contains(screen(failing), "bad object") {
		t.Fatal("commit load error not shown")
	}
}

func TestCommentSaveErrorIsShown(t *testing.T) {
	st := &fakeStore{viewed: map[string]string{}, err: errors.New("disk full")}
	m := newModel(t, sample, st)
	m, _ = press(t, m, "C")
	m = typeText(t, m, "x")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)
	if !strings.Contains(screen(m), "disk full") {
		t.Fatal("save error not shown")
	}
	st.comments = []review.Comment{{ID: "a", Path: "README.md", Body: "y"}}
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "l", "g", "g")
	m, cmd = press(t, m, "d", "d")
	m = run(t, m, cmd)
	if !strings.Contains(screen(m), "disk full") {
		t.Fatal("delete error not shown")
	}
}

func TestHelpOverlayAndQuit(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "?")
	if out := screen(m); !strings.Contains(out, "help · esc closes") || !strings.Contains(out, "mark file viewed") {
		t.Fatalf("help not shown:\n%s", out)
	}
	m, cmd := press(t, m, "q", "?")
	if !m.help || cmd != nil {
		t.Fatal("only esc closes the help, q and ? do nothing there")
	}
	m, _ = press(t, m, "?", "esc")
	if m.help {
		t.Fatal("esc did not close help")
	}
	for _, k := range []string{"q", "ctrl+c"} {
		if _, cmd := press(t, m, k); cmd == nil {
			t.Fatalf("%s did not quit", k)
		} else if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Fatalf("%s did not produce tea.QuitMsg", k)
		}
	}
}

func TestHighlightColorsSyntaxAndFallsBack(t *testing.T) {
	lines := review.ParseLines("@@ -1 +1 @@\n+func a() { return \"x\" } // note")
	tokens := highlight(lexerFor("main.go"), lines)
	if len(tokens[0]) != 0 {
		t.Fatalf("hunk line got tokens: %+v", tokens[0])
	}
	colors := map[string]string{}
	for _, tk := range tokens[1] {
		colors[strings.TrimSpace(tk.text)] = ansi.Strip(tk.style.Render("x")) + tk.style.Render("x")
	}
	if !strings.Contains(colors["func"], "235;175;57") {
		t.Fatalf("keyword not amber: %q", colors["func"])
	}
	if !strings.Contains(colors[`"x"`], "174;199;108") {
		t.Fatalf("string not green: %q", colors[`"x"`])
	}
	plain := highlight(lexerFor("notes.unknownext"), review.ParseLines("@@ -1 +1 @@\n+just\ttext"))
	if got := plain[1][0].text; got != "just    text" {
		t.Fatalf("fallback token = %q, want tabs expanded", got)
	}
}

func TestChangedWordsMarksOnlyTheDifferingWords(t *testing.T) {
	marked := func(s string, hit []bool) string {
		var b strings.Builder
		for i, c := range []byte(s) {
			if hit[i] {
				b.WriteByte(c)
			} else {
				b.WriteByte('.')
			}
		}
		return b.String()
	}
	for _, tc := range []struct{ name, a, b, wantOld, wantNew string }{
		{"one word", "return foo(a, b)", "return foo(a, c)", "..............b.", "..............c."},
		{"spaces between changed words", "a b c", "a x y c", "..b..", "..x y.."},
		{"appended argument", "call(a)", "call(a, longer)", ".......", "......, longer."},
		{"whitespace only", "a b", "a  b", ". .", ".  ."},
		{"whole rewrite", "func old() {}", "var x = 42", "", ""},
		{"blank line", "", "text", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			oldHit, newHit := changedWords(tc.a, tc.b)
			if tc.wantOld == "" {
				if oldHit != nil || newHit != nil {
					t.Fatalf("lines too different should not be highlighted, got %q %q", marked(tc.a, oldHit), marked(tc.b, newHit))
				}
				return
			}
			if got := marked(tc.a, oldHit); got != tc.wantOld {
				t.Errorf("old = %q, want %q", got, tc.wantOld)
			}
			if got := marked(tc.b, newHit); got != tc.wantNew {
				t.Errorf("new = %q, want %q", got, tc.wantNew)
			}
		})
	}
	if oldHit, _ := changedWords(strings.Repeat("a ", 600), strings.Repeat("b ", 600)); oldHit != nil {
		t.Fatal("very long lines should be skipped")
	}
}

func TestMarkWordsPairsRemovedAndAddedLinesInOrder(t *testing.T) {
	lines := review.ParseLines("@@ -1,3 +1,3 @@\n keep := 1\n-x := 1\n-y := 2\n+x := 3\n+totally different\n tail")
	tokens := plainTokens(lines)
	background := func(row int, text string) any {
		for _, tk := range tokens[row] {
			if tk.text == text {
				return tk.style.GetBackground()
			}
		}
		t.Fatalf("no %q token on row %d: %+v", text, row, tokens[row])
		return nil
	}
	if background(2, "1") != colorDelWord || background(4, "3") != colorAddWord || background(2, "x := ") == colorDelWord || background(4, "x := ") == colorAddWord {
		t.Fatal("the first removed and added lines should only highlight the changed number")
	}
	if len(tokens[1]) != 1 || len(tokens[3]) != 1 || len(tokens[5]) != 1 || len(tokens[6]) != 1 {
		t.Fatal("context lines and lines too different should stay unhighlighted")
	}
}

func TestChangedWordsAreHighlightedOnBothTabs(t *testing.T) {
	bg := func(c color.Color) string {
		r, g, b, _ := c.RGBA()
		return fmt.Sprintf("48;2;%d;%d;%dm", r>>8, g>>8, b>>8)
	}
	m, _ := loaded(t)
	m, _ = press(t, m, "l")
	if out := strings.Join(m.diffLines(), "\n"); !strings.Contains(out, bg(colorDelWord)) || !strings.Contains(out, bg(colorAddWord)) {
		t.Fatalf("the Agent diff should highlight old and a:\n%s", out)
	}
	pulls, _ := pullsModel(t)
	pulls = pressRun(t, pulls, "g", "d")
	if pulls.diff == nil {
		t.Fatal("gd should open the review diff")
	}
	if out := strings.Join(pulls.diff.diffLines(), "\n"); !strings.Contains(out, bg(colorDelWord)) || !strings.Contains(out, bg(colorAddWord)) {
		t.Fatalf("the review diff should highlight old and a:\n%s", out)
	}
}

func TestSmallHelpers(t *testing.T) {
	for want, c := range map[string]review.Comment{"file": {}, "L3": {Start: 3, End: 3}, "L3-5": {Start: 3, End: 5}} {
		if got := lineLabel(c); got != want {
			t.Errorf("lineLabel(%+v) = %q, want %q", c, got, want)
		}
	}
	for want, c := range map[string]review.Comment{"draft": {}, "sent": {Sent: true}, "replied": {Sent: true, Replies: []string{"ok"}}} {
		if got := commentState(c); got != want {
			t.Errorf("commentState(%+v) = %q, want %q", c, got, want)
		}
	}
	for s, want := range map[review.FileStatus]string{review.StatusAdded: "A", review.StatusRemoved: "D", review.StatusRenamed: "R", review.StatusModified: "M"} {
		if got := statusLetter(s); got != want {
			t.Errorf("statusLetter(%s) = %q", s, got)
		}
	}
	if number(0) != "" || number(12) != "12" {
		t.Fatal("number formatting wrong")
	}
	if got := ansi.Strip(fit("abcdef", 3, lipgloss.NewStyle())); got != "abc" {
		t.Fatalf("fit truncate = %q", got)
	}
	if got := ansi.Strip(fit("ab", 4, lipgloss.NewStyle())); got != "ab  " {
		t.Fatalf("fit pad = %q", got)
	}
	if box("t", "", nil, 3, 5, nil) != "" {
		t.Fatal("box too narrow should render nothing")
	}
	if got := ansi.Strip(box("a-very-long-title", "right", nil, 12, 2, colorPrimary)); strings.Contains(got, "right") {
		t.Fatalf("right label should be dropped when it does not fit: %q", got)
	}
	if got := ansi.Strip(renderFileHeader(review.File{Path: strings.Repeat("x", 50)}, false, false, 20)); lipgloss.Width(got) != 20 {
		t.Fatalf("narrow header width = %d", lipgloss.Width(got))
	}
}

type fakeAgent struct {
	prompts []string
	err     error
}

func (a *fakeAgent) Prompt(_ context.Context, text string) (string, error) {
	a.prompts = append(a.prompts, text)
	return "opencode", a.err
}

func withComment(t *testing.T, agent review.Agent) Model {
	t.Helper()
	st := &fakeStore{viewed: map[string]string{}, comments: []review.Comment{{ID: "c1", Kind: "nit", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "rename"}}}
	m := New(review.NewService(sample, st, agent, "feature"), "", "main")
	m = update(t, m, m.Init()())
	return update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
}

func TestSendCommentsPromptsTheAgent(t *testing.T) {
	agent := &fakeAgent{}
	m := withComment(t, agent)

	m, cmd := press(t, m, "S")
	if !strings.Contains(screen(m), "sending 1 comments") {
		t.Fatal("no sending notice")
	}
	m = run(t, m, cmd)

	if len(agent.prompts) != 1 || !strings.Contains(agent.prompts[0], "nit on a/main.go line 2") || !strings.Contains(agent.prompts[0], "rename") {
		t.Fatalf("prompts = %q", agent.prompts)
	}
	if out := screen(m); !strings.Contains(out, "sent 1 comments to opencode, replies show under each one") || !strings.Contains(out, "L2 · sent") || len(m.comments) != 1 {
		t.Fatalf("sent comment not kept and marked sent:\n%s", out)
	}
	if again, cmd := press(t, m, "S"); cmd != nil || !strings.Contains(screen(again), "no unsent comments to send") {
		t.Fatal("S should skip comments already sent")
	}

	failing := withComment(t, &fakeAgent{err: errors.New("no Claude Code session open")})
	failing, cmd = press(t, failing, "S")
	failing = run(t, failing, cmd)
	if !strings.Contains(screen(failing), "no Claude Code session open") || len(failing.comments) != 1 {
		t.Fatal("agent error not shown or comments dropped")
	}

	empty, _ := loaded(t)
	if empty, cmd = press(t, empty, "S"); cmd != nil || !strings.Contains(screen(empty), "no unsent comments to send") {
		t.Fatal("S without comments should explain")
	}
}

func TestCopyCommentsWritesClipboard(t *testing.T) {
	var copied string
	orig := writeClipboard
	writeClipboard = func(s string) error { copied = s; return nil }
	t.Cleanup(func() { writeClipboard = orig })
	m := withComment(t, nil)

	m, cmd := press(t, m, "y")
	updated, osc := m.Update(cmd())
	m = updated.(Model)

	if !strings.Contains(copied, "nit on a/main.go line 2") || !strings.Contains(copied, "func a() {}") {
		t.Fatalf("clipboard = %q", copied)
	}
	if osc == nil || !strings.Contains(screen(m), "copied 1 comments") {
		t.Fatal("copy should also set the terminal clipboard and confirm")
	}

	empty, _ := loaded(t)
	if empty, cmd = press(t, empty, "y"); cmd != nil || !strings.Contains(screen(empty), "no unsent comments to copy") {
		t.Fatal("y without comments should explain")
	}
	stale, _ := loaded(t)
	stale.comments = []review.Comment{{Path: "x"}}
	stale, cmd = press(t, stale, "y")
	stale = run(t, stale, cmd)
	if !strings.Contains(screen(stale), "no unsent comments on feature") {
		t.Fatalf("prompt error not shown:\n%s", screen(stale))
	}
}

func TestCommentsAndEditorAreDrawnInBorderedBoxes(t *testing.T) {
	m := withComment(t, nil)
	if out := screen(m); !strings.Contains(out, "╭─  nit") || !strings.Contains(out, "rename") {
		t.Fatalf("comment box missing:\n%s", out)
	}
	m, _ = press(t, m, "C")
	if out := screen(m); !strings.Contains(out, "╭─  comment") || !strings.Contains(out, "C-s save") {
		t.Fatalf("editor box missing:\n%s", out)
	}
}

func TestDeleteAllCommentsNeedsSecondPress(t *testing.T) {
	m := withComment(t, nil)

	m, cmd := press(t, m, "D")
	if cmd != nil || !strings.Contains(screen(m), "press D again to delete all 1 comments") {
		t.Fatal("first D should ask for confirmation")
	}
	m, cmd = press(t, m, "D")
	m = run(t, m, cmd)
	if out := screen(m); len(m.comments) != 0 || strings.Contains(out, " nit") || !strings.Contains(out, "deleted 1 comments") {
		t.Fatalf("comments not deleted:\n%s", out)
	}

	m, cmd = press(t, m, "D")
	if cmd != nil || !strings.Contains(screen(m), "no comments to delete") {
		t.Fatal("D without comments should explain")
	}

	kept := withComment(t, nil)
	kept, _ = press(t, kept, "D", "j")
	if len(kept.comments) != 1 || kept.prefix != "" {
		t.Fatal("another key after D should cancel")
	}

	failing := withComment(t, nil)
	failing.svc = review.NewService(sample, &fakeStore{viewed: map[string]string{}, comments: []review.Comment{{Path: "x"}}, err: errors.New("disk full")}, nil, "feature")
	failing.comments = []review.Comment{{Path: "x"}}
	failing, cmd = press(t, failing, "D", "D")
	failing = run(t, failing, cmd)
	if !strings.Contains(screen(failing), "disk full") {
		t.Fatalf("delete all error not shown:\n%s", screen(failing))
	}
}

func TestPollingShowsAgentRepliesUnderComments(t *testing.T) {
	m := withComment(t, nil)
	st := m.svc

	tickCmd := func() tea.Cmd {
		_, cmd := m.Update(loadedMsg{comments: m.comments})
		return cmd
	}()
	if tickCmd == nil {
		t.Fatal("loading should start polling")
	}
	if err := st.Reply(context.Background(), "c1", "Renamed to `start`."); err != nil {
		t.Fatal(err)
	}
	ticked, cmd := m.Update(tickMsg{})
	polled := cmd()
	updated, next := ticked.(Model).Update(polled)
	got := updated.(Model)

	out := screen(got)
	if !strings.Contains(out, "L2 · replied") || !strings.Contains(out, " agent reply") || !strings.Contains(out, "Renamed to") || !strings.Contains(out, "start") {
		t.Fatalf("reply not shown:\n%s", out)
	}
	if next == nil {
		t.Fatal("polling should continue")
	}
	same, _ := got.Update(polled)
	if !reflect.DeepEqual(same.(Model).comments, got.comments) {
		t.Fatal("unchanged poll altered comments")
	}
}

func pollOnce(t *testing.T, m Model) (Model, tea.Cmd) {
	t.Helper()
	ticked, cmd := m.Update(tickMsg{})
	updated, next := ticked.(Model).Update(cmd())
	return updated.(Model), next
}

func mutableSample() *fakeSource {
	files := map[string][]review.File{}
	for k, v := range sample.files {
		files[k] = slices.Clone(v)
	}
	return &fakeSource{commits: slices.Clone(sample.commits), files: files}
}

func TestRefreshShowsNewChangesAndKeepsCursorOnSameLine(t *testing.T) {
	src := mutableSample()
	m := newModel(t, src, &fakeStore{viewed: map[string]string{}})
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 4)
	m.offset = m.cursor - 3

	src.files[""] = append([]review.File{{Path: "0new.go", Status: review.StatusAdded, Additions: 1, Patch: "@@ -0,0 +1 @@\n+package zero"}}, src.files[""]...)
	m, next := pollOnce(t, m)

	if next == nil {
		t.Fatal("polling stopped")
	}
	if out := screen(m); !strings.Contains(out, "0new.go") {
		t.Fatalf("new file not shown:\n%s", out)
	}
	r := m.rows[m.cursor]
	if m.files[r.file].Path != "a/main.go" || r.line != 4 || m.cursor-m.offset != 3 || m.fileCursor != 2 {
		t.Fatalf("cursor moved to %s line %d, screen row %d, fileCursor %d", m.files[r.file].Path, r.line, m.cursor-m.offset, m.fileCursor)
	}

	src.files[""] = []review.File{{Path: "a/main.go", Status: review.StatusModified, Patch: "@@ -1 +1 @@\n-x\n+y"}}
	m, _ = pollOnce(t, m)
	if r := m.rows[m.cursor]; r.kind != rowFile || m.files[r.file].Path != "a/main.go" {
		t.Fatalf("cursor not moved to the file header when its line vanished: %+v", r)
	}
	src.files[""] = nil
	m, _ = pollOnce(t, m)
	if out := screen(m); !strings.Contains(out, "no changes between main and feature") {
		t.Fatalf("emptied branch not shown:\n%s", out)
	}
}

func TestRefreshKeepsSelectedCommitBySHA(t *testing.T) {
	src := mutableSample()
	m := newModel(t, src, &fakeStore{viewed: map[string]string{}})
	m, _ = press(t, m, "tab", "tab")
	m, cmd := press(t, m, "j", "j")
	m = run(t, m, cmd)

	src.commits = append(src.commits, review.Commit{SHA: "3333333cccc", Subject: "follow up"})
	src.files["2222222bbbb"] = []review.File{{Path: "a/main.go", Status: review.StatusModified, Patch: "@@ -1 +1,2 @@\n package main\n+func again() {}"}}
	m, _ = pollOnce(t, m)
	if out := screen(m); m.commitSHA() != "2222222bbbb" || !strings.Contains(out, "3333333 follow up") || !strings.Contains(out, "func again() {}") {
		t.Fatalf("selected commit lost or not refreshed (commit %d):\n%s", m.commit, out)
	}

	src.commits = []review.Commit{{SHA: "9999999dddd", Subject: "rebased"}}
	m, _ = pollOnce(t, m)
	if m.commit != 0 || !strings.Contains(screen(m), "func again() {}") {
		t.Fatalf("rebased away commit should fall back to all changes without loading stale files, commit=%d", m.commit)
	}
	m, _ = pollOnce(t, m)
	if strings.Contains(screen(m), "func again() {}") || !strings.Contains(screen(m), "README.md") {
		t.Fatalf("all changes not loaded on the next poll:\n%s", screen(m))
	}
}

func TestRefreshAppliesViewedStateAndIgnoresErrors(t *testing.T) {
	src := mutableSample()
	st := &fakeStore{viewed: map[string]string{}}
	m := newModel(t, src, st)
	m, cmd := press(t, m, "j", "-")
	m = run(t, m, cmd)
	st.viewed = map[string]string{}

	m, _ = pollOnce(t, m)
	if m.viewed["a/main.go"] || !strings.Contains(screen(m), "func a() {}") {
		t.Fatal("viewed state cleared elsewhere was not picked up")
	}

	src.err = errors.New("index.lock exists")
	m, next := pollOnce(t, m)
	if next == nil || m.err != nil || len(m.files) != 3 {
		t.Fatalf("poll error should be ignored and polling continue, err=%v files=%d", m.err, len(m.files))
	}
}

func TestFileAndHunkKeysJumpForwardAndBackward(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "l")

	m, _ = press(t, m, "f")
	if m.cursor != rowOf(t, m, rowFile, 1, 0) || m.fileCursor != 1 {
		t.Fatalf("f moved to row %d", m.cursor)
	}
	m, _ = press(t, m, "h")
	if m.cursor != rowOf(t, m, rowLine, 1, 2) {
		t.Fatalf("h moved to row %d", m.cursor)
	}
	m, _ = press(t, m, "H")
	if m.cursor != rowOf(t, m, rowLine, 0, 1) {
		t.Fatalf("H moved to row %d", m.cursor)
	}
	m, _ = press(t, m, "f", "f", "F")
	if m.cursor != rowOf(t, m, rowFile, 1, 0) {
		t.Fatalf("F moved to row %d", m.cursor)
	}
	before := m.cursor
	m, _ = press(t, m, "f", "f", "f")
	if m.fileCursor != 2 || m.cursor == before {
		t.Fatalf("f past the last file should stay on it, fileCursor=%d", m.fileCursor)
	}

	files, _ := loaded(t)
	files, _ = press(t, files, "f")
	if files.focus != focusFiles || files.fileCursor != 1 {
		t.Fatalf("f from the files pane should move the file selection, fileCursor=%d", files.fileCursor)
	}
	paged, _ := loaded(t)
	paged, _ = press(t, paged, "l", "pgdown")
	if paged.cursor == 0 {
		t.Fatal("pgdown no longer pages")
	}
}

func listModel(t *testing.T) (Model, *fakeStore) {
	t.Helper()
	st := &fakeStore{viewed: map[string]string{}, comments: []review.Comment{
		{ID: "a", Kind: "note", Path: "a/main.go", Side: review.SideRight, Start: 3, End: 3, Body: "second func\nmore"},
		{ID: "b", Kind: "comment", Path: "README.md", Body: "title?"},
		{ID: "c", Kind: "suggestion", Commit: "2222222bbbb", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "rename only", Sent: true},
	}}
	return newModel(t, sample, st), st
}

func TestCommentsPaneIsHiddenUntilToggled(t *testing.T) {
	m, _ := listModel(t)
	if strings.Contains(screen(m), "╭─ comments") {
		t.Fatal("comments pane should start hidden")
	}

	m, _ = press(t, m, "t")

	out := screen(m)
	for _, want := range []string{"╭─ comments", " note", "a/main.go L3  draft  second func", "README.md file", "a/main.go L2 @2222222  sent  rename only"} {
		if !strings.Contains(out, want) {
			t.Errorf("comments pane missing %q:\n%s", want, out)
		}
	}
	if m.focus != focusComments || lipgloss.Height(m.render()) != 40 {
		t.Fatalf("focus=%d height=%d", m.focus, lipgloss.Height(m.render()))
	}
	m, _ = press(t, m, "tab")
	if m.focus != focusCommits {
		t.Fatalf("tab from comments should wrap to commits, got %d", m.focus)
	}
	m, _ = press(t, m, "shift+tab")
	if m.focus != focusComments {
		t.Fatalf("shift+tab should come back to comments, got %d", m.focus)
	}
	m, _ = press(t, m, "t")
	if strings.Contains(screen(m), "╭─ comments") || m.focus != focusDiff {
		t.Fatal("t should hide the pane and return to the diff")
	}

	empty, _ := loaded(t)
	empty, _ = press(t, empty, "t")
	if !strings.Contains(screen(empty), "no comments, press c on a line") {
		t.Fatal("empty pane should explain how to add a comment")
	}
	if _, cmd := press(t, empty, "enter"); cmd != nil {
		t.Fatal("enter with no comments should do nothing")
	}
	if empty, cmd := press(t, empty, "d", "d"); cmd != nil || !strings.Contains(screen(empty), "no comment here") {
		t.Fatal("dd with no comments should explain")
	}
}

func TestCommentsPaneNavigatesToComments(t *testing.T) {
	m, _ := listModel(t)
	m, _ = press(t, m, "t", "j", "k", "k", "enter")
	if m.focus != focusDiff || m.cursor != rowOf(t, m, rowLine, 1, 4) {
		t.Fatalf("enter did not jump to the line comment: focus=%d cursor=%d", m.focus, m.cursor)
	}

	m, _ = press(t, m, "tab", "j", "enter")
	if m.cursor != rowOf(t, m, rowFile, 0, 0) {
		t.Fatalf("file comment should jump to its header, cursor=%d", m.cursor)
	}

	m.focus = focusComments
	m, cmd := press(t, m, "j", "j", "j")
	if m.listCursor != 2 {
		t.Fatalf("list cursor = %d, want clamped to 2", m.listCursor)
	}
	m, cmd = press(t, m, "enter")
	m = run(t, m, cmd)
	if m.commitSHA() != "2222222bbbb" || m.focus != focusDiff || m.rows[m.cursor].kind != rowLine || m.lines[0][m.rows[m.cursor].line].New != 2 {
		t.Fatalf("did not switch to the commit and jump: commit=%q focus=%d", m.commitSHA(), m.focus)
	}

	m.focus = focusComments
	m.listCursor = 0
	m, cmd = press(t, m, "enter")
	m = run(t, m, cmd)
	if m.commit != 0 || m.cursor != rowOf(t, m, rowLine, 1, 4) {
		t.Fatalf("did not switch back to all changes: commit=%d", m.commit)
	}
}

func TestCommentsPaneHandlesMissingTargetsAndDeletes(t *testing.T) {
	m, st := listModel(t)
	st.comments = append(st.comments, review.Comment{ID: "d", Commit: "gone", Path: "x.go", Body: "old"}, review.Comment{ID: "e", Path: "gone.go", Body: "moved"})
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "t", "j", "j", "j")

	m, cmd := press(t, m, "enter")
	if cmd != nil || !strings.Contains(screen(m), "no longer on the branch") {
		t.Fatal("comment on a vanished commit should explain")
	}
	m, _ = press(t, m, "j", "enter")
	if !strings.Contains(screen(m), "gone.go is not in this diff") || m.focus != focusComments {
		t.Fatal("comment on a missing file should explain and keep focus")
	}

	m, cmd = press(t, m, "d", "d")
	m = run(t, m, cmd)
	if len(st.comments) != 4 || slices.ContainsFunc(st.comments, func(c review.Comment) bool { return c.ID == "e" }) {
		t.Fatalf("dd in the pane deleted the wrong comment: %+v", st.comments)
	}
	if out := screen(m); strings.Contains(out, "gone.go") {
		t.Fatalf("deleted comment still listed:\n%s", out)
	}
}

func click(t *testing.T, m Model, x, y int) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.MouseClickMsg{X: x, Y: y, Button: tea.MouseLeft})
	return updated.(Model), cmd
}

func wheel(t *testing.T, m Model, x, y int, button tea.MouseButton) (Model, tea.Cmd) {
	t.Helper()
	updated, cmd := m.Update(tea.MouseWheelMsg{X: x, Y: y, Button: button})
	return updated.(Model), cmd
}

func TestViewEnablesMouse(t *testing.T) {
	m, _ := loaded(t)
	if !m.View().ReportFocus {
		t.Fatal("the view should report focus changes")
	}
	if m.View().MouseMode != tea.MouseModeCellMotion {
		t.Fatal("mouse should be enabled")
	}
}

func TestClickSelectsCommitsAndFiles(t *testing.T) {
	m, _ := loaded(t)
	m, cmd := click(t, m, 5, m.panesHeight()-m.commitsHeight()+4)
	if m.focus != focusCommits || m.commit != 2 {
		t.Fatalf("focus=%d commit=%d", m.focus, m.commit)
	}
	m = run(t, m, cmd)
	if len(m.files) != 1 || m.files[0].Path != "a/main.go" {
		t.Fatalf("files not loaded for clicked commit: %+v", m.files)
	}
	if _, cmd := click(t, m, 5, m.panesHeight()-m.commitsHeight()+3); cmd == nil {
		t.Fatal("clicking another commit should load it")
	}

	m, _ = loaded(t)
	m, _ = click(t, m, 5, 4)
	if m.focus != focusFiles || m.fileCursor != 1 || m.rows[m.cursor].kind != rowFile || m.rows[m.cursor].file != 1 {
		t.Fatalf("focus=%d fileCursor=%d cursor=%d", m.focus, m.fileCursor, m.cursor)
	}
	m, _ = click(t, m, 5, 3)
	if m.fileCursor != 1 {
		t.Fatalf("clicking a directory should keep the file, got %d", m.fileCursor)
	}
	m, _ = click(t, m, 5, 5)
	if m.fileCursor != 2 {
		t.Fatalf("fileCursor=%d", m.fileCursor)
	}
}

func TestClickMovesDiffCursor(t *testing.T) {
	m, _ := loaded(t)
	target := rowOf(t, m, rowLine, 1, 2)
	m, _ = click(t, m, 60, 2+target-m.offset)
	if m.focus != focusDiff || m.cursor != target || m.fileCursor != 1 {
		t.Fatalf("focus=%d cursor=%d want %d", m.focus, m.cursor, target)
	}
	m, _ = click(t, m, 60, 2+len(m.rows)+1)
	if m.cursor != target {
		t.Fatal("clicking below the last row should keep the cursor")
	}
	m, _ = press(t, m, "c")
	m = typeText(t, m, "hi")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)
	text := slices.IndexFunc(m.rows, func(r row) bool { return r.kind == rowText && r.file == 1 })
	if text < 0 {
		t.Fatal("expected comment rows")
	}
	if m, _ = click(t, m, 60, 2+text-m.offset); m.cursor != target {
		t.Fatalf("clicking comment text should keep the cursor, got %d", m.cursor)
	}
}

func TestClickOnBordersOnlyFocuses(t *testing.T) {
	m, _ := loaded(t)
	before := m.cursor
	m, cmd := click(t, m, 60, 1)
	if m.focus != focusDiff || m.cursor != before || cmd != nil {
		t.Fatalf("title click should only focus: focus=%d cursor=%d", m.focus, m.cursor)
	}
	m, _ = click(t, m, 5, m.panesHeight())
	if m.focus != focusCommits || m.commit != 0 {
		t.Fatalf("bottom border click should only focus: focus=%d commit=%d", m.focus, m.commit)
	}
	m, _ = click(t, m, 60, 0)
	m, _ = click(t, m, 60, 39)
	if m.focus != focusCommits {
		t.Fatalf("tab bar and footer clicks should do nothing, focus=%d", m.focus)
	}
	m, _ = click(t, m, 60, m.panesHeight())
	if m.focus != focusDiff || m.cursor != before {
		t.Fatalf("diff bottom border click should only focus: cursor=%d", m.cursor)
	}
	right, _ := m.Update(tea.MouseClickMsg{X: 5, Y: 4, Button: tea.MouseRight})
	if right.(Model).fileCursor != m.fileCursor {
		t.Fatal("right click should be ignored")
	}
}

func TestClickCommentsPaneGoesToComment(t *testing.T) {
	m, _ := listModel(t)
	m, _ = press(t, m, "t")
	top := 1 + m.panesHeight()
	m, _ = click(t, m, 10, top)
	if m.focus != focusComments {
		t.Fatalf("pane title click should focus the pane, got %d", m.focus)
	}
	m, cmd := click(t, m, 10, top+1)
	if cmd != nil || m.focus != focusDiff || m.listCursor != 0 || m.cursor != rowOf(t, m, rowLine, 1, 4) {
		t.Fatalf("focus=%d listCursor=%d cursor=%d", m.focus, m.listCursor, m.cursor)
	}
	m, cmd = click(t, m, 10, top+3)
	if m.listCursor != 2 || cmd == nil {
		t.Fatalf("clicking a comment from another commit should switch commit, listCursor=%d", m.listCursor)
	}
	m, _ = click(t, m, 10, 38)
	if m.listCursor != 2 {
		t.Fatal("clicking the status line should not select a comment")
	}

	hidden, _ := listModel(t)
	hidden, _ = click(t, hidden, 10, 37)
	if hidden.focus == focusComments {
		t.Fatal("clicks below the panes should be ignored while the pane is hidden")
	}
}

func longModel(t *testing.T) Model {
	t.Helper()
	lines := make([]string, 100)
	for i := range lines {
		lines[i] = fmt.Sprintf("+line %d", i+1)
	}
	patch := "@@ -0,0 +1,100 @@\n" + strings.Join(lines, "\n")
	return newModel(t, fakeSource{files: map[string][]review.File{"": {{Path: "long.go", Status: review.StatusAdded, Additions: 100, Patch: patch}}}}, &fakeStore{viewed: map[string]string{}})
}

func TestWheelScrollsTheDiffWithoutMovingTheCursor(t *testing.T) {
	m := longModel(t)
	m, _ = wheel(t, m, 60, 10, tea.MouseWheelDown)
	if m.offset != 3 || m.cursor != 0 || m.focus != focusDiff {
		t.Fatalf("wheel down should scroll 3 rows and keep the cursor, offset=%d cursor=%d", m.offset, m.cursor)
	}
	m, _ = wheel(t, m, 60, 10, tea.MouseWheelUp)
	m, _ = wheel(t, m, 60, 10, tea.MouseWheelUp)
	if m.offset != 0 || m.cursor != 0 {
		t.Fatalf("wheel up should stop at the top, offset=%d cursor=%d", m.offset, m.cursor)
	}
	for range 50 {
		m, _ = wheel(t, m, 60, 10, tea.MouseWheelDown)
	}
	if last := len(m.rows) - m.diffHeight(); m.offset != last || m.cursor != 0 || !strings.Contains(screen(m), "line 100") {
		t.Fatalf("wheel down should stop with the last row at the bottom, offset=%d want %d", m.offset, last)
	}
	m, _ = press(t, m, "j")
	if m.cursor == 0 || m.offset > m.cursor {
		t.Fatalf("moving the cursor should bring it back into view, offset=%d cursor=%d", m.offset, m.cursor)
	}
}

func TestWheelScrollsPaneUnderPointer(t *testing.T) {
	m, _ := loaded(t)
	m, _ = wheel(t, m, 5, 5, tea.MouseWheelDown)
	if m.fileCursor != 1 {
		t.Fatalf("wheel over files should select the next file, got %d", m.fileCursor)
	}
	m, cmd := wheel(t, m, 5, m.panesHeight()-m.commitsHeight()+3, tea.MouseWheelDown)
	if m.commit != 1 || cmd == nil {
		t.Fatalf("wheel over commits should select the next commit, got %d", m.commit)
	}
	if m, _ = wheel(t, m, 5, m.panesHeight()-m.commitsHeight()+3, tea.MouseWheelLeft); m.commit != 1 {
		t.Fatal("horizontal wheel should be ignored")
	}

	l, _ := listModel(t)
	l, _ = press(t, l, "t")
	y := 2 + l.panesHeight()
	for range 5 {
		l, _ = wheel(t, l, 10, y, tea.MouseWheelDown)
	}
	if l.listCursor != len(l.comments)-1 {
		t.Fatalf("wheel should stop at the last comment, got %d", l.listCursor)
	}
	l, _ = wheel(t, l, 10, y, tea.MouseWheelUp)
	if l.listCursor != len(l.comments)-2 {
		t.Fatalf("listCursor=%d", l.listCursor)
	}
}

func TestMouseIgnoredWhileEditingOrHelp(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "?")
	if m, _ = click(t, m, 5, 4); m.fileCursor != 0 {
		t.Fatal("clicks should be ignored on the help screen")
	}
	if m, _ = wheel(t, m, 60, 10, tea.MouseWheelDown); m.cursor != 0 {
		t.Fatal("wheel should be ignored on the help screen")
	}
	m, _ = press(t, m, "esc", "l", "j", "j", "c")
	if !m.editing {
		t.Fatal("editor should open")
	}
	cursor := m.cursor
	if m, _ = click(t, m, 5, 4); !m.editing || m.fileCursor != 0 {
		t.Fatal("clicks should be ignored while editing")
	}
	if m, _ = wheel(t, m, 60, 10, tea.MouseWheelDown); m.cursor != cursor {
		t.Fatal("wheel should be ignored while editing")
	}
}

func TestExplorerToggleHidesCommitsAndFiles(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "space", "e")
	out := screen(m)
	if strings.Contains(out, "╭─ files") || strings.Contains(out, "╭─ commits") || !strings.Contains(out, "╭─ continuous diff") {
		t.Fatalf("e should hide commits and files:\n%s", out)
	}
	if m.focus != focusDiff || m.diffWidth() != 118 {
		t.Fatalf("focus=%d diffWidth=%d", m.focus, m.diffWidth())
	}
	if r := m.render(); lipgloss.Height(r) != 40 || lipgloss.Width(r) != 120 {
		t.Fatalf("view should fill the terminal, got %dx%d", lipgloss.Width(r), lipgloss.Height(r))
	}
	if m, _ = press(t, m, "tab"); m.focus != focusDiff {
		t.Fatalf("tab should stay on the diff, got %d", m.focus)
	}
	m, _ = press(t, m, "t", "tab")
	if m.focus != focusDiff {
		t.Fatalf("tab from comments should skip hidden panes, got %d", m.focus)
	}
	if m, _ = press(t, m, "shift+tab"); m.focus != focusComments {
		t.Fatalf("shift+tab should go back to comments, got %d", m.focus)
	}
	m, _ = press(t, m, "t")
	target := rowOf(t, m, rowLine, 0, 1)
	if m, _ = click(t, m, 5, 2+target-m.offset); m.focus != focusDiff || m.cursor != target {
		t.Fatalf("left clicks should hit the diff, focus=%d cursor=%d", m.focus, m.cursor)
	}

	m, _ = press(t, m, "space", "e")
	if !strings.Contains(screen(m), "╭─ files") || m.focus != focusFiles {
		t.Fatal("e again should bring the panes back and focus the files")
	}
	if out := screen(m); strings.Index(out, "╭─ files") > strings.Index(out, "╭─ commits") {
		t.Fatalf("the commits pane should sit below the files:\n%s", out)
	}
	if m, _ = press(t, m, "shift+tab"); m.focus != focusCommits {
		t.Fatalf("tab should reach commits again, got %d", m.focus)
	}

	d, _ := loaded(t)
	d, _ = press(t, d, "l", "space", "e")
	if d.focus != focusDiff {
		t.Fatal("hiding the explorer should keep focus on the diff")
	}
}

func TestWrapTokensSplitsAtCodeWidth(t *testing.T) {
	red, blue := lipgloss.NewStyle().Foreground(colorDel), lipgloss.NewStyle().Foreground(colorAdd)
	parts := wrapTokens([]token{{text: "abcd", style: red}, {text: "efg", style: blue}}, lineGutter+3)
	want := [][]token{{{text: "abc", style: red}}, {{text: "d", style: red}, {text: "ef", style: blue}}, {{text: "g", style: blue}}}
	if !reflect.DeepEqual(parts, want) {
		t.Fatalf("got %+v", parts)
	}
	if parts := wrapTokens([]token{{text: "日本語", style: red}}, lineGutter+4); len(parts) != 2 || parts[0][0].text != "日本" || parts[1][0].text != "語" {
		t.Fatalf("wide runes should count two cells: %+v", parts)
	}
	if parts := wrapTokens(nil, 80); len(parts) != 1 || parts[0] != nil {
		t.Fatalf("empty line should be one part: %+v", parts)
	}
	if parts := wrapTokens([]token{{text: "ab", style: red}}, 3); len(parts) != 2 {
		t.Fatalf("narrow widths should still wrap one cell per row: %+v", parts)
	}
}

var long = "x := \"" + strings.Repeat("abcdefghij", 30) + "\""

func wrapModel(t *testing.T) Model {
	t.Helper()
	patch := "@@ -1,2 +1,3 @@\n a\n+" + long + "\n+short\n b"
	return newModel(t, fakeSource{files: map[string][]review.File{"": {{Path: "w.go", Status: review.StatusModified, Patch: patch}}}}, &fakeStore{viewed: map[string]string{}})
}

func TestDiffSoftWrapsLongLines(t *testing.T) {
	m := wrapModel(t)
	first := rowOf(t, m, rowLine, 0, 2)
	parts := len(wrapTokens(m.tokens[0][2], m.diffWidth()))
	if parts < 3 {
		t.Fatalf("expected the long line to wrap, got %d parts", parts)
	}
	for p := 1; p < parts; p++ {
		if r := m.rows[first+p]; r.kind != rowText || r.line != 2 || r.part != p {
			t.Fatalf("row %d = %+v", first+p, r)
		}
	}
	out := screen(m)
	joined := ""
	for line := range strings.SplitSeq(out, "\n") {
		if i := strings.Index(line, "│"); i >= 0 && strings.Contains(line, "abcdefghij") {
			code := strings.TrimRight(strings.TrimSuffix(strings.TrimRight(line, " "), "│"), " ")
			joined += strings.TrimLeft(code[strings.LastIndex(code, "│")+len("│"):], " ▌0123456789+")
		}
	}
	if !strings.Contains(joined, strings.Repeat("abcdefghij", 30)) {
		t.Fatalf("wrapped rows should show the whole line:\n%s", out)
	}
	if lipgloss.Width(m.render()) != 120 {
		t.Fatal("wrapped rows should not overflow the view")
	}

	m, _ = press(t, m, "l", "j", "j", "j", "j")
	if m.cursor != first+parts {
		t.Fatalf("j should skip wrapped rows, cursor=%d want %d", m.cursor, first+parts)
	}
	m, _ = press(t, m, "k")
	if m.cursor != first {
		t.Fatalf("k should land on the wrapped line, cursor=%d", m.cursor)
	}
	cursorRows := 0
	for _, l := range m.diffLines()[first-m.offset : first-m.offset+parts] {
		if strings.Contains(ansi.Strip(l), "▌") {
			cursorRows++
		}
	}
	if cursorRows != parts {
		t.Fatalf("every row of the cursor line should be highlighted, got %d of %d", cursorRows, parts)
	}

	m, _ = press(t, m, "j")
	m, _ = click(t, m, 60, 2+first+1-m.offset)
	if m.cursor != first {
		t.Fatalf("clicking a wrapped row should select its line, cursor=%d", m.cursor)
	}

	wide := update(t, m, tea.WindowSizeMsg{Width: 400, Height: 40})
	if wide.cursor != rowOf(t, wide, rowLine, 0, 2) || wide.rows[wide.cursor+1].part != 0 {
		t.Fatal("a wide terminal should fit the line on one row and keep the cursor on it")
	}
}

func TestWrappedLineStaysVisibleAtBottom(t *testing.T) {
	m := wrapModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 11})
	m, _ = press(t, m, "l", "j", "j", "j")
	first := rowOf(t, m, rowLine, 0, 2)
	parts := len(wrapTokens(m.tokens[0][2], m.diffWidth()))
	if m.cursor != first || first+parts-1 >= m.offset+m.diffHeight() || m.cursor < m.offset {
		t.Fatalf("cursor=%d offset=%d height=%d parts=%d", m.cursor, m.offset, m.diffHeight(), parts)
	}
}

func TestEditorRevealIgnoresWrappedRows(t *testing.T) {
	m := wrapModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 16})
	m, _ = press(t, m, "l", "j", "j", "c")
	spacer := -1
	for i, r := range m.rows {
		if r.kind == rowText && r.text == "" && r.part == 0 {
			spacer = i
		}
	}
	if !m.editing || spacer < 0 || spacer >= m.offset+m.diffHeight() {
		t.Fatalf("editor should be revealed: spacer=%d offset=%d height=%d", spacer, m.offset, m.diffHeight())
	}
	if spacer > rowOf(t, m, rowLine, 0, 2) {
		t.Fatal("the editor spacer should come before the wrapped line")
	}
}

func TestTallWrappedLineKeepsCursorVisible(t *testing.T) {
	m := wrapModel(t)
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 9})
	m, _ = press(t, m, "l", "j", "j", "j")
	if m.cursor != m.offset {
		t.Fatalf("a line taller than the pane should start at the top, cursor=%d offset=%d", m.cursor, m.offset)
	}
}

func TestExplorerShowsLineCounts(t *testing.T) {
	m, _ := loaded(t)
	out := screen(m)
	for _, want := range []string{"files ──", "+4 -1 · 1/3", "A README.md              +2 -0", "M main.go              +2 -1", "M img.png                +0 -0"} {
		if !strings.Contains(out, want) {
			t.Errorf("explorer missing %q:\n%s", want, out)
		}
	}
	m, cmd := press(t, m, "-")
	m = run(t, m, cmd)
	if !strings.Contains(screen(m), "A README.md              +2 -0 ") {
		t.Fatalf("viewed mark should follow the counts:\n%s", screen(m))
	}

	name := strings.Repeat("n", 60) + ".go"
	l := newModel(t, fakeSource{files: map[string][]review.File{"": {{Path: name, Status: review.StatusAdded, Additions: 12, Deletions: 3}}}}, &fakeStore{viewed: map[string]string{}})
	out = screen(l)
	row := out[strings.Index(out, "│▌A nnn"):]
	row = row[:strings.Index(row, "\n")]
	if !strings.Contains(row, "n +12 -3") || !strings.Contains(out, "+12 -3 · 1/1") {
		t.Fatalf("long names should be cut before the counts:\n%s", out)
	}
}

func TestLeavingSuggestionRemovesOnlyTheBlock(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "c")
	for _, tc := range []struct{ body, want string }{
		{"keep\n\n```suggestion\nfunc x() {}\n```\nafter", "keep\nafter"},
		{"```suggestion\nfunc x() {}", ""},
		{"no block here", "no block here"},
	} {
		m.editor.kind = slices.Index(commentKinds, "suggestion")
		m.editor.area.SetValue(tc.body)
		m, _ = press(t, m, "tab")
		if got := m.editor.area.Value(); got != tc.want || commentKinds[m.editor.kind] != "comment" {
			t.Errorf("%q: got %q kind %s, want %q", tc.body, got, commentKinds[m.editor.kind], tc.want)
		}
	}
}

func TestIssueCommentKeepsItsType(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "c")
	m = typeText(t, m, "This panics on nil.")
	m, _ = press(t, m, "tab", "tab")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)
	if len(st.comments) != 1 || st.comments[0].Kind != "issue" {
		t.Fatalf("stored %+v, want an issue comment", st.comments)
	}
	if !strings.Contains(screen(m), " issue") {
		t.Fatalf("issue comment should show its type label:\n%s", screen(m))
	}
}

func TestCommentKindsHaveTheirOwnColors(t *testing.T) {
	seen := map[color.Color]bool{}
	for _, k := range commentKinds {
		seen[kindColor(k)] = true
	}
	if len(seen) != len(commentKinds) || kindColor("question") != colorPrimary {
		t.Fatal("each kind needs its own color, with old kinds falling back to the accent")
	}
	prefix := func(c color.Color) string {
		p, _, _ := strings.Cut(lipgloss.NewStyle().Foreground(c).Render("x"), "x")
		return p
	}
	if out := renderComment(review.Comment{Kind: "note", Start: 1, End: 1, Body: "hi"}, nil, 80); !strings.Contains(out[0], prefix(colorNote)+"╭") || !strings.Contains(out[0], prefix(colorNote)+" note") {
		t.Fatalf("note box should use the note color: %q", out[0])
	}
	if out := renderComment(review.Comment{Kind: "issue", Start: 1, End: 1, Body: "hi"}, nil, 80); !strings.Contains(out[0], prefix(colorIssue)+" issue") {
		t.Fatalf("issue box should use the red issue color and label: %q", out[0])
	}

	m, _ := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "c")
	for _, k := range []string{"comment", "note", "issue", "suggestion"} {
		box := strings.Join(m.diffLines(), "\n")
		if commentKinds[m.editor.kind] != k || !strings.Contains(box, prefix(kindColor(k))+"╭") || !strings.Contains(box, " "+k) {
			t.Fatalf("editor border should follow the %s type", k)
		}
		m, _ = press(t, m, "tab")
	}

	l, _ := listModel(t)
	l, _ = press(t, l, "t")
	if !strings.Contains(strings.Join(l.listLines(), ""), prefix(colorAdd)) {
		t.Fatal("comments pane should color suggestions")
	}
}

func TestCommentsOnDeletedFilesArePruned(t *testing.T) {
	st := &fakeStore{viewed: map[string]string{}, comments: []review.Comment{
		{ID: "a", Path: "README.md", Body: "keep"},
		{ID: "b", Path: "gone.go", Body: "drop"},
	}}
	svc := review.NewService(sample, st, nil, "feature")
	svc.Exists = func(path string) bool { return path != "gone.go" }
	m := New(svc, "", "main")
	m = update(t, m, m.Init()())
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	if len(m.comments) != 1 || m.comments[0].ID != "a" || !strings.Contains(screen(m), "removed 1 comments on deleted files") {
		t.Fatalf("load should prune: %+v\n%s", m.comments, screen(m))
	}

	st.comments = append(st.comments, review.Comment{ID: "c", Path: "gone.go", Body: "late"})
	m = update(t, m, m.poll()())
	if len(m.comments) != 1 || !strings.Contains(screen(m), "removed 1 comments") {
		t.Fatalf("poll should prune: %+v", m.comments)
	}

	st.err = errors.New("disk full")
	st.comments = append(st.comments, review.Comment{ID: "d", Path: "gone.go", Body: "stuck"})
	if msg := m.poll()().(polledMsg); msg.err == nil {
		t.Fatal("prune errors should fail the poll")
	}
	if msg := m.Init()().(loadedMsg); msg.err == nil {
		t.Fatal("prune errors should fail the load")
	}
}

func TestRefreshReusesHighlightingForUnchangedFiles(t *testing.T) {
	m, _ := loaded(t)
	marker := []token{{text: "cached", style: lipgloss.NewStyle()}}
	for i := range m.tokens[0] {
		m.tokens[0][i] = marker
	}
	files := slices.Clone(m.files)
	files[1].Patch = "@@ -1 +1 @@\n-x\n+changed"
	m.refreshFiles(m.parsed().parse(files))
	if m.tokens[0][1][0].text != "cached" {
		t.Fatal("unchanged file should keep its highlighting")
	}
	if got := m.tokens[1][2]; len(got) == 0 || !strings.Contains(got[0].text+got[len(got)-1].text, "changed") {
		t.Fatalf("changed file should be highlighted again: %+v", got)
	}
	if m.lines[1][2].Text != "changed" {
		t.Fatalf("changed file should be parsed again: %+v", m.lines[1])
	}

	renamed := slices.Clone(files)
	renamed[0].Path = "OTHER.md"
	m.setFiles(m.parsed().parse(renamed))
	if m.tokens[0][1][0].text == "cached" {
		t.Fatal("a file with a different path should be highlighted again")
	}
}

func TestWrapTokensKeepsShortLinesWhole(t *testing.T) {
	short := []token{{text: "ab"}, {text: "cd"}}
	if parts := wrapTokens(short, lineGutter+4); len(parts) != 1 || len(parts[0]) != 2 {
		t.Fatalf("a line that fits should stay one part: %+v", parts)
	}
	if parts := wrapTokens([]token{{text: "日本語"}}, lineGutter+6); len(parts) != 1 {
		t.Fatalf("wide runes with more bytes than cells should still fit: %+v", parts)
	}
}

func TestReviewedFilesMoveToReviewSection(t *testing.T) {
	m, _ := loaded(t)
	if strings.Contains(screen(m), "Reviewed") {
		t.Fatal("review section should only show once a file is reviewed")
	}
	m.viewed = map[string]bool{"a/main.go": true}
	lines, files := m.fileLines()
	var text []string
	for _, l := range lines {
		text = append(text, strings.TrimRight(ansi.Strip(l), " "))
	}
	want := []string{"▌A README.md              +2 -0", " M img.png                +0 -0", "", " Reviewed", " M a/main.go              +2 -1 "}
	if !reflect.DeepEqual(text, want) {
		t.Fatalf("explorer = %q", text)
	}
	if !reflect.DeepEqual(files, []int{0, 2, -1, -1, 1}) {
		t.Fatalf("row files = %v", files)
	}

	if m.focus != focusFiles {
		t.Fatalf("focus=%d", m.focus)
	}
	var order []int
	for range 4 {
		order = append(order, m.fileCursor)
		m, _ = press(t, m, "j")
	}
	if !reflect.DeepEqual(order, []int{0, 2, 1, 1}) {
		t.Fatalf("j should follow the explorer order, got %v", order)
	}
	m, _ = press(t, m, "k")
	if m.fileCursor != 2 {
		t.Fatalf("k should go back up the explorer, got %d", m.fileCursor)
	}
	m, _ = wheel(t, m, 5, 5, tea.MouseWheelDown)
	if m.fileCursor != 1 {
		t.Fatalf("wheel should follow the explorer order, got %d", m.fileCursor)
	}
	m, _ = click(t, m, 5, 2)
	if m.fileCursor != 0 {
		t.Fatalf("click on the first row = %d", m.fileCursor)
	}
	m, _ = click(t, m, 5, 5)
	if m.fileCursor != 0 {
		t.Fatal("clicking the review header should not select a file")
	}
	m, _ = click(t, m, 5, 6)
	if m.fileCursor != 1 {
		t.Fatalf("clicking a reviewed file should select it, got %d", m.fileCursor)
	}
}

func TestMarkingReviewedInExplorerSelectsNextFile(t *testing.T) {
	m, st := loaded(t)
	m, cmd := press(t, m, "-")
	if m.fileCursor != 1 {
		t.Fatalf("should move to the next file in the tree, got %d", m.fileCursor)
	}
	m = run(t, m, cmd)
	if st.viewed["README.md"] == "" || m.fileCursor != 1 {
		t.Fatalf("README.md should be reviewed and a/main.go selected: %v %d", st.viewed, m.fileCursor)
	}
	m, _ = press(t, m, "j")
	m, cmd = press(t, m, "-")
	m = run(t, m, cmd)
	if m.fileCursor != 1 {
		t.Fatalf("the last file in the tree should fall back to the one above, got %d", m.fileCursor)
	}
	m, cmd = press(t, m, "-")
	m = run(t, m, cmd)
	if m.fileCursor != 1 || len(st.viewed) != 3 {
		t.Fatalf("with nothing left to review the selection should stay, got %d", m.fileCursor)
	}
	m, cmd = press(t, m, "-")
	m = run(t, m, cmd)
	if m.fileCursor != 1 || st.viewed["a/main.go"] != "" {
		t.Fatalf("unmarking should keep the selection, got %d", m.fileCursor)
	}

	d, _ := loaded(t)
	d, _ = press(t, d, "l", "j")
	before := d.cursor
	if d, _ = press(t, d, "-"); d.cursor != before || d.fileCursor != 0 {
		t.Fatal("marking from the diff should not move the selection")
	}
}

func TestScrollingTheDiffFocusesIt(t *testing.T) {
	m, _ := loaded(t)
	if m.focus != focusFiles {
		t.Fatalf("focus=%d", m.focus)
	}
	m, _ = wheel(t, m, 60, 10, tea.MouseWheelDown)
	if m.focus != focusDiff || m.cursor != 0 {
		t.Fatalf("wheel over the diff should focus it without moving the cursor, focus=%d cursor=%d", m.focus, m.cursor)
	}
	if !strings.Contains(strings.Join(m.diffLines(), "\n"), "▌") {
		t.Fatal("the cursor should be visible once the diff has focus")
	}
	m, _ = wheel(t, m, 5, 5, tea.MouseWheelDown)
	if m.focus != focusFiles {
		t.Fatalf("wheel over the files should focus them, got %d", m.focus)
	}
	m, _ = wheel(t, m, 5, m.panesHeight()-m.commitsHeight()+3, tea.MouseWheelDown)
	if m.focus != focusCommits {
		t.Fatalf("wheel over the commits should focus them, got %d", m.focus)
	}
	if m, _ = wheel(t, m, 60, 0, tea.MouseWheelDown); m.focus != focusCommits {
		t.Fatal("wheel over the tab bar should do nothing")
	}
	if m, _ = wheel(t, m, 60, 38, tea.MouseWheelDown); m.focus != focusCommits {
		t.Fatal("wheel over the status line should do nothing while the comments pane is hidden")
	}

	for _, k := range []string{"ctrl+d", "pgdown"} {
		p, _ := loaded(t)
		p, _ = press(t, p, k)
		if p.focus != focusDiff || p.cursor == 0 {
			t.Errorf("%s from the files pane should focus and scroll the diff, focus=%d cursor=%d", k, p.focus, p.cursor)
		}
	}
	p, _ := loaded(t)
	p, _ = press(t, p, "ctrl+d", "ctrl+u")
	if p.focus != focusDiff || p.cursor != 0 {
		t.Fatalf("ctrl+u should scroll back up, cursor=%d", p.cursor)
	}
	p, _ = press(t, p, "t", "pgup")
	if p.focus != focusDiff {
		t.Fatalf("page keys from the comments pane should focus the diff, got %d", p.focus)
	}
}

func TestSearchFindsMatchesAcrossFiles(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "/")
	if !m.searching || !strings.Contains(screen(m), "SEARCH") || !strings.Contains(screen(m), "confirm") {
		t.Fatalf("/ should open the search prompt:\n%s", screen(m))
	}
	m = typeText(t, m, "func")
	first := rowOf(t, m, rowLine, 1, 2)
	if len(m.matches) != 3 || m.cursor != first || m.focus != focusDiff {
		t.Fatalf("matches=%v cursor=%d want %d", m.matches, m.cursor, first)
	}
	if out := screen(m); !strings.Contains(out, "/func▌  1/3") {
		t.Fatalf("status should show the query and position:\n%s", out)
	}
	var positions []int
	for _, k := range []string{"ctrl+n", "ctrl+n", "ctrl+n", "ctrl+p"} {
		m, _ = press(t, m, k)
		positions = append(positions, m.matchPosition())
	}
	if !reflect.DeepEqual(positions, []int{2, 3, 1, 3}) {
		t.Fatalf("ctrl+n and ctrl+p should step and wrap, got %v", positions)
	}

	m, _ = press(t, m, "enter")
	if m.searching || !strings.Contains(screen(m), "/func  3/3") {
		t.Fatalf("enter should keep the results:\n%s", screen(m))
	}
	m, _ = press(t, m, "ctrl+n")
	if m.matchPosition() != 1 {
		t.Fatalf("ctrl+n should work after confirming, got %d", m.matchPosition())
	}
	if m, _ = press(t, m, "k"); m.matchPosition() != 0 || !strings.Contains(screen(m), "0/3") {
		t.Fatal("moving above the first match should count no matches")
	}
	m, _ = press(t, m, "esc")
	if m.query != "" || m.matches != nil || strings.Contains(screen(m), "/func") {
		t.Fatal("esc should clear the search")
	}
}

func TestSearchEditingAndSmartCase(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "FUNC")
	if len(m.matches) != 0 || !strings.Contains(screen(m), "no matches") {
		t.Fatalf("an uppercase query should match case: %v", m.matches)
	}
	for range 4 {
		m, _ = press(t, m, "backspace")
	}
	m = typeText(t, m, "Title")
	if len(m.matches) != 1 || m.cursor != rowOf(t, m, rowLine, 0, 1) {
		t.Fatalf("matches=%v cursor=%d", m.matches, m.cursor)
	}
	m, _ = press(t, m, "backspace", "backspace", "backspace", "backspace", "backspace", "backspace")
	if m.query != "" || len(m.matches) != 0 || !strings.Contains(screen(m), "/▌  0/0") {
		t.Fatalf("backspace past the start should leave an empty query: %q", m.query)
	}
	if m, _ = press(t, m, "up"); !m.searching || m.query != "" {
		t.Fatal("keys without text should be ignored while searching")
	}
	m = typeText(t, m, "text")
	m, _ = press(t, m, "esc")
	if m.searching || m.query != "" || len(m.matches) != 0 {
		t.Fatal("esc should cancel the search")
	}
	if _, cmd := press(t, m, "/", "ctrl+c"); cmd == nil {
		t.Fatal("ctrl+c should still quit while searching")
	}
}

func TestSearchSkipsReviewedFilesAndFollowsLayout(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "func")
	m, _ = press(t, m, "enter", "-")
	m = update(t, m, viewedMsg{viewed: map[string]bool{"a/main.go": true}})
	if len(m.matches) != 0 || !strings.Contains(screen(m), "no matches") {
		t.Fatalf("collapsed files should not be searched: %v", m.matches)
	}
	if m, _ = press(t, m, "ctrl+n"); m.matchPosition() != 0 {
		t.Fatal("ctrl+n with no matches should do nothing")
	}
	m = update(t, m, viewedMsg{viewed: map[string]bool{}})
	if len(m.matches) != 3 {
		t.Fatalf("matches should come back when the file expands: %v", m.matches)
	}
}

func TestEscClearsRangeBeforeSearch(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "func")
	m, _ = press(t, m, "enter", "v", "esc")
	if m.anchor >= 0 || m.query != "func" {
		t.Fatal("first esc should only stop the range")
	}
	if m, _ = press(t, m, "esc"); m.query != "" {
		t.Fatal("second esc should clear the search")
	}
}

func TestMatchesAreHighlighted(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "old")
	hl := "48;2;235;175;57"
	found := false
	for _, l := range m.diffLines() {
		if strings.Contains(ansi.Strip(l), "func old") && strings.Contains(l, hl) {
			found = true
		}
	}
	if !found {
		t.Fatal("the matching text should be highlighted")
	}
}

func TestMarkMatchesSplitsTokens(t *testing.T) {
	a, b := lipgloss.NewStyle().Foreground(colorAdd), lipgloss.NewStyle().Foreground(colorDel)
	out := markMatches([]token{{text: "foo(", style: a}, {text: "bar) foo", style: b}}, "o(b")
	var texts []string
	var hits []bool
	for _, t := range out {
		texts = append(texts, t.text)
		hits = append(hits, t.style.GetBackground() != nil && t.style.GetBackground() != lipgloss.NoColor{})
	}
	if !reflect.DeepEqual(texts, []string{"fo", "o(", "b", "ar) foo"}) || !reflect.DeepEqual(hits, []bool{false, true, true, false}) {
		t.Fatalf("texts=%q hits=%v", texts, hits)
	}
	if got := markMatches([]token{{text: "aXa aXa"}}, "x"); len(got) != 5 {
		t.Fatalf("every match should be marked: %+v", got)
	}
	tokens := []token{{text: "İstanbul"}}
	if got := markMatches(tokens, "stan"); !reflect.DeepEqual(got, tokens) {
		t.Fatal("lines whose lowercase changes length should be left alone")
	}
	if got := markMatches(tokens, ""); !reflect.DeepEqual(got, tokens) {
		t.Fatal("an empty query should change nothing")
	}
}

func TestEditorCommandPrefersVisualThenEditor(t *testing.T) {
	envOf := func(vars map[string]string) func(string) string {
		return func(k string) string { return vars[k] }
	}
	for _, tc := range []struct {
		vars map[string]string
		want []string
	}{
		{map[string]string{"VISUAL": "nvim -u NONE", "EDITOR": "vim"}, []string{"nvim", "-u", "NONE", "+7", "/r/a.go"}},
		{map[string]string{"VISUAL": "  ", "EDITOR": "vim"}, []string{"vim", "+7", "/r/a.go"}},
		{map[string]string{}, []string{"nvim", "+7", "/r/a.go"}},
	} {
		if got := editorCommand(envOf(tc.vars), "/r/a.go", 7).Args; !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%v: got %v, want %v", tc.vars, got, tc.want)
		}
	}
}

func TestCurrentLinePicksTheWorkingTreeLine(t *testing.T) {
	m, _ := loaded(t)
	if m.currentLine() != 1 {
		t.Fatal("the files pane should open at the top")
	}
	m, _ = press(t, m, "l")
	for _, tc := range []struct {
		kind             rowKind
		file, line, want int
	}{
		{rowFile, 1, 0, 1},
		{rowHunk, 1, 0, 1},
		{rowLine, 1, 2, 2},
		{rowLine, 1, 3, 2},
		{rowLine, 1, 5, 4},
	} {
		m.cursor = rowOf(t, m, tc.kind, tc.file, tc.line)
		if got := m.currentLine(); got != tc.want {
			t.Errorf("row %+v: line %d, want %d", tc, got, tc.want)
		}
	}

	d := newModel(t, fakeSource{files: map[string][]review.File{"": {
		{Path: "gone.go", Status: review.StatusRemoved, Patch: "@@ -1,2 +0,0 @@\n-a\n-b"},
		{Path: "tail.go", Status: review.StatusModified, Patch: "@@ -1,2 +1,1 @@\n keep\n-drop"},
	}}}, &fakeStore{viewed: map[string]string{}})
	d, _ = press(t, d, "l")
	if d.cursor = rowOf(t, d, rowLine, 0, 2); d.currentLine() != 1 {
		t.Fatal("a deleted file should open at line 1")
	}
	if d.cursor = rowOf(t, d, rowLine, 1, 2); d.currentLine() != 1 {
		t.Fatalf("a removed last line should use the line above, got %d", d.currentLine())
	}
}

func TestOpenFileWithGF(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "a"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a", "main.go"), []byte("package main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := &fakeStore{viewed: map[string]string{}}
	m := New(review.NewService(sample, st, nil, "feature"), root, "main")
	m = update(t, m, m.Init()())
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})

	if m, cmd := press(t, m, "g", "f"); cmd != nil || !strings.Contains(m.warning, "README.md is not in the working tree") {
		t.Fatalf("a missing file should not open: %q", m.notice)
	}
	m, _ = press(t, m, "j")
	if _, cmd := press(t, m, "g", "f"); cmd == nil {
		t.Fatal("gf on a file in the working tree should open the editor")
	}

	m = update(t, m, editedMsg{path: "a/main.go"})
	if m.notice != "edited a/main.go" || m.err != nil {
		t.Fatalf("notice=%q err=%v", m.notice, m.err)
	}
	if _, cmd := m.Update(editedMsg{path: "a/main.go"}); cmd == nil {
		t.Fatal("returning from the editor should refresh right away")
	}
	m = update(t, m, editedMsg{path: "a/main.go", err: errors.New("exit status 1")})
	if m.notice != "" || m.err == nil || !strings.Contains(screen(m), "exit status 1") {
		t.Fatal("an editor failure should be shown")
	}

	empty := newModel(t, fakeSource{}, &fakeStore{viewed: map[string]string{}})
	if _, cmd := press(t, empty, "g", "f"); cmd != nil {
		t.Fatal("gf with no files should do nothing")
	}
}

func TestNAndShiftNStepThroughMatches(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "/")
	m = typeText(t, m, "fun")
	m, _ = press(t, m, "n")
	if m.query != "funn" || !m.searching {
		t.Fatalf("n should type while the prompt is open, query=%q", m.query)
	}
	m, _ = press(t, m, "backspace", "enter")
	var positions []int
	for _, k := range []string{"n", "n", "n", "N", "N"} {
		m, _ = press(t, m, k)
		positions = append(positions, m.matchPosition())
	}
	if !reflect.DeepEqual(positions, []int{2, 3, 1, 3, 2}) {
		t.Fatalf("n and N should step and wrap like vim, got %v", positions)
	}
	if m.showList {
		t.Fatal("n should no longer open the comments pane")
	}
	if m, _ = press(t, m, "t"); !m.showList || m.focus != focusComments {
		t.Fatal("t should open the comments pane")
	}
}

func TestExplorerSelectionFollowsTheDiffCursor(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "l")
	selected := func(m Model) string {
		lines, _ := m.fileLines()
		for _, l := range lines {
			if strings.HasPrefix(ansi.Strip(l), "▌") {
				return strings.TrimSpace(ansi.Strip(l))
			}
		}
		return ""
	}
	for _, tc := range []struct {
		file int
		want string
	}{{0, "▌A README.md"}, {1, "▌  M main.go"}, {2, "▌M img.png"}} {
		for m.rows[m.cursor].file != tc.file {
			m, _ = press(t, m, "j")
		}
		if got := selected(m); !strings.HasPrefix(got, tc.want) {
			t.Fatalf("cursor in file %d: explorer selection %q, want %q", tc.file, got, tc.want)
		}
	}
	accent, _, _ := strings.Cut(lipgloss.NewStyle().Foreground(colorPrimary).Render("x"), "x")
	lines, _ := m.fileLines()
	if !strings.Contains(strings.Join(lines, ""), accent) {
		t.Fatal("the selected file should use the accent color even when the diff has focus")
	}
}

func TestChangeKeysVisitEachBlockOfChanges(t *testing.T) {
	patch := "@@ -1,8 +1,8 @@\n a\n-b\n+B\n c\n d\n+e\n f\n-g\n h\n@@ -20,2 +20,2 @@\n x\n-y\n+Y"
	m := newModel(t, fakeSource{files: map[string][]review.File{"": {
		{Path: "one.go", Status: review.StatusModified, Patch: patch},
		{Path: "two.go", Status: review.StatusModified, Patch: "@@ -1 +1 @@\n-old\n+new"},
	}}}, &fakeStore{viewed: map[string]string{}})
	want := []int{rowOf(t, m, rowLine, 0, 2), rowOf(t, m, rowLine, 0, 6), rowOf(t, m, rowLine, 0, 8), rowOf(t, m, rowLine, 0, 12), rowOf(t, m, rowLine, 1, 1)}
	var got []int
	for range want {
		m, _ = press(t, m, "h")
		got = append(got, m.cursor)
	}
	if !reflect.DeepEqual(got, want) || m.focus != focusDiff {
		t.Fatalf("h visited %v, want %v (focus %d)", got, want, m.focus)
	}
	if m, _ = press(t, m, "h"); m.cursor != want[4] {
		t.Fatal("h on the last change should stay")
	}
	m, _ = press(t, m, "H", "H")
	if m.cursor != want[2] {
		t.Fatalf("H should go back one block at a time, got %d", m.cursor)
	}
	m.cursor = rowOf(t, m, rowLine, 0, 3)
	if m, _ = press(t, m, "H"); m.cursor != want[0] {
		t.Fatalf("H inside a block should go to its start, got %d", m.cursor)
	}
}

func TestCommentEditorUsesVimModes(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "c")
	if m.editor.normal || !strings.Contains(screen(m), "INSERT") || !strings.Contains(screen(m), "esc jj normal") {
		t.Fatal("the comment editor should start in insert mode")
	}
	m = typeText(t, m, "helo world")
	m = typeText(t, m, "jj")
	if !m.editor.normal || m.editor.area.Value() != "helo world" {
		t.Fatalf("jj should switch to normal mode without typing, value=%q", m.editor.area.Value())
	}
	m = typeText(t, m, "0llil")
	m, _ = press(t, m, "esc")
	m = typeText(t, m, "Ax")
	if m.editor.area.Value() != "hello worldx" {
		t.Fatalf("i, l, 0, A, and esc should edit like vim, value=%q", m.editor.area.Value())
	}
	m, cmd := press(t, m, "esc", "ctrl+s")
	m = run(t, m, cmd)
	if len(st.comments) != 1 || st.comments[0].Body != "hello worldx" {
		t.Fatalf("normal mode edits should save, comments=%+v", st.comments)
	}
}

func TestMarkdownViewHighlightsAndDrawsTheCursor(t *testing.T) {
	a := textarea.New()
	a.Prompt, a.ShowLineNumbers = "", false
	a.SetWidth(30)
	a.Placeholder = "write here"
	if got := markdownView(a, false); !strings.Contains(ansi.Strip(got), "write here") {
		t.Fatalf("an empty box should show its placeholder: %q", got)
	}
	a.Focus()
	a.SetValue("# Title\n- **item**")
	got := markdownView(a, false)
	if plain := ansi.Strip(got); !strings.Contains(plain, "# Title") || !strings.Contains(plain, "- **item**") {
		t.Fatalf("the text should stay the same: %q", plain)
	}
	lines := strings.Split(got, "\n")
	if want := lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Render("# Title"); !strings.HasPrefix(lines[0], strings.TrimSuffix(want, "\x1b[m")) {
		t.Fatalf("the heading should be bold and primary: %q", lines[0])
	}
	if i := strings.Index(lines[1], "\x1b[7"); i < 0 || !strings.HasSuffix(ansi.Strip(lines[1][:i]), "**item**") {
		t.Fatalf("the cursor should show after the last character: %q", lines[1])
	}
	a.MoveToBegin()
	if got := strings.Split(markdownView(a, false), "\n")[0]; !strings.HasPrefix(got, lipgloss.NewStyle().Bold(true).Foreground(colorPrimary).Reverse(true).Render("#")) {
		t.Fatalf("the cursor should reverse the character under it: %q", got)
	}
	if lines := strings.Split(markdownView(a, false), "\n"); strings.Contains(lines[1], "\x1b[7") {
		t.Fatalf("moving the cursor should clear it from the line it left: %q", lines[1])
	}
	a.SetValue("plain\n- **item**")
	a.MoveToBegin()
	if plain := ansi.Strip(markdownView(a, false)); !strings.HasPrefix(plain, "plain") || strings.Contains(plain, "# Title") {
		t.Fatalf("an edit should show the new text, not the cached one: %q", plain)
	}
	a.Blur()
	if got := markdownView(a, false); strings.Contains(got, "\x1b[7m") {
		t.Fatal("a blurred box should not draw a cursor")
	}
}

func TestOnlyEscClosesTheCommentEditor(t *testing.T) {
	m, st := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "c")
	m = typeText(t, m, "q")
	if !m.editing || m.editor.area.Value() != "q" {
		t.Fatal("q in insert mode should type")
	}
	if m, _ = press(t, m, "esc", "q"); !m.editing {
		t.Fatal("q in normal mode should not close the editor")
	}
	if m, _ = press(t, m, "esc"); m.editing || len(st.comments) != 0 {
		t.Fatal("esc in normal mode should close the editor without saving")
	}
	if _, cmd := press(t, m, "q"); cmd == nil {
		t.Fatal("q outside the editor should still quit")
	} else if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("q outside the editor should still quit")
	}
}

func TestCtrlWMovesBetweenPanes(t *testing.T) {
	m, _ := loaded(t)
	steps := []struct {
		key  string
		want focusArea
	}{
		{"k", focusFiles},
		{"h", focusFiles},
		{"j", focusCommits},
		{"j", focusCommits},
		{"k", focusFiles},
		{"l", focusDiff},
		{"l", focusDiff},
		{"j", focusDiff},
		{"h", focusFiles},
	}
	for i, s := range steps {
		if m, _ = press(t, m, "ctrl+w", s.key); m.focus != s.want {
			t.Fatalf("step %d ctrl+w %s: focus=%d want %d", i, s.key, m.focus, s.want)
		}
	}
	m, _ = press(t, m, "t")
	for i, s := range []struct {
		key  string
		want focusArea
	}{{"k", focusDiff}, {"j", focusComments}, {"h", focusFiles}, {"j", focusCommits}, {"j", focusComments}, {"k", focusDiff}} {
		if m, _ = press(t, m, "ctrl+w", s.key); m.focus != s.want {
			t.Fatalf("with comments, step %d ctrl+w %s: focus=%d want %d", i, s.key, m.focus, s.want)
		}
	}
	m, _ = press(t, m, "space", "e")
	if m, _ = press(t, m, "ctrl+w", "h"); m.focus != focusDiff {
		t.Fatal("ctrl+w h should stay on the diff when the left panes are hidden")
	}
	before := m.cursor
	if m, _ = press(t, m, "ctrl+w", "j"); m.focus != focusComments || m.cursor != before {
		t.Fatal("the key after ctrl+w should only move focus, not the cursor")
	}
	m, _ = press(t, m, "ctrl+w", "k")
	if m, _ = press(t, m, "j"); m.cursor == before {
		t.Fatal("j alone should still move the cursor")
	}
}

func TestCtrlWDeletesAWordWhileTyping(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "c")
	m = typeText(t, m, "two words")
	if m, _ = press(t, m, "ctrl+w"); !m.editing || m.editor.area.Value() != "two " {
		t.Fatalf("ctrl+w should delete a word in the editor, value=%q", m.editor.area.Value())
	}
}

func TestAgentFooterShowsOnlyTheEssentialKeys(t *testing.T) {
	m, _ := loaded(t)
	if got := ansi.Strip(m.footer()); got != "c comment  v range  t comments  S send  / search  ? help" {
		t.Fatalf("footer = %q", got)
	}
}

func TestInsertModeShowsABarCursor(t *testing.T) {
	m, _ := pullsModel(t)
	m = runAction(t, m, "comment")
	m = typeText(t, m, "hello")
	v := m.View()
	if v.Cursor == nil || v.Cursor.Shape != tea.CursorBar || strings.ContainsFunc(v.Content, func(r rune) bool { return r >= cursorBase }) {
		t.Fatalf("insert mode should show a bar cursor without the marker, cursor=%+v", v.Cursor)
	}
	lines := strings.Split(ansi.Strip(v.Content), "\n")
	if row := lines[v.Cursor.Y]; !strings.HasSuffix(strings.TrimRight(ansi.Truncate(row, v.Cursor.X, ""), " "), "hello") {
		t.Fatalf("the bar should sit right after the typed text, x=%d row=%q", v.Cursor.X, row)
	}
	m = pressRun(t, m, "esc", "0")
	if v := m.View(); v.Cursor != nil || !strings.Contains(v.Content, ";7mh") {
		t.Fatal("normal mode should draw the block cursor and no bar")
	}
	m = pressRun(t, m, "i")
	v = m.View()
	if v.Cursor == nil || !strings.Contains(ansi.Strip(v.Content), "hello") {
		t.Fatalf("the character under the bar should stay visible:\n%s", ansi.Strip(v.Content))
	}
	m = pressRun(t, m, "esc", "esc", "g", "d", "l")
	m.diff.cursor = rowOf(t, *m.diff, rowLine, 1, 3)
	m = pressRun(t, m, "c")
	m = typeText(t, m, "x")
	if v := m.View(); v.Cursor == nil || v.Cursor.Shape != tea.CursorBar {
		t.Fatal("the review diff comment editor should show a bar in insert mode")
	}
	agent, _ := loaded(t)
	if v := agent.View(); v.Cursor != nil {
		t.Fatal("no text box focused, no cursor")
	}
}

func TestBarCursorRestoresTheCharacterUnderIt(t *testing.T) {
	screen, c := barCursor("x\n> a" + mark("b") + "c")
	if screen != "x\n> abc" || c == nil || c.X != 3 || c.Y != 1 || c.Shape != tea.CursorBar {
		t.Fatalf("barCursor = %q, %+v", screen, c)
	}
	if screen, _ := barCursor("ab" + mark("")); screen != "ab " {
		t.Fatalf("an empty cell should restore as a space, got %q", screen)
	}
	if screen, _ := barCursor("ab" + mark("🙂")); screen != "ab " {
		t.Fatalf("a character the mark can't carry should restore as a space, got %q", screen)
	}
	if screen, c := barCursor("plain"); screen != "plain" || c != nil {
		t.Fatal("without a mark there is no cursor")
	}
}

func TestModalFieldsShowABarCursor(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "A")
	v := m.View()
	if v.Cursor == nil || v.Cursor.Shape != tea.CursorBar || !strings.Contains(ansi.Strip(v.Content), "search     type to filter") {
		t.Fatalf("the focused search field should show a bar and keep its placeholder:\n%s", ansi.Strip(v.Content))
	}
	m = typeText(t, m, "mer")
	v = m.View()
	row := ansi.Strip(strings.Split(v.Content, "\n")[v.Cursor.Y])
	if !strings.HasSuffix(ansi.Truncate(row, v.Cursor.X, ""), "mer") {
		t.Fatalf("the bar should sit after the typed text, x=%d row=%q", v.Cursor.X, row)
	}
}

func TestPollUnmarksAViewedFileOnceItChanges(t *testing.T) {
	files := slices.Clone(sample.files[""])
	src := fakeSource{commits: sample.commits, files: map[string][]review.File{"": files}}
	m := update(t, newModel(t, src, &fakeStore{viewed: map[string]string{}}), tea.WindowSizeMsg{Width: 120, Height: 40})
	m, _ = press(t, m, "j")
	m, cmd := press(t, m, "-")
	m = run(t, m, cmd)
	if m = run(t, m, m.poll()); !m.viewed["a/main.go"] {
		t.Fatal("a poll without changes should keep the file viewed")
	}
	files = slices.Clone(files)
	src.files[""] = files
	files[1].Patch = "@@ -1,3 +1,5 @@\n package main\n-func old() {}\n+func a() {}\n+func b() {}\n+func c() {}\n // end"
	m = run(t, m, m.poll())
	if m.viewed["a/main.go"] || !strings.Contains(screen(m), "0/3 viewed") || !strings.Contains(fmt.Sprint(m.lines[1]), "func c() {}") {
		t.Fatalf("a change to a viewed file should unmark and expand it:\n%s", screen(m))
	}
}

func TestCommitsPaneStaysShortAndScrolls(t *testing.T) {
	var commits []review.Commit
	for i := range 40 {
		commits = append(commits, review.Commit{SHA: fmt.Sprintf("%07dabc", i), Subject: fmt.Sprintf("commit %02d", i)})
	}
	src := fakeSource{commits: commits, files: sample.files}
	m := update(t, newModel(t, src, &fakeStore{viewed: map[string]string{}}), tea.WindowSizeMsg{Width: 120, Height: 80})
	if m.commitsHeight() != commitsMax || m.panesHeight()/3 <= commitsMax {
		t.Fatalf("a tall window with many commits should cap the pane at %d rows, got %d", commitsMax, m.commitsHeight())
	}
	m, _ = press(t, m, "tab", "tab")
	for range 20 {
		m, _ = press(t, m, "j")
	}
	if out := screen(m); !strings.Contains(out, "commit 19") || strings.Contains(out, "commit 00") {
		t.Fatalf("moving down the commits should scroll the pane:\n%s", out)
	}
	if few := update(t, newModel(t, sample, &fakeStore{viewed: map[string]string{}}), tea.WindowSizeMsg{Width: 120, Height: 80}); few.commitsHeight() != len(sample.commits)+3 {
		t.Fatal("a few commits should only take the rows they need")
	}
}

func TestLoadShowsThePlainDiffThenColorsIt(t *testing.T) {
	m := New(review.NewService(sample, &fakeStore{viewed: map[string]string{}}, nil, "feature"), "", "main")
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	loaded := m.Init()().(loadedMsg)
	if toks := loaded.diff.tokens[1]; len(toks[1]) != 1 || toks[1][0].text != "package main" {
		t.Fatalf("the first load should skip highlighting, got %+v", toks[1])
	}
	m = update(t, m, loaded)
	colored := parsed{}.parse(m.files)
	stale := parsed{}.parse(slices.Clone(m.files))
	stale.files[0].Patch = "@@ -1 +1 @@\n+other"
	m = update(t, m, colorMsg(stale))
	if len(m.tokens[0][1]) != 1 {
		t.Fatal("colors for a patch that changed since should be dropped")
	}
	m = update(t, m, colorMsg(colored))
	if !reflect.DeepEqual(m.tokens, colored.tokens) || len(m.tokens[1][1]) < 2 {
		t.Fatalf("the colors should replace the plain lines, got %+v", m.tokens[1][1])
	}
	m.refreshFiles(parsed{files: m.files, lines: m.lines, tokens: loaded.diff.tokens})
	if !reflect.DeepEqual(m.tokens, colored.tokens) {
		t.Fatal("a refresh carrying older plain lines for unchanged files should keep the colors")
	}
}

func TestPollFollowsACheckedOutBranch(t *testing.T) {
	m, _ := loaded(t)
	current := "feature"
	m.svc.CurrentBranch = func(context.Context) (string, error) { return current, nil }

	current = "other"
	m = update(t, m, m.poll()())
	out := screen(m)
	if m.svc.Branch() != "other" || !strings.Contains(out, " other") || !strings.Contains(out, "switched to branch other") {
		t.Fatalf("branch = %q, view:\n%s", m.svc.Branch(), out)
	}

	m.svc.CurrentBranch = func(context.Context) (string, error) { return "", errors.New("git broke") }
	m = update(t, m, m.poll()())
	if m.failures != 1 || m.svc.Branch() != "other" {
		t.Fatalf("a failed branch read should fail the poll, failures=%d branch=%q", m.failures, m.svc.Branch())
	}
}

func motion(t *testing.T, m Model, y int) Model {
	t.Helper()
	return update(t, m, tea.MouseMotionMsg{X: 60, Y: y, Button: tea.MouseLeft})
}

func TestDraggingOverTheDiffSelectsLines(t *testing.T) {
	m := longModel(t)
	first, third := rowOf(t, m, rowLine, 0, 1), rowOf(t, m, rowLine, 0, 3)
	m, _ = click(t, m, 60, first+2)
	if m.cursor != first || m.anchor != -1 {
		t.Fatalf("a click should only move the cursor, cursor=%d anchor=%d", m.cursor, m.anchor)
	}
	m = motion(t, m, third+2)
	if m.anchor != first || m.cursor != third || !strings.Contains(screen(m), "VISUAL") {
		t.Fatalf("dragging should start visual mode from the clicked line, anchor=%d cursor=%d:\n%s", m.anchor, m.cursor, screen(m))
	}
	m = motion(t, m, rowOf(t, m, rowHunk, 0, 0)+2)
	if m.cursor != third {
		t.Fatalf("dragging over a hunk header should keep the selection, cursor=%d", m.cursor)
	}
	m = update(t, m, tea.MouseReleaseMsg{X: 60, Y: third + 2, Button: tea.MouseLeft})
	if m = motion(t, m, first+2); m.cursor != third || m.anchor != first {
		t.Fatalf("moving after the release should not change the selection, anchor=%d cursor=%d", m.anchor, m.cursor)
	}
	if target, ok := m.lineTarget(); !ok || target.Start != 1 || target.End != 3 {
		t.Fatalf("a comment should cover the dragged lines, got %+v %v", target, ok)
	}
	if m, _ = click(t, m, 60, first+2); m.anchor != -1 {
		t.Fatal("a new click should clear the selection")
	}
}

func TestDraggingPastThePaneScrollsTheSelection(t *testing.T) {
	m := longModel(t)
	bottom := m.diffHeight() - 1
	m, _ = click(t, m, 60, bottom+2)
	start := m.cursor
	m = motion(t, m, m.panesHeight()+5)
	if m.cursor != start+1 || m.anchor != start || m.offset != 1 {
		t.Fatalf("dragging below the pane should extend down and scroll, cursor=%d anchor=%d offset=%d", m.cursor, m.anchor, m.offset)
	}
	m = motion(t, m, 0)
	if m.cursor != start || m.anchor != start {
		t.Fatalf("dragging above the pane should extend up, cursor=%d", m.cursor)
	}
}

func TestDraggingStaysInTheClickedFile(t *testing.T) {
	m, _ := loaded(t)
	readme := rowOf(t, m, rowLine, 0, 1)
	m, _ = click(t, m, 60, readme+2)
	if m = motion(t, m, rowOf(t, m, rowLine, 1, 2)+2); m.cursor != readme || m.anchor != -1 {
		t.Fatalf("dragging into another file should not select it, cursor=%d anchor=%d", m.cursor, m.anchor)
	}
}

func TestSelectedLinesShareTheVisualColor(t *testing.T) {
	l := review.Line{Kind: review.LineAdded, New: 1, Text: "x"}
	if out := renderLine(l, plainTokens([]review.Line{l})[0], 40, 0, true, true); !strings.Contains(out, "48;2;24;68;58") || strings.Contains(out, "48;2;50;38;24") {
		t.Fatalf("the cursor line inside a selection should use the visual color: %q", out)
	}
	if out := renderLine(l, plainTokens([]review.Line{l})[0], 40, 0, true, false); !strings.Contains(out, "48;2;50;38;24") {
		t.Fatalf("the cursor line outside a selection should keep the cursor color: %q", out)
	}
}

func TestEditSavesTheCommentAndShowsTheChange(t *testing.T) {
	m, st := loaded(t)
	st.comments = []review.Comment{{ID: "a", Kind: "nit", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "rename this"}}
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	if m, _ = press(t, m, "e"); !m.editing || m.editor.area.Value() != "rename this" || !strings.Contains(screen(m), "L2 · edit") || strings.Count(screen(m), "rename this") != 1 {
		t.Fatalf("e should replace the comment with an editor holding its body:\n%s", screen(m))
	}
	m = typeText(t, m, "!")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)
	if c := st.comments[0]; c.Body != "rename this!" || c.Sent || len(st.comments) != 1 {
		t.Fatalf("edit not saved in place: %+v", st.comments)
	}
	if out := screen(m); m.editing || !strings.Contains(out, "rename this!") {
		t.Fatalf("saved edit should show under the comment:\n%s", out)
	}
}

func TestEditRefusesCommentsNotWrittenLocally(t *testing.T) {
	m, st := loaded(t)
	st.comments = []review.Comment{{ID: "a", Author: "octocat", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "nit"}}
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	if m, _ = press(t, m, "e"); m.editing || !strings.Contains(m.warning, "only your own comments can be edited") {
		t.Fatalf("e should refuse a GitHub comment, warning %q", m.warning)
	}
}

func TestEditExplainsWhenNoCommentIsUnderTheCursor(t *testing.T) {
	m, _ := loaded(t)
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	if m, _ = press(t, m, "e"); m.editing || m.warning != "no comment here to edit" {
		t.Fatalf("e without a comment should explain, warning %q", m.warning)
	}
}

func TestEditHintShowsOnlyOverAnOwnComment(t *testing.T) {
	m, st := loaded(t)
	st.comments = []review.Comment{{ID: "a", Path: "a/main.go", Side: review.SideRight, Start: 2, End: 2, Body: "nit"}}
	m = update(t, m, m.Init()())
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	if !strings.Contains(m.footer(), "edit") {
		t.Fatalf("footer should offer edit over the comment: %q", m.footer())
	}
	m.cursor = rowOf(t, m, rowLine, 1, 4)
	if strings.Contains(m.footer(), "edit") {
		t.Fatalf("footer should not offer edit away from the comment: %q", m.footer())
	}
}

func TestLeaderKeyToggleTheExplorer(t *testing.T) {
	m, _ := loaded(t)
	m = m.WithLeader(",")
	if m, _ = press(t, m, ",", "e"); !m.hideTree {
		t.Fatal(", e should hide the explorer")
	}
	if m, _ = press(t, m, "e"); !m.hideTree || m.warning != "no comment here to edit" {
		t.Fatalf("e alone should edit, not toggle the explorer, warning %q", m.warning)
	}
}
