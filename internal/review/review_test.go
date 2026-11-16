package review_test

import (
	"context"
	"errors"
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

var pr = review.PullRequest{Owner: "octo", Repo: "demo", Number: 7}

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
