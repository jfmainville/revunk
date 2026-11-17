package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jfmainville/revunk/internal/provider/github"
	"github.com/jfmainville/revunk/internal/review"
	"github.com/jfmainville/revunk/internal/store"
)

func TestUsageHidesAgentCommands(t *testing.T) {
	if strings.Contains(usage, "reply") || !strings.Contains(usage, "tui") || !strings.Contains(usage, "-cleanup") {
		t.Fatalf("usage should list tui and hide reply:\n%s", usage)
	}
}

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

func gitRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"-c", "user.name=t", "-c", "user.email=t@example.com", "commit", "-q", "--allow-empty", "-m", "base"}, {"checkout", "-q", "-b", "feature"}} {
		if out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

func TestSetupWiresBranchAndBase(t *testing.T) {
	dir := gitRepo(t)
	for base, want := range map[string]string{"": "main…feature", "develop": "develop…feature"} {
		model, err := setup(context.Background(), dir, base)
		if err != nil {
			t.Fatal(err)
		}
		updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		out := ansi.Strip(updated.View().Content)
		if !strings.Contains(out, want) || strings.Contains(out, filepath.Base(dir)+" ·") {
			t.Fatalf("base %q: view missing %q or repo name:\n%s", base, want, out)
		}
	}
}

func TestRunReplyAddsReplyToComment(t *testing.T) {
	ctx := context.Background()
	dir := gitRepo(t)
	t.Chdir(dir)
	svc, _, _, _, err := service(ctx, ".", "")
	if err != nil {
		t.Fatal(err)
	}
	if svc.ReplyCommand == "" {
		t.Fatal("reply command not set to the running binary")
	}
	if err := os.WriteFile(filepath.Join(dir, "here.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if !svc.Exists("here.txt") || svc.Exists("gone.txt") {
		t.Fatal("Exists should check files in the repository root")
	}
	if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Body: "why?"}); err != nil {
		t.Fatal(err)
	}
	comments, _ := svc.Comments(ctx)

	if err := run([]string{"reply", comments[0].ID, "Because", "it", "is", "faster."}); err != nil {
		t.Fatal(err)
	}

	comments, _ = svc.Comments(ctx)
	if !reflect.DeepEqual(comments[0].Replies, []string{"Because it is faster."}) {
		t.Fatalf("replies = %q", comments[0].Replies)
	}
	if err := run([]string{"reply", "only-id"}); err == nil || !strings.Contains(err.Error(), "usage") {
		t.Fatalf("missing text error = %v", err)
	}
	if err := run([]string{"reply", "nope", "x"}); err == nil || !strings.Contains(err.Error(), "no comment nope") {
		t.Fatalf("unknown id error = %v", err)
	}
	t.Chdir(t.TempDir())
	if err := run([]string{"reply", "id", "x"}); err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Fatalf("outside repository error = %v", err)
	}
}

func TestPullRequestReviewReadsThePullRequestFromGitHub(t *testing.T) {
	var calls [][]string
	client := github.Client{Run: func(_ context.Context, args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte(`[{"sha":"abc","commit":{"message":"fix"}}]`), nil
	}}
	svc := pullRequestReview(client, nil)(review.PullRequest{Repo: "o/r", Number: 7})
	if svc.Branch() != "o/r#7" {
		t.Fatalf("branch = %q", svc.Branch())
	}
	commits, err := svc.Commits(context.Background())
	if err != nil || len(commits) != 1 || commits[0].Subject != "fix" || calls[0][len(calls[0])-1] != "repos/o/r/pulls/7/commits?per_page=100" {
		t.Fatalf("commits = %+v, %v, calls = %q", commits, err, calls)
	}
}

func TestServiceDropsStateOfDeletedBranches(t *testing.T) {
	ctx := context.Background()
	dir := gitRepo(t)
	st, err := store.Open(filepath.Join(dir, ".git", "revunk.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, branch := range []string{"feature", "main", "deleted", "o/r#7"} {
		if err := st.AddComment(ctx, branch, review.Comment{Path: "a.go", Body: "x"}); err != nil {
			t.Fatal(err)
		}
	}
	if out, err := exec.Command("git", "-C", dir, "checkout", "-q", "--detach").CombinedOutput(); err != nil {
		t.Fatalf("detach: %v\n%s", err, out)
	}
	if err := st.AddComment(ctx, "HEAD", review.Comment{Path: "a.go", Body: "x"}); err != nil {
		t.Fatal(err)
	}

	if _, _, _, _, err := service(ctx, dir, ""); err != nil {
		t.Fatal(err)
	}

	for branch, want := range map[string]int{"feature": 1, "main": 1, "HEAD": 1, "o/r#7": 1, "deleted": 0} {
		if comments, _ := st.Comments(ctx, branch); len(comments) != want {
			t.Errorf("%s has %d comments, want %d", branch, len(comments), want)
		}
	}
}

func TestRunCleanupRemovesTheStateFile(t *testing.T) {
	dir := gitRepo(t)
	t.Chdir(dir)
	path := filepath.Join(dir, ".git", "revunk.json")
	if err := os.WriteFile(path, []byte(`{"Comments":{"feature":[{"Body":"x"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := run([]string{"-cleanup"}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("state file still there: %v", err)
	}
	if err := run([]string{"-cleanup"}); err != nil {
		t.Fatalf("cleanup without a state file = %v", err)
	}

	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "child"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-cleanup"}); err == nil {
		t.Fatal("cleanup should report a state path it cannot remove")
	}

	t.Chdir(t.TempDir())
	if err := run([]string{"-cleanup"}); err == nil || !strings.Contains(err.Error(), "git repository") {
		t.Fatalf("outside repository error = %v", err)
	}
}
