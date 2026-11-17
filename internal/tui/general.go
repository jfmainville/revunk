package tui

import (
	"context"
	"fmt"

	tea "charm.land/bubbletea/v2"
)

type generalMsg struct {
	agent string
	err   error
}

func (m Model) sendGeneral(text string) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		agent, err := svc.SendGeneral(context.Background(), text)
		return generalMsg{agent: agent, err: err}
	}
}

func (m *Model) applyGeneral(msg generalMsg) {
	if msg.err != nil && m.tab == tabPulls {
		m.pulls.err = msg.err
		return
	}
	if msg.err != nil {
		m.err = msg.err
		return
	}
	m.notice = fmt.Sprintf("sent the general comment to %s", msg.agent)
}
