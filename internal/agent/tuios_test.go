package agent

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
)

func fakeTuios(t *testing.T, body string) string {
	t.Helper()
	return fakeBinary(t, "tuios", body)
}

func tuiosListing(agents string) string {
	return "if [ \"$1\" = list-agents ]; then printf '%s' '{\"agents\":[" + agents + "]}'; exit 0; fi"
}

func tuiosPrompt(t *testing.T, pane review.AgentPane) (string, error) {
	t.Helper()
	return Tuios{Dir: "/repo"}.Prompt(context.Background(), pane, "fix it")
}

func TestTuiosPanesListsKnownAgentsInRepoExceptSelf(t *testing.T) {
	fakeTuios(t, tuiosListing(`{"harness_id":"nano","cwd":"/repo","window_id":"w1"},
{"harness_id":"claude-code","cwd":"/repo-other","window_id":"w2"},
{"harness_id":"claude-code","cwd":"/repo","window_id":"self"},
{"harness_id":"opencode","cwd":"/repo/sub","window_id":"w4"},
{"harness_id":"claude-code","cwd":"/repo","window_id":"w5"}`))
	t.Setenv("TUIOS_PANE_ID", "self")

	panes, err := Tuios{Dir: "/repo"}.Panes(context.Background())

	want := []review.AgentPane{{ID: "w4", Name: "opencode"}, {ID: "w5", Name: "Claude Code"}}
	if err != nil || !reflect.DeepEqual(panes, want) {
		t.Fatalf("Panes = %+v, %v, want %+v", panes, err, want)
	}
}

func TestTuiosPanesReportsListErrors(t *testing.T) {
	fakeTuios(t, "printf 'not json'")
	if _, err := (Tuios{Dir: "/repo"}).Panes(context.Background()); err == nil || !strings.Contains(err.Error(), "read tuios agents") {
		t.Fatalf("bad list error = %v", err)
	}
	t.Setenv("PATH", t.TempDir())
	if _, err := (Tuios{Dir: "/repo"}).Panes(context.Background()); err == nil || !strings.Contains(err.Error(), "tuios list-agents") {
		t.Fatalf("missing tuios error = %v", err)
	}
}

func TestTuiosPromptQueuesToThePane(t *testing.T) {
	log := fakeTuios(t, "")

	name, err := tuiosPrompt(t, review.AgentPane{ID: "w4", Name: "opencode"})

	if err != nil || name != "opencode" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	if got := calls(t, log); strings.Join(got, "\n") != "queue -w w4 fix it" {
		t.Fatalf("calls = %q", got)
	}
}

func TestTuiosPromptReportsQueueErrors(t *testing.T) {
	fakeTuios(t, "echo pane_gone; exit 1")
	if name, err := tuiosPrompt(t, review.AgentPane{ID: "w1", Name: "Claude Code"}); name != "Claude Code" || err == nil || !strings.Contains(err.Error(), "tuios queue") || !strings.Contains(err.Error(), "pane_gone") {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
}

func TestTuiosPromptStartsClaudeWithoutPane(t *testing.T) {
	log := fakeTuios(t, "")

	name, err := tuiosPrompt(t, review.AgentPane{})

	if err != nil || name != "a new Claude Code pane" {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
	if got := calls(t, log); strings.Join(got, "\n") != "start-agent claude --cwd /repo --prompt fix it" {
		t.Fatalf("calls = %q", got)
	}
}

func TestTuiosPromptReportsStartErrors(t *testing.T) {
	fakeTuios(t, "echo not_ready; exit 1")
	if name, err := tuiosPrompt(t, review.AgentPane{}); name != "a new Claude Code pane" || err == nil || !strings.Contains(err.Error(), "not_ready") {
		t.Fatalf("Prompt = %q, %v", name, err)
	}
}

func TestNewSelectsMultiplexerFromPaneEnvironment(t *testing.T) {
	cases := map[string]struct {
		tuios, herdr string
		want         review.Agent
	}{
		"tuios pane": {tuios: "w1", want: Tuios{Dir: "/repo"}},
		"herdr pane": {herdr: "p1", want: Agent{Dir: "/repo"}},
		"both panes": {tuios: "w1", herdr: "p1", want: Tuios{Dir: "/repo"}},
		"no pane":    {want: noMultiplexer{}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("TUIOS_PANE_ID", tc.tuios)
			t.Setenv("HERDR_PANE_ID", tc.herdr)
			if got := New("/repo"); got != tc.want {
				t.Fatalf("New = %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestHarnessNamesCoverCommonAgents(t *testing.T) {
	for id, want := range map[string]string{"codex": "Codex", "pi": "pi", "gemini": "Gemini", "cursor": "Cursor"} {
		if names[id] != want {
			t.Errorf("herdr names[%q] = %q, want %q", id, names[id], want)
		}
	}
	for id, want := range map[string]string{"codex": "Codex", "pi": "pi", "gemini-cli": "Gemini", "cursor-agent": "Cursor"} {
		if tuiosNames[id] != want {
			t.Errorf("tuios names[%q] = %q, want %q", id, tuiosNames[id], want)
		}
	}
}

func TestNoMultiplexerReturnsErrors(t *testing.T) {
	t.Setenv("TUIOS_PANE_ID", "")
	t.Setenv("HERDR_PANE_ID", "")
	if _, err := New("/repo").Prompt(context.Background(), review.AgentPane{}, "fix it"); err == nil || !strings.Contains(err.Error(), "no multiplexer found") {
		t.Fatalf("Prompt error = %v", err)
	}
	if _, err := New("/repo").Panes(context.Background()); err == nil || !strings.Contains(err.Error(), "no multiplexer found") {
		t.Fatalf("Panes error = %v", err)
	}
}
