package review_test

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
	"github.com/jfmainville/revunk/internal/store"
)

type fakeSource struct {
	commits []review.Commit
	files   map[string][]review.File
	err     error
}

func (f *fakeSource) Commits(context.Context) ([]review.Commit, error) {
	return f.commits, f.err
}

func (f *fakeSource) Files(_ context.Context, commit string) ([]review.File, error) {
	return f.files[commit], f.err
}

type failingStore struct {
	viewedErr, setErr error
}

func (s failingStore) SetViewed(context.Context, string, string, string) error { return s.setErr }
func (s failingStore) Viewed(context.Context, string) (map[string]string, error) {
	return map[string]string{}, s.viewedErr
}
func (s failingStore) AddComment(context.Context, string, review.Comment) error { return s.setErr }
func (s failingStore) Comments(context.Context, string) ([]review.Comment, error) {
	return nil, s.viewedErr
}
func (s failingStore) DeleteComment(context.Context, string, int) error { return s.setErr }

func newStore(t *testing.T) *store.JSON {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "revunk.json"))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCommitShort(t *testing.T) {
	for sha, want := range map[string]string{"0123456789abcdef": "0123456", "abc": "abc", "": ""} {
		if got := (review.Commit{SHA: sha}).Short(); got != want {
			t.Errorf("Short(%q) = %q, want %q", sha, got, want)
		}
	}
}

