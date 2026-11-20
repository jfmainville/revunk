package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
)

func pullRequestRepo(t *testing.T) (dir, head string) {
	t.Helper()
	dir = repo(t)
	remote := filepath.Join(t.TempDir(), "acme", "revunk")
	if err := os.MkdirAll(remote, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, remote, "init", "-q", "--bare")
	git(t, dir, "remote", "add", "origin", remote)
	git(t, dir, "checkout", "-q", "-b", "scratch")
	write(t, dir, "pr.txt", "from the pull request\n")
	head = commit(t, dir, "pull request")
	git(t, dir, "push", "-q", "origin", "HEAD:refs/pull/7/head")
	git(t, dir, "checkout", "-q", "feature")
	git(t, dir, "branch", "-q", "-D", "scratch")
	return dir, head
}

var pullRequest = review.PullRequest{Repo: "acme/revunk", Number: 7, Head: "reviewed"}

func TestCheckoutPullRequest_createsBranchAtHead(t *testing.T) {
	dir, head := pullRequestRepo(t)
	if err := CheckoutPullRequest(context.Background(), dir, pullRequest); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD = %s, want the pull request head %s", got, head)
	}
	if got := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "reviewed" {
		t.Fatalf("current branch = %q, want reviewed", got)
	}
}

func TestCheckoutPullRequest_fastForwardsExistingBranch(t *testing.T) {
	dir, head := pullRequestRepo(t)
	git(t, dir, "branch", "reviewed", "main")
	if err := CheckoutPullRequest(context.Background(), dir, pullRequest); err != nil {
		t.Fatal(err)
	}
	if got := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "reviewed" {
		t.Fatalf("current branch = %q, want reviewed", got)
	}
	if got := git(t, dir, "rev-parse", "HEAD"); got != head {
		t.Fatalf("HEAD = %s, want the pull request head %s", got, head)
	}
}

func TestCheckoutPullRequest_refusesDivergedBranch(t *testing.T) {
	dir, _ := pullRequestRepo(t)
	git(t, dir, "checkout", "-q", "-b", "reviewed", "main")
	write(t, dir, "other.txt", "local only\n")
	local := commit(t, dir, "local")
	git(t, dir, "checkout", "-q", "feature")
	if err := CheckoutPullRequest(context.Background(), dir, pullRequest); err == nil {
		t.Fatal("a diverged branch should not be checked out")
	}
	if got := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature" {
		t.Fatalf("current branch = %q, want feature to stay checked out", got)
	}
	if got := git(t, dir, "rev-parse", "reviewed"); got != local {
		t.Fatalf("reviewed = %s, want the local commit %s kept", got, local)
	}
}

func TestCheckoutPullRequest_keepsConflictingLocalChanges(t *testing.T) {
	dir, _ := pullRequestRepo(t)
	write(t, dir, "pr.txt", "my own file\n")
	if err := CheckoutPullRequest(context.Background(), dir, pullRequest); err == nil {
		t.Fatal("a checkout that would overwrite local changes should be refused")
	}
	data, err := os.ReadFile(filepath.Join(dir, "pr.txt"))
	if err != nil || string(data) != "my own file\n" {
		t.Fatalf("local file = %q, %v, want it preserved", data, err)
	}
	if got := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature" {
		t.Fatalf("current branch = %q, want feature to stay checked out", got)
	}
}

func TestCheckoutPullRequest_refusesOtherRepository(t *testing.T) {
	dir, _ := pullRequestRepo(t)
	other := review.PullRequest{Repo: "acme/other", Number: 7, Head: "reviewed"}
	if err := CheckoutPullRequest(context.Background(), dir, other); err == nil {
		t.Fatal("a pull request from another repository should be refused")
	}
	if got := git(t, dir, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature" {
		t.Fatalf("current branch = %q, want feature to stay checked out", got)
	}
}

func TestCheckoutPullRequest_requiresOrigin(t *testing.T) {
	dir := repo(t)
	if err := CheckoutPullRequest(context.Background(), dir, pullRequest); err == nil {
		t.Fatal("a repository without origin should be refused")
	}
}

func TestSameRepo(t *testing.T) {
	tests := []struct {
		name, remote string
		want         bool
	}{
		{"https with suffix", "https://github.com/acme/revunk.git", true},
		{"ssh with suffix", "git@github.com:acme/revunk.git", true},
		{"trailing newline", "https://github.com/acme/revunk\n", true},
		{"case differs", "https://github.com/Acme/Revunk", true},
		{"other name", "https://github.com/acme/revunk-fork.git", false},
		{"other owner", "https://github.com/other/revunk.git", false},
		{"empty", "", false},
		{"single segment", "revunk", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameRepo(tt.remote, "acme/revunk"); got != tt.want {
				t.Fatalf("sameRepo(%q) = %v, want %v", tt.remote, got, tt.want)
			}
		})
	}
}
