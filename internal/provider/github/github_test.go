package github

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jfmainville/revunk/internal/review"
)

type fakeGH struct {
	calls   [][]string
	replies []string
	err     error
}

func (f *fakeGH) run(_ context.Context, args ...string) ([]byte, error) {
	f.calls = append(f.calls, args)
	if f.err != nil {
		return nil, f.err
	}
	if len(f.replies) == 0 {
		return []byte(`{"data":{}}`), nil
	}
	r := f.replies[0]
	f.replies = f.replies[1:]
	return []byte(r), nil
}

func (f *fakeGH) client() Client {
	return Client{Run: f.run}
}

func arg(call []string, prefix string) string {
	for _, a := range call {
		if v, ok := strings.CutPrefix(a, prefix); ok {
			return v
		}
	}
	return ""
}

const detailJSON = `{"data":{"viewer":{"login":"me"},"repository":{"mergeCommitAllowed":true,"squashMergeAllowed":false,"rebaseMergeAllowed":true,"pullRequest":{
  "id":"PR_1","number":7,"title":"Add login","body":"Adds **login**","url":"https://github.com/o/r/pull/7","state":"OPEN","isDraft":true,"mergeStateStatus":"BLOCKED",
  "headRefName":"login","baseRefName":"main","updatedAt":"2026-11-16T10:00:00Z","additions":10,"deletions":2,"changedFiles":3,
  "author":{"login":"ana"},
  "assignees":{"nodes":[{"login":"me"}]},
  "reviewRequests":{"nodes":[{"requestedReviewer":{"login":"bob"}},{"requestedReviewer":{"slug":"core"}}]},
  "latestReviews":{"nodes":[{"author":{"login":"bob"},"state":"CHANGES_REQUESTED"},{"author":{"login":"cy"},"state":"APPROVED"}]},
  "labels":{"nodes":[{"name":"auth"}]},
  "pending":{"nodes":[{"id":"PRR_pending"}]},
  "reviews":{"nodes":[
    {"id":"PRR_1","author":{"login":"cy"},"body":"","state":"APPROVED","submittedAt":"2026-11-16T09:00:00Z","reactionGroups":[]},
    {"id":"PRR_pending","author":{"login":"me"},"body":"","state":"PENDING","submittedAt":null,"reactionGroups":[]}]},
  "comments":{"nodes":[{"id":"IC_1","author":{"login":"ana"},"body":"Ready","createdAt":"2026-11-16T08:00:00Z",
    "reactionGroups":[{"content":"THUMBS_UP","viewerHasReacted":true,"reactors":{"totalCount":2}},{"content":"EYES","viewerHasReacted":false,"reactors":{"totalCount":0}}]}]},
  "reviewThreads":{"nodes":[
    {"id":"T_1","isResolved":true,"isOutdated":false,"path":"a.go","line":12,"startLine":10,"diffSide":"RIGHT","subjectType":"LINE",
     "comments":{"nodes":[
       {"id":"C_1","author":{"login":"bob"},"body":"Rename","createdAt":"2026-11-16T09:30:00Z","pullRequestReview":{"state":"COMMENTED"},"reactionGroups":[]},
       {"id":"C_2","author":{"login":"ana"},"body":"Done","createdAt":"2026-11-16T09:40:00Z","pullRequestReview":{"state":"COMMENTED"},"reactionGroups":[]}]}},
    {"id":"T_2","isResolved":false,"isOutdated":false,"path":"b.go","line":3,"startLine":null,"diffSide":"LEFT","subjectType":"LINE",
     "comments":{"nodes":[{"id":"C_3","author":{"login":"me"},"body":"` + "```suggestion\\nx\\n```" + `","createdAt":"2026-11-16T09:50:00Z","pullRequestReview":{"state":"PENDING"},"reactionGroups":[]}]}},
    {"id":"T_3","isResolved":false,"isOutdated":false,"path":"c.go","line":null,"startLine":null,"diffSide":"RIGHT","subjectType":"FILE",
     "comments":{"nodes":[{"id":"C_4","author":{"login":"bob"},"body":"Whole file","createdAt":"2026-11-16T09:55:00Z","pullRequestReview":{"state":"COMMENTED"},"reactionGroups":[]}]}},
    {"id":"T_4","isResolved":false,"isOutdated":true,"path":"d.go","line":null,"startLine":null,"diffSide":"RIGHT","subjectType":"LINE",
     "comments":{"nodes":[{"id":"C_5","author":{"login":"bob"},"body":"Old","createdAt":"2026-11-16T07:00:00Z","pullRequestReview":{"state":"COMMENTED"},"reactionGroups":[]}]}},
    {"id":"T_5","isResolved":false,"isOutdated":false,"path":"e.go","line":1,"startLine":null,"diffSide":"RIGHT","subjectType":"LINE","comments":{"nodes":[]}}]},
  "commits":{"nodes":[{"commit":{"oid":"abc1234567","messageHeadline":"add login"}}]},
  "head":{"nodes":[{"commit":{"statusCheckRollup":{"contexts":{"nodes":[
    {"name":"build","status":"COMPLETED","conclusion":"SUCCESS"},
    {"name":"test","status":"IN_PROGRESS","conclusion":""},
    {"context":"ci/legacy","state":"FAILURE"}]}}}}]},
  "files":{"nodes":[{"path":"a.go","additions":8,"deletions":2,"changeType":"MODIFIED"},{"path":"n.go","additions":2,"deletions":0,"changeType":"ADDED"}]}
}}}}`

