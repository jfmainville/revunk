package store

import (
	"context"
	"maps"
	"sync"

	"github.com/jfmainville/revunk/internal/review"
)

type Memory struct {
	mu     sync.Mutex
	viewed map[review.PullRequest]map[string]bool
}

func NewMemory() *Memory {
	return &Memory{viewed: map[review.PullRequest]map[string]bool{}}
}

func (m *Memory) SetViewed(_ context.Context, pr review.PullRequest, path string, viewed bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.viewed[pr] == nil {
		m.viewed[pr] = map[string]bool{}
	}
	m.viewed[pr][path] = viewed
	return nil
}

func (m *Memory) Viewed(_ context.Context, pr review.PullRequest) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return maps.Clone(m.viewed[pr]), nil
}
