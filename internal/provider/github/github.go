package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jfmainville/revunk/internal/review"
)

type Client struct {
	Run func(ctx context.Context, args ...string) ([]byte, error)
}

func gh(ctx context.Context, args ...string) ([]byte, error) {
	out, err := exec.CommandContext(ctx, "gh", args...).Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return nil, fmt.Errorf("gh %s: %s", args[0], bytes.TrimSpace(exitErr.Stderr))
		}
		return nil, fmt.Errorf("gh %s: %w", args[0], err)
	}
	return out, nil
}

func (c Client) run(ctx context.Context, args ...string) ([]byte, error) {
	if c.Run == nil {
		return gh(ctx, args...)
	}
	return c.Run(ctx, args...)
}

func (c Client) graphql(ctx context.Context, query string, vars map[string]any, out any) error {
	args := []string{"api", "graphql", "-f", "query=" + query}
	keys := make([]string, 0, len(vars))
	for k := range vars {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		switch v := vars[k].(type) {
		case string:
			args = append(args, "-f", k+"="+v)
		default:
			args = append(args, "-F", fmt.Sprintf("%s=%v", k, v))
		}
	}
	data, err := c.run(ctx, args...)
	if err != nil {
		return err
	}
	var resp struct {
		Data   json.RawMessage
		Errors []struct{ Message string }
	}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("read GitHub response: %w", err)
	}
	if len(resp.Errors) > 0 {
		return errors.New(resp.Errors[0].Message)
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(resp.Data, out)
}

