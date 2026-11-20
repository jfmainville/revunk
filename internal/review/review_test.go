package review_test

import (
	"context"
	"errors"
	"fmt"
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
func (s failingStore) UpdateComments(context.Context, string, []string, func(*review.Comment)) error {
	return s.setErr
}
func (s failingStore) DeleteComments(context.Context, string, []string) error {
	return s.setErr
}

func idOf(t *testing.T, svc *review.Service, body string) string {
	t.Helper()
	comments, err := svc.Comments(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range comments {
		if c.Body == body {
			return c.ID
		}
	}
	t.Fatalf("no comment %q", body)
	return ""
}

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
	svc := review.NewService(&fakeSource{err: sourceErr}, newStore(t), nil, "feature")

	if _, err := svc.Commits(context.Background()); !errors.Is(err, sourceErr) || !strings.Contains(err.Error(), "feature") {
		t.Fatalf("Commits: got %v", err)
	}
	if _, err := svc.Files(context.Background(), ""); !errors.Is(err, sourceErr) {
		t.Fatalf("Files: got %v", err)
	}
	if _, err := svc.ToggleViewed(context.Background(), "a.go"); !errors.Is(err, sourceErr) {
		t.Fatalf("ToggleViewed: got %v", err)
	}
}

func TestCommitsAndFilesReturnSourceData(t *testing.T) {
	commits := []review.Commit{{SHA: "a1", Subject: "first"}}
	files := []review.File{{Path: "a.go"}}
	svc := review.NewService(&fakeSource{commits: commits, files: map[string][]review.File{"a1": files}}, newStore(t), nil, "feature")

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
	svc := review.NewService(source, newStore(t), nil, "feature")

	viewed, err := svc.ToggleViewed(ctx, "a.go")
	if err != nil || !viewed {
		t.Fatalf("first toggle = %v, %v", viewed, err)
	}
	got, err := svc.Viewed(ctx, source.files[""])
	if err != nil || !got["a.go"] || got["b.go"] {
		t.Fatalf("Viewed = %v, %v", got, err)
	}

	source.files[""][0].Patch = "+v2"
	if got, _ := svc.Viewed(ctx, source.files[""]); got["a.go"] {
		t.Fatal("file stayed viewed after its diff changed")
	}

	if viewed, _ := svc.ToggleViewed(ctx, "a.go"); !viewed {
		t.Fatal("toggling a changed file should mark it viewed again")
	}
	if viewed, _ := svc.ToggleViewed(ctx, "a.go"); viewed {
		t.Fatal("second toggle should unmark the file")
	}
	if got, _ := svc.Viewed(ctx, source.files[""]); got["a.go"] {
		t.Fatal("unmarked file still viewed")
	}
}

func TestViewedIsScopedToBranch(t *testing.T) {
	ctx := context.Background()
	source := &fakeSource{files: map[string][]review.File{"": {{Path: "a.go", Patch: "+a"}}}}
	st := newStore(t)
	if _, err := review.NewService(source, st, nil, "one").ToggleViewed(ctx, "a.go"); err != nil {
		t.Fatal(err)
	}

	if got, _ := review.NewService(source, st, nil, "two").Viewed(ctx, source.files[""]); got["a.go"] {
		t.Fatal("viewed state leaked to another branch")
	}
}

func TestToggleViewedRejectsUnknownFile(t *testing.T) {
	svc := review.NewService(&fakeSource{}, newStore(t), nil, "feature")
	if _, err := svc.ToggleViewed(context.Background(), "nope.go"); err == nil || !strings.Contains(err.Error(), "nope.go") {
		t.Fatalf("got %v, want error naming the file", err)
	}
}

func TestViewedReturnsStoreErrors(t *testing.T) {
	ctx := context.Background()
	storeErr := errors.New("disk full")
	source := &fakeSource{files: map[string][]review.File{"": {{Path: "a.go"}}}}

	readFails := review.NewService(source, failingStore{viewedErr: storeErr}, nil, "b")
	if _, err := readFails.Viewed(ctx, source.files[""]); !errors.Is(err, storeErr) {
		t.Fatalf("Viewed: got %v", err)
	}
	if _, err := readFails.ToggleViewed(ctx, "a.go"); !errors.Is(err, storeErr) {
		t.Fatalf("ToggleViewed read: got %v", err)
	}
	writeFails := review.NewService(source, failingStore{setErr: storeErr}, nil, "b")
	if _, err := writeFails.ToggleViewed(ctx, "a.go"); !errors.Is(err, storeErr) {
		t.Fatalf("ToggleViewed write: got %v", err)
	}
}

func TestAddCommentStoresCommentsPerBranch(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := review.NewService(&fakeSource{}, st, nil, "feature")
	c := review.Comment{Kind: "nit", Path: "a.go", Side: review.SideRight, Start: 2, End: 3, Body: "Use `ctx` here."}

	if err := svc.AddComment(ctx, c); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Comments(ctx)
	if err != nil || len(got) != 1 {
		t.Fatalf("Comments = %v, %v", got, err)
	}
	c.ID = got[0].ID
	if !reflect.DeepEqual(got, []review.Comment{c}) {
		t.Fatalf("Comments = %v", got)
	}
	if other, _ := review.NewService(&fakeSource{}, st, nil, "other").Comments(ctx); len(other) != 0 {
		t.Fatalf("comments leaked to another branch: %v", other)
	}
}

func TestAddCommentRejectsBlankBodyAndReturnsStoreErrors(t *testing.T) {
	ctx := context.Background()
	svc := review.NewService(&fakeSource{}, newStore(t), nil, "feature")
	if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Body: " \n "}); err == nil {
		t.Fatal("blank comment accepted")
	}
	if got, _ := svc.Comments(ctx); len(got) != 0 {
		t.Fatalf("blank comment stored: %v", got)
	}

	storeErr := errors.New("read only")
	failing := review.NewService(&fakeSource{}, failingStore{setErr: storeErr, viewedErr: storeErr}, nil, "feature")
	if err := failing.AddComment(ctx, review.Comment{Path: "a.go", Body: "hi"}); !errors.Is(err, storeErr) {
		t.Fatalf("AddComment: got %v", err)
	}
	if _, err := failing.Comments(ctx); !errors.Is(err, storeErr) {
		t.Fatalf("Comments: got %v", err)
	}
	if err := failing.DeleteComment(ctx, "id"); !errors.Is(err, storeErr) {
		t.Fatalf("DeleteComment: got %v", err)
	}
}

