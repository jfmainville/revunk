package review

import (
	"context"
	"fmt"
)

type FileStatus string

const (
	StatusAdded    FileStatus = "added"
	StatusModified FileStatus = "modified"
	StatusRemoved  FileStatus = "removed"
	StatusRenamed  FileStatus = "renamed"
)

type PullRequest struct {
	Owner  string
	Repo   string
	Number int
}

func (pr PullRequest) String() string {
	return fmt.Sprintf("%s/%s#%d", pr.Owner, pr.Repo, pr.Number)
}

type File struct {
	Path      string
	Status    FileStatus
	Additions int
	Deletions int
	Patch     string
}

type Source interface {
	Files(ctx context.Context, pr PullRequest) ([]File, error)
}

type Store interface {
	SetViewed(ctx context.Context, pr PullRequest, path string, viewed bool) error
	Viewed(ctx context.Context, pr PullRequest) (map[string]bool, error)
}

type Agent interface {
	Run(ctx context.Context, prompt string) (string, error)
}

type Service struct {
	source Source
	store  Store
	agent  Agent
}

func NewService(source Source, store Store, agent Agent) *Service {
	return &Service{source: source, store: store, agent: agent}
}

func (s *Service) Files(ctx context.Context, pr PullRequest) ([]File, error) {
	files, err := s.source.Files(ctx, pr)
	if err != nil {
		return nil, fmt.Errorf("fetch files for %s: %w", pr, err)
	}
	return files, nil
}

func (s *Service) Viewed(ctx context.Context, pr PullRequest) (map[string]bool, error) {
	return s.store.Viewed(ctx, pr)
}

func (s *Service) ToggleViewed(ctx context.Context, pr PullRequest, path string) (bool, error) {
	viewed, err := s.store.Viewed(ctx, pr)
	if err != nil {
		return false, err
	}
	next := !viewed[path]
	if err := s.store.SetViewed(ctx, pr, path, next); err != nil {
		return false, err
	}
	return next, nil
}

func (s *Service) Ask(ctx context.Context, pr PullRequest, file File, question string) (string, error) {
	prompt := fmt.Sprintf("You are reviewing pull request %s.\nFile: %s (%s)\n\nDiff:\n%s\n\nQuestion: %s\n", pr, file.Path, file.Status, file.Patch, question)
	answer, err := s.agent.Run(ctx, prompt)
	if err != nil {
		return "", fmt.Errorf("ask agent about %s: %w", file.Path, err)
	}
	return answer, nil
}
