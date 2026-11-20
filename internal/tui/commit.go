package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jfmainville/revunk/internal/review"
)

const commitInfoWidth = 90

func (m Model) selectedCommit() (review.Commit, bool) {
	if m.commit == 0 || m.commit > len(m.commits) {
		return review.Commit{}, false
	}
	return m.commits[m.commit-1], true
}

func (m Model) renderCommitInfo(background string) string {
	c, ok := m.selectedCommit()
	if !ok {
		return background
	}
	x := m.leftWidth()
	width := max(min(m.width-x, commitInfoWidth), 20)
	lines := m.commitInfoLines(c, width-4)
	height := min(len(lines)+2, m.panesHeight())
	overlay := box("commit details", c.Short(), lines, width, height, colorPrimary)
	y := max(m.panesHeight()-height, 0)
	return lipgloss.NewCompositor(lipgloss.NewLayer(background), lipgloss.NewLayer(overlay).X(x).Y(y).Z(1)).Render()
}

func (m Model) commitInfoLines(c review.Commit, width int) []string {
	title := lipgloss.NewStyle().Bold(true).Render(c.Subject)
	lines := []string{
		" " + dimStyle.Render("hash    ") + c.SHA,
		" " + dimStyle.Render("author  ") + c.Author,
		" " + dimStyle.Render("date    ") + c.Date.Local().Format("2006-01-02 15:04"),
		"",
		" " + title,
	}
	for l := range strings.SplitSeq(ansi.Wrap(c.Body, width, ""), "\n") {
		if l != "" {
			lines = append(lines, " "+l)
		}
	}
	lines = append(lines, "")
	switch {
	case m.err != nil:
		return append(lines, " "+errorStyle.Render(m.err.Error()))
	case len(m.files) == 0:
		return append(lines, " "+dimStyle.Render("no file changes"))
	}
	lines = append(lines, " "+primaryStyle.Render(fmt.Sprintf("files %d", len(m.files))))
	for _, f := range m.files {
		lines = append(lines, fmt.Sprintf(" %s %s  +%d -%d", dimStyle.Render(string(f.Status)), f.Path, f.Additions, f.Deletions))
	}
	return lines
}