func TestDeleteCommentRemovesOnlyThatComment(t *testing.T) {
	ctx := context.Background()
	svc := review.NewService(&fakeSource{}, newStore(t), nil, "feature")
	for _, body := range []string{"one", "two", "three"} {
		if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Body: body}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DeleteComment(ctx, idOf(t, svc, "two")); err != nil {
		t.Fatal(err)
	}

	got, _ := svc.Comments(ctx)
	if len(got) != 2 || got[0].Body != "one" || got[1].Body != "three" {
		t.Fatalf("Comments after delete = %v", got)
	}
}

func TestResolveCommentTogglesStateAndLeavesItOutOfThePrompt(t *testing.T) {
	ctx := context.Background()
	svc := review.NewService(&fakeSource{}, newStore(t), nil, "feature")
	for _, body := range []string{"still-open", "now-resolved"} {
		if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Body: body}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.ResolveComment(ctx, idOf(t, svc, "now-resolved"), true); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Comments(ctx)
	if len(got) != 2 || got[0].Resolved || !got[1].Resolved {
		t.Fatalf("Comments after resolve = %+v", got)
	}
	text, err := svc.Prompt(ctx)
	if err != nil || !strings.Contains(text, "still-open") || strings.Contains(text, "now-resolved") {
		t.Fatalf("prompt should skip resolved comments: %q, %v", text, err)
	}

	if err := svc.ResolveComment(ctx, idOf(t, svc, "still-open"), true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Prompt(ctx); err == nil || !strings.Contains(err.Error(), "no unsent comments") {
		t.Fatalf("prompt with only resolved comments = %v", err)
	}

	if err := svc.ResolveComment(ctx, idOf(t, svc, "now-resolved"), false); err != nil {
		t.Fatal(err)
	}
	if got, _ = svc.Comments(ctx); got[1].Resolved {
		t.Fatal("resolving with false should reopen the comment")
	}

	storeErr := errors.New("locked")
	failing := review.NewService(&fakeSource{}, failingStore{setErr: storeErr}, nil, "feature")
	if err := failing.ResolveComment(ctx, "id", true); !errors.Is(err, storeErr) {
		t.Fatalf("ResolveComment: got %v", err)
	}
}

func TestFollowUpAnswersTheAgentAndResendsTheThread(t *testing.T) {
	ctx := context.Background()
	svc := review.NewService(&fakeSource{}, newStore(t), nil, "feature")
	if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Body: "rename it"}); err != nil {
		t.Fatal(err)
	}
	id := idOf(t, svc, "rename it")
	if err := svc.FollowUp(ctx, id, "and?"); err == nil || !strings.Contains(err.Error(), "no agent reply") {
		t.Fatalf("follow-up before an agent reply = %v", err)
	}

	if err := svc.Reply(ctx, id, "renamed to b"); err != nil {
		t.Fatal(err)
	}
	if err := svc.FollowUp(ctx, id, "  "); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty follow-up = %v", err)
	}
	if err := svc.FollowUp(ctx, "nope", "x"); err == nil || !strings.Contains(err.Error(), "no comment nope") {
		t.Fatalf("unknown comment = %v", err)
	}
	if err := svc.FollowUp(ctx, id, "why b?"); err != nil {
		t.Fatal(err)
	}

	got, _ := svc.Comments(ctx)
	if got[0].Sent || got[0].Answered() || !reflect.DeepEqual(got[0].Replies, []string{"renamed to b", review.YourReply + "why b?"}) {
		t.Fatalf("comment after follow-up = %+v", got[0])
	}
	text, err := svc.Prompt(ctx)
	if err != nil || !strings.Contains(text, "rename it\n\nThread so far, answer the reviewer's last reply:\n\nAgent: renamed to b\n\nReviewer: why b?\n") {
		t.Fatalf("prompt should carry the thread:\n%s\n%v", text, err)
	}
	if err := svc.FollowUp(ctx, id, "again"); err == nil {
		t.Fatal("a second follow-up should wait for the agent")
	}
	if err := svc.Reply(ctx, id, "short for b"); err != nil {
		t.Fatal(err)
	}
	if got, _ = svc.Comments(ctx); !got[0].Answered() {
		t.Fatal("a new agent reply should allow another follow-up")
	}
	if (review.Comment{Author: "bob", Replies: []string{"x"}}).Answered() {
		t.Fatal("pull request comments are never answered by the agent")
	}
}