func TestPullRequestMapsEveryDetail(t *testing.T) {
	f := &fakeGH{replies: []string{detailJSON}}
	pr, err := f.client().PullRequest(context.Background(), "o/r", 7)
	if err != nil {
		t.Fatal(err)
	}
	call := f.calls[0]
	if call[0] != "api" || call[1] != "graphql" || arg(call, "owner=") != "o" || arg(call, "name=") != "r" || arg(call, "number=") != "7" {
		t.Fatalf("call = %q", call)
	}
	if pr.ID != "PR_1" || pr.Repo != "o/r" || pr.Title != "Add login" || !pr.Draft || pr.Author != "ana" || pr.Head != "login" || pr.Base != "main" ||
		pr.Additions != 10 || pr.Deletions != 2 || pr.ChangedFiles != 3 || pr.Viewer != "me" || pr.PendingReview != "PRR_pending" || !pr.Detailed {
		t.Fatalf("pr = %+v", pr)
	}
	if pr.MergeState != "BLOCKED" || !reflect.DeepEqual(pr.MergeMethods, []review.MergeMethod{review.MergeCommit, review.MergeRebase}) {
		t.Fatalf("merge state %q methods %q should follow the repository settings", pr.MergeState, pr.MergeMethods)
	}
	if !reflect.DeepEqual(pr.Assignees, []string{"me"}) || !reflect.DeepEqual(pr.Labels, []string{"auth"}) {
		t.Fatalf("assignees %q labels %q", pr.Assignees, pr.Labels)
	}
	wantReviewers := []review.Reviewer{{Login: "bob", State: "PENDING"}, {Login: "cy", State: "APPROVED"}, {Login: "core", State: "PENDING"}}
	if !reflect.DeepEqual(pr.Reviewers, wantReviewers) {
		t.Fatalf("reviewers = %+v", pr.Reviewers)
	}
	wantChecks := []review.Check{{Name: "build", State: "SUCCESS"}, {Name: "test", State: "IN_PROGRESS"}, {Name: "ci/legacy", State: "FAILURE"}}
	if !reflect.DeepEqual(pr.Checks, wantChecks) {
		t.Fatalf("checks = %+v", pr.Checks)
	}
	if !reflect.DeepEqual(pr.Commits, []review.Commit{{SHA: "abc1234567", Subject: "add login"}}) {
		t.Fatalf("commits = %+v", pr.Commits)
	}
	if pr.Files[0].Status != review.StatusModified || pr.Files[1].Status != review.StatusAdded || pr.Files[1].Additions != 2 {
		t.Fatalf("files = %+v", pr.Files)
	}

	var first []string
	for _, th := range pr.Threads {
		first = append(first, th.Notes[0].ID)
	}
	if want := []string{"C_5", "IC_1", "PRR_1", "C_1", "C_3", "C_4"}; !reflect.DeepEqual(first, want) {
		t.Fatalf("threads should be sorted by time without the pending review or empty threads: %q", first)
	}
	byID := map[string]review.Thread{}
	for _, th := range pr.Threads {
		byID[th.ID] = th
	}
	if th := byID["T_1"]; th.Start != 10 || th.End != 12 || th.Side != review.SideRight || !th.Resolved || len(th.Notes) != 2 {
		t.Fatalf("range thread = %+v", th)
	}
	if th := byID["T_2"]; th.Start != 3 || th.End != 3 || th.Side != review.SideLeft || th.Notes[0].State != "PENDING" {
		t.Fatalf("single line thread = %+v", th)
	}
	if th := byID["T_3"]; th.Start != 0 || th.End != 0 {
		t.Fatalf("file thread = %+v", th)
	}
	if !byID["T_4"].Outdated {
		t.Fatal("outdated flag lost")
	}
	conversation := pr.Threads[1].Notes[0]
	if want := []review.Reaction{{Content: "THUMBS_UP", Count: 2, Mine: true}}; !reflect.DeepEqual(conversation.Reactions, want) {
		t.Fatalf("reactions should drop empty groups: %+v", conversation.Reactions)
	}
	if r := pr.Threads[2].Notes[0]; r.State != "APPROVED" || !r.CreatedAt.Equal(time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)) {
		t.Fatalf("review note should use submittedAt: %+v", r)
	}
}

