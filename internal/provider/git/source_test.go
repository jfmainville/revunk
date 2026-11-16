package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
)

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "t")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "t")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	git(t, dir, "init", "-q", "-b", "main")
	write(t, dir, "keep.go", "package keep\n\nfunc A() {}\n")
	write(t, dir, "old.txt", "bye\n")
	write(t, dir, "move.txt", "same content\nacross rename\n")
	commit(t, dir, "base")
	git(t, dir, "checkout", "-q", "-b", "feature")
	return dir
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, dir, path, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, path), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func commit(t *testing.T, dir, msg string) string {
	t.Helper()
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", msg)
	return git(t, dir, "rev-parse", "HEAD")
}

func byPath(files []review.File) map[string]review.File {
	m := map[string]review.File{}
	for _, f := range files {
		m[f.Path] = f
	}
	return m
}

func TestCommitsListsBranchCommitsOldestFirst(t *testing.T) {
	dir := repo(t)
	write(t, dir, "keep.go", "package keep\n\nfunc A() { _ = 1 }\n")
	first := commit(t, dir, "change keep")
	write(t, dir, "new file.go", "package keep\n")
	second := commit(t, dir, "add new: file")

	commits, err := Source{Dir: dir, Base: "main"}.Commits(context.Background())

	if err != nil {
		t.Fatal(err)
	}
	if len(commits) != 2 || commits[0].SHA != first || commits[1].SHA != second {
		t.Fatalf("commits = %+v", commits)
	}
	if commits[1].Subject != "add new: file" {
		t.Fatalf("subject = %q", commits[1].Subject)
	}
}

func TestCommitsIsEmptyWithoutBranchCommits(t *testing.T) {
	commits, err := Source{Dir: repo(t), Base: "main"}.Commits(context.Background())
	if err != nil || len(commits) != 0 {
		t.Fatalf("Commits = %v, %v", commits, err)
	}
}

func TestFilesCoversWholeBranchWithStatuses(t *testing.T) {
	dir := repo(t)
	write(t, dir, "keep.go", "package keep\n\nfunc A() { _ = 1 }\n")
	commit(t, dir, "change keep")
	write(t, dir, "new file.go", "package keep\n")
	if err := os.Remove(filepath.Join(dir, "old.txt")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "mv", "move.txt", "moved.txt")
	commit(t, dir, "add, remove, rename")

	files, err := Source{Dir: dir, Base: "main"}.Files(context.Background(), "")

	if err != nil {
		t.Fatal(err)
	}
	got := byPath(files)
	if len(files) != 4 {
		t.Fatalf("files = %+v", files)
	}
	keep := got["keep.go"]
	if keep.Status != review.StatusModified || keep.Additions != 1 || keep.Deletions != 1 || !strings.HasPrefix(keep.Patch, "@@ -1,3 +1,3 @@") {
		t.Fatalf("keep.go = %+v", keep)
	}
	if got["new file.go"].Status != review.StatusAdded || got["new file.go"].Additions != 1 {
		t.Fatalf("new file.go = %+v", got["new file.go"])
	}
	if got["old.txt"].Status != review.StatusRemoved || got["old.txt"].Deletions != 1 {
		t.Fatalf("old.txt = %+v", got["old.txt"])
	}
	if got["moved.txt"].Status != review.StatusRenamed || got["moved.txt"].Patch != "" {
		t.Fatalf("moved.txt = %+v", got["moved.txt"])
	}
}

func TestFilesForOneCommitOnlyShowsThatCommit(t *testing.T) {
	dir := repo(t)
	write(t, dir, "keep.go", "package keep\n\nfunc A() { _ = 1 }\n")
	first := commit(t, dir, "change keep")
	write(t, dir, "b.go", "package b\n")
	commit(t, dir, "add b")

	files, err := Source{Dir: dir, Base: "main"}.Files(context.Background(), first)

	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "keep.go" {
		t.Fatalf("files = %+v", files)
	}
}

func TestErrorsIncludeGitMessage(t *testing.T) {
	src := Source{Dir: repo(t), Base: "nope"}
	if _, err := src.Commits(context.Background()); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Commits error = %v", err)
	}
	if _, err := src.Files(context.Background(), ""); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("Files error = %v", err)
	}
	if _, err := (Source{Dir: filepath.Join(t.TempDir(), "missing")}).Files(context.Background(), ""); err == nil {
		t.Fatal("expected an error outside a repository")
	}
}

