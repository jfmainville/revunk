package store

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
)

func TestStatePersistsAcrossOpen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "nested", "revunk.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	comment := review.Comment{Path: "a.go", Side: review.SideRight, Start: 1, End: 2, Body: "nit"}
	if err := s.SetViewed(ctx, "feature", "a.go", "h1"); err != nil {
		t.Fatal(err)
	}
	if err := s.AddComment(ctx, "feature", comment); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	viewed, _ := reopened.Viewed(ctx, "feature")
	if viewed["a.go"] != "h1" {
		t.Fatalf("viewed hash not persisted: %v", viewed)
	}
	comments, _ := reopened.Comments(ctx, "feature")
	if !reflect.DeepEqual(comments, []review.Comment{comment}) {
		t.Fatalf("comments not persisted: %v", comments)
	}
}

func TestSetViewedWithEmptyHashClearsFile(t *testing.T) {
	ctx := context.Background()
	s, _ := Open(filepath.Join(t.TempDir(), "s.json"))
	_ = s.SetViewed(ctx, "b", "a.go", "h1")

	if err := s.SetViewed(ctx, "b", "a.go", ""); err != nil {
		t.Fatal(err)
	}

	if viewed, _ := s.Viewed(ctx, "b"); len(viewed) != 0 {
		t.Fatalf("expected no viewed files, got %v", viewed)
	}
}

func TestReadsReturnCopiesAndEmptyValues(t *testing.T) {
	ctx := context.Background()
	s, _ := Open(filepath.Join(t.TempDir(), "s.json"))

	viewed, _ := s.Viewed(ctx, "unknown")
	if viewed == nil || len(viewed) != 0 {
		t.Fatalf("Viewed for unknown branch = %v, want empty map", viewed)
	}
	viewed["x"] = "mutated"
	if again, _ := s.Viewed(ctx, "unknown"); len(again) != 0 {
		t.Fatal("caller mutation leaked into the store")
	}

	_ = s.AddComment(ctx, "b", review.Comment{Body: "one"})
	comments, _ := s.Comments(ctx, "b")
	comments[0].Body = "mutated"
	if again, _ := s.Comments(ctx, "b"); again[0].Body != "one" {
		t.Fatal("caller mutation leaked into stored comments")
	}
}

func TestOpenFailsOnCorruptOrUnreadableFile(t *testing.T) {
	dir := t.TempDir()
	corrupt := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(corrupt, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(corrupt); err == nil {
		t.Fatal("expected an error for corrupt JSON")
	}
	if _, err := Open(dir); err == nil {
		t.Fatal("expected an error when the path is a directory")
	}
}

func TestWritesFailWhenDirectoryIsNotWritable(t *testing.T) {
	ctx := context.Background()
	blocker := filepath.Join(t.TempDir(), "sub")
	s, err := Open(filepath.Join(blocker, "revunk.json"))
	if err != nil {
		t.Fatal(err)
	}
	s.state.Comments["b"] = []review.Comment{{Body: "x"}}
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.SetViewed(ctx, "b", "a.go", "h"); err == nil {
		t.Fatal("SetViewed succeeded without a writable directory")
	}
	if err := s.AddComment(ctx, "b", review.Comment{Body: "x"}); err == nil {
		t.Fatal("AddComment succeeded without a writable directory")
	}
	if err := s.DeleteComment(ctx, "b", 0); err == nil {
		t.Fatal("DeleteComment succeeded without a writable directory")
	}
}

func TestDeleteCommentPersistsAndRejectsBadIndex(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "s.json")
	s, _ := Open(path)
	_ = s.AddComment(ctx, "b", review.Comment{Body: "keep"})
	_ = s.AddComment(ctx, "b", review.Comment{Body: "drop"})

	if err := s.DeleteComment(ctx, "b", 1); err != nil {
		t.Fatal(err)
	}
	for _, i := range []int{-1, 1} {
		if err := s.DeleteComment(ctx, "b", i); err == nil {
			t.Fatalf("DeleteComment(%d) succeeded on a one-comment branch", i)
		}
	}

	reopened, _ := Open(path)
	if got, _ := reopened.Comments(ctx, "b"); len(got) != 1 || got[0].Body != "keep" {
		t.Fatalf("persisted comments = %v", got)
	}
}