func TestDeleteAllCommentsClearsOnlyThisBranch(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := review.NewService(&fakeSource{}, st, nil, "feature")
	if err := svc.DeleteAllComments(ctx); err != nil {
		t.Fatalf("clearing no comments = %v", err)
	}
	for _, branch := range []string{"feature", "feature", "other"} {
		if err := st.AddComment(ctx, branch, review.Comment{Path: "a.go", Body: "x"}); err != nil {
			t.Fatal(err)
		}
	}

	if err := svc.DeleteAllComments(ctx); err != nil {
		t.Fatal(err)
	}

	if got, _ := svc.Comments(ctx); len(got) != 0 {
		t.Fatalf("comments left = %v", got)
	}
	if got, _ := st.Comments(ctx, "other"); len(got) != 1 {
		t.Fatalf("other branch comments = %v", got)
	}
	storeErr := errors.New("locked")
	if err := review.NewService(&fakeSource{}, failingStore{viewedErr: storeErr}, nil, "b").DeleteAllComments(ctx); !errors.Is(err, storeErr) {
		t.Fatalf("store error = %v", err)
	}
}

type fakeAgent struct {
	text   string
	err    error
	during func()
}

func (a *fakeAgent) Prompt(_ context.Context, text string) (string, error) {
	a.text = text
	if a.during != nil {
		a.during()
	}
	return "opencode", a.err
}

func promptService(t *testing.T, agent review.Agent) *review.Service {
	t.Helper()
	source := &fakeSource{files: map[string][]review.File{
		"":          {{Path: "a.go", Patch: "@@ -1,3 +1,3 @@\n keep\n-old\n+new\n tail"}},
		"abcdef123": {{Path: "b.go", Patch: "@@ -0,0 +1 @@\n+b"}},
	}}
	svc := review.NewService(source, newStore(t), agent, "feature")
	for _, c := range []review.Comment{
		{Kind: "issue", Path: "a.go", Side: review.SideRight, Start: 1, End: 2, Body: "fix it"},
		{Kind: "question", Path: "a.go", Side: review.SideLeft, Start: 2, End: 2, Body: "why remove?"},
		{Kind: "nit", Path: "a.go", Body: "whole file"},
		{Kind: "praise", Commit: "abcdef123", Path: "b.go", Side: review.SideRight, Start: 1, End: 1, Body: "nice"},
	} {
		if err := svc.AddComment(context.Background(), c); err != nil {
			t.Fatal(err)
		}
	}
	return svc
}