func TestParseLinesNumbersBothSides(t *testing.T) {
	patch := "@@ -10,3 +20,3 @@ func a() {\n ctx\n-old\n+new\n+more\n\\ No newline at end of file\n tail\n"

	got := review.ParseLines(patch)

	want := []review.Line{
		{Kind: review.LineHunk, Text: "@@ -10,3 +20,3 @@ func a() {"},
		{Kind: review.LineContext, Old: 10, New: 20, Text: "ctx"},
		{Kind: review.LineRemoved, Old: 11, Text: "old"},
		{Kind: review.LineAdded, New: 21, Text: "new"},
		{Kind: review.LineAdded, New: 22, Text: "more"},
		{Kind: review.LineContext, Old: 12, New: 23, Text: "tail"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestParseLinesHandlesSingleLineHunksAndEmptyPatch(t *testing.T) {
	got := review.ParseLines("@@ -0,0 +1 @@\n+only\n@@ -5 +6 @@\n-gone")
	if got[1].New != 1 || got[3].Old != 5 {
		t.Fatalf("wrong numbering: %+v", got)
	}
	if lines := review.ParseLines(""); len(lines) != 0 {
		t.Fatalf("empty patch produced %+v", lines)
	}
	if lines := review.ParseLines("@@ bogus\n+x"); lines[1].New != 0 {
		t.Fatalf("malformed hunk header should start at 0, got %+v", lines)
	}
}

func TestCommentFileLevel(t *testing.T) {
	if !(review.Comment{Path: "a.go"}).FileLevel() {
		t.Fatal("comment without lines should be file level")
	}
	if (review.Comment{Path: "a.go", Start: 1, End: 1}).FileLevel() {
		t.Fatal("line comment reported as file level")
	}
}

func TestCommitsAndFilesWrapSourceErrors(t *testing.T) {
	sourceErr := errors.New("boom")
	svc := review.NewService(&fakeSource{err: sourceErr}, newStore(t), "feature")

	if _, err := svc.Commits(context.Background()); !errors.Is(err, sourceErr) || !strings.Contains(err.Error(), "feature") {
		t.Fatalf("Commits: got %v", err)
	}
	if _, err := svc.Files(context.Background(), ""); !errors.Is(err, sourceErr) {
		t.Fatalf("Files: got %v", err)
	}
	if _, err := svc.Viewed(context.Background()); !errors.Is(err, sourceErr) {
		t.Fatalf("Viewed: got %v", err)
	}
	if _, err := svc.ToggleViewed(context.Background(), "a.go"); !errors.Is(err, sourceErr) {
		t.Fatalf("ToggleViewed: got %v", err)
	}
}

func TestCommitsAndFilesReturnSourceData(t *testing.T) {
	commits := []review.Commit{{SHA: "a1", Subject: "first"}}
	files := []review.File{{Path: "a.go"}}
	svc := review.NewService(&fakeSource{commits: commits, files: map[string][]review.File{"a1": files}}, newStore(t), "feature")

	gotCommits, err := svc.Commits(context.Background())
	if err != nil || !reflect.DeepEqual(gotCommits, commits) {
		t.Fatalf("Commits = %v, %v", gotCommits, err)
	}
	gotFiles, err := svc.Files(context.Background(), "a1")
	if err != nil || !reflect.DeepEqual(gotFiles, files) {
		t.Fatalf("Files = %v, %v", gotFiles, err)
	}
	if svc.Branch() != "feature" {
		t.Fatalf("Branch = %q", svc.Branch())
	}
}

func TestViewedResetsWhenFileChanges(t *testing.T) {
	ctx := context.Background()
	source := &fakeSource{files: map[string][]review.File{"": {{Path: "a.go", Patch: "+v1"}, {Path: "b.go", Patch: "+b"}}}}
	svc := review.NewService(source, newStore(t), "feature")

	viewed, err := svc.ToggleViewed(ctx, "a.go")
	if err != nil || !viewed {
		t.Fatalf("first toggle = %v, %v", viewed, err)
	}
	got, err := svc.Viewed(ctx)
	if err != nil || !got["a.go"] || got["b.go"] {
		t.Fatalf("Viewed = %v, %v", got, err)
	}

	source.files[""][0].Patch = "+v2"
	if got, _ := svc.Viewed(ctx); got["a.go"] {
		t.Fatal("file stayed viewed after its diff changed")
	}

	if viewed, _ := svc.ToggleViewed(ctx, "a.go"); !viewed {
		t.Fatal("toggling a changed file should mark it viewed again")
	}
	if viewed, _ := svc.ToggleViewed(ctx, "a.go"); viewed {
		t.Fatal("second toggle should unmark the file")
	}
	if got, _ := svc.Viewed(ctx); got["a.go"] {
		t.Fatal("unmarked file still viewed")
	}
}

func TestViewedIsScopedToBranch(t *testing.T) {
	ctx := context.Background()
	source := &fakeSource{files: map[string][]review.File{"": {{Path: "a.go", Patch: "+a"}}}}
	st := newStore(t)
	if _, err := review.NewService(source, st, "one").ToggleViewed(ctx, "a.go"); err != nil {
		t.Fatal(err)
	}

	if got, _ := review.NewService(source, st, "two").Viewed(ctx); got["a.go"] {
		t.Fatal("viewed state leaked to another branch")
	}
}

func TestToggleViewedRejectsUnknownFile(t *testing.T) {
	svc := review.NewService(&fakeSource{}, newStore(t), "feature")
	if _, err := svc.ToggleViewed(context.Background(), "nope.go"); err == nil || !strings.Contains(err.Error(), "nope.go") {
		t.Fatalf("got %v, want error naming the file", err)
	}
}

func TestViewedReturnsStoreErrors(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("disk full")
	source := &fakeSource{files: map[string][]review.File{"": {{Path: "a.go"}}}}

	readFails := review.NewService(source, failingStore{viewedErr: storeErr}, "b")
	if _, err := readFails.Viewed(ctx); !errors.Is(err, storeErr) {
		t.Fatalf("Viewed: got %v", err)
	}
	if _, err := readFails.ToggleViewed(ctx, "a.go"); !errors.Is(err, storeErr) {
		t.Fatalf("ToggleViewed read: got %v", err)
	}
	writeFails := review.NewService(source, failingStore{setErr: storeErr}, "b")
	if _, err := writeFails.ToggleViewed(ctx, "a.go"); !errors.Is(err, storeErr) {
		t.Fatalf("ToggleViewed write: got %v", err)
	}
}

func TestAddCommentStoresCommentsPerBranch(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := review.NewService(&fakeSource{}, st, "feature")
	c := review.Comment{Kind: "nit", Path: "a.go", Side: review.SideRight, Start: 2, End: 3, Body: "Use `ctx` here."}

	if err := svc.AddComment(ctx, c); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Comments(ctx)
	if err != nil || !reflect.DeepEqual(got, []review.Comment{c}) {
		t.Fatalf("Comments = %v, %v", got, err)
	}
	if other, _ := review.NewService(&fakeSource{}, st, "other").Comments(ctx); len(other) != 0 {
		t.Fatalf("comments leaked to another branch: %v", other)
	}
}

func TestAddCommentRejectsBlankBodyAndReturnsStoreErrors(t *testing.T) {
	ctx := context.Background()
	svc := review.NewService(&fakeSource{}, newStore(t), "feature")
	if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Body: " \n "}); err == nil {
		t.Fatal("blank comment accepted")
	}
	if got, _ := svc.Comments(ctx); len(got) != 0 {
		t.Fatalf("blank comment stored: %v", got)
	}

	storeErr := errors.New("read only")
	failing := review.NewService(&fakeSource{}, failingStore{setErr: storeErr, viewedErr: storeErr}, "feature")
	if err := failing.AddComment(ctx, review.Comment{Path: "a.go", Body: "hi"}); !errors.Is(err, storeErr) {
		t.Fatalf("AddComment: got %v", err)
	}
	if _, err := failing.Comments(ctx); !errors.Is(err, storeErr) {
		t.Fatalf("Comments: got %v", err)
	}
	if err := failing.DeleteComment(ctx, 0); !errors.Is(err, storeErr) {
		t.Fatalf("DeleteComment: got %v", err)
	}
}

func TestDeleteCommentRemovesOnlyThatComment(t *testing.T) {
	ctx := context.Background()
	svc := review.NewService(&fakeSource{}, newStore(t), "feature")
	for _, body := range []string{"one", "two", "three"} {
		if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Body: body}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DeleteComment(ctx, 1); err != nil {
		t.Fatal(err)
	}

	got, _ := svc.Comments(ctx)
	if len(got) != 2 || got[0].Body != "one" || got[1].Body != "three" {
		t.Fatalf("Comments after delete = %v", got)
	}
	if err := svc.DeleteComment(ctx, 5); err == nil {
		t.Fatal("deleting a missing comment should fail")
	}
}