func TestPullRequestErrors(t *testing.T) {
	if _, err := (&fakeGH{}).client().PullRequest(context.Background(), "norepo", 1); err == nil || !strings.Contains(err.Error(), "owner/name") {
		t.Fatalf("bad repo error = %v", err)
	}
	f := &fakeGH{replies: []string{`{"data":{"viewer":{"login":"me"},"repository":{"pullRequest":null}}}`}}
	if _, err := f.client().PullRequest(context.Background(), "o/r", 9); err == nil || !strings.Contains(err.Error(), "no pull request o/r#9") {
		t.Fatalf("missing error = %v", err)
	}
	f = &fakeGH{replies: []string{`{"errors":[{"message":"Could not resolve to a Repository"}]}`}}
	if _, err := f.client().PullRequest(context.Background(), "o/r", 9); err == nil || err.Error() != "Could not resolve to a Repository" {
		t.Fatalf("graphql error = %v", err)
	}
	f = &fakeGH{replies: []string{`not json`}}
	if _, err := f.client().PullRequest(context.Background(), "o/r", 9); err == nil || !strings.Contains(err.Error(), "read GitHub response") {
		t.Fatalf("bad json error = %v", err)
	}
	f = &fakeGH{err: errors.New("gh api: HTTP 401")}
	if _, err := f.client().PullRequest(context.Background(), "o/r", 9); err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("run error = %v", err)
	}
}

func TestPullRequestsSearchesWithTheFilter(t *testing.T) {
	f := &fakeGH{replies: []string{`[{"number":3,"title":"Fix","url":"u","isDraft":true,"updatedAt":"2026-11-16T10:00:00Z","author":{"login":"ana"},"repository":{"nameWithOwner":"o/r"}}]`}}
	prs, err := f.client().PullRequests(context.Background(), "assignee:@me")
	if err != nil {
		t.Fatal(err)
	}
	if call := f.calls[0]; call[0] != "search" || call[1] != "prs" || call[2] != "assignee:@me" || !slices.Contains(call, "--state=open") {
		t.Fatalf("call = %q", call)
	}
	want := review.PullRequest{Repo: "o/r", Number: 3, Title: "Fix", URL: "u", State: "OPEN", Draft: true, Author: "ana", UpdatedAt: time.Date(2026, 11, 16, 10, 0, 0, 0, time.UTC)}
	if len(prs) != 1 || !reflect.DeepEqual(prs[0], want) {
		t.Fatalf("prs = %+v", prs)
	}
	if _, err := (&fakeGH{replies: []string{"{"}}).client().PullRequests(context.Background(), "x"); err == nil {
		t.Fatal("expected a parse error")
	}
	if _, err := (&fakeGH{err: errors.New("offline")}).client().PullRequests(context.Background(), "x"); err == nil {
		t.Fatal("expected the run error")
	}
}

var pr = review.PullRequest{ID: "PR_1", Repo: "o/r", Number: 7, Assignees: []string{"me", "old"}, Reviewers: []review.Reviewer{{Login: "bob"}}}

