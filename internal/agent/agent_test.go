package agent

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
)

func fakeBinary(t *testing.T, name, body string) string {
	t.Helper()
	bin := t.TempDir()
	log := filepath.Join(bin, "log")
	script := "#!/bin/sh\necho \"$*\" >> " + log + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
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

func prompt(t *testing.T, pane review.AgentPane) (string, error) {
	t.Helper()
	return Agent{Dir: "/repo"}.Prompt(context.Background(), pane, "fix it")
}

func TestPanesListsKnownAgentsInRepoExceptSelf(t *testing.T) {
	fakeBinary(t, "herdr", listing(`{"agent":"nano","cwd":"/repo","pane_id":"p1"},
{"agent":"claude","cwd":"/repo-other","pane_id":"p2"},
{"agent":"claude","cwd":"/repo","pane_id":"self"},
{"agent":"opencode","cwd":"/repo/sub","pane_id":"p4"},
{"agent":"claude","cwd":"/repo","pane_id":"p5"}`))
	t.Setenv("HERDR_PANE_ID", "self")

	panes, err := Agent{Dir: "/repo"}.Panes(context.Background())

	want := []review.AgentPane{{ID: "p4", Name: "opencode"}, {ID: "p5", Name: "Claude Code"}}
	if err != nil || !reflect.DeepEqual(panes, want) {
		t.Fatalf("Panes = %+v, %v, want %+v", panes, err, want)
	}
}

func TestPanesReportsListErrors(t *testing.T) {
	fakeBinary(t, "herdr", "printf 'not json'")
	if _, err := (Agent{Dir: "/repo"}).Panes(context.Background()); err == nil || !strings.Contains(err.Error(), "read herdr agents") {
		t.Fatalf("bad list error = %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (Agent{Dir: "/repo"}).Panes(context.Background()); err == nil || !strings.Contains(err.Error(), "herdr agent list") {
		t.Fatalf("missing herdr error = %v", err)
	}
}

func TestPromptSendsToThePane(t *testing.T) {
	log := fakeBinary(t, "herdr", "")

	name, err := prompt(t, review.AgentPane{ID: "p4", Name: "opencode"})

	if err != nil || name != "opencode" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	if got := calls(t, log); strings.Join(got, "\n") != "agent prompt p4 fix it" {
		t.Fatalf("calls = %q", got)
	}
}

func TestPromptReportsPromptErrors(t *testing.T) {
	fakeBinary(t, "herdr", "echo agent_blocked; exit 1")
	if name, err := prompt(t, review.AgentPane{ID: "p1", Name: "Claude Code"}); name != "Claude Code" || err == nil || !strings.Contains(err.Error(), "herdr agent prompt") || !strings.Contains(err.Error(), "agent_blocked") {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
}

func TestPromptOutsideHerdrWithoutPaneFails(t *testing.T) {
	fakeBinary(t, "herdr", "")
	if _, err := prompt(t, review.AgentPane{}); err == nil || !strings.Contains(err.Error(), "no agent session open in /repo") {
		t.Fatalf("error = %v", err)
	}
}

func TestPromptStartsNewPaneInsideHerdr(t *testing.T) {
	log := fakeBinary(t, "herdr", "if [ \"$2\" = split ]; then printf '{\"result\":{\"pane\":{\"pane_id\":\"w1:p9\"}}}'; fi")
	t.Setenv("HERDR_PANE_ID", "w1:p1")

	name, err := prompt(t, review.AgentPane{})

	if err != nil || name != "a new Claude Code pane" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	want := []string{"pane split --current --direction right --cwd /repo --no-focus", "agent start revunk-w1-p9 --kind claude --pane w1:p9", "agent prompt w1:p9 fix it"}
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
		fakeBinary(t, "herdr", body)
		t.Setenv("HERDR_PANE_ID", "w1:p1")
		if _, err := prompt(t, review.AgentPane{}); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s error = %v", want, err)
		}
	}
}
