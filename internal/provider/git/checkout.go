package git

import (
	"context"
	"fmt"
	"strings"

	"github.com/jfmainville/revunk/internal/review"
)

func CheckoutPullRequest(ctx context.Context, dir string, pr review.PullRequest) error {
	remote, err := run(ctx, dir, "remote", "get-url", "origin")
	if err != nil {
		return err
	}
	if !sameRepo(remote, pr.Repo) {
		return fmt.Errorf("the origin of this repository is not %s", pr.Repo)
	}
	if _, err := run(ctx, dir, "fetch", "--quiet", "origin", fmt.Sprintf("refs/pull/%d/head", pr.Number)); err != nil {
		return err
	}
	if _, err := run(ctx, dir, "show-ref", "--verify", "--quiet", "refs/heads/"+pr.Head); err != nil {
		_, err = run(ctx, dir, "checkout", "--quiet", "-b", pr.Head, "FETCH_HEAD")
		return err
	}
	if _, err := run(ctx, dir, "merge-base", "--is-ancestor", pr.Head, "FETCH_HEAD"); err != nil {
		return fmt.Errorf("%s has commits that #%d does not, check it out manually", pr.Head, pr.Number)
	}
	if _, err := run(ctx, dir, "checkout", "--quiet", pr.Head); err != nil {
		return err
	}
	_, err = run(ctx, dir, "merge", "--quiet", "--ff-only", "FETCH_HEAD")
	return err
}

func sameRepo(remote, repo string) bool {
	name := repoName(remote)
	return name != "" && strings.EqualFold(name, repo)
}

func repoName(remote string) string {
	path := strings.TrimSuffix(strings.TrimSpace(remote), ".git")
	parts := strings.FieldsFunc(path, func(r rune) bool { return r == '/' || r == ':' })
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[len(parts)-2:], "/")
}