func TestEditPullRequest(t *testing.T) {
	f := &fakeGH{}
	if err := f.client().EditPullRequest(context.Background(), pr, "  ", "b"); err == nil || len(f.calls) > 0 {
		t.Fatal("an empty title should be refused before calling gh")
	}
	if err := f.client().EditPullRequest(context.Background(), pr, "New", "Body"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"pr", "edit", "7", "--repo", "o/r", "--title", "New", "--body", "Body"}; !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("call = %q", f.calls[0])
	}
}

func TestSetPeopleOnlySendsChanges(t *testing.T) {
	f := &fakeGH{}
	if err := f.client().SetPeople(context.Background(), pr, []string{"me", "old"}, []string{"bob"}); err != nil || len(f.calls) != 0 {
		t.Fatalf("no change should not call gh: %v %q", err, f.calls)
	}
	if err := f.client().SetPeople(context.Background(), pr, []string{"me", "new"}, []string{"cy"}); err != nil {
		t.Fatal(err)
	}
	want := []string{"pr", "edit", "7", "--repo", "o/r", "--add-assignee", "new", "--remove-assignee", "old", "--add-reviewer", "cy", "--remove-reviewer", "bob"}
	if !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("call = %q", f.calls[0])
	}
}

func TestCommentReplyReactResolve(t *testing.T) {
	ctx := context.Background()
	f := &fakeGH{}
	c := f.client()
	if c.Comment(ctx, pr, " ") == nil || c.Reply(ctx, "T", "") == nil || len(f.calls) > 0 {
		t.Fatal("empty bodies should be refused before calling gh")
	}
	if err := c.Comment(ctx, pr, "hi"); err != nil {
		t.Fatal(err)
	}
	if want := []string{"pr", "comment", "7", "--repo", "o/r", "--body", "hi"}; !reflect.DeepEqual(f.calls[0], want) {
		t.Fatalf("comment call = %q", f.calls[0])
	}
	steps := []struct {
		do   func() error
		want []string
	}{
		{func() error { return c.Reply(ctx, "T_1", "ok") }, []string{"addPullRequestReviewThreadReply", "thread=T_1", "body=ok"}},
		{func() error { return c.React(ctx, "C_1", "HEART", true) }, []string{"addReaction", "subject=C_1", "content=HEART"}},
		{func() error { return c.React(ctx, "C_1", "HEART", false) }, []string{"removeReaction"}},
		{func() error { return c.Resolve(ctx, "T_1", true) }, []string{"resolveReviewThread", "thread=T_1"}},
		{func() error { return c.Resolve(ctx, "T_1", false) }, []string{"unresolveReviewThread"}},
	}
	for i, s := range steps {
		if err := s.do(); err != nil {
			t.Fatal(err)
		}
		call := strings.Join(f.calls[i+1], "\n")
		for _, w := range s.want {
			if !strings.Contains(call, w) {
				t.Errorf("step %d: call missing %q:\n%s", i, w, call)
			}
		}
	}
	if !strings.Contains(strings.Join(f.calls[3], " "), "removeReaction(") || strings.Contains(strings.Join(f.calls[3], " "), "addReaction(") {
		t.Fatal("removing a reaction should not add one")
	}
	if strings.Contains(strings.Join(f.calls[4], " "), "unresolve") {
		t.Fatal("resolving should not unresolve")
	}
}

func TestSubmitReviewUsesThePendingReview(t *testing.T) {
	ctx := context.Background()
	f := &fakeGH{}
	pending := pr
	pending.PendingReview = "PRR_9"
	if err := f.client().SubmitReview(ctx, pending, review.EventApprove, "LGTM"); err != nil {
		t.Fatal(err)
	}
	if err := f.client().SubmitReview(ctx, pr, review.EventRequestChanges, "Fix it"); err != nil {
		t.Fatal(err)
	}
	first, second := strings.Join(f.calls[0], "\n"), strings.Join(f.calls[1], "\n")
	if !strings.Contains(first, "submitPullRequestReview") || !strings.Contains(first, "review=PRR_9") || !strings.Contains(first, "event=APPROVE") || !strings.Contains(first, "body=LGTM") {
		t.Fatalf("pending submit = %s", first)
	}
	if !strings.Contains(second, "addPullRequestReview(") || !strings.Contains(second, "pr=PR_1") || !strings.Contains(second, "event=REQUEST_CHANGES") {
		t.Fatalf("new review submit = %s", second)
	}
}

