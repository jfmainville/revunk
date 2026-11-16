package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestRunHelpSucceeds(t *testing.T) {
	for _, arg := range []string{"help", "-h", "--help"} {
		if err := run([]string{arg}); err != nil {
			t.Errorf("run(%q) = %v, want nil", arg, err)
		}
	}
}

func TestRunRejectsUnknownCommandAndBadFlags(t *testing.T) {
	if err := run([]string{"serve"}); err == nil {
		t.Fatal("expected an error for an unknown command")
	}
	if err := run([]string{"-base"}); err == nil {
		t.Fatal("expected an error for a flag without a value")
	}
}

func TestRunTUIFailsOutsideGitRepository(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := run(nil); err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Fatalf("error = %v", err)
	}
}

func TestSetupWiresBranchAndBase(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "base"}, {"checkout", "-q", "-b", "feature"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}

	for base, want := range map[string]string{"": "main…feature", "develop": "develop…feature"} {
		model, err := setup(context.Background(), dir, base)
		if err != nil {
			t.Fatal(err)
		}
		updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		out := ansi.Strip(updated.View().Content)
		if !strings.Contains(out, want) || !strings.Contains(out, filepath.Base(dir)) {
			t.Fatalf("base %q: view missing %q or repo name:\n%s", base, want, out)
		}
	}
}
