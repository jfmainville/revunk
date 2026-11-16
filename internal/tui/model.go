package tui

import (
	"context"
	"fmt"
	"strings"

	"charm.land/bubbles/v2/help"
	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jfmainville/revunk/internal/review"
)

const fileListWidth = 32

var (
	paneStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("240"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	answerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
)

type keyMap struct {
	Up, Down, PageUp, PageDown, ToggleViewed, Comment, Quit key.Binding
}

func (k keyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.ToggleViewed, k.Comment, k.Quit, k.PageUp, k.PageDown}
}

func (k keyMap) FullHelp() [][]key.Binding { return [][]key.Binding{k.ShortHelp()} }

var keys = keyMap{
	Up:           key.NewBinding(key.WithKeys("up", "k"), key.WithHelp("↑/k", "prev file")),
	Down:         key.NewBinding(key.WithKeys("down", "j"), key.WithHelp("↓/j", "next file")),
	PageUp:       key.NewBinding(key.WithKeys("pgup", "b"), key.WithHelp("pgup/b", "scroll up")),
	PageDown:     key.NewBinding(key.WithKeys("pgdown", "f"), key.WithHelp("pgdn/f", "scroll down")),
	ToggleViewed: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "toggle viewed")),
	Comment:      key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "ask claude")),
	Quit:         key.NewBinding(key.WithKeys("q", "ctrl+c"), key.WithHelp("q", "quit")),
}

type filesLoadedMsg struct {
	files  []review.File
	viewed map[string]bool
	err    error
}

type viewedToggledMsg struct {
	path   string
	viewed bool
	err    error
}

type answeredMsg struct {
	path   string
	answer string
	err    error
}

type Model struct {
	svc     *review.Service
	pr      review.PullRequest
	files   []review.File
	viewed  map[string]bool
	cursor  int
	err     error
	answers map[string]string
	pending string
	input   textinput.Model
	width   int
	height  int
	diff    viewport.Model
	help    help.Model
}

func New(svc *review.Service, pr review.PullRequest) Model {
	diff := viewport.New()
	diff.KeyMap = viewport.KeyMap{PageUp: keys.PageUp, PageDown: keys.PageDown}
	diff.SoftWrap = true
	input := textinput.New()
	input.Prompt = "Ask Claude: "
	input.Placeholder = "question about this file's diff"
	return Model{svc: svc, pr: pr, viewed: map[string]bool{}, answers: map[string]string{}, diff: diff, input: input, help: help.New()}
}

func (m Model) Init() tea.Cmd {
	return func() tea.Msg {
		ctx := context.Background()
		files, err := m.svc.Files(ctx, m.pr)
		if err != nil {
			return filesLoadedMsg{err: err}
		}
		viewed, err := m.svc.Viewed(ctx, m.pr)
		return filesLoadedMsg{files: files, viewed: viewed, err: err}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.resize()
	case filesLoadedMsg:
		m.files, m.err = msg.files, msg.err
		if msg.viewed != nil {
			m.viewed = msg.viewed
		}
		m.showSelected()
	case viewedToggledMsg:
		m.err = msg.err
		if msg.err == nil {
			m.viewed[msg.path] = msg.viewed
		}
	case answeredMsg:
		m.pending, m.err = "", msg.err
		if msg.err == nil {
			m.answers[msg.path] = msg.answer
			if len(m.files) > 0 && m.files[m.cursor].Path == msg.path {
				m.showSelected()
				m.diff.GotoBottom()
			}
		}
	case tea.KeyPressMsg:
		if m.input.Focused() {
			return m.updateInput(msg)
		}
		switch {
		case key.Matches(msg, keys.Quit):
			return m, tea.Quit
		case key.Matches(msg, keys.Up):
			m.moveCursor(-1)
		case key.Matches(msg, keys.Down):
			m.moveCursor(1)
		case key.Matches(msg, keys.ToggleViewed):
			return m, m.toggleViewed()
		case key.Matches(msg, keys.Comment):
			if len(m.files) == 0 || m.pending != "" {
				return m, nil
			}
			return m, m.input.Focus()
		}
	}
	var cmd tea.Cmd
	m.diff, cmd = m.diff.Update(msg)
	return m, cmd
}

func (m Model) updateInput(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.input.Blur()
		m.input.Reset()
		return m, nil
	case "enter":
		question := strings.TrimSpace(m.input.Value())
		m.input.Blur()
		m.input.Reset()
		if question == "" {
			return m, nil
		}
		file := m.files[m.cursor]
		m.pending = file.Path
		return m, func() tea.Msg {
			answer, err := m.svc.Ask(context.Background(), m.pr, file, question)
			return answeredMsg{path: file.Path, answer: answer, err: err}
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *Model) moveCursor(delta int) {
	if len(m.files) == 0 {
		return
	}
	m.cursor = min(max(m.cursor+delta, 0), len(m.files)-1)
	m.showSelected()
}

func (m *Model) showSelected() {
	if len(m.files) == 0 {
		m.diff.SetContent(mutedStyle.Render("No files to review."))
		return
	}
	file := m.files[m.cursor]
	content := file.Patch
	if answer, ok := m.answers[file.Path]; ok {
		content += "\n\n" + answerStyle.Render("Claude:") + "\n" + answer
	}
	m.diff.SetContent(content)
	m.diff.GotoTop()
}

func (m Model) toggleViewed() tea.Cmd {
	if len(m.files) == 0 {
		return nil
	}
	path := m.files[m.cursor].Path
	return func() tea.Msg {
		viewed, err := m.svc.ToggleViewed(context.Background(), m.pr, path)
		return viewedToggledMsg{path: path, viewed: viewed, err: err}
	}
}

func (m *Model) resize() {
	frameW, frameH := paneStyle.GetFrameSize()
	m.diff.SetWidth(max(m.width-fileListWidth-2*frameW, 0))
	m.diff.SetHeight(max(m.paneHeight()-frameH, 0))
	m.help.SetWidth(m.width)
	m.input.SetWidth(max(m.width-lipgloss.Width(m.input.Prompt)-1, 0))
}

func (m Model) paneHeight() int {
	return max(m.height-2, 0)
}

func (m Model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

func (m Model) render() string {
	if m.width == 0 {
		return ""
	}
	frameW, frameH := paneStyle.GetFrameSize()
	innerH := max(m.paneHeight()-frameH, 0)

	fileList := lipgloss.NewStyle().Width(fileListWidth).Height(innerH).MaxHeight(innerH).Render(m.renderFiles())
	diff := lipgloss.NewStyle().Width(max(m.width-fileListWidth-2*frameW, 0)).Height(innerH).Render(m.diff.View())

	panes := lipgloss.JoinHorizontal(lipgloss.Top, paneStyle.Render(fileList), paneStyle.Render(diff))
	status := mutedStyle.Render(m.pr.String())
	if m.pending != "" {
		status = mutedStyle.Render("Asking Claude about " + m.pending + "…")
	}
	if m.err != nil {
		status = errorStyle.Render(m.err.Error())
	}
	footer := m.help.View(keys)
	if m.input.Focused() {
		footer = m.input.View()
	}
	return lipgloss.JoinVertical(lipgloss.Left, status, panes, footer)
}

func (m Model) renderFiles() string {
	var b strings.Builder
	for i, f := range m.files {
		mark := " "
		if m.viewed[f.Path] {
			mark = "✓"
		}
		line := fmt.Sprintf("%s %s +%d -%d", mark, f.Path, f.Additions, f.Deletions)
		if i == m.cursor {
			line = selectedStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
