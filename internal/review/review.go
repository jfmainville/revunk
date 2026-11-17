package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

type FileStatus string

const (
	StatusAdded    FileStatus = "added"
	StatusModified FileStatus = "modified"
	StatusRemoved  FileStatus = "removed"
	StatusRenamed  FileStatus = "renamed"
)

type Commit struct {
	SHA     string
	Subject string
}

func (c Commit) Short() string {
	return c.SHA[:min(7, len(c.SHA))]
}

type File struct {
	Path      string
	Status    FileStatus
	Additions int
	Deletions int
	Patch     string
}

type LineKind int

const (
	LineContext LineKind = iota
	LineAdded
	LineRemoved
	LineHunk
)

type Line struct {
	Kind LineKind
	Old  int
	New  int
	Text string
}

func ParseLines(patch string) []Line {
	var lines []Line
	oldNo, newNo := 0, 0
	for raw := range strings.SplitSeq(strings.TrimSuffix(patch, "\n"), "\n") {
		switch {
		case strings.HasPrefix(raw, "@@"):
			oldNo, newNo = hunkStart(raw)
			lines = append(lines, Line{Kind: LineHunk, Text: raw})
		case strings.HasPrefix(raw, "+"):
			lines = append(lines, Line{Kind: LineAdded, New: newNo, Text: raw[1:]})
			newNo++
		case strings.HasPrefix(raw, "-"):
			lines = append(lines, Line{Kind: LineRemoved, Old: oldNo, Text: raw[1:]})
			oldNo++
		case strings.HasPrefix(raw, " "):
			lines = append(lines, Line{Kind: LineContext, Old: oldNo, New: newNo, Text: raw[1:]})
			oldNo++
			newNo++
		}
	}
	return lines
}

func hunkStart(header string) (int, int) {
	fields := strings.Fields(header)
	if len(fields) < 3 {
		return 0, 0
	}
	start := func(field string) int {
		n, _ := strconv.Atoi(strings.SplitN(field[1:], ",", 2)[0])
		return n
	}
	return start(fields[1]), start(fields[2])
}

type Side string

const (
	SideLeft  Side = "left"
	SideRight Side = "right"
)

type Comment struct {
	ID       string
	Commit   string
	Kind     string
	Path     string
	Side     Side
	Start    int
	End      int
	Body     string
	Sent     bool
	Replies  []string
	Author   string
	Resolved bool
}

const YourReply = "**you** "

func (c Comment) FileLevel() bool {
	return c.End == 0
}

func (c Comment) Answered() bool {
	return c.Author == "" && len(c.Replies) > 0 && !strings.HasPrefix(c.Replies[len(c.Replies)-1], YourReply)
}

type Source interface {
	Commits(ctx context.Context) ([]Commit, error)
	Files(ctx context.Context, commit string) ([]File, error)
}

type Store interface {
	SetViewed(ctx context.Context, branch, path, hash string) error
	Viewed(ctx context.Context, branch string) (map[string]string, error)
	AddComment(ctx context.Context, branch string, c Comment) error
	Comments(ctx context.Context, branch string) ([]Comment, error)
	UpdateComments(ctx context.Context, branch string, ids []string, update func(*Comment)) error
	DeleteComments(ctx context.Context, branch string, ids []string) error
}

type PullRequest struct {
	ID            string
	Repo          string
	Number        int
	Title         string
	Body          string
	URL           string
	State         string
	Draft         bool
	Author        string
	Head          string
	Base          string
	UpdatedAt     time.Time
	Additions     int
	Deletions     int
	ChangedFiles  int
	Viewer        string
	PendingReview string
	Assignees     []string
	Reviewers     []Reviewer
	Labels        []string
	Checks        []Check
	Commits       []Commit
	Files         []File
	Threads       []Thread
	Detailed      bool
}

type Reviewer struct {
	Login string
	State string
}

type Check struct {
	Name  string
	State string
}

type Thread struct {
	ID       string
	Path     string
	Side     Side
	Start    int
	End      int
	Outdated bool
	Resolved bool
	Notes    []Note
}

type Note struct {
	ID        string
	Author    string
	Body      string
	State     string
	CreatedAt time.Time
	Reactions []Reaction
}

type Reaction struct {
	Content string
	Count   int
	Mine    bool
}

type ReviewEvent string

const (
	EventComment        ReviewEvent = "COMMENT"
	EventApprove        ReviewEvent = "APPROVE"
	EventRequestChanges ReviewEvent = "REQUEST_CHANGES"
)

