package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jfmainville/revunk/internal/review"
)

func commitSource(files []review.File) fakeSource {
	return fakeSource{
		commits: []review.Commit{{
			SHA: "1111111aaaa", Subject: "add readme", Author: "Ada", Date: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC),
			Body: "explain the readme in detail",
		}},
		files: map[string][]review.File{"1111111aaaa": files},
	}
}

func selectFirstCommit(t *testing.T, m Model) Model {
	t.Helper()
	m, _ = press(t, m, "tab", "tab")
	m, cmd := press(t, m, "j")
	return run(t, m, cmd)
}

func TestCommitInfoShowsMetadataMessageAndFiles(t *testing.T) {
	files := []review.File{{Path: "README.md", Status: review.StatusAdded, Additions: 2}}
	m := newModel(t, commitSource(files), &fakeStore{viewed: map[string]string{}})
	m = selectFirstCommit(t, m)

	m, _ = press(t, m, "K")
	out := screen(m)
	date := review.Commit{Date: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}.Date.Local().Format("2006-01-02 15:04")
	for _, want := range []string{"commit details", "1111111aaaa", "Ada", date, "add readme", "explain the readme in detail", "files 1", "README.md", "+2 -0"} {
		if !strings.Contains(out, want) {
			t.Errorf("overlay missing %q:\n%s", want, out)
		}
	}
}

func TestCommitInfoNeedsASelectedCommit(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "K")
	if m.commitInfo || m.warning != "select a commit to see its details" {
		t.Fatalf("commitInfo = %v, warning = %q", m.commitInfo, m.warning)
	}
}

func TestCommitInfoSaysWhenCommitHasNoFileChanges(t *testing.T) {
	m := newModel(t, commitSource(nil), &fakeStore{viewed: map[string]string{}})
	m = selectFirstCommit(t, m)
	m, _ = press(t, m, "K")
	if !strings.Contains(screen(m), "no file changes") {
		t.Fatalf("empty commit not explained:\n%s", screen(m))
	}
}

func TestCommitInfoShowsFileLoadFailure(t *testing.T) {
	m := newModel(t, commitSource(nil), &fakeStore{viewed: map[string]string{}})
	m, _ = press(t, m, "tab", "tab", "j")
	m = update(t, m, filesMsg{commit: 1, err: errors.New("bad object")})
	m, _ = press(t, m, "K")
	out := screen(m)
	if !strings.Contains(out, "bad object") || strings.Contains(out, "no file changes") {
		t.Fatalf("load failure not shown:\n%s", out)
	}
}

func TestCommitInfoClosesWithEscAndKeepsOtherKeysOut(t *testing.T) {
	m := newModel(t, commitSource(nil), &fakeStore{viewed: map[string]string{}})
	m = selectFirstCommit(t, m)
	m, _ = press(t, m, "K", "j")
	if !m.commitInfo || m.commit != 1 {
		t.Fatalf("keys leaked through the overlay: open = %v, commit = %d", m.commitInfo, m.commit)
	}
	m, _ = press(t, m, "esc")
	if m.commitInfo {
		t.Fatal("esc did not close the commit details")
	}
	if strings.Contains(screen(m), "commit details") {
		t.Fatal("closed overlay still drawn")
	}
}

func TestCommitInfoLinesWrapLongMessages(t *testing.T) {
	m := Model{}
	c := review.Commit{SHA: "abc", Subject: "subject", Body: strings.Repeat("word ", 30)}
	var body []string
	for _, l := range m.commitInfoLines(c, 20) {
		if strings.Contains(ansi.Strip(l), "word") {
			body = append(body, l)
		}
	}
	if len(body) < 2 {
		t.Fatalf("long body not wrapped over several lines: %q", body)
	}
	for _, l := range body {
		if w := lipgloss.Width(l); w > 21 {
			t.Fatalf("body line is %d wide, want at most 21: %q", w, l)
		}
	}
}
