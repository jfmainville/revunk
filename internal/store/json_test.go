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
	if len(comments) != 1 || len(comments[0].ID) != 8 {
		t.Fatalf("comment ID not assigned: %v", comments)
	}
	comment.ID = comments[0].ID
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
	if err := os.WriteFile(blocker, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := s.SetViewed(ctx, "b", "a.go", "h"); err == nil {
		t.Fatal("SetViewed succeeded without a writable directory")
	}
	if err := s.AddComment(ctx, "b", review.Comment{Body: "x"}); err == nil {
		t.Fatal("AddComment succeeded without a writable directory")
	}
	if err := s.DeleteComments(ctx, "b", []string{"x"}); err == nil {
		t.Fatal("DeleteComments succeeded without a writable directory")
	}
	if err := s.UpdateComments(ctx, "b", []string{"x"}, func(*review.Comment) {}); err == nil {
		t.Fatal("UpdateComments succeeded without a writable directory")
	}
	if _, err := s.Comments(ctx, "b"); err == nil {
		t.Fatal("Comments succeeded on an unreadable path")
	}
	if _, err := s.Viewed(ctx, "b"); err == nil {
		t.Fatal("Viewed succeeded on an unreadable path")
	}
}

func commentIDs(t *testing.T, s *JSON, branch string) map[string]string {
	t.Helper()
	comments, err := s.Comments(context.Background(), branch)
	if err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{}
	for _, c := range comments {
		ids[c.Body] = c.ID
	}
	return ids
}

func TestUpdateAndDeleteCommentsByID(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "s.json")
	s, _ := Open(path)
	for _, body := range []string{"keep", "reply", "drop"} {
		_ = s.AddComment(ctx, "b", review.Comment{Body: body})
	}
	ids := commentIDs(t, s, "b")

	if err := s.UpdateComments(ctx, "b", []string{ids["reply"], "missing"}, func(c *review.Comment) { c.Replies = append(c.Replies, "done") }); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteComments(ctx, "b", []string{ids["drop"], "missing"}); err != nil {
		t.Fatal(err)
	}

	reopened, _ := Open(path)
	got, _ := reopened.Comments(ctx, "b")
	if len(got) != 2 || got[0].Body != "keep" || got[0].Replies != nil || got[1].Body != "reply" || !reflect.DeepEqual(got[1].Replies, []string{"done"}) {
		t.Fatalf("persisted comments = %+v", got)
	}
}

func TestWritesFromAnotherInstanceAreKept(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "s.json")
	tui, _ := Open(path)
	cli, _ := Open(path)
	_ = tui.AddComment(ctx, "b", review.Comment{Body: "question"})
	id := commentIDs(t, tui, "b")["question"]

	if err := cli.UpdateComments(ctx, "b", []string{id}, func(c *review.Comment) { c.Replies = []string{"answer"} }); err != nil {
		t.Fatal(err)
	}
	if err := tui.SetViewed(ctx, "b", "a.go", "h"); err != nil {
		t.Fatal(err)
	}

	got, _ := tui.Comments(ctx, "b")
	if len(got) != 1 || !reflect.DeepEqual(got[0].Replies, []string{"answer"}) {
		t.Fatalf("reply from another instance lost: %+v", got)
	}
}

func TestOpenGivesOlderCommentsAnID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := os.WriteFile(path, []byte(`{"Comments":{"b":[{"Body":"old"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	first := commentIDs(t, s, "b")["old"]
	again, _ := Open(path)
	if first == "" || commentIDs(t, again, "b")["old"] != first {
		t.Fatalf("ID not assigned once and saved: %q", first)
	}
}
