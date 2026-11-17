package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jfmainville/revunk/internal/review"
)

type Source struct {
	Client Client
	Repo   string
	Number int
}

type apiFile struct {
	Filename  string
	Status    string
	Additions int
	Deletions int
	Patch     string
}

func files(in []apiFile) []review.File {
	out := make([]review.File, len(in))
	for i, f := range in {
		out[i] = review.File{Path: f.Filename, Status: changeStatus(f.Status), Additions: f.Additions, Deletions: f.Deletions, Patch: f.Patch}
	}
	return out
}

func (s Source) Commits(ctx context.Context) ([]review.Commit, error) {
	found, err := apiList[struct {
		SHA    string
		Commit struct{ Message string }
	}](ctx, s.Client, fmt.Sprintf("repos/%s/pulls/%d/commits?per_page=100", s.Repo, s.Number))
	if err != nil {
		return nil, err
	}
	commits := make([]review.Commit, len(found))
	for i, c := range found {
		subject, _, _ := strings.Cut(c.Commit.Message, "\n")
		commits[i] = review.Commit{SHA: c.SHA, Subject: subject}
	}
	return commits, nil
}

func (s Source) Files(ctx context.Context, commit string) ([]review.File, error) {
	if commit == "" {
		found, err := apiList[apiFile](ctx, s.Client, fmt.Sprintf("repos/%s/pulls/%d/files?per_page=100", s.Repo, s.Number))
		return files(found), err
	}
	data, err := s.Client.run(ctx, "api", fmt.Sprintf("repos/%s/commits/%s", s.Repo, commit))
	if err != nil {
		return nil, err
	}
	var c struct{ Files []apiFile }
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("read commit %s: %w", commit, err)
	}
	return files(c.Files), nil
}

type Review struct {
	review.Store
	Client Client
	Repo   string
	Number int
}

func (r Review) Comments(ctx context.Context, _ string) ([]review.Comment, error) {
	pr, err := r.Client.PullRequest(ctx, r.Repo, r.Number)
	if err != nil {
		return nil, err
	}
	var comments []review.Comment
	for _, t := range pr.Threads {
		if t.ID == "" || t.Outdated {
			continue
		}
		first := t.Notes[0]
		c := review.Comment{
			ID: first.ID, Kind: "comment", Path: t.Path, Side: t.Side, Start: t.Start, End: t.End, Body: first.Body,
			Sent: first.State != "PENDING", Author: first.Author, Resolved: t.Resolved,
		}
		if strings.Contains(first.Body, "```suggestion") {
			c.Kind = "suggestion"
		}
		for _, n := range t.Notes[1:] {
			c.Replies = append(c.Replies, "**@"+n.Author+"** "+n.Body)
		}
		comments = append(comments, c)
	}
	return comments, nil
}

func (r Review) AddComment(ctx context.Context, _ string, c review.Comment) error {
	pr, err := r.Client.PullRequest(ctx, r.Repo, r.Number)
	if err != nil {
		return err
	}
	return r.Client.addThread(ctx, pr, c)
}

func (r Review) UpdateComments(context.Context, string, []string, func(*review.Comment)) error {
	return errors.New("pull request comments change on GitHub")
}

func (r Review) DeleteComments(ctx context.Context, _ string, ids []string) error {
	for _, id := range ids {
		if err := r.Client.deleteComment(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
