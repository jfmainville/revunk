package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fakeHerdr(t *testing.T, body string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	t.Setenv("HERDR_PANE_ID", "")
	return log
}

func calls(t *testing.T, log string) []string {
	t.Helper()
	out, err := os.ReadFile(log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

func listing(agents string) string {
	return "if [ \"$2\" = list ]; then printf '%s' '{\"result\":{\"agents\":[" + agents + "]}}'; exit 0; fi"
}

func prompt(t *testing.T) (string, error) {
	t.Helper()
	return Agent{Dir: "/repo"}.Prompt(context.Background(), "fix it")
}

func TestPromptSendsToFirstSessionInRepo(t *testing.T) {
	log := fakeHerdr(t, listing(`{"agent":"codex","cwd":"/repo","pane_id":"p1"},
{"agent":"claude","cwd":"/repo-other","pane_id":"p2"},
{"agent":"claude","cwd":"/repo","pane_id":"self"},
{"agent":"opencode","cwd":"/repo/sub","pane_id":"p4"},
{"agent":"claude","cwd":"/repo","pane_id":"p5"}`))
	t.Setenv("HERDR_PANE_ID", "self")

	name, err := prompt(t)

	if err != nil || name != "opencode" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	if got := calls(t, log); got[len(got)-1] != "agent prompt p4 fix it" {
		t.Fatalf("calls = %q", got)
	}
}

func TestPromptNamesClaudeCodeAndReportsPromptErrors(t *testing.T) {
	fakeHerdr(t, listing(`{"agent":"claude","cwd":"/repo","pane_id":"p1"}`)+"\necho agent_blocked; exit 1")
	if name, err := prompt(t); name != "Claude Code" || err == nil || !strings.Contains(err.Error(), "herdr agent prompt") || !strings.Contains(err.Error(), "agent_blocked") {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
}

func TestPromptOutsideHerdrWithoutSessionFails(t *testing.T) {
	fakeHerdr(t, listing(`{"agent":"claude","cwd":"/elsewhere","pane_id":"p1"}`))
	if _, err := prompt(t); err == nil || !strings.Contains(err.Error(), "no Claude Code or opencode session open in /repo") {
		t.Fatalf("error = %v", err)
	}
}

func TestPromptReportsListErrors(t *testing.T) {
	fakeHerdr(t, "printf 'not json'")
	if _, err := prompt(t); err == nil || !strings.Contains(err.Error(), "read herdr agents") {
		t.Fatalf("bad list error = %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := prompt(t); err == nil || !strings.Contains(err.Error(), "herdr agent list") {
		t.Fatalf("missing herdr error = %v", err)
	}
}

func TestPromptStartsNewPaneInsideHerdr(t *testing.T) {
	log := fakeHerdr(t, listing("")+"\nif [ \"$2\" = split ]; then printf '{\"result\":{\"pane\":{\"pane_id\":\"w1:p9\"}}}'; fi")
	t.Setenv("HERDR_PANE_ID", "w1:p1")

	name, err := prompt(t)

	if err != nil || name != "a new Claude Code pane" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	want := []string{"agent list", "pane split --current --direction right --cwd /repo --no-focus", "agent start revunk-w1-p9 --kind claude --pane w1:p9", "agent prompt w1:p9 fix it"}
	if got := calls(t, log); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls = %q", got)
	}
}

func TestStartErrors(t *testing.T) {
	split := "if [ \"$2\" = split ]; then printf '{\"result\":{\"pane\":{\"pane_id\":\"p\"}}}'; fi\n"
	cases := map[string]string{
		"down":             "if [ \"$2\" = split ]; then echo down; exit 1; fi",
		"returned no pane": "if [ \"$2\" = split ]; then printf '{}'; fi",
		"busy":             split + "if [ \"$2\" = start ]; then echo busy; exit 1; fi",
	}
	for want, body := range cases {
		fakeHerdr(t, listing("")+"\n"+body)
		t.Setenv("HERDR_PANE_ID", "w1:p1")
		if _, err := prompt(t); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s error = %v", want, err)
		}
	}
}

func TestFocusPaneMovesHerdrFocusFromThisPane(t *testing.T) {
	log := fakeHerdr(t, "")
	if err := FocusPane(context.Background(), "left"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Fatal("outside herdr nothing should run")
	}
	t.Setenv("HERDR_PANE_ID", "p7")
	if err := FocusPane(context.Background(), "down"); err != nil {
		t.Fatal(err)
	}
	if got := calls(t, log); len(got) != 1 || got[0] != "pane focus --direction down --pane p7" {
		t.Fatalf("calls=%q", got)
	}
	fakeHerdr(t, "exit 1")
	t.Setenv("HERDR_PANE_ID", "p7")
	if err := FocusPane(context.Background(), "up"); err == nil {
		t.Fatal("herdr errors should be returned")
	}
}
