package store

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
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
	mu   sync.Mutex
	path string
}

func Open(path string) (*JSON, error) {
	s := &JSON{path: path}
	err := s.change(func(st *state) bool {
		missing := false
		for _, comments := range st.Comments {
			for i := range comments {
				if comments[i].ID == "" {
					comments[i].ID, missing = newID(), true
				}
			}
		}
		return missing
	})
	if err != nil {
		return nil, err
	}
	return s, nil
}

func newID() string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *JSON) load() (state, error) {
	st := state{Viewed: map[string]map[string]string{}, Comments: map[string][]review.Comment{}}
	data, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return st, nil
	case err != nil:
		return st, err
	}
	return st, json.Unmarshal(data, &st)
}

func (s *JSON) read() (state, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load()
}

func (s *JSON) change(update func(*state) bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := s.load()
	if err != nil {
		return err
	}
	if !update(&st) {
		return nil
	}
	data, err := json.MarshalIndent(st, "", "  ")
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
	return s.change(func(st *state) bool {
		if st.Viewed[branch] == nil {
			st.Viewed[branch] = map[string]string{}
		}
		if hash == "" {
			delete(st.Viewed[branch], path)
		} else {
			st.Viewed[branch][path] = hash
		}
		return true
	})
}

func (s *JSON) Viewed(_ context.Context, branch string) (map[string]string, error) {
	st, err := s.read()
	if err != nil {
		return nil, err
	}
	if st.Viewed[branch] == nil {
		return map[string]string{}, nil
	}
	return st.Viewed[branch], nil
}

func (s *JSON) AddComment(_ context.Context, branch string, c review.Comment) error {
	c.ID = newID()
	return s.change(func(st *state) bool {
		st.Comments[branch] = append(st.Comments[branch], c)
		return true
	})
}

func (s *JSON) Comments(_ context.Context, branch string) ([]review.Comment, error) {
	st, err := s.read()
	return st.Comments[branch], err
}

func (s *JSON) UpdateComments(_ context.Context, branch string, ids []string, update func(*review.Comment)) error {
	return s.change(func(st *state) bool {
		for i, c := range st.Comments[branch] {
			if slices.Contains(ids, c.ID) {
				update(&st.Comments[branch][i])
			}
		}
		return true
	})
}

func (s *JSON) DeleteComments(_ context.Context, branch string, ids []string) error {
	return s.change(func(st *state) bool {
		st.Comments[branch] = slices.DeleteFunc(st.Comments[branch], func(c review.Comment) bool {
			return slices.Contains(ids, c.ID)
		})
		return true
	})
}
