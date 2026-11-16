package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/jfmainville/revunk/internal/review"
)

type state struct {
	Viewed   map[string]map[string]string
	Comments map[string][]review.Comment
}

type JSON struct {
	mu    sync.Mutex
	path  string
	state state
}

func Open(path string) (*JSON, error) {
	s := &JSON{path: path}
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return nil, err
	default:
		if err := json.Unmarshal(data, &s.state); err != nil {
			return nil, err
		}
	}
	if s.state.Viewed == nil {
		s.state.Viewed = map[string]map[string]string{}
	}
	if s.state.Comments == nil {
		s.state.Comments = map[string][]review.Comment{}
	}
	return s, nil
}

func (s *JSON) save() error {
	data, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *JSON) SetViewed(_ context.Context, branch, path, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.state.Viewed[branch] == nil {
		s.state.Viewed[branch] = map[string]string{}
	}
	if hash == "" {
		delete(s.state.Viewed[branch], path)
	} else {
		s.state.Viewed[branch][path] = hash
	}
	return s.save()
}

func (s *JSON) Viewed(_ context.Context, branch string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	viewed := maps.Clone(s.state.Viewed[branch])
	if viewed == nil {
		viewed = map[string]string{}
	}
	return viewed, nil
}

func (s *JSON) AddComment(_ context.Context, branch string, c review.Comment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.state.Comments[branch] = append(s.state.Comments[branch], c)
	return s.save()
}

func (s *JSON) DeleteComments(_ context.Context, branch string, from, to int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	comments := s.state.Comments[branch]
	if from < 0 || to > len(comments) || from >= to {
		return fmt.Errorf("no comments %d to %d on %s", from, to, branch)
	}
	s.state.Comments[branch] = slices.Delete(comments, from, to)
	return s.save()
}

func (s *JSON) Comments(_ context.Context, branch string) ([]review.Comment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return slices.Clone(s.state.Comments[branch]), nil
}
