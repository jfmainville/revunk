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

type Service struct {
	source Source
	store  Store
}

func NewService(source Source, store Store) *Service {
	return &Service{source: source, store: store}
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
