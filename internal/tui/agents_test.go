package tui

import (
	"errors"
	"strings"
	"testing"

	"github.com/jfmainville/revunk/internal/review"
)

var (
	codexPane  = review.AgentPane{ID: "w1", Name: "Codex"}
	claudePane = review.AgentPane{ID: "w2", Name: "Claude Code"}
)

func withAgents(t *testing.T, preferred string, panes ...review.AgentPane) (Model, *fakeAgent, *[]string) {
	t.Helper()
	agent, saved := &fakeAgent{panes: panes}, &[]string{}
	m := withComment(t, agent).WithAgent(preferred, func(name string) error {
		*saved = append(*saved, name)
		return nil
	})
	return m, agent, saved
}

func TestSendUsesTheDefaultAgentWithoutAsking(t *testing.T) {
	m, agent, saved := withAgents(t, "claude code", codexPane, claudePane)

	m = pressRun(t, m, "S")

	if m.modal.open || len(agent.sent) != 1 || agent.sent[0] != claudePane || len(*saved) != 0 {
		t.Fatalf("sent to %+v, modal %v, saved %q", agent.sent, m.modal.open, *saved)
	}
	if !strings.Contains(screen(m), "sent 1 comments to Claude Code") {
		t.Fatalf("no sent notice:\n%s", screen(m))
	}
}

func TestSendAsksWhenSeveralAgentsAreOpenAndRemembersThePick(t *testing.T) {
	m, agent, saved := withAgents(t, "", codexPane, claudePane)

	m = pressRun(t, m, "S")
	out := screen(m)
	if !m.modal.open || len(agent.sent) != 0 || !strings.Contains(out, "send 1 comments to") || !strings.Contains(out, "Codex · w1") || !strings.Contains(out, "Claude Code · w2") {
		t.Fatalf("picker not shown before sending:\n%s", out)
	}

	m = pressRun(t, m, "down", "enter")

	if m.modal.open || len(agent.sent) != 1 || agent.sent[0] != claudePane || m.agent != "Claude Code" || strings.Join(*saved, ",") != "Claude Code" {
		t.Fatalf("sent to %+v, default %q, saved %q", agent.sent, m.agent, *saved)
	}
	if again := pressRun(t, withCommentAgain(t, m), "S"); again.modal.open || len(agent.sent) != 2 || agent.sent[1] != claudePane {
		t.Fatal("the picked agent should be used next time without asking")
	}
}

func withCommentAgain(t *testing.T, m Model) Model {
	t.Helper()
	if err := m.svc.EditComment(t.Context(), review.Comment{ID: "c1", Kind: "nit", Body: "rename again"}); err != nil {
		t.Fatal(err)
	}
	comments, err := m.svc.Comments(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	m.comments = comments
	return m
}

func TestSendToAnUnavailableDefaultAgentAsksInstead(t *testing.T) {
	m, agent, _ := withAgents(t, "Gemini", codexPane)

	m = pressRun(t, m, "S")

	if !m.modal.open || len(agent.sent) != 0 || !strings.Contains(screen(m), "the default agent Gemini is not open, pick another") {
		t.Fatalf("unavailable default not reported, sent to %+v:\n%s", agent.sent, screen(m))
	}
	if m = pressRun(t, m, "esc"); m.modal.open || len(agent.sent) != 0 {
		t.Fatal("cancelling the picker should send nothing")
	}

	none, agent, _ := withAgents(t, "Gemini")
	none = pressRun(t, none, "S")
	if none.modal.open || len(agent.sent) != 0 || !strings.Contains(screen(none), "the default agent Gemini is not open in this session, nothing was sent") {
		t.Fatalf("unavailable default with no agents should not start one, sent to %+v:\n%s", agent.sent, screen(none))
	}
}

func TestSendWithoutDefaultStartsAnAgentWhenNoneIsOpen(t *testing.T) {
	m, agent, _ := withAgents(t, "")

	m = pressRun(t, m, "S")

	if m.modal.open || len(agent.sent) != 1 || agent.sent[0] != (review.AgentPane{}) || !strings.Contains(screen(m), "sent 1 comments to a new Claude Code pane") {
		t.Fatalf("sent to %+v:\n%s", agent.sent, screen(m))
	}
}

func TestAgentKeyChangesTheDefaultWithoutSending(t *testing.T) {
	m, agent, saved := withAgents(t, "Codex", codexPane, claudePane)

	m = pressRun(t, m, "g", "a")
	out := screen(m)
	if !m.modal.open || m.modal.cursor != 0 || !strings.Contains(out, "default agent for comments") || !strings.Contains(out, iconCheck+" Codex") {
		t.Fatalf("picker should open on the current default:\n%s", out)
	}
	m = pressRun(t, m, "ctrl+n", "up", "j", "enter")

	if len(agent.sent) != 0 || m.agent != "Claude Code" || strings.Join(*saved, ",") != "Claude Code" || !strings.Contains(screen(m), "Claude Code is now the default agent for comments") {
		t.Fatalf("sent to %+v, default %q, saved %q:\n%s", agent.sent, m.agent, *saved, screen(m))
	}

	single, agent, _ := withAgents(t, "", codexPane)
	if single = pressRun(t, single, "g", "a"); !single.modal.open || len(agent.sent) != 0 {
		t.Fatal("changing the default should open the picker even with one agent")
	}
	empty, _, _ := withAgents(t, "")
	if empty = pressRun(t, empty, "g", "a"); empty.modal.open || !strings.Contains(screen(empty), "no agent is open in this session") {
		t.Fatalf("no agents not explained:\n%s", screen(empty))
	}
}

func TestPickingAnAgentReportsSaveErrors(t *testing.T) {
	m, _, _ := withAgents(t, "", codexPane, claudePane)
	m = m.WithAgent("", func(string) error { return errors.New("read-only file system") })

	m = pressRun(t, m, "g", "a", "enter")

	if m.agent != "Codex" || !strings.Contains(screen(m), "save Codex as the default agent: read-only file system") {
		t.Fatalf("save error not shown:\n%s", screen(m))
	}
}
