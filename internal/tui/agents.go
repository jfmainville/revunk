package tui

import (
	"context"
	"fmt"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/jfmainville/revunk/internal/review"
)

type panesMsg struct {
	panes []review.AgentPane
	send  bool
	err   error
}

func (m Model) WithAgent(name string, save func(string) error) Model {
	m.agent, m.saveAgent = name, save
	return m
}

func (m Model) listPanes(send bool) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		panes, err := svc.AgentPanes(context.Background())
		return panesMsg{panes: panes, send: send, err: err}
	}
}

func (m *Model) applyPanes(msg panesMsg) tea.Cmd {
	m.notice = ""
	if msg.err != nil {
		m.err = msg.err
		return nil
	}
	pane, ok := review.PreferredPane(msg.panes, m.agent)
	switch {
	case msg.send && ok:
		return m.sendTo(pane)
	case len(msg.panes) == 0:
		m.warning = "no agent is open in this session"
		if m.agent != "" {
			m.warning = fmt.Sprintf("the default agent %s is not open in this session", m.agent)
		}
		if msg.send {
			m.warning += ", nothing was sent"
		}
		return nil
	}
	d := modalState{open: true, kind: modalAgents, panes: msg.panes, send: msg.send, title: "default agent for comments"}
	d.normal = true
	if msg.send {
		d.title = fmt.Sprintf("send %d comments to", m.unsent())
	}
	if m.agent != "" && !ok {
		d.warning = fmt.Sprintf("the default agent %s is not open, pick another", m.agent)
	}
	d.cursor = max(slices.IndexFunc(msg.panes, func(p review.AgentPane) bool { return strings.EqualFold(p.Name, m.agent) }), 0)
	m.modal = d
	return nil
}

func (m Model) updateAgents(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key, k, d := msg.String(), m.keys, &m.modal
	switch {
	case is(key, k.PickDown), is(key, k.Down):
		d.cursor = min(d.cursor+1, len(d.panes)-1)
	case is(key, k.PickUp), is(key, k.Up):
		d.cursor = max(d.cursor-1, 0)
	case is(key, k.Open):
		pane, send := d.panes[d.cursor], d.send
		m.modal = modalState{}
		return m, m.pickAgent(pane, send)
	}
	return m, nil
}

func (m *Model) pickAgent(pane review.AgentPane, send bool) tea.Cmd {
	m.agent = pane.Name
	save := m.saveAgent
	remember := func() tea.Msg {
		if err := save(pane.Name); err != nil {
			return actionMsg{err: fmt.Errorf("save %s as the default agent: %w", pane.Name, err)}
		}
		return nil
	}
	if !send {
		m.notice = pane.Name + " is now the default agent for comments"
		return remember
	}
	return tea.Batch(remember, m.sendTo(pane))
}
