package store

import (
	"context"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
)

var pr = review.PullRequest{Owner: "octo", Repo: "demo", Number: 1}

func TestViewedIsEmptyForUnknownPullRequest(t *testing.T) {
	viewed, err := NewMemory().Viewed(context.Background(), pr)
	if err != nil {
		t.Fatal(err)
	}
	if len(viewed) != 0 {
		t.Fatalf("got %v, want empty", viewed)
	}
}

func TestSetViewedPersistsAndOverwrites(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()

	if err := m.SetViewed(ctx, pr, "a.go", true); err != nil {
		t.Fatal(err)
	}
	if err := m.SetViewed(ctx, pr, "b.go", true); err != nil {
		t.Fatal(err)
	}
	if err := m.SetViewed(ctx, pr, "a.go", false); err != nil {
		t.Fatal(err)
	}

	viewed, err := m.Viewed(ctx, pr)
	if err != nil {
		t.Fatal(err)
	}
	if viewed["a.go"] || !viewed["b.go"] {
		t.Fatalf("got %v, want a.go unviewed and b.go viewed", viewed)
	}
}

func TestViewedReturnsCopy(t *testing.T) {
	ctx := context.Background()
	m := NewMemory()
	if err := m.SetViewed(ctx, pr, "a.go", true); err != nil {
		t.Fatal(err)
	}

	viewed, _ := m.Viewed(ctx, pr)
	viewed["a.go"] = false
	viewed["b.go"] = true

	again, _ := m.Viewed(ctx, pr)
	if !again["a.go"] || again["b.go"] {
		t.Fatalf("mutating the returned map changed the store, got %v", again)
	}
}