func TestMergePinsTheReviewedHeadCommit(t *testing.T) {
	ctx := context.Background()
	f := &fakeGH{}
	reviewed := pr
	reviewed.Commits = []review.Commit{{SHA: "old1"}, {SHA: "head2"}}
	if err := f.client().Merge(ctx, reviewed, review.MergeSquash); err != nil {
		t.Fatal(err)
	}
	call := strings.Join(f.calls[0], "\n")
	if !strings.Contains(call, "mergePullRequest(") || !strings.Contains(call, "pr=PR_1") || !strings.Contains(call, "method=SQUASH") ||
		!strings.Contains(call, "expectedHeadOid: $head") || !strings.Contains(call, "head=head2") {
		t.Fatalf("merge call = %s", call)
	}
	if err := f.client().Merge(ctx, pr, review.MergeCommit); err != nil {
		t.Fatal(err)
	}
	if call := strings.Join(f.calls[1], "\n"); strings.Contains(call, "expectedHeadOid") || !strings.Contains(call, "method=MERGE") {
		t.Fatalf("a pull request without commits should merge without a pinned head: %s", call)
	}
	f = &fakeGH{replies: []string{`{"errors":[{"message":"Pull request is not mergeable"}]}`}}
	if err := f.client().Merge(ctx, reviewed, review.MergeRebase); err == nil || err.Error() != "Pull request is not mergeable" {
		t.Fatalf("GitHub's refusal should come back as the error, got %v", err)
	}
	f = &fakeGH{err: errors.New("gh api: HTTP 403")}
	if err := f.client().Merge(ctx, reviewed, review.MergeRebase); err == nil || !strings.Contains(err.Error(), "403") {
		t.Fatalf("permission errors should come back, got %v", err)
	}
}

func TestReviewCommentsComeFromThreadsOnCode(t *testing.T) {
	f := &fakeGH{replies: []string{detailJSON}}
	r := Review{Client: f.client(), Repo: "o/r", Number: 7}
	comments, err := r.Comments(context.Background(), "ignored")
	if err != nil {
		t.Fatal(err)
	}
	want := []review.Comment{
		{ID: "C_1", Kind: "comment", Path: "a.go", Side: review.SideRight, Start: 10, End: 12, Body: "Rename", Sent: true, Author: "bob", Resolved: true, Replies: []string{"**@ana** Done"}},
		{ID: "C_3", Kind: "suggestion", Path: "b.go", Side: review.SideLeft, Start: 3, End: 3, Body: "```suggestion\nx\n```", Author: "me"},
		{ID: "C_4", Kind: "comment", Path: "c.go", Side: review.SideRight, Body: "Whole file", Sent: true, Author: "bob"},
	}
	if !reflect.DeepEqual(comments, want) {
		t.Fatalf("comments =\n%+v\nwant\n%+v", comments, want)
	}
	if _, err := (Review{Client: (&fakeGH{err: errors.New("offline")}).client(), Repo: "o/r"}).Comments(context.Background(), ""); err == nil {
		t.Fatal("expected the fetch error")
	}
}

