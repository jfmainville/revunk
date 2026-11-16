package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type env struct {
	bin, log string
}

func newEnv(t *testing.T) env {
	t.Helper()
	bin := t.TempDir()
	t.Setenv("PATH", bin)
	t.Setenv("HERDR_PANE_ID", "")
	t.Setenv("TMUX", "")
	t.Setenv("TMUX_PANE", "")
	return env{bin: bin, log: filepath.Join(bin, "log")}
}

func (e env) fake(t *testing.T, name, body string) {
	t.Helper()
	script := "#!/bin/sh\necho \"" + name + " $*\" >> " + e.log + "\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(e.bin, name), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
}

func (e env) calls(t *testing.T) []string {
	t.Helper()
	out, err := os.ReadFile(e.log)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSpace(string(out)), "\n")
}

const herdrAgents = `{"result":{"agents":[
{"agent":"codex","cwd":"/repo","pane_id":"p1"},
{"agent":"claude","cwd":"/repo-other","pane_id":"p2"},
{"agent":"claude","cwd":"/repo","pane_id":"self"},
{"agent":"opencode","cwd":"/repo/sub","pane_id":"p4"},
{"agent":"claude","cwd":"/repo","pane_id":"p5"}]}}`

func herdrScript(list, rest string) string {
	return "if [ \"$2\" = list ]; then printf '%s' '" + list + "'; exit 0; fi\n" + rest
}

func prompt(t *testing.T, text string) (string, error) {
	t.Helper()
	return Agent{Dir: "/repo"}.Prompt(context.Background(), text)
}

func TestPromptSendsToFirstHerdrSessionInRepo(t *testing.T) {
	e := newEnv(t)
	t.Setenv("HERDR_PANE_ID", "self")
	e.fake(t, "herdr", herdrScript(herdrAgents, ""))

	name, err := prompt(t, "fix it")

	if err != nil || name != "opencode" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	if got := e.calls(t); got[len(got)-1] != "herdr agent prompt p4 fix it" {
		t.Fatalf("calls = %q", got)
	}
}

func TestPromptReportsHerdrPromptError(t *testing.T) {
	e := newEnv(t)
	e.fake(t, "herdr", herdrScript(`{"result":{"agents":[{"agent":"claude","cwd":"/repo","pane_id":"p1"}]}}`, "echo agent_blocked; exit 1"))
	if name, err := prompt(t, "x"); name != "Claude Code" || err == nil || !strings.Contains(err.Error(), "agent_blocked") {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
}

func TestPromptPastesIntoTmuxPaneWhenHerdrHasNoSession(t *testing.T) {
	e := newEnv(t)
	t.Setenv("TMUX_PANE", "%0")
	e.fake(t, "herdr", "printf 'not json'")
	e.fake(t, "tmux", "if [ \"$1\" = list-panes ]; then printf '%%0\\tclaude\\t/repo\\n%%1\\tzsh\\t/repo\\nbad\\n%%2\\tclaude\\t/repo/cmd\\n'; fi\nif [ \"$1\" = load-buffer ]; then while IFS= read -r l || [ -n \"$l\" ]; do echo \"stdin $l\" >> "+e.log+"; done; fi")

	name, err := prompt(t, "fix it")

	if err != nil || name != "Claude Code" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	want := []string{"herdr agent list", "tmux list-panes -a -F #{pane_id}\t#{pane_current_command}\t#{pane_current_path}", "tmux load-buffer -b revunk -", "stdin fix it", "tmux paste-buffer -p -d -b revunk -t %2", "tmux send-keys -t %2 Enter"}
	if got := e.calls(t); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls = %q", got)
	}
}

func TestTmuxSendStopsAtFirstError(t *testing.T) {
	for _, failing := range []string{"load-buffer", "paste-buffer", "send-keys"} {
		e := newEnv(t)
		e.fake(t, "tmux", "if [ \"$1\" = list-panes ]; then printf '%%1\\topencode\\t/repo\\n'; exit 0; fi\nif [ \"$1\" = "+failing+" ]; then echo nope; exit 1; fi")
		if _, err := prompt(t, "x"); err == nil || !strings.Contains(err.Error(), "tmux "+failing) {
			t.Fatalf("%s error = %v", failing, err)
		}
	}
}

func TestPromptStartsNewHerdrPaneWithoutSession(t *testing.T) {
	e := newEnv(t)
	t.Setenv("HERDR_PANE_ID", "w1:p1")
	e.fake(t, "herdr", herdrScript(`{"result":{"agents":[]}}`, "if [ \"$2\" = split ]; then printf '{\"result\":{\"pane\":{\"pane_id\":\"w1:p9\"}}}'; fi"))

	name, err := prompt(t, "fix it")

	if err != nil || name != "a new Claude Code pane" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	want := []string{"herdr agent list", "herdr pane split --current --direction right --cwd /repo --no-focus", "herdr agent start revunk-w1-p9 --kind claude --pane w1:p9", "herdr agent prompt w1:p9 fix it"}
	if got := e.calls(t); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("calls = %q", got)
	}
}

func TestHerdrStartErrors(t *testing.T) {
	cases := map[string]string{
		"split":     "if [ \"$2\" = split ]; then echo down; exit 1; fi",
		"no pane":   "if [ \"$2\" = split ]; then printf '{}'; fi",
		"start":     "if [ \"$2\" = split ]; then printf '{\"result\":{\"pane\":{\"pane_id\":\"p\"}}}'; fi\nif [ \"$2\" = start ]; then echo busy; exit 1; fi",
		"no prompt": "if [ \"$2\" = split ]; then printf '{\"result\":{\"pane\":{\"pane_id\":\"p\"}}}'; fi\nif [ \"$2\" = prompt ]; then echo stalled; exit 1; fi",
	}
	want := map[string]string{"split": "down", "no pane": "returned no pane", "start": "busy", "no prompt": "stalled"}
	for name, body := range cases {
		e := newEnv(t)
		t.Setenv("HERDR_PANE_ID", "w1:p1")
		e.fake(t, "herdr", herdrScript(`{"result":{"agents":[]}}`, body))
		if _, err := prompt(t, "x"); err == nil || !strings.Contains(err.Error(), want[name]) {
			t.Fatalf("%s error = %v", name, err)
		}
	}
}

func TestPromptOpensTmuxWindowWithoutSession(t *testing.T) {
	e := newEnv(t)
	t.Setenv("TMUX", "/tmp/tmux-1000/default,1,0")
	e.fake(t, "tmux", "")

	name, err := prompt(t, "fix it")

	if err != nil || name != "a new Claude Code tmux window" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	if got := e.calls(t); got[len(got)-1] != "tmux new-window -d -c /repo claude fix it" {
		t.Fatalf("calls = %q", got)
	}
}

func TestPromptStartsBackgroundClaudeOutsideMultiplexers(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	e.fake(t, "claude", "pwd >> "+e.log)

	name, err := Agent{Dir: dir}.Prompt(context.Background(), "fix it")

	if err != nil || !strings.Contains(name, "background Claude Code session") {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	if got := e.calls(t); strings.Join(got, "\n") != "claude --bg fix it\n"+dir {
		t.Fatalf("calls = %q", got)
	}

	newEnv(t)
	if _, err := prompt(t, "x"); err == nil || !strings.Contains(err.Error(), "claude --bg") {
		t.Fatalf("missing claude error = %v", err)
	}
}