func (c Client) BranchPullRequest(ctx context.Context) (review.PullRequest, error) {
	data, err := c.run(ctx, "pr", "view", "--json=url")
	if err != nil {
		return review.PullRequest{}, err
	}
	var found struct{ URL string }
	if err := json.Unmarshal(data, &found); err != nil {
		return review.PullRequest{}, fmt.Errorf("read pull request: %w", err)
	}
	u, err := url.Parse(found.URL)
	if err != nil {
		return review.PullRequest{}, err
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	number := 0
	if len(parts) == 4 && parts[2] == "pull" {
		number, _ = strconv.Atoi(parts[3])
	}
	if number == 0 {
		return review.PullRequest{}, fmt.Errorf("read pull request: unexpected url %q", found.URL)
	}
	return c.PullRequest(ctx, parts[0]+"/"+parts[1], number)
}

func (c Client) PullRequests(ctx context.Context, filter string) ([]review.PullRequest, error) {
	data, err := c.run(ctx, "search", "prs", filter, "--state=open", "--limit=50", "--json=number,title,url,isDraft,updatedAt,author,repository")
	if err != nil {
		return nil, err
	}
	var found []struct {
		Number     int
		Title      string
		URL        string
		IsDraft    bool
		UpdatedAt  time.Time
		Author     struct{ Login string }
		Repository struct{ NameWithOwner string }
	}
	if err := json.Unmarshal(data, &found); err != nil {
		return nil, fmt.Errorf("read pull requests: %w", err)
	}
	prs := make([]review.PullRequest, len(found))
	for i, f := range found {
		prs[i] = review.PullRequest{
			Repo: f.Repository.NameWithOwner, Number: f.Number, Title: f.Title, URL: f.URL,
			State: "OPEN", Draft: f.IsDraft, Author: f.Author.Login, UpdatedAt: f.UpdatedAt,
		}
	}
	return prs, nil
}

type login struct{ Login string }

type reactionGroup struct {
	Content          string
	ViewerHasReacted bool
	Reactors         struct{ TotalCount int }
}

type note struct {
	ID                string
	Author            login
	Body              string
	State             string
	CreatedAt         time.Time
	SubmittedAt       time.Time
	PullRequestReview struct{ State string }
	ReactionGroups    []reactionGroup
}

func (n note) note() review.Note {
	out := review.Note{ID: n.ID, Author: n.Author.Login, Body: n.Body, State: n.State, CreatedAt: n.CreatedAt}
	if n.PullRequestReview.State == "PENDING" {
		out.State = "PENDING"
	}
	if out.CreatedAt.IsZero() {
		out.CreatedAt = n.SubmittedAt
	}
	for _, g := range n.ReactionGroups {
		if g.Reactors.TotalCount > 0 || g.ViewerHasReacted {
			out.Reactions = append(out.Reactions, review.Reaction{Content: g.Content, Count: g.Reactors.TotalCount, Mine: g.ViewerHasReacted})
		}
	}
	return out
}

const noteFields = `id author { login } body createdAt reactionGroups { content viewerHasReacted reactors { totalCount } }`

const pullRequestQuery = `query($owner: String!, $name: String!, $number: Int!) {
  viewer { login }
  repository(owner: $owner, name: $name) {
    pullRequest(number: $number) {
      id number title body url state isDraft headRefName baseRefName updatedAt additions deletions changedFiles
      author { login }
      assignees(first: 50) { nodes { login } }
      reviewRequests(first: 50) { nodes { requestedReviewer { ... on User { login } ... on Team { slug } } } }
      latestReviews(first: 50) { nodes { author { login } state } }
      labels(first: 50) { nodes { name } }
      pending: reviews(states: PENDING, first: 1) { nodes { id } }
      reviews(first: 100) { nodes { id author { login } body state submittedAt reactionGroups { content viewerHasReacted reactors { totalCount } } } }
      comments(first: 100) { nodes { ` + noteFields + ` } }
      reviewThreads(first: 100) { nodes { id isResolved isOutdated path line startLine diffSide subjectType
        comments(first: 100) { nodes { ` + noteFields + ` pullRequestReview { state } } } } }
      commits(last: 100) { nodes { commit { oid messageHeadline } } }
      head: commits(last: 1) { nodes { commit { statusCheckRollup { contexts(first: 100) { nodes {
        ... on CheckRun { name status conclusion } ... on StatusContext { context state } } } } } } }
      files(first: 100) { nodes { path additions deletions changeType } }
    }
  }
}`

func splitRepo(repo string) (string, string, error) {
	owner, name, ok := strings.Cut(repo, "/")
	if !ok || owner == "" || name == "" {
		return "", "", fmt.Errorf("repository %q is not owner/name", repo)
	}
	return owner, name, nil
}

func (c Client) PullRequest(ctx context.Context, repo string, number int) (review.PullRequest, error) {
	owner, name, err := splitRepo(repo)
	if err != nil {
		return review.PullRequest{}, err
	}
	var data struct {
		Viewer     login
		Repository struct {
			PullRequest *struct {
				ID, Title, Body, URL, State string
				Number                      int
				IsDraft                     bool
				HeadRefName, BaseRefName    string
				UpdatedAt                   time.Time
				Additions, Deletions        int
				ChangedFiles                int
				Author                      login
				Assignees                   struct{ Nodes []login }
				ReviewRequests              struct {
					Nodes []struct{ RequestedReviewer struct{ Login, Slug string } }
				}
				LatestReviews struct {
					Nodes []struct {
						Author login
						State  string
					}
				}
				Labels        struct{ Nodes []struct{ Name string } }
				Pending       struct{ Nodes []struct{ ID string } }
				Reviews       struct{ Nodes []note }
				Comments      struct{ Nodes []note }
				ReviewThreads struct {
					Nodes []struct {
						ID              string
						IsResolved      bool
						IsOutdated      bool
						Path            string
						Line, StartLine int
						DiffSide        string
						SubjectType     string
						Comments        struct{ Nodes []note }
					}
				}
				Commits struct {
					Nodes []struct {
						Commit struct{ Oid, MessageHeadline string }
					}
				}
				Head struct {
					Nodes []struct {
						Commit struct {
							StatusCheckRollup *struct {
								Contexts struct {
									Nodes []struct{ Name, Status, Conclusion, Context, State string }
								}
							}
						}
					}
				}
				Files struct {
					Nodes []struct {
						Path                 string
						Additions, Deletions int
						ChangeType           string
					}
				}
			}
		}
	}
	if err := c.graphql(ctx, pullRequestQuery, map[string]any{"owner": owner, "name": name, "number": number}, &data); err != nil {
		return review.PullRequest{}, err
	}
	p := data.Repository.PullRequest
	if p == nil {
		return review.PullRequest{}, fmt.Errorf("no pull request %s#%d", repo, number)
	}
	pr := review.PullRequest{
		ID: p.ID, Repo: repo, Number: p.Number, Title: p.Title, Body: p.Body, URL: p.URL, State: p.State,
		Draft: p.IsDraft, Author: p.Author.Login, Head: p.HeadRefName, Base: p.BaseRefName, UpdatedAt: p.UpdatedAt,
		Additions: p.Additions, Deletions: p.Deletions, ChangedFiles: p.ChangedFiles, Viewer: data.Viewer.Login, Detailed: true,
	}
	for _, a := range p.Assignees.Nodes {
		pr.Assignees = append(pr.Assignees, a.Login)
	}
	for _, r := range p.LatestReviews.Nodes {
		pr.Reviewers = append(pr.Reviewers, review.Reviewer{Login: r.Author.Login, State: r.State})
	}
	for _, r := range p.ReviewRequests.Nodes {
		name := r.RequestedReviewer.Login
		if name == "" {
			name = r.RequestedReviewer.Slug
		}
		i := slices.IndexFunc(pr.Reviewers, func(rv review.Reviewer) bool { return rv.Login == name })
		if i < 0 {
			pr.Reviewers = append(pr.Reviewers, review.Reviewer{Login: name, State: "PENDING"})
		} else {
			pr.Reviewers[i].State = "PENDING"
		}
	}
	for _, l := range p.Labels.Nodes {
		pr.Labels = append(pr.Labels, l.Name)
	}
	if len(p.Pending.Nodes) > 0 {
		pr.PendingReview = p.Pending.Nodes[0].ID
	}
	for _, n := range p.Head.Nodes {
		if n.Commit.StatusCheckRollup == nil {
			continue
		}
		for _, ctx := range n.Commit.StatusCheckRollup.Contexts.Nodes {
			if ctx.Context != "" {
				pr.Checks = append(pr.Checks, review.Check{Name: ctx.Context, State: ctx.State})
				continue
			}
			state := ctx.Conclusion
			if ctx.Status != "COMPLETED" {
				state = ctx.Status
			}
			pr.Checks = append(pr.Checks, review.Check{Name: ctx.Name, State: state})
		}
	}
	for _, n := range p.Commits.Nodes {
		pr.Commits = append(pr.Commits, review.Commit{SHA: n.Commit.Oid, Subject: n.Commit.MessageHeadline})
	}
	for _, f := range p.Files.Nodes {
		pr.Files = append(pr.Files, review.File{Path: f.Path, Status: changeStatus(f.ChangeType), Additions: f.Additions, Deletions: f.Deletions})
	}
	for _, c := range p.Comments.Nodes {
		pr.Threads = append(pr.Threads, review.Thread{Notes: []review.Note{c.note()}})
	}
	for _, r := range p.Reviews.Nodes {
		if r.State != "PENDING" {
			pr.Threads = append(pr.Threads, review.Thread{Notes: []review.Note{r.note()}})
		}
	}
	for _, t := range p.ReviewThreads.Nodes {
		th := review.Thread{ID: t.ID, Path: t.Path, Side: review.SideRight, Start: t.StartLine, End: t.Line, Outdated: t.IsOutdated, Resolved: t.IsResolved}
		if t.DiffSide == "LEFT" {
			th.Side = review.SideLeft
		}
		if th.Start == 0 {
			th.Start = th.End
		}
		if t.SubjectType == "FILE" {
			th.Start, th.End = 0, 0
		}
		for _, c := range t.Comments.Nodes {
			th.Notes = append(th.Notes, c.note())
		}
		if len(th.Notes) > 0 {
			pr.Threads = append(pr.Threads, th)
		}
	}
	slices.SortStableFunc(pr.Threads, func(a, b review.Thread) int { return a.Notes[0].CreatedAt.Compare(b.Notes[0].CreatedAt) })
	return pr, nil
}

func changeStatus(s string) review.FileStatus {
	switch strings.ToLower(s) {
	case "added":
		return review.StatusAdded
	case "deleted", "removed":
		return review.StatusRemoved
	case "renamed":
		return review.StatusRenamed
	}
	return review.StatusModified
}

func number(pr review.PullRequest) string {
	return strconv.Itoa(pr.Number)
}

func (c Client) EditPullRequest(ctx context.Context, pr review.PullRequest, title, body string) error {
	if strings.TrimSpace(title) == "" {
		return errors.New("title is empty")
	}
	_, err := c.run(ctx, "pr", "edit", number(pr), "--repo", pr.Repo, "--title", title, "--body", body)
	return err
}

func (c Client) SetPeople(ctx context.Context, pr review.PullRequest, assignees, reviewers []string) error {
	var current []string
	for _, r := range pr.Reviewers {
		current = append(current, r.Login)
	}
	args := []string{"pr", "edit", number(pr), "--repo", pr.Repo}
	for _, change := range []struct {
		flag     string
		from, to []string
	}{{"assignee", pr.Assignees, assignees}, {"reviewer", current, reviewers}} {
		if added := missing(change.to, change.from); len(added) > 0 {
			args = append(args, "--add-"+change.flag, strings.Join(added, ","))
		}
		if removed := missing(change.from, change.to); len(removed) > 0 {
			args = append(args, "--remove-"+change.flag, strings.Join(removed, ","))
		}
	}
	if len(args) == 5 {
		return nil
	}
	_, err := c.run(ctx, args...)
	return err
}

func missing(want, have []string) []string {
	var out []string
	for _, w := range want {
		if !slices.Contains(have, w) {
			out = append(out, w)
		}
	}
	return out
}

func (c Client) Comment(ctx context.Context, pr review.PullRequest, body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("comment is empty")
	}
	_, err := c.run(ctx, "pr", "comment", number(pr), "--repo", pr.Repo, "--body", body)
	return err
}