type Host interface {
	PullRequests(ctx context.Context, filter string) ([]PullRequest, error)
	PullRequest(ctx context.Context, repo string, number int) (PullRequest, error)
	BranchPullRequest(ctx context.Context) (PullRequest, error)
	Assignable(ctx context.Context, repo string) ([]string, error)
	EditPullRequest(ctx context.Context, pr PullRequest, title, body string) error
	SetPeople(ctx context.Context, pr PullRequest, assignees, reviewers []string) error
	Comment(ctx context.Context, pr PullRequest, body string) error
	Reply(ctx context.Context, threadID, body string) error
	React(ctx context.Context, subjectID, content string, add bool) error
	Resolve(ctx context.Context, threadID string, resolved bool) error
	SubmitReview(ctx context.Context, pr PullRequest, event ReviewEvent, body string) error
}

type Agent interface {
	Prompt(ctx context.Context, text string) (string, error)
}

type Service struct {
	source       Source
	store        Store
	agent        Agent
	branch       string
	ReplyCommand string
	Exists       func(path string) bool
}

func NewService(source Source, store Store, agent Agent, branch string) *Service {
	return &Service{source: source, store: store, agent: agent, branch: branch}
}

func (s *Service) Branch() string {
	return s.branch
}

func (s *Service) Commits(ctx context.Context) ([]Commit, error) {
	commits, err := s.source.Commits(ctx)
	if err != nil {
		return nil, fmt.Errorf("list commits on %s: %w", s.branch, err)
	}
	return commits, nil
}

func (s *Service) Files(ctx context.Context, commit string) ([]File, error) {
	files, err := s.source.Files(ctx, commit)
	if err != nil {
		return nil, fmt.Errorf("fetch files for %s: %w", s.branch, err)
	}
	return files, nil
}

func (s *Service) Viewed(ctx context.Context, files []File) (map[string]bool, error) {
	stored, err := s.store.Viewed(ctx, s.branch)
	if err != nil {
		return nil, err
	}
	viewed := map[string]bool{}
	for _, f := range files {
		if stored[f.Path] == patchHash(f.Patch) {
			viewed[f.Path] = true
		}
	}
	return viewed, nil
}

func (s *Service) ToggleViewed(ctx context.Context, path string) (bool, error) {
	files, err := s.Files(ctx, "")
	if err != nil {
		return false, err
	}
	stored, err := s.store.Viewed(ctx, s.branch)
	if err != nil {
		return false, err
	}
	for _, f := range files {
		if f.Path != path {
			continue
		}
		hash := patchHash(f.Patch)
		if stored[path] == hash {
			return false, s.store.SetViewed(ctx, s.branch, path, "")
		}
		return true, s.store.SetViewed(ctx, s.branch, path, hash)
	}
	return false, fmt.Errorf("%s is not changed on %s", path, s.branch)
}

func patchHash(patch string) string {
	sum := sha256.Sum256([]byte(patch))
	return hex.EncodeToString(sum[:])
}

func (s *Service) Comments(ctx context.Context) ([]Comment, error) {
	return s.store.Comments(ctx, s.branch)
}

func (s *Service) AddComment(ctx context.Context, c Comment) error {
	if strings.TrimSpace(c.Body) == "" {
		return errors.New("comment is empty")
	}
	return s.store.AddComment(ctx, s.branch, c)
}

func (s *Service) DeleteComment(ctx context.Context, id string) error {
	return s.store.DeleteComments(ctx, s.branch, []string{id})
}

func (s *Service) ResolveComment(ctx context.Context, id string, resolved bool) error {
	return s.store.UpdateComments(ctx, s.branch, []string{id}, func(c *Comment) { c.Resolved = resolved })
}

func (s *Service) DeleteAllComments(ctx context.Context) error {
	comments, err := s.Comments(ctx)
	if err != nil {
		return err
	}
	return s.store.DeleteComments(ctx, s.branch, ids(comments))
}

func (s *Service) PruneComments(ctx context.Context, files []File) (int, error) {
	if s.Exists == nil {
		return 0, nil
	}
	comments, err := s.Comments(ctx)
	if err != nil || len(comments) == 0 {
		return 0, err
	}
	removed := map[string]bool{}
	for _, f := range files {
		if f.Status == StatusRemoved {
			removed[f.Path] = true
		}
	}
	var stale []Comment
	for _, c := range comments {
		if !s.Exists(c.Path) && !(removed[c.Path] && (c.Side == SideLeft || c.FileLevel())) {
			stale = append(stale, c)
		}
	}
	if len(stale) == 0 {
		return 0, nil
	}
	return len(stale), s.store.DeleteComments(ctx, s.branch, ids(stale))
}

