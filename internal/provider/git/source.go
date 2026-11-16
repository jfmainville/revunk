package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/jfmainville/revunk/internal/review"
)

type Source struct {
	Dir  string
	Base string
}

func run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir, "-c", "core.quotePath=false"}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return "", fmt.Errorf("git %s: %s", args[0], bytes.TrimSpace(exitErr.Stderr))
		}
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}

func (s Source) Commits(ctx context.Context) ([]review.Commit, error) {
	out, err := run(ctx, s.Dir, "log", "--reverse", "--format=%H%x1f%s", s.Base+"..HEAD")
	if err != nil {
		return nil, err
	}
	var commits []review.Commit
	for line := range strings.SplitSeq(strings.TrimSpace(out), "\n") {
		sha, subject, ok := strings.Cut(line, "\x1f")
		if ok {
			commits = append(commits, review.Commit{SHA: sha, Subject: subject})
		}
	}
	return commits, nil
}

func (s Source) Files(ctx context.Context, commit string) ([]review.File, error) {
	if commit != "" {
		out, err := run(ctx, s.Dir, "show", "--no-color", "--no-ext-diff", "-M", "--format=", commit)
		if err != nil {
			return nil, err
		}
		return parseDiff(out), nil
	}
	base, err := run(ctx, s.Dir, "merge-base", s.Base, "HEAD")
	if err != nil {
		return nil, err
	}
	out, err := run(ctx, s.Dir, "diff", "--no-color", "--no-ext-diff", "-M", strings.TrimSpace(base))
	if err != nil {
		return nil, err
	}
	files := parseDiff(out)
	untracked, err := run(ctx, s.Dir, "ls-files", "--others", "--exclude-standard", "-z")
	if err != nil {
		return nil, err
	}
	for path := range strings.SplitSeq(strings.TrimSuffix(untracked, "\x00"), "\x00") {
		if path != "" {
			files = append(files, newFile(filepath.Join(s.Dir, path), path))
		}
	}
	slices.SortFunc(files, func(a, b review.File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}

const maxUntrackedSize = 1 << 20

func newFile(abs, path string) review.File {
	f := review.File{Path: path, Status: review.StatusAdded}
	data, err := os.ReadFile(abs)
	if err != nil || len(data) == 0 || len(data) > maxUntrackedSize || bytes.IndexByte(data, 0) >= 0 {
		return f
	}
	lines := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	f.Additions = len(lines)
	f.Patch = fmt.Sprintf("@@ -0,0 +1,%d @@\n+", len(lines)) + strings.Join(lines, "\n+")
	return f
}

func parseDiff(out string) []review.File {
	var files []review.File
	for _, chunk := range strings.Split(out, "diff --git ")[1:] {
		header, patch, _ := strings.Cut(chunk, "\n@@")
		if patch != "" {
			patch = "@@" + strings.TrimSuffix(patch, "\n")
		}
		f := review.File{Status: review.StatusModified, Patch: patch}
		lines := strings.Split(header, "\n")
		if _, b, ok := strings.Cut(lines[0], " b/"); ok {
			f.Path = b
		}
		for _, line := range lines[1:] {
			line = strings.TrimSuffix(line, "\t")
			switch {
			case strings.HasPrefix(line, "new file mode"):
				f.Status = review.StatusAdded
			case strings.HasPrefix(line, "deleted file mode"):
				f.Status = review.StatusRemoved
			case strings.HasPrefix(line, "rename to "):
				f.Status = review.StatusRenamed
				f.Path = strings.TrimPrefix(line, "rename to ")
			case strings.HasPrefix(line, "+++ b/"):
				f.Path = strings.TrimPrefix(line, "+++ b/")
			case strings.HasPrefix(line, "--- a/") && f.Status == review.StatusRemoved:
				f.Path = strings.TrimPrefix(line, "--- a/")
			}
		}
		for line := range strings.SplitSeq(patch, "\n") {
			switch {
			case strings.HasPrefix(line, "+"):
				f.Additions++
			case strings.HasPrefix(line, "-"):
				f.Deletions++
			}
		}
		files = append(files, f)
	}
	return files
}

func Branch(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--abbrev-ref", "HEAD")
	return strings.TrimSpace(out), err
}

func Root(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	return strings.TrimSpace(out), err
}

func DefaultBase(ctx context.Context, dir string) string {
	out, err := run(ctx, dir, "symbolic-ref", "--short", "refs/remotes/origin/HEAD")
	if err != nil {
		return "main"
	}
	return strings.TrimSpace(out)
}

func StatePath(ctx context.Context, dir string) (string, error) {
	out, err := run(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out) + "/revunk.json", nil
}
