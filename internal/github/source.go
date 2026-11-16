// Package github is the GitHub integration boundary implementing review.Source.
package github

import (
	"context"

	"github.com/jfmainville/revunk/internal/review"
)

// SampleSource returns fixed data until the GitHub API client is implemented.
type SampleSource struct{}

func (SampleSource) Files(_ context.Context, _ review.PullRequest) ([]review.File, error) {
	return []review.File{
		{
			Path:      "cmd/revunk/main.go",
			Status:    review.StatusAdded,
			Additions: 3,
			Patch:     "@@ -0,0 +1,3 @@\n+package main\n+\n+func main() {}",
		},
		{
			Path:      "README.md",
			Status:    review.StatusModified,
			Additions: 1,
			Deletions: 1,
			Patch:     "@@ -1 +1 @@\n-# revunk\n+# Revunk",
		},
		{
			Path:      "docs/old.md",
			Status:    review.StatusRemoved,
			Deletions: 1,
			Patch:     "@@ -1 +0,0 @@\n-Outdated notes",
		},
	}, nil
}