func ids(comments []Comment) []string {
	out := make([]string, len(comments))
	for i, c := range comments {
		out[i] = c.ID
	}
	return out
}

func (s *Service) Reply(ctx context.Context, id, body string) error {
	return s.addReply(ctx, id, body, false)
}

func (s *Service) FollowUp(ctx context.Context, id, body string) error {
	return s.addReply(ctx, id, body, true)
}

func (s *Service) addReply(ctx context.Context, id, body string, mine bool) error {
	if strings.TrimSpace(body) == "" {
		return errors.New("reply is empty")
	}
	comments, err := s.Comments(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(comments, func(c Comment) bool { return c.ID == id })
	switch {
	case i < 0:
		return fmt.Errorf("no comment %s on %s", id, s.branch)
	case mine && !comments[i].Answered():
		return fmt.Errorf("comment %s has no agent reply to answer", id)
	}
	return s.store.UpdateComments(ctx, s.branch, []string{id}, func(c *Comment) {
		if mine {
			body, c.Sent = YourReply+body, false
		}
		c.Replies = append(c.Replies, body)
	})
}

func (s *Service) Prompt(ctx context.Context) (string, error) {
	text, _, err := s.prompt(ctx)
	return text, err
}

func (s *Service) prompt(ctx context.Context) (string, []Comment, error) {
	all, err := s.Comments(ctx)
	if err != nil {
		return "", nil, err
	}
	var comments []Comment
	for _, c := range all {
		if !c.Sent && !c.Resolved {
			comments = append(comments, c)
		}
	}
	if len(comments) == 0 {
		return "", nil, errors.New("no unsent comments on " + s.branch)
	}
	command := s.ReplyCommand
	if command == "" {
		command = "revunk"
	}
	files := map[string][]File{}
	var b strings.Builder
	fmt.Fprintf(&b, "Address these review comments on branch %s. After handling each comment, reply to it with `%s reply <id> \"<what you changed, or your answer>\"`.\n", s.branch, command)
	for i, c := range comments {
		if _, ok := files[c.Commit]; !ok {
			if files[c.Commit], err = s.Files(ctx, c.Commit); err != nil {
				return "", nil, err
			}
		}
		fmt.Fprintf(&b, "\n## %d. %s on %s", i+1, c.Kind, c.Path)
		if !c.FileLevel() {
			side := ""
			if c.Side == SideLeft {
				side = "removed "
			}
			lines := fmt.Sprintf("lines %d-%d", c.Start, c.End)
			if c.Start == c.End {
				lines = fmt.Sprintf("line %d", c.End)
			}
			b.WriteString(" " + side + lines)
		}
		if c.Commit != "" {
			fmt.Fprintf(&b, " in commit %s", Commit{SHA: c.Commit}.Short())
		}
		fmt.Fprintf(&b, " (id %s)\n\n", c.ID)
		if code := commented(files[c.Commit], c); len(code) > 0 {
			b.WriteString("```\n" + strings.Join(code, "\n") + "\n```\n\n")
		}
		b.WriteString(c.Body + "\n")
		if len(c.Replies) > 0 {
			b.WriteString("\nThread so far, answer the reviewer's last reply:\n")
		}
		for _, r := range c.Replies {
			if body, ok := strings.CutPrefix(r, YourReply); ok {
				b.WriteString("\nReviewer: " + body + "\n")
			} else {
				b.WriteString("\nAgent: " + r + "\n")
			}
		}
	}
	return b.String(), comments, nil
}

func commented(files []File, c Comment) []string {
	if c.FileLevel() {
		return nil
	}
	var code []string
	for _, f := range files {
		if f.Path != c.Path {
			continue
		}
		for _, l := range ParseLines(f.Patch) {
			skip, n := LineRemoved, l.New
			if c.Side == SideLeft {
				skip, n = LineAdded, l.Old
			}
			if l.Kind != LineHunk && l.Kind != skip && n >= c.Start && n <= c.End {
				code = append(code, l.Text)
			}
		}
	}
	return code
}

func (s *Service) SendComments(ctx context.Context) (string, error) {
	if s.agent == nil {
		return "", errors.New("no coding agent is configured")
	}
	text, sent, err := s.prompt(ctx)
	if err != nil {
		return "", err
	}
	name, err := s.agent.Prompt(ctx, text)
	if err != nil {
		return name, err
	}
	return name, s.store.UpdateComments(ctx, s.branch, ids(sent), func(c *Comment) { c.Sent = true })
}
