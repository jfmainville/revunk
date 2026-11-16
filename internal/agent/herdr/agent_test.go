package herdr

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const agents = `{"result":{"agents":[
{"agent":"codex","cwd":"/repo","pane_id":"p1"},
{"agent":"claude","cwd":"/repo-other","pane_id":"p2"},
{"agent":"claude","cwd":"/repo","pane_id":"self"},
{"agent":"claude","cwd":"/repo/sub","pane_id":"p4"},
{"agent":"claude","cwd":"/repo","pane_id":"p5"}]}}`

func fakeHerdr(t *testing.T, list string, promptExit int) string {
	t.Helper()
	dir := t.TempDir()
	log := filepath.Join(dir, "log")
	script := "#!/bin/sh\nif [ \"$2\" = list ]; then printf '%s' '" + list + "'; exit 0; fi\nprintf '%s\\n' \"$@\" > " + log + "\necho '{\"error\":\"agent_blocked\"}'\nexit " + string(rune('0'+promptExit)) + "\n"
	if err := os.WriteFile(filepath.Join(dir, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("HERDR_PANE_ID", "self")
	return log
}

func TestPromptSendsToFirstClaudeSessionInRepo(t *testing.T) {
	log := fakeHerdr(t, agents, 0)

	if err := (Agent{Dir: "/repo"}).Prompt(context.Background(), "fix\nthis"); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	if want := "agent\nprompt\np4\nfix\nthis\n"; string(got) != want {
		t.Fatalf("herdr args = %q, want %q", got, want)
	}
}

func TestPromptFailsWithoutMatchingSession(t *testing.T) {
	fakeHerdr(t, `{"result":{"agents":[{"agent":"claude","cwd":"/elsewhere","pane_id":"p1"}]}}`, 0)
	if err := (Agent{Dir: "/repo"}).Prompt(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "no Claude Code session open in /repo") {
		t.Fatalf("error = %v", err)
	}
}

func TestPromptReportsHerdrErrors(t *testing.T) {
	fakeHerdr(t, agents, 1)
	if err := (Agent{Dir: "/repo"}).Prompt(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "agent_blocked") {
		t.Fatalf("prompt error = %v", err)
	}
	fakeHerdr(t, "not json", 0)
	if err := (Agent{Dir: "/repo"}).Prompt(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "read herdr agents") {
		t.Fatalf("bad list error = %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if err := (Agent{Dir: "/repo"}).Prompt(context.Background(), "x"); err == nil || !strings.Contains(err.Error(), "herdr agent list") {
		t.Fatalf("missing herdr error = %v", err)
	}
}