func TestPromptListsEveryCommentWithItsCode(t *testing.T) {
	svc := promptService(t, nil)
	svc.ReplyCommand = "/bin/revunk"
	got, err := svc.Prompt(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := "Address these review comments on branch feature. After handling each comment, reply to it with `/bin/revunk reply <id> \"<what you changed, or your answer>\"`.\n" +
		"\n## 1. issue on a.go lines 1-2 (id " + idOf(t, svc, "fix it") + ")\n\n```\nkeep\nnew\n```\n\nfix it\n" +
		"\n## 2. question on a.go removed line 2 (id " + idOf(t, svc, "why remove?") + ")\n\n```\nold\n```\n\nwhy remove?\n" +
		"\n## 3. nit on a.go (id " + idOf(t, svc, "whole file") + ")\n\nwhole file\n" +
		"\n## 4. praise on b.go line 1 in commit abcdef1 (id " + idOf(t, svc, "nice") + ")\n\n```\nb\n```\n\nnice\n"
	if got != want {
		t.Fatalf("prompt =\n%s\nwant\n%s", got, want)
	}
}

func TestPromptErrors(t *testing.T) {
	ctx := context.Background()
	if _, err := review.NewService(&fakeSource{}, newStore(t), nil, "feature").Prompt(ctx); err == nil || !strings.Contains(err.Error(), "no unsent comments on feature") {
		t.Fatalf("empty prompt error = %v", err)
	}
	storeErr := errors.New("locked")
	if _, err := review.NewService(&fakeSource{}, failingStore{viewedErr: storeErr}, nil, "b").Prompt(ctx); !errors.Is(err, storeErr) {
		t.Fatalf("store error = %v", err)
	}
	st := newStore(t)
	if err := st.AddComment(ctx, "b", review.Comment{Path: "a.go", Body: "x"}); err != nil {
		t.Fatal(err)
	}
	sourceErr := errors.New("bad revision")
	if _, err := review.NewService(&fakeSource{err: sourceErr}, st, nil, "b").Prompt(ctx); !errors.Is(err, sourceErr) {
		t.Fatalf("source error = %v", err)
	}
}

func TestSendCommentsPromptsTheAgent(t *testing.T) {
	ctx := context.Background()
	agent := &fakeAgent{}
	svc := promptService(t, agent)
	if name, err := svc.SendComments(ctx); err != nil || name != "opencode" || !strings.Contains(agent.text, "## 4. praise") {
		t.Fatalf("SendComments = %q, %v, sent %q", name, err, agent.text)
	}
	if left, _ := svc.Comments(ctx); len(left) != 4 || !left[0].Sent || !left[3].Sent {
		t.Fatalf("sent comments not marked sent: %+v", left)
	}
	if _, err := svc.SendComments(ctx); err == nil || !strings.Contains(err.Error(), "no unsent comments") {
		t.Fatalf("second send = %v", err)
	}
	agentErr := errors.New("blocked")
	failing := promptService(t, &fakeAgent{err: agentErr})
	if _, err := failing.SendComments(ctx); !errors.Is(err, agentErr) {
		t.Fatalf("agent error = %v", err)
	}
	if left, _ := failing.Comments(ctx); len(left) != 4 || left[0].Sent {
		t.Fatalf("comments marked sent after a failed send: %+v", left)
	}
	busy := &fakeAgent{}
	svc = promptService(t, busy)
	busy.during = func() {
		_ = svc.AddComment(ctx, review.Comment{Kind: "issue", Path: "a.go", Body: "added while working"})
	}
	if _, err := svc.SendComments(ctx); err != nil {
		t.Fatal(err)
	}
	if left, _ := svc.Comments(ctx); len(left) != 5 || left[4].Sent {
		t.Fatalf("comment added while sending was marked sent: %+v", left)
	}
	if _, err := svc.Prompt(ctx); err != nil {
		t.Fatalf("unsent comment not offered again: %v", err)
	}
	if _, err := promptService(t, nil).SendComments(ctx); err == nil || !strings.Contains(err.Error(), "no coding agent") {
		t.Fatalf("missing agent error = %v", err)
	}
	empty := &fakeAgent{}
	if _, err := review.NewService(&fakeSource{}, newStore(t), empty, "x").SendComments(ctx); err == nil || empty.text != "" {
		t.Fatalf("empty send = %v, sent %q", err, empty.text)
	}
}

func TestPromptDefaultsReplyCommandToRevunk(t *testing.T) {
	got, err := promptService(t, nil).Prompt(context.Background())
	if err != nil || !strings.Contains(got, "`revunk reply <id>") {
		t.Fatalf("Prompt = %q, %v", got, err)
	}
}

func TestReplyAddsToTheComment(t *testing.T) {
	ctx := context.Background()
	svc := promptService(t, nil)
	id := idOf(t, svc, "why remove?")

	for _, body := range []string{"It was unused.", "Restored it."} {
		if err := svc.Reply(ctx, id, body); err != nil {
			t.Fatal(err)
		}
	}

	comments, _ := svc.Comments(ctx)
	if got := comments[1].Replies; !reflect.DeepEqual(got, []string{"It was unused.", "Restored it."}) {
		t.Fatalf("replies = %q", got)
	}
	if comments[0].Replies != nil {
		t.Fatal("reply landed on another comment")
	}
	if err := svc.Reply(ctx, id, "  "); err == nil {
		t.Fatal("blank reply accepted")
	}
	if err := svc.Reply(ctx, "nope", "x"); err == nil || !strings.Contains(err.Error(), "no comment nope on feature") {
		t.Fatalf("unknown id error = %v", err)
	}
	storeErr := errors.New("locked")
	if err := review.NewService(&fakeSource{}, failingStore{viewedErr: storeErr}, nil, "b").Reply(ctx, id, "x"); !errors.Is(err, storeErr) {
		t.Fatalf("store error = %v", err)
	}
}

func TestPruneCommentsDeletesCommentsOnDeletedFiles(t *testing.T) {
	ctx := context.Background()
	src := &fakeSource{files: map[string][]review.File{"": {
		{Path: "kept.go", Status: review.StatusModified},
		{Path: "dropped.go", Status: review.StatusRemoved},
	}}}
	st := newStore(t)
	svc := review.NewService(src, st, nil, "feature")
	for _, c := range []review.Comment{
		{Path: "kept.go", Side: review.SideRight, Start: 1, End: 1, Body: "exists"},
		{Path: "gone.go", Side: review.SideRight, Start: 1, End: 1, Body: "added then deleted"},
		{Path: "gone.go", Commit: "abc", Body: "commit comment on deleted file"},
		{Path: "dropped.go", Side: review.SideRight, Start: 2, End: 2, Body: "lines no longer exist"},
		{Path: "dropped.go", Side: review.SideLeft, Start: 2, End: 2, Body: "why remove this"},
		{Path: "dropped.go", Body: "why delete the file"},
	} {
		if err := svc.AddComment(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	if n, err := svc.PruneComments(ctx, src.files[""]); n != 0 || err != nil {
		t.Fatalf("without Exists nothing should be pruned, got %d %v", n, err)
	}
	svc.Exists = func(path string) bool { return path == "kept.go" }
	n, err := svc.PruneComments(ctx, src.files[""])
	if err != nil || n != 3 {
		t.Fatalf("pruned %d, err %v", n, err)
	}
	comments, _ := svc.Comments(ctx)
	var bodies []string
	for _, c := range comments {
		bodies = append(bodies, c.Body)
	}
	if want := []string{"exists", "why remove this", "why delete the file"}; !reflect.DeepEqual(bodies, want) {
		t.Fatalf("kept %v, want %v", bodies, want)
	}
	if n, err := svc.PruneComments(ctx, src.files[""]); n != 0 || err != nil {
		t.Fatalf("second prune should find nothing, got %d %v", n, err)
	}
}

func TestPruneCommentsErrors(t *testing.T) {
	ctx := context.Background()
	never := func(string) bool { return false }

	empty := review.NewService(&fakeSource{err: errors.New("git broke")}, newStore(t), nil, "b")
	empty.Exists = never
	if n, err := empty.PruneComments(ctx, nil); n != 0 || err != nil {
		t.Fatalf("no comments should skip the diff, got %d %v", n, err)
	}

	broken := review.NewService(&fakeSource{}, failingStore{viewedErr: errors.New("read failed")}, nil, "b")
	broken.Exists = never
	if _, err := broken.PruneComments(ctx, nil); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("comment read error not returned: %v", err)
	}

}

func TestParsePullRequestURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://github.com/o/r/pull/7":                   "o/r#7",
		"  http://www.github.com/o/r/pull/12/files?w=1  ": "o/r#12",
		"github.com/o/r/pull/3#discussion_r1":             "o/r#3",
		"https://github.com/o/r/pull/9/commits/abc1234":   "o/r#9",
	} {
		repo, number, err := review.ParsePullRequestURL(raw)
		if got := fmt.Sprintf("%s#%d", repo, number); err != nil || got != want {
			t.Errorf("%q = %s, %v, want %s", raw, got, err, want)
		}
	}
	for _, raw := range []string{
		"://x",
		"ftp://github.com/o/r/pull/7",
		"https://gitlab.com/o/r/pull/7",
		"https://github.com/o/r/issues/7",
		"https://github.com/o/r/pull",
		"https://github.com/o/r/pull/x",
		"https://github.com/o/r/pull/0",
		"https://github.com//r/pull/7",
		"o/r#7",
	} {
		if _, _, err := review.ParsePullRequestURL(raw); err == nil || !strings.Contains(err.Error(), "is not a GitHub pull request URL") {
			t.Errorf("%q should be rejected, err=%v", raw, err)
		}
	}
	if _, _, err := review.ParsePullRequestURL("  "); err == nil || err.Error() != "the pull request URL is empty" {
		t.Fatalf("empty err = %v", err)
	}
}