func (c Client) Reply(ctx context.Context, threadID, body string) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("reply is empty")
	}
	return c.graphql(ctx, `mutation($thread: ID!, $body: String!) {
  addPullRequestReviewThreadReply(input: {pullRequestReviewThreadId: $thread, body: $body}) { clientMutationId }
}`, map[string]any{"thread": threadID, "body": body}, nil)
}

func (c Client) React(ctx context.Context, subjectID, content string, add bool) error {
	mutation := "addReaction"
	if !add {
		mutation = "removeReaction"
	}
	return c.graphql(ctx, `mutation($subject: ID!, $content: ReactionContent!) {
  `+mutation+`(input: {subjectId: $subject, content: $content}) { clientMutationId }
}`, map[string]any{"subject": subjectID, "content": content}, nil)
}

func (c Client) Resolve(ctx context.Context, threadID string, resolved bool) error {
	mutation := "resolveReviewThread"
	if !resolved {
		mutation = "unresolveReviewThread"
	}
	return c.graphql(ctx, `mutation($thread: ID!) {
  `+mutation+`(input: {threadId: $thread}) { clientMutationId }
}`, map[string]any{"thread": threadID}, nil)
}

func (c Client) SubmitReview(ctx context.Context, pr review.PullRequest, event review.ReviewEvent, body string) error {
	if pr.PendingReview != "" {
		return c.graphql(ctx, `mutation($review: ID!, $event: PullRequestReviewEvent!, $body: String!) {
  submitPullRequestReview(input: {pullRequestReviewId: $review, event: $event, body: $body}) { clientMutationId }
}`, map[string]any{"review": pr.PendingReview, "event": string(event), "body": body}, nil)
	}
	return c.graphql(ctx, `mutation($pr: ID!, $event: PullRequestReviewEvent!, $body: String!) {
  addPullRequestReview(input: {pullRequestId: $pr, event: $event, body: $body}) { clientMutationId }
}`, map[string]any{"pr": pr.ID, "event": string(event), "body": body}, nil)
}