func TestRunReportsMissingGitBinary(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := run(context.Background(), ".", "status"); err == nil || !strings.Contains(err.Error(), "git status") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseDiffHandlesBinaryAndEmptyOutput(t *testing.T) {
	if files := parseDiff(""); len(files) != 0 {
		t.Fatalf("empty diff produced %+v", files)
	}
	files := parseDiff("diff --git a/img.png b/img.png\nindex 1..2 100644\nBinary files a/img.png and b/img.png differ\n")
	if len(files) != 1 || files[0].Path != "img.png" || files[0].Patch != "" || files[0].Status != review.StatusModified {
		t.Fatalf("binary diff = %+v", files)
	}
}

func TestBranchDefaultBaseAndStatePath(t *testing.T) {
	ctx := context.Background()
	dir := repo(t)

	if b, err := Branch(ctx, dir); err != nil || b != "feature" {
		t.Fatalf("Branch = %q, %v", b, err)
	}
	if root, err := Root(ctx, dir); err != nil || root != dir {
		t.Fatalf("Root = %q, %v", root, err)
	}
	if base := DefaultBase(ctx, dir); base != "main" {
		t.Fatalf("DefaultBase without origin = %q", base)
	}
	git(t, dir, "update-ref", "refs/remotes/origin/main", "main")
	git(t, dir, "symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	if base := DefaultBase(ctx, dir); base != "origin/main" {
		t.Fatalf("DefaultBase with origin = %q", base)
	}
	path, err := StatePath(ctx, dir)
	if err != nil || path != filepath.Join(dir, ".git", "revunk.json") {
		t.Fatalf("StatePath = %q, %v", path, err)
	}
	if _, err := StatePath(ctx, t.TempDir()); err == nil {
		t.Fatal("StatePath outside a repository should fail")
	}
}

func TestFilesIncludesUncommittedAndUntrackedChanges(t *testing.T) {
	dir := repo(t)
	write(t, dir, "keep.go", "package keep\n\nfunc A() { _ = 1 }\n")
	commit(t, dir, "change keep")
	write(t, dir, "keep.go", "package keep\n\nfunc A() { _ = 2 }\n")
	write(t, dir, "old.txt", "staged\n")
	git(t, dir, "add", "old.txt")
	write(t, dir, "new file.go", "package keep\nvar X = 1\n")
	write(t, dir, "empty.txt", "")
	write(t, dir, "blob.bin", "a\x00b")
	write(t, dir, ".gitignore", "*.log\n")
	write(t, dir, "debug.log", "ignored\n")

	files, err := Source{Dir: dir, Base: "main"}.Files(context.Background(), "")

	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	if want := []string{".gitignore", "blob.bin", "empty.txt", "keep.go", "new file.go", "old.txt"}; strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("paths = %q, want %q", paths, want)
	}
	got := byPath(files)
	if !strings.Contains(got["keep.go"].Patch, "+func A() { _ = 2 }") {
		t.Fatalf("unstaged change missing: %q", got["keep.go"].Patch)
	}
	if !strings.Contains(got["old.txt"].Patch, "+staged") {
		t.Fatalf("staged change missing: %q", got["old.txt"].Patch)
	}
	added := got["new file.go"]
	if added.Status != review.StatusAdded || added.Additions != 2 || added.Patch != "@@ -0,0 +1,2 @@\n+package keep\n+var X = 1" {
		t.Fatalf("untracked file = %+v", added)
	}
	if got["blob.bin"].Patch != "" || got["empty.txt"].Patch != "" || got["empty.txt"].Status != review.StatusAdded {
		t.Fatalf("binary or empty untracked files = %+v %+v", got["blob.bin"], got["empty.txt"])
	}
	if lines := review.ParseLines(added.Patch); len(lines) != 3 || lines[2].New != 2 {
		t.Fatalf("untracked patch does not parse: %+v", lines)
	}
}

func TestNewFileSkipsUnreadableAndLargeFiles(t *testing.T) {
	dir := t.TempDir()
	if f := newFile(filepath.Join(dir, "missing"), "missing"); f.Patch != "" || f.Status != review.StatusAdded {
		t.Fatalf("missing file = %+v", f)
	}
	write(t, dir, "big.txt", strings.Repeat("x\n", maxUntrackedSize))
	if f := newFile(filepath.Join(dir, "big.txt"), "big.txt"); f.Patch != "" {
		t.Fatal("large file should have no patch")
	}
}

func TestFilesReportsListingErrors(t *testing.T) {
	dir := repo(t)
	if err := os.WriteFile(filepath.Join(dir, ".git", "index"), []byte("corrupt"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := (Source{Dir: dir, Base: "main"}).Files(context.Background(), ""); err == nil {
		t.Fatal("expected an error with a corrupt index")
	}
}
