package tui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

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
	m := New(review.NewService(source, st, nil, "feature"), "revunk", "main")
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
	"down":      {Code: tea.KeyDown},
	"up":        {Code: tea.KeyUp},
	"pgdown":    {Code: tea.KeyPgDown},
	"pgup":      {Code: tea.KeyPgUp},
	"ctrl+c":    {Code: 'c', Mod: tea.ModCtrl},
	"ctrl+d":    {Code: 'd', Mod: tea.ModCtrl},
	"ctrl+u":    {Code: 'u', Mod: tea.ModCtrl},
	"ctrl+s":    {Code: 's', Mod: tea.ModCtrl},
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
	return ansi.Strip(m.render())
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

	for _, want := range []string{"Agent", "revunk · local changes on feature", "commits", "all changes", "1111111 add readme", "files", "a/", "M main.go", "continuous diff", "main…feature", "A  README.md", "+2 -0", "unseen", "@@ -1,3 +1,4 @@", "no textual changes", "NORMAL", "⎇ feature", "0 comments · 0/3 viewed"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen missing %q", want)
		}
	}
	if !strings.Contains(out, "2      -  func old() {}") || !strings.Contains(out, "2 +  func a() {}") {
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
	m := New(review.NewService(sample, &fakeStore{viewed: map[string]string{}}, nil, "feature"), "revunk", "main")
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
	m, cmd := press(t, m, "r")
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
	if m.rows[m.cursor].kind != rowHunk || m.rows[m.cursor].file != 0 {
		t.Fatalf("{ did not reach the previous hunk: %+v", m.rows[m.cursor])
	}
	m, _ = press(t, m, "}")
	if m.cursor != rowOf(t, m, rowHunk, 1, 0) {
		t.Fatalf("} did not reach the next hunk")
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
	commentAt := strings.Index(out, "◆ note")
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
	if !strings.Contains(out, "◆ comment") || !strings.Contains(out, "file · draft") || strings.Index(out, "Whole file") > strings.Index(out, "package main") {
		t.Fatalf("file comment not shown below its header:\n%s", out)
	}

	m, _ = press(t, m, "c")
	if !m.editing || !m.editor.target.FileLevel() {
		t.Fatal("c in the diff on a file header should open a file comment")
	}
	m, _ = press(t, m, "esc")
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

	m, _ = press(t, m, "ctrl+s")
	if !m.editing || !strings.Contains(screen(m), "write something") {
		t.Fatal("blank comment should keep the editor open with a notice")
	}
	m.editor.area.SetValue("Use this:")
	m, _ = press(t, m, "tab", "tab")
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
	m, _ = press(t, m, "tab", "tab", "tab")
	if strings.Count(m.editor.area.Value(), "```suggestion") != 1 {
		t.Fatalf("suggestion injected twice: %q", m.editor.area.Value())
	}
	m, _ = press(t, m, "shift+tab")
	if commentKinds[m.editor.kind] != "note" {
		t.Fatalf("shift+tab kind = %s", commentKinds[m.editor.kind])
	}
	m, _ = press(t, m, "esc")
	if m.editing || len(st.comments) != 0 {
		t.Fatal("esc should close the editor without saving")
	}

	m.cursor = rowOf(t, m, rowLine, 1, 2)
	m, _ = press(t, m, "c", "tab", "tab")
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

	m, cmd := press(t, m, "r")
	m = run(t, m, cmd)

	out := screen(m)
	if !strings.Contains(out, "✓") || !strings.Contains(out, "viewed") || strings.Contains(out, "func a() {}") || !strings.Contains(out, "1/3 viewed") {
		t.Fatalf("viewed file not collapsed and marked:\n%s", out)
	}
	if st.viewed["a/main.go"] == "" {
		t.Fatal("viewed state not stored")
	}

	m, cmd = press(t, m, "r")
	m = run(t, m, cmd)
	if !strings.Contains(screen(m), "func a() {}") {
		t.Fatal("unmarking did not expand the file")
	}

	failing := newModel(t, sample, &fakeStore{viewed: map[string]string{}, err: errors.New("read-only")})
	failing, cmd = press(t, failing, "r")
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
	m, cmd := press(t, m, "q")
	if m.help || cmd != nil {
		t.Fatal("q in help should close help, not quit")
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
	tokens := highlight("main.go", lines)
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
	plain := highlight("notes.unknownext", review.ParseLines("@@ -1 +1 @@\n+just\ttext"))
	if got := plain[1][0].text; got != "just    text" {
		t.Fatalf("fallback token = %q, want tabs expanded", got)
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
	if box("t", "", nil, 3, 5, false) != "" {
		t.Fatal("box too narrow should render nothing")
	}
	if got := ansi.Strip(box("a-very-long-title", "right", nil, 12, 2, true)); strings.Contains(got, "right") {
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
	m := New(review.NewService(sample, st, agent, "feature"), "revunk", "main")
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
	if out := screen(m); !strings.Contains(out, "╭─ ◆ nit") || !strings.Contains(out, "rename") {
		t.Fatalf("comment box missing:\n%s", out)
	}
	m, _ = press(t, m, "C")
	if out := screen(m); !strings.Contains(out, "╭─ comment") || !strings.Contains(out, "^s save") {
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
	if out := screen(m); len(m.comments) != 0 || strings.Contains(out, "◆ nit") || !strings.Contains(out, "deleted 1 comments") {
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
	if !strings.Contains(out, "L2 · replied") || !strings.Contains(out, "↳ agent reply") || !strings.Contains(out, "Renamed to") || !strings.Contains(out, "start") {
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
	m, cmd := press(t, m, "j", "r")
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
	if m.cursor != rowOf(t, m, rowHunk, 1, 0) {
		t.Fatalf("h moved to row %d", m.cursor)
	}
	m, _ = press(t, m, "H")
	if m.cursor != rowOf(t, m, rowHunk, 0, 0) {
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

	m, _ = press(t, m, "n")

	out := screen(m)
	for _, want := range []string{"╭─ comments", "◆ note", "a/main.go L3  draft  second func", "README.md file", "a/main.go L2 @2222222  sent  rename only"} {
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
	m, _ = press(t, m, "n")
	if strings.Contains(screen(m), "╭─ comments") || m.focus != focusDiff {
		t.Fatal("n should hide the pane and return to the diff")
	}

	empty, _ := loaded(t)
	empty, _ = press(t, empty, "n")
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
	m, _ = press(t, m, "n", "j", "k", "k", "enter")
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
	m, _ = press(t, m, "n", "j", "j", "j")

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
