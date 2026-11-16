package github

import (
	"context"
	"strings"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
)

func TestSampleSourceFilesMatchTheirPatches(t *testing.T) {
	files, err := SampleSource{}.Files(context.Background(), review.PullRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("expected sample files")
	}

	for _, f := range files {
		if f.Path == "" || f.Status == "" {
			t.Errorf("file %+v is missing a path or status", f)
		}
		var additions, deletions int
		for _, line := range strings.Split(f.Patch, "\n") {
			switch {
			case strings.HasPrefix(line, "@@"):
			case strings.HasPrefix(line, "+"):
				additions++
			case strings.HasPrefix(line, "-"):
				deletions++
			}
		}
		if additions != f.Additions || deletions != f.Deletions {
			t.Errorf("%s reports +%d -%d but its patch has +%d -%d", f.Path, f.Additions, f.Deletions, additions, deletions)
		}
	}
}
