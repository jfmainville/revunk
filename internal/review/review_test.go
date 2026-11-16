package review_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
	"github.com/jfmainville/revunk/internal/store"
)

type fakeSource struct {
	files []review.File
	err   error
}

func (f fakeSource) Files(context.Context, review.PullRequest) ([]review.File, error) {
	return f.files, f.err
}

type failingStore struct {
	viewedErr, setErr error
}

func (s failingStore) SetViewed(context.Context, review.PullRequest, string, bool) error {
	return s.setErr
}

func (s failingStore) Viewed(context.Context, review.PullRequest) (map[string]bool, error) {
	return map[string]bool{}, s.viewedErr
}

var pr = review.PullRequest{Owner: "octo", Repo: "demo", Number: 7}

func TestPullRequestString(t *testing.T) {
	if got := pr.String(); got != "octo/demo#7" {
		t.Fatalf("got %q, want %q", got, "octo/demo#7")
	}
}

func TestFilesReturnsSourceFiles(t *testing.T) {
	want := []review.File{{Path: "a.go", Status: review.StatusAdded, Additions: 1}}
	svc := review.NewService(fakeSource{files: want}, store.NewMemory())

	got, err := svc.Files(context.Background(), pr)

	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestFilesWrapsSourceError(t *testing.T) {
	sourceErr := errors.New("boom")
	svc := review.NewService(fakeSource{err: sourceErr}, store.NewMemory())

	_, err := svc.Files(context.Background(), pr)

	if !errors.Is(err, sourceErr) {
		t.Fatalf("expected wrapped source error, got %v", err)
	}
}

func TestToggleViewedFlipsAndPersistsPerPullRequest(t *testing.T) {
	ctx := context.Background()
	svc := review.NewService(fakeSource{}, store.NewMemory())
	other := review.PullRequest{Owner: "octo", Repo: "demo", Number: 8}

	for i, want := range []bool{true, false, true} {
		got, err := svc.ToggleViewed(ctx, pr, "main.go")
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("toggle %d: got %v, want %v", i, got, want)
		}
	}

	viewed, err := svc.Viewed(ctx, pr)
	if err != nil {
		t.Fatal(err)
	}
	if !viewed["main.go"] {
		t.Fatal("expected main.go to stay viewed")
	}
	otherViewed, err := svc.Viewed(ctx, other)
	if err != nil {
		t.Fatal(err)
	}
	if otherViewed["main.go"] {
		t.Fatal("viewed state leaked to another pull request")
	}
}

func TestViewedAndToggleViewedReturnStoreErrors(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("disk full")

	readFails := review.NewService(fakeSource{}, failingStore{viewedErr: storeErr})
	if _, err := readFails.Viewed(ctx, pr); !errors.Is(err, storeErr) {
		t.Fatalf("Viewed: got %v, want %v", err, storeErr)
	}
	if _, err := readFails.ToggleViewed(ctx, pr, "a.go"); !errors.Is(err, storeErr) {
		t.Fatalf("ToggleViewed read: got %v, want %v", err, storeErr)
	}

	writeFails := review.NewService(fakeSource{}, failingStore{setErr: storeErr})
	if _, err := writeFails.ToggleViewed(ctx, pr, "a.go"); !errors.Is(err, storeErr) {
		t.Fatalf("ToggleViewed write: got %v, want %v", err, storeErr)
	}
}
