package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func script(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRunSendsPromptOnStdinInPrintMode(t *testing.T) {
	cli := CLI{Path: script(t, `echo "args: $*"; cat; echo`)}

	out, err := cli.Run(context.Background(), "explain this diff")

	if err != nil {
		t.Fatal(err)
	}
	if out != "args: -p\nexplain this diff" {
		t.Fatalf("got %q, want args and prompt with surrounding whitespace trimmed", out)
	}
}

func TestRunReturnsStderrOnFailure(t *testing.T) {
	cli := CLI{Path: script(t, `echo "not logged in" >&2; exit 1`)}

	out, err := cli.Run(context.Background(), "hi")

	if err == nil || !strings.Contains(err.Error(), "not logged in") {
		t.Fatalf("expected error with stderr, got %v", err)
	}
	if out != "" {
		t.Fatalf("got output %q on failure", out)
	}
}

func TestRunFailsWhenBinaryIsMissing(t *testing.T) {
	cli := CLI{Path: filepath.Join(t.TempDir(), "missing")}

	if _, err := cli.Run(context.Background(), "hi"); err == nil {
		t.Fatal("expected an error for a missing binary")
	}
}

func TestRunStopsWhenContextIsCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := (CLI{Path: script(t, "cat")}).Run(ctx, "hi"); err == nil {
		t.Fatal("expected an error for a canceled context")
	}
}
