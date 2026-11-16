package review

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
	Commit string
	Kind   string
	Path   string
	Side   Side
	Start  int
	End    int
	Body   string
}

func (c Comment) FileLevel() bool {
	return c.End == 0
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
	DeleteComment(ctx context.Context, branch string, index int) error
}

type Agent interface {
	Prompt(ctx context.Context, text string) (string, error)
}

type Service struct {
	source Source
	store  Store
	agent  Agent
	branch string
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

func (s *Service) Viewed(ctx context.Context) (map[string]bool, error) {
	files, err := s.Files(ctx, "")
	if err != nil {
		return nil, err
	}
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

func (s *Service) DeleteComment(ctx context.Context, index int) error {
	return s.store.DeleteComment(ctx, s.branch, index)
}

func (s *Service) Prompt(ctx context.Context) (string, error) {
	comments, err := s.Comments(ctx)
	if err != nil {
		return "", err
	}
	if len(comments) == 0 {
		return "", errors.New("no comments on " + s.branch)
	}
	files := map[string][]File{}
	var b strings.Builder
	fmt.Fprintf(&b, "Address these review comments on branch %s.\n", s.branch)
	for i, c := range comments {
		if _, ok := files[c.Commit]; !ok {
			if files[c.Commit], err = s.Files(ctx, c.Commit); err != nil {
				return "", err
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
		b.WriteString("\n\n")
		if code := commented(files[c.Commit], c); len(code) > 0 {
			b.WriteString("```\n" + strings.Join(code, "\n") + "\n```\n\n")
		}
		b.WriteString(c.Body + "\n")
	}
	return b.String(), nil
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
	text, err := s.Prompt(ctx)
	if err != nil {
		return "", err
	}
	return s.agent.Prompt(ctx, text)
}