func TestSyncBranchFollowsTheCheckedOutBranch(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	if err := st.AddComment(ctx, "two", review.Comment{Path: "a.go", Body: "on two"}); err != nil {
		t.Fatal(err)
	}
	svc := review.NewService(&fakeSource{}, st, nil, "one")
	if got, err := svc.SyncBranch(ctx); err != nil || got != svc {
		t.Fatalf("without CurrentBranch got %p, %v, want the same service", got, err)
	}

	current := "one"
	svc.CurrentBranch = func(context.Context) (string, error) { return current, nil }
	if got, err := svc.SyncBranch(ctx); err != nil || got != svc {
		t.Fatalf("unchanged branch got %p, %v, want the same service", got, err)
	}

	current = "two"
	got, err := svc.SyncBranch(ctx)
	if err != nil || got.Branch() != "two" || svc.Branch() != "one" {
		t.Fatalf("got %v, %v, original %q", got, err, svc.Branch())
	}
	if comments, err := got.Comments(ctx); err != nil || len(comments) != 1 || comments[0].Body != "on two" {
		t.Fatalf("comments after switch = %+v, %v", comments, err)
	}
}

func TestSyncBranchWrapsErrors(t *testing.T) {
	gitErr := errors.New("not a repository")
	svc := review.NewService(&fakeSource{}, newStore(t), nil, "one")
	svc.CurrentBranch = func(context.Context) (string, error) { return "", gitErr }
	if got, err := svc.SyncBranch(context.Background()); !errors.Is(err, gitErr) || got != nil {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestEditCommentReplacesBodyAndResendsIt(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := review.NewService(&fakeSource{}, st, nil, "feature")
	if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Kind: "nit", Body: "old"}); err != nil {
		t.Fatal(err)
	}
	id := idOf(t, svc, "old")
	if err := st.UpdateComments(ctx, "feature", []string{id}, func(c *review.Comment) { c.Sent = true }); err != nil {
		t.Fatal(err)
	}

	if err := svc.EditComment(ctx, review.Comment{ID: id, Kind: "question", Body: "new"}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Comments(ctx)
	if len(got) != 1 || got[0].ID != id || got[0].Body != "new" || got[0].Kind != "question" || got[0].Sent {
		t.Fatalf("Comments after edit = %+v", got)
	}
}

func TestEditCommentRejectsEditsThatCannotBeSaved(t *testing.T) {
	ctx := context.Background()
	st := newStore(t)
	svc := review.NewService(&fakeSource{}, st, nil, "feature")
	if err := svc.AddComment(ctx, review.Comment{Path: "a.go", Body: "mine"}); err != nil {
		t.Fatal(err)
	}
	if err := st.AddComment(ctx, "feature", review.Comment{Path: "a.go", Author: "octocat", Body: "theirs"}); err != nil {
		t.Fatal(err)
	}
	mine, theirs := idOf(t, svc, "mine"), idOf(t, svc, "theirs")

	for name, c := range map[string]review.Comment{
		"empty body":     {ID: mine, Body: "  "},
		"unknown id":     {ID: "missing", Body: "x"},
		"someone else's": {ID: theirs, Body: "x"},
	} {
		if err := svc.EditComment(ctx, c); err == nil {
			t.Errorf("%s: EditComment should fail", name)
		}
	}
	if got, _ := svc.Comments(ctx); len(got) != 2 || got[0].Body != "mine" || got[1].Body != "theirs" {
		t.Fatalf("failed edits should leave comments unchanged: %+v", got)
	}
}