func (c Client) startReview(ctx context.Context, pr review.PullRequest) (string, error) {
	if pr.PendingReview != "" {
		return pr.PendingReview, nil
	}
	var data struct {
		AddPullRequestReview struct {
			PullRequestReview struct{ ID string }
		}
	}
	err := c.graphql(ctx, `mutation($pr: ID!) {
  addPullRequestReview(input: {pullRequestId: $pr}) { pullRequestReview { id } }
}`, map[string]any{"pr": pr.ID}, &data)
	return data.AddPullRequestReview.PullRequestReview.ID, err
}

func (c Client) addThread(ctx context.Context, pr review.PullRequest, cm review.Comment) error {
	reviewID, err := c.startReview(ctx, pr)
	if err != nil {
		return err
	}
	vars := map[string]any{"review": reviewID, "path": cm.Path, "body": cm.Body}
	params := "$review: ID!, $path: String!, $body: String!"
	input := "pullRequestReviewId: $review, path: $path, body: $body"
	if cm.FileLevel() {
		input += ", subjectType: FILE"
	} else {
		side := "RIGHT"
		if cm.Side == review.SideLeft {
			side = "LEFT"
		}
		vars["line"], vars["side"] = cm.End, side
		params += ", $line: Int!, $side: DiffSide!"
		input += ", line: $line, side: $side"
		if cm.Start != cm.End {
			vars["start"] = cm.Start
			params += ", $start: Int!"
			input += ", startLine: $start, startSide: $side"
		}
	}
	return c.graphql(ctx, "mutation("+params+") {\n  addPullRequestReviewThread(input: {"+input+"}) { clientMutationId }\n}", vars, nil)
}

func (c Client) deleteComment(ctx context.Context, id string) error {
	return c.graphql(ctx, `mutation($id: ID!) {
  deletePullRequestReviewComment(input: {id: $id}) { clientMutationId }
}`, map[string]any{"id": id}, nil)
}

func apiList[T any](ctx context.Context, c Client, path string) ([]T, error) {
	data, err := c.run(ctx, "api", "--paginate", path)
	if err != nil {
		return nil, err
	}
	var all []T
	dec := json.NewDecoder(bytes.NewReader(data))
	for {
		var page []T
		err := dec.Decode(&page)
		if errors.Is(err, io.EOF) {
			return all, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		all = append(all, page...)
	}
}
