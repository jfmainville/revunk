package tui

import (
	"errors"
	"strings"
	"testing"
)

func TestGeneralCommentGoesStraightToTheAgent(t *testing.T) {
	agent := &fakeAgent{}
	m := withComment(t, agent)
	m, _ = press(t, m, "M")
	if out := screen(m); !m.modal.open || !strings.Contains(out, "general comment, sent straight to the agent") || m.modalWidth(modalGeneral) >= m.modalWidth(modalEdit) || m.modal.area.Height() > 12 {
		t.Fatalf("M should open a comment modal the size of the others, smaller than the edit modal:\n%s", out)
	}
	if m, _ = press(t, m, "ctrl+s"); !m.modal.open || m.warning != "write something before saving" || len(agent.prompts) != 0 {
		t.Fatal("an empty comment should not be sent")
	}
	m = typeText(t, m, "Split the parser")
	m, cmd := press(t, m, "ctrl+s")
	m = run(t, m, cmd)
	if len(agent.prompts) != 1 || !strings.Contains(agent.prompts[0], "branch feature") || !strings.HasSuffix(agent.prompts[0], "Split the parser") {
		t.Fatalf("prompts = %q", agent.prompts)
	}
	if m.modal.open || m.notice != "sent the general comment to opencode" || len(m.comments) != 1 || m.comments[0].Sent {
		t.Fatalf("the comment should be sent without touching the review comments, notice=%q", m.notice)
	}
	m, _ = press(t, m, "M")
	m = typeText(t, m, "nope")
	if m, _ = press(t, m, "esc", "esc"); m.modal.open || len(agent.prompts) != 1 {
		t.Fatal("esc should cancel without sending")
	}

	failing := withComment(t, &fakeAgent{err: errors.New("no Claude Code session open")})
	failing, _ = press(t, failing, "M")
	failing = typeText(t, failing, "hi")
	failing, cmd = press(t, failing, "ctrl+s")
	if failing = run(t, failing, cmd); !strings.Contains(screen(failing), "no Claude Code session open") {
		t.Fatalf("agent errors should show:\n%s", screen(failing))
	}

	pulls, _ := pullsModel(t)
	pulls = pressRun(t, pulls, "M")
	pulls = typeText(t, pulls, "hi")
	if pulls = pressRun(t, pulls, "ctrl+s"); !strings.Contains(screen(pulls), "no coding agent is configured") {
		t.Fatalf("M should work on the Pull Request tab and show errors there:\n%s", screen(pulls))
	}
	if pulls = pressRun(t, pulls, "g", "d", "M"); pulls.diff.modal.open {
		t.Fatal("the review diff has no agent, so M does nothing there")
	}
}