func TestReviewAddCommentStartsAPendingReviewWhenNeeded(t *testing.T) {
	noPending := strings.Replace(detailJSON, `"pending":{"nodes":[{"id":"PRR_pending"}]}`, `"pending":{"nodes":[]}`, 1)
	f := &fakeGH{replies: []string{noPending, `{"data":{"addPullRequestReview":{"pullRequestReview":{"id":"PRR_new"}}}}`}}
	r := Review{Client: f.client(), Repo: "o/r", Number: 7}
	err := r.AddComment(context.Background(), "", review.Comment{Path: "a.go", Side: review.SideLeft, Start: 4, End: 6, Body: "why?"})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 3 || !strings.Contains(strings.Join(f.calls[1], " "), "addPullRequestReview(input: {pullRequestId: $pr})") {
		t.Fatalf("calls = %q", f.calls)
	}
	thread := strings.Join(f.calls[2], "\n")
	for _, w := range []string{"addPullRequestReviewThread", "review=PRR_new", "path=a.go", "body=why?", "line=6", "start=4", "side=LEFT", "startLine: $start"} {
		if !strings.Contains(thread, w) {
			t.Errorf("thread call missing %q:\n%s", w, thread)
		}
	}

	f = &fakeGH{replies: []string{detailJSON}}
	r.Client = f.client()
	if err := r.AddComment(context.Background(), "", review.Comment{Path: "c.go", Body: "file"}); err != nil {
		t.Fatal(err)
	}
	thread = strings.Join(f.calls[1], "\n")
	if len(f.calls) != 2 || !strings.Contains(thread, "review=PRR_pending") || !strings.Contains(thread, "subjectType: FILE") || strings.Contains(thread, "line=") {
		t.Fatalf("file comment should reuse the pending review: %q", f.calls)
	}

	f = &fakeGH{replies: []string{detailJSON}}
	r.Client = f.client()
	if err := r.AddComment(context.Background(), "", review.Comment{Path: "a.go", Side: review.SideRight, Start: 2, End: 2, Body: "x"}); err != nil {
		t.Fatal(err)
	}
	if thread = strings.Join(f.calls[1], "\n"); !strings.Contains(thread, "side=RIGHT") || strings.Contains(thread, "startLine") {
		t.Fatalf("single line comment = %s", thread)
	}

	r.Client = (&fakeGH{err: errors.New("offline")}).client()
	if err := r.AddComment(context.Background(), "", review.Comment{Path: "a.go", Body: "x"}); err == nil {
		t.Fatal("expected the fetch error")
	}
	f = &fakeGH{replies: []string{noPending, `{"errors":[{"message":"forbidden"}]}`}}
	r.Client = f.client()
	if err := r.AddComment(context.Background(), "", review.Comment{Path: "a.go", Body: "x"}); err == nil || err.Error() != "forbidden" || len(f.calls) != 2 {
		t.Fatalf("start review error = %v after %d calls", err, len(f.calls))
	}
}

func TestReviewDeleteAndUpdate(t *testing.T) {
	f := &fakeGH{}
	r := Review{Client: f.client()}
	if err := r.DeleteComments(context.Background(), "", []string{"C_1", "C_2"}); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 2 || !strings.Contains(strings.Join(f.calls[1], " "), "id=C_2") || !strings.Contains(strings.Join(f.calls[0], " "), "deletePullRequestReviewComment") {
		t.Fatalf("calls = %q", f.calls)
	}
	f = &fakeGH{err: errors.New("not yours")}
	r.Client = f.client()
	if err := r.DeleteComments(context.Background(), "", []string{"a", "b"}); err == nil || len(f.calls) != 1 {
		t.Fatal("deleting should stop at the first error")
	}
	if err := r.UpdateComments(context.Background(), "", nil, nil); err == nil {
		t.Fatal("updating comments is not supported")
	}
}

func TestSourceReadsCommitsAndFilesAcrossPages(t *testing.T) {
	f := &fakeGH{replies: []string{
		`[{"sha":"aaa","commit":{"message":"first\n\nbody"}}][{"sha":"bbb","commit":{"message":"second"}}]`,
		`[{"filename":"a.go","status":"modified","additions":1,"deletions":1,"patch":"@@ -1 +1 @@\n-a\n+b"}]` + "\n" + `[{"filename":"b.go","status":"removed","additions":0,"deletions":2}]`,
		`{"files":[{"filename":"c.go","status":"renamed","additions":0,"deletions":0}]}`,
	}}
	s := Source{Client: f.client(), Repo: "o/r", Number: 7}
	commits, err := s.Commits(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(commits, []review.Commit{{SHA: "aaa", Subject: "first"}, {SHA: "bbb", Subject: "second"}}) {
		t.Fatalf("commits = %+v", commits)
	}
	files, err := s.Files(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	want := []review.File{{Path: "a.go", Status: review.StatusModified, Additions: 1, Deletions: 1, Patch: "@@ -1 +1 @@\n-a\n+b"}, {Path: "b.go", Status: review.StatusRemoved, Deletions: 2}}
	if !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %+v", files)
	}
	files, err = s.Files(context.Background(), "ccc")
	if err != nil || len(files) != 1 || files[0].Status != review.StatusRenamed {
		t.Fatalf("commit files = %+v %v", files, err)
	}
	if got := []string{f.calls[0][2], f.calls[1][2], f.calls[2][1]}; !reflect.DeepEqual(got, []string{"repos/o/r/pulls/7/commits?per_page=100", "repos/o/r/pulls/7/files?per_page=100", "repos/o/r/commits/ccc"}) {
		t.Fatalf("paths = %q", got)
	}
}

func TestSourceErrors(t *testing.T) {
	bad := Source{Client: (&fakeGH{replies: []string{"[1]", "nope", "nope"}}).client(), Repo: "o/r"}
	if _, err := bad.Commits(context.Background()); err == nil {
		t.Fatal("expected a parse error for commits")
	}
	if _, err := bad.Files(context.Background(), ""); err == nil {
		t.Fatal("expected a parse error for files")
	}
	if _, err := bad.Files(context.Background(), "x"); err == nil {
		t.Fatal("expected a parse error for a commit")
	}
	offline := Source{Client: (&fakeGH{err: errors.New("offline")}).client()}
	if _, err := offline.Commits(context.Background()); err == nil {
		t.Fatal("expected the run error")
	}
	if _, err := offline.Files(context.Background(), "x"); err == nil {
		t.Fatal("expected the run error")
	}
}

func TestChangeStatus(t *testing.T) {
	for in, want := range map[string]review.FileStatus{"ADDED": review.StatusAdded, "removed": review.StatusRemoved, "DELETED": review.StatusRemoved, "renamed": review.StatusRenamed, "copied": review.StatusModified, "": review.StatusModified} {
		if got := changeStatus(in); got != want {
			t.Errorf("changeStatus(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGHRunsTheCLIAndReportsStderr(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", dir)
	if _, err := (Client{}).PullRequests(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "gh search") {
		t.Fatalf("missing gh error = %v", err)
	}
	script := "#!/bin/sh\nif [ \"$1\" = search ]; then echo '[]'; exit 0; fi\necho 'HTTP 404: Not Found' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(dir, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if prs, err := (Client{}).PullRequests(context.Background(), "x"); err != nil || len(prs) != 0 {
		t.Fatalf("prs = %v, %v", prs, err)
	}
	if err := (Client{}).Comment(context.Background(), pr, "x"); err == nil || err.Error() != "gh pr: HTTP 404: Not Found" {
		t.Fatalf("stderr error = %v", err)
	}
}

func TestBranchPullRequestLoadsThePullRequestOfTheBranch(t *testing.T) {
	gh := &fakeGH{replies: []string{`{"url":"https://github.com/o/r/pull/7"}`, detailJSON}}
	pr, err := gh.client().BranchPullRequest(context.Background())
	if err != nil || pr.Repo != "o/r" || pr.Number != 7 || pr.Title != "Add login" {
		t.Fatalf("pr=%+v err=%v", pr, err)
	}
	if want := []string{"pr", "view", "--json=url"}; !slices.Equal(gh.calls[0], want) || arg(gh.calls[1], "owner=") != "o" || arg(gh.calls[1], "number=") != "7" {
		t.Fatalf("calls=%q", gh.calls)
	}
}

func TestBranchPullRequestErrors(t *testing.T) {
	for name, reply := range map[string]string{
		"bad json":   `nope`,
		"bad url":    `{"url":"://x"}`,
		"not a pull": `{"url":"https://github.com/o/r/issues/7"}`,
		"no number":  `{"url":"https://github.com/o/r/pull/x"}`,
		"other host": `{"url":"https://ghe.example.com/o/r/pull/7"}`,
	} {
		gh := &fakeGH{replies: []string{reply}}
		if _, err := gh.client().BranchPullRequest(context.Background()); err == nil || len(gh.calls) != 1 {
			t.Errorf("%s should fail without loading a pull request, err=%v", name, err)
		}
	}
	gh := &fakeGH{err: errors.New("no pull requests found for branch")}
	if _, err := gh.client().BranchPullRequest(context.Background()); err == nil {
		t.Fatal("gh errors should be returned")
	}
}

func TestAssignableListsEveryPage(t *testing.T) {
	gh := &fakeGH{replies: []string{`[{"login":"ana"}][{"login":"bob"}]`}}
	got, err := gh.client().Assignable(context.Background(), "o/r")
	if err != nil || !slices.Equal(got, []string{"ana", "bob"}) || !slices.Contains(gh.calls[0], "repos/o/r/assignees?per_page=100") {
		t.Fatalf("got=%q err=%v calls=%q", got, err, gh.calls)
	}
	if _, err := (&fakeGH{err: errors.New("boom")}).client().Assignable(context.Background(), "o/r"); err == nil {
		t.Fatal("gh errors should be returned")
	}
}
