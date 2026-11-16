package tui

import (
	"context"
	"fmt"
	"path"
	"strings"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/atotto/clipboard"

	"github.com/jfmainville/revunk/internal/review"
)

type focusArea int

const (
	focusCommits focusArea = iota
	focusFiles
	focusDiff
)

var commentKinds = []string{"comment", "issue", "suggestion", "nit", "question", "praise"}

type rowKind int

const (
	rowFile rowKind = iota
	rowHunk
	rowLine
	rowText
)

type row struct {
	kind rowKind
	file int
	line int
	text string
}

type editorState struct {
	area   textarea.Model
	target review.Comment
	kind   int
}

type loadedMsg struct {
	commits  []review.Commit
	files    []review.File
	viewed   map[string]bool
	comments []review.Comment
	err      error
}

type filesMsg struct {
	commit int
	files  []review.File
	err    error
}

type viewedMsg struct {
	viewed map[string]bool
	err    error
}

type commentsMsg struct {
	comments []review.Comment
	err      error
}

type actionMsg struct {
	notice    string
	clipboard string
	err       error
}

var writeClipboard = clipboard.WriteAll

type Model struct {
	svc        *review.Service
	repo       string
	base       string
	commits    []review.Commit
	commit     int
	files      []review.File
	lines      [][]review.Line
	tokens     [][][]token
	viewed     map[string]bool
	comments   []review.Comment
	rendered   map[int][]string
	rows       []row
	cursor     int
	offset     int
	anchor     int
	fileCursor int
	focus      focusArea
	editing    bool
	editor     editorState
	prefix     string
	help       bool
	loading    bool
	err        error
	notice     string
	width      int
	height     int
}

func New(svc *review.Service, repo, base string) Model {
	return Model{
		svc:      svc,
		repo:     repo,
		base:     base,
		viewed:   map[string]bool{},
		rendered: map[int][]string{},
		anchor:   -1,
		focus:    focusFiles,
		loading:  true,
	}
}

func (m Model) Init() tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		ctx := context.Background()
		var msg loadedMsg
		if msg.commits, msg.err = svc.Commits(ctx); msg.err != nil {
			return msg
		}
		if msg.files, msg.err = svc.Files(ctx, ""); msg.err != nil {
			return msg
		}
		if msg.viewed, msg.err = svc.Viewed(ctx); msg.err != nil {
			return msg
		}
		msg.comments, msg.err = svc.Comments(ctx)
		return msg
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.rendered = map[int][]string{}
		if m.editing {
			m.editor.area.SetWidth(m.editorWidth())
		}
		m.relayout()
	case loadedMsg:
		m.loading, m.err = false, msg.err
		m.commits = msg.commits
		if msg.viewed != nil {
			m.viewed = msg.viewed
		}
		m.comments = msg.comments
		m.setFiles(msg.files)
	case filesMsg:
		if msg.commit != m.commit {
			return m, nil
		}
		m.err = msg.err
		if msg.err == nil {
			m.setFiles(msg.files)
		}
	case viewedMsg:
		m.err = msg.err
		if msg.err == nil {
			m.viewed = msg.viewed
			m.relayout()
		}
	case commentsMsg:
		m.err = msg.err
		if msg.err == nil {
			m.comments = msg.comments
			m.rendered = map[int][]string{}
			m.relayout()
		}
	case actionMsg:
		m.err, m.notice = msg.err, msg.notice
		if msg.clipboard != "" {
			return m, tea.SetClipboard(msg.clipboard)
		}
	case tea.KeyPressMsg:
		m.notice = ""
		if m.editing {
			return m.updateEditor(msg)
		}
		return m.updateKey(msg)
	}
	return m, nil
}

func (m *Model) setFiles(files []review.File) {
	m.files = files
	m.lines = make([][]review.Line, len(files))
	m.tokens = make([][][]token, len(files))
	for i, f := range files {
		m.lines[i] = review.ParseLines(f.Patch)
		m.tokens[i] = highlight(f.Path, m.lines[i])
	}
	m.cursor, m.offset, m.fileCursor, m.anchor = 0, 0, 0, -1
	m.layout()
}

func (m Model) updateKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	if m.help {
		if key == "esc" || key == "?" || key == "q" {
			m.help = false
		}
		return m, nil
	}
	if m.prefix != "" {
		seq := m.prefix + key
		m.prefix = ""
		switch seq {
		case "gg":
			m.cursor, m.offset = 0, 0
			m.syncFile()
		case "]f":
			m.jump(rowFile, 1)
		case "[f":
			m.jump(rowFile, -1)
		case "dd":
			return m, m.deleteComment()
		}
		return m, nil
	}
	switch key {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "?":
		m.help = true
	case "g", "]", "[", "d":
		m.prefix = key
	case "tab":
		m.focus = (m.focus + 1) % 3
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
	case "h":
		m.focus = focusFiles
	case "l":
		m.focus = focusDiff
	case "esc":
		m.anchor = -1
	case "r":
		return m, m.toggleViewed()
	case "C":
		return m, m.openEditor(review.Comment{Commit: m.commitSHA(), Path: m.currentPath()})
	case "S":
		return m, m.sendComments()
	case "y":
		return m, m.copyComments()
	}
	switch m.focus {
	case focusCommits:
		switch key {
		case "j", "down":
			return m, m.selectCommit(1)
		case "k", "up":
			return m, m.selectCommit(-1)
		}
	case focusFiles:
		switch key {
		case "j", "down":
			m.selectFile(m.fileCursor + 1)
		case "k", "up":
			m.selectFile(m.fileCursor - 1)
		case "enter":
			m.focus = focusDiff
		case "c":
			return m, m.openEditor(review.Comment{Commit: m.commitSHA(), Path: m.currentPath()})
		}
	case focusDiff:
		page := max(m.diffHeight(), 1)
		switch key {
		case "j", "down":
			m.move(1)
		case "k", "up":
			m.move(-1)
		case "ctrl+d":
			m.move(page / 2)
		case "ctrl+u":
			m.move(-page / 2)
		case "pgdown", "f":
			m.move(page)
		case "pgup", "b":
			m.move(-page)
		case "G":
			m.move(len(m.rows))
		case "}":
			m.jump(rowHunk, 1)
		case "{":
			m.jump(rowHunk, -1)
		case "v", "V":
			if m.anchor >= 0 {
				m.anchor = -1
			} else if len(m.rows) > 0 && m.rows[m.cursor].kind == rowLine {
				m.anchor = m.cursor
			}
		case "c":
			target, ok := m.lineTarget()
			if !ok {
				m.notice = "move to a line or file header to comment"
				return m, nil
			}
			return m, m.openEditor(target)
		}
	}
	return m, nil
}

func (m Model) updateEditor(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg.String() {
	case "ctrl+c":
		return m, tea.Quit
	case "esc":
		m.editing = false
		m.relayout()
		return m, nil
	case "ctrl+s":
		body := strings.TrimSpace(m.editor.area.Value())
		if body == "" {
			m.notice = "write something before saving"
			return m, nil
		}
		c := m.editor.target
		c.Kind, c.Body = commentKinds[m.editor.kind], body
		m.editing = false
		m.relayout()
		return m, m.saveComment(c)
	case "tab":
		m.editor.kind = (m.editor.kind + 1) % len(commentKinds)
	case "shift+tab":
		m.editor.kind = (m.editor.kind + len(commentKinds) - 1) % len(commentKinds)
	case "ctrl+g":
		t := m.editor.target
		if t.FileLevel() || t.Side == review.SideLeft {
			m.notice = "suggestions apply to added or unchanged lines"
			return m, nil
		}
		m.editor.area.InsertString("```suggestion\n" + strings.Join(m.original(t), "\n") + "\n```")
	default:
		m.editor.area, cmd = m.editor.area.Update(msg)
	}
	m.editor.area.SetHeight(min(max(m.editor.area.LineCount(), 3), 12))
	m.relayout()
	m.revealEditor()
	return m, cmd
}

func (m Model) commitSHA() string {
	if m.commit == 0 {
		return ""
	}
	return m.commits[m.commit-1].SHA
}

func (m Model) currentFile() int {
	if m.focus == focusDiff && len(m.rows) > 0 {
		return m.rows[m.cursor].file
	}
	return m.fileCursor
}

func (m Model) currentPath() string {
	if len(m.files) == 0 {
		return ""
	}
	return m.files[m.currentFile()].Path
}

func (m *Model) selectCommit(delta int) tea.Cmd {
	next := min(max(m.commit+delta, 0), len(m.commits))
	if next == m.commit {
		return nil
	}
	m.commit = next
	svc, sha := m.svc, m.commitSHA()
	return func() tea.Msg {
		files, err := svc.Files(context.Background(), sha)
		return filesMsg{commit: next, files: files, err: err}
	}
}

func (m *Model) selectFile(i int) {
	if len(m.files) == 0 {
		return
	}
	m.fileCursor = min(max(i, 0), len(m.files)-1)
	for r, rw := range m.rows {
		if rw.kind == rowFile && rw.file == m.fileCursor {
			m.cursor, m.offset = r, r
			m.anchor = -1
			return
		}
	}
}

func (m *Model) move(delta int) {
	step := 1
	if delta < 0 {
		step, delta = -1, -delta
	}
	for range delta {
		next := m.cursor + step
		for next >= 0 && next < len(m.rows) && m.rows[next].kind == rowText {
			next += step
		}
		if next < 0 || next >= len(m.rows) {
			break
		}
		m.cursor = next
	}
	m.syncFile()
}

func (m *Model) jump(kind rowKind, step int) {
	for i := m.cursor + step; i >= 0 && i < len(m.rows); i += step {
		if m.rows[i].kind == kind {
			m.cursor = i
			if kind == rowFile {
				m.offset = i
			}
			m.syncFile()
			return
		}
	}
}

func (m *Model) syncFile() {
	if len(m.rows) == 0 {
		return
	}
	m.fileCursor = m.rows[m.cursor].file
	h := max(m.diffHeight(), 1)
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+h {
		m.offset = m.cursor - h + 1
	}
}

func (m *Model) revealEditor() {
	last := -1
	for i, r := range m.rows {
		if r.kind == rowText && r.text == "" {
			last = i
		}
	}
	if h := max(m.diffHeight(), 1); last >= m.offset+h {
		m.offset = last - h + 1
	}
}

func (m Model) lineTarget() (review.Comment, bool) {
	if len(m.rows) == 0 {
		return review.Comment{}, false
	}
	r := m.rows[m.cursor]
	target := review.Comment{Commit: m.commitSHA(), Path: m.files[r.file].Path}
	if r.kind == rowFile {
		return target, true
	}
	if r.kind != rowLine {
		return target, false
	}
	from, to := m.cursor, m.cursor
	if m.anchor >= 0 {
		from, to = min(m.anchor, m.cursor), max(m.anchor, m.cursor)
	}
	var left, right []int
	for _, rw := range m.rows[from : to+1] {
		if rw.kind != rowLine || rw.file != r.file {
			continue
		}
		l := m.lines[rw.file][rw.line]
		if l.Kind == review.LineRemoved {
			left = append(left, l.Old)
		} else {
			right = append(right, l.New)
		}
	}
	target.Side = review.SideRight
	nums := right
	if len(right) == 0 {
		target.Side, nums = review.SideLeft, left
	}
	target.Start, target.End = nums[0], nums[len(nums)-1]
	return target, true
}

func (m *Model) openEditor(target review.Comment) tea.Cmd {
	if target.Path == "" {
		return nil
	}
	area := textarea.New()
	area.ShowLineNumbers = false
	area.Prompt = ""
	area.Placeholder = "write it like you'd say it… markdown and ```suggestion blocks work"
	area.SetWidth(m.editorWidth())
	area.SetHeight(3)
	cmd := area.Focus()
	m.editor = editorState{area: area, target: target}
	m.editing, m.anchor, m.focus = true, -1, focusDiff
	m.relayout()
	m.revealEditor()
	return cmd
}

func (m Model) editorWidth() int {
	return max(commentWidth(m.diffWidth())-4, 10)
}

func (m Model) original(c review.Comment) []string {
	for fi, f := range m.files {
		if f.Path != c.Path {
			continue
		}
		var out []string
		for _, l := range m.lines[fi] {
			if l.Kind != review.LineRemoved && l.Kind != review.LineHunk && l.New >= c.Start && l.New <= c.End {
				out = append(out, l.Text)
			}
		}
		return out
	}
	return nil
}

func (m Model) saveComment(c review.Comment) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		if err := svc.AddComment(context.Background(), c); err != nil {
			return commentsMsg{err: err}
		}
		comments, err := svc.Comments(context.Background())
		return commentsMsg{comments: comments, err: err}
	}
}

func (m *Model) deleteComment() tea.Cmd {
	if len(m.rows) == 0 {
		return nil
	}
	r := m.rows[m.cursor]
	index := -1
	for i, c := range m.comments {
		if !m.visible(c, m.files[r.file].Path) {
			continue
		}
		if (r.kind == rowFile && c.FileLevel()) || (r.kind == rowLine && !c.FileLevel() && anchoredAt(c, m.lines[r.file][r.line])) {
			index = i
		}
	}
	if index < 0 {
		m.notice = "no comment here to delete"
		return nil
	}
	svc := m.svc
	return func() tea.Msg {
		if err := svc.DeleteComment(context.Background(), index); err != nil {
			return commentsMsg{err: err}
		}
		comments, err := svc.Comments(context.Background())
		return commentsMsg{comments: comments, err: err}
	}
}

func (m *Model) sendComments() tea.Cmd {
	if len(m.comments) == 0 {
		m.notice = "no comments to send"
		return nil
	}
	svc, n := m.svc, len(m.comments)
	m.notice = "sending comments to Claude Code…"
	return func() tea.Msg {
		if err := svc.SendComments(context.Background()); err != nil {
			return actionMsg{err: err}
		}
		return actionMsg{notice: fmt.Sprintf("sent %d comments to Claude Code", n)}
	}
}

func (m *Model) copyComments() tea.Cmd {
	if len(m.comments) == 0 {
		m.notice = "no comments to copy"
		return nil
	}
	svc, n := m.svc, len(m.comments)
	return func() tea.Msg {
		text, err := svc.Prompt(context.Background())
		if err != nil {
			return actionMsg{err: err}
		}
		_ = writeClipboard(text)
		return actionMsg{notice: fmt.Sprintf("copied %d comments", n), clipboard: text}
	}
}

func (m Model) toggleViewed() tea.Cmd {
	p := m.currentPath()
	if p == "" {
		return nil
	}
	svc := m.svc
	return func() tea.Msg {
		ctx := context.Background()
		if _, err := svc.ToggleViewed(ctx, p); err != nil {
			return viewedMsg{err: err}
		}
		viewed, err := svc.Viewed(ctx)
		return viewedMsg{viewed: viewed, err: err}
	}
}

func (m Model) visible(c review.Comment, path string) bool {
	return c.Path == path && c.Commit == m.commitSHA()
}

func anchoredAt(c review.Comment, l review.Line) bool {
	if c.Side == review.SideLeft {
		return l.Kind != review.LineAdded && l.Kind != review.LineHunk && l.Old == c.End
	}
	return l.Kind != review.LineRemoved && l.Kind != review.LineHunk && l.New == c.End
}

func (m *Model) relayout() {
	if len(m.rows) == 0 {
		m.layout()
		return
	}
	cur := m.rows[m.cursor]
	m.layout()
	for i, r := range m.rows {
		if r.kind == cur.kind && r.file == cur.file && r.line == cur.line {
			m.cursor = i
			m.syncFile()
			return
		}
	}
	m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
	m.syncFile()
}

func (m *Model) layout() {
	m.rows = nil
	for fi, f := range m.files {
		m.rows = append(m.rows, row{kind: rowFile, file: fi})
		for ci, c := range m.comments {
			if m.visible(c, f.Path) && (c.FileLevel() || !m.anchored(c, fi)) {
				m.addComment(ci, fi)
			}
		}
		if m.editing && m.editor.target.Path == f.Path && m.editor.target.FileLevel() {
			m.addEditor(fi)
		}
		if m.viewed[f.Path] {
			continue
		}
		if len(m.lines[fi]) == 0 {
			m.rows = append(m.rows, row{kind: rowText, file: fi, text: dimStyle.Render(strings.Repeat(" ", gutterWidth) + "no textual changes")})
		}
		for li, l := range m.lines[fi] {
			if l.Kind == review.LineHunk {
				m.rows = append(m.rows, row{kind: rowHunk, file: fi, line: li})
				continue
			}
			m.rows = append(m.rows, row{kind: rowLine, file: fi, line: li})
			for ci, c := range m.comments {
				if m.visible(c, f.Path) && !c.FileLevel() && anchoredAt(c, l) {
					m.addComment(ci, fi)
				}
			}
			if m.editing && m.editor.target.Path == f.Path && !m.editor.target.FileLevel() && anchoredAt(m.editor.target, l) {
				m.addEditor(fi)
			}
		}
	}
}

func (m Model) anchored(c review.Comment, file int) bool {
	for _, l := range m.lines[file] {
		if anchoredAt(c, l) {
			return true
		}
	}
	return false
}

func (m *Model) addComment(index, file int) {
	lines, ok := m.rendered[index]
	if !ok {
		c := m.comments[index]
		var original []string
		if c.Side == review.SideRight {
			original = m.original(c)
		}
		lines = renderComment(c, original, m.diffWidth())
		m.rendered[index] = lines
	}
	for _, l := range lines {
		m.rows = append(m.rows, row{kind: rowText, file: file, text: l})
	}
}

func (m *Model) addEditor(file int) {
	chips := make([]string, len(commentKinds))
	for i, k := range commentKinds {
		chips[i] = dimStyle.Render(k)
		if i == m.editor.kind {
			chips[i] = lipgloss.NewStyle().Background(colorPrimary).Foreground(colorOnPrimary).Render(k)
		}
	}
	lines := append([]string{strings.Join(chips, "  ")}, strings.Split(m.editor.area.View(), "\n")...)
	lines = append(lines, hints(editorHints...))
	for _, l := range commentBox("comment", lineLabel(m.editor.target), lines, m.diffWidth(), true) {
		m.rows = append(m.rows, row{kind: rowText, file: file, text: l})
	}
	m.rows = append(m.rows, row{kind: rowText, file: file})
}

var editorHints = []string{"^s", "save", "⏎", "newline", "⇥", "type", "^g", "suggestion", "esc", "cancel"}

func hints(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, primaryStyle.Render(pairs[i])+" "+dimStyle.Render(pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

func (m Model) leftWidth() int {
	return min(36, m.width/3)
}

func (m Model) diffWidth() int {
	return max(m.width-m.leftWidth()-2, 0)
}

func (m Model) panesHeight() int {
	return max(m.height-3, 0)
}

func (m Model) diffHeight() int {
	return max(m.panesHeight()-2, 0)
}

func (m Model) commitsHeight() int {
	return min(len(m.commits)+3, max(m.panesHeight()/3, 3))
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
	var panes string
	if m.help {
		panes = box("help · esc closes", "", helpLines(), m.width, m.panesHeight(), true)
	} else {
		left := lipgloss.JoinVertical(lipgloss.Left,
			box("commits", fmt.Sprintf("%d", len(m.commits)), m.commitLines(), m.leftWidth(), m.commitsHeight(), m.focus == focusCommits),
			box("files", m.filesCounter(), m.fileLines(), m.leftWidth(), m.panesHeight()-m.commitsHeight(), m.focus == focusFiles),
		)
		panes = lipgloss.JoinHorizontal(lipgloss.Top, left, box("continuous diff", m.rangeLabel(), m.diffLines(), m.width-m.leftWidth(), m.panesHeight(), m.focus == focusDiff))
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.tabBar(), panes, m.statusLine(), ansiFit(m.footer(), m.width))
}

func ansiFit(s string, width int) string {
	return fit(s, width, lipgloss.NewStyle())
}

func box(title, right string, lines []string, width, height int, focused bool) string {
	if width < 4 || height < 2 {
		return ""
	}
	border, titleStyle := lipgloss.NewStyle().Foreground(colorRule), dimStyle
	if focused {
		border, titleStyle = lipgloss.NewStyle().Foreground(colorPrimary), primaryStyle
	}
	inner := width - 2
	head := border.Render("╭─ ") + titleStyle.Render(title) + " "
	tail := ""
	if right != "" {
		tail = " " + dimStyle.Render(right) + " "
	}
	fill := width - lipgloss.Width(head) - lipgloss.Width(tail) - 2
	if fill < 0 {
		tail, fill = "", max(width-lipgloss.Width(head)-2, 0)
	}
	out := []string{ansiFit(head+border.Render(strings.Repeat("─", fill))+tail+border.Render("─╮"), width)}
	for i := range height - 2 {
		line := ""
		if i < len(lines) {
			line = lines[i]
		}
		out = append(out, border.Render("│")+ansiFit(line, inner)+border.Render("│"))
	}
	out = append(out, border.Render("╰"+strings.Repeat("─", inner)+"╯"))
	return strings.Join(out, "\n")
}

func (m Model) tabBar() string {
	tab := lipgloss.NewStyle().Background(colorPrimary).Foreground(colorOnPrimary).Render(" Agent ")
	left := tab + "  " + dimStyle.Render(m.repo+" · local changes on "+m.svc.Branch())
	right := dimStyle.Render("▌ ") + "revunk"
	if gap := m.width - lipgloss.Width(left) - lipgloss.Width(right); gap > 0 {
		return left + strings.Repeat(" ", gap) + right
	}
	return ansiFit(left, m.width)
}

func (m Model) rangeLabel() string {
	if m.commit == 0 {
		return m.base + "…" + m.svc.Branch()
	}
	return m.commits[m.commit-1].Short()
}

func (m Model) filesCounter() string {
	if len(m.files) == 0 {
		return "0/0"
	}
	return fmt.Sprintf("%d/%d", m.fileCursor+1, len(m.files))
}

func (m Model) commitLines() []string {
	inner := m.leftWidth() - 2
	entries := []string{"all commits"}
	for _, c := range m.commits {
		entries = append(entries, c.Short()+" "+c.Subject)
	}
	lines := make([]string, len(entries))
	for i, e := range entries {
		if i == m.commit {
			style := lipgloss.NewStyle().Background(colorCursorline).Foreground(colorForeground)
			if m.focus == focusCommits {
				style = style.Foreground(colorPrimary)
			}
			lines[i] = fit(style.Render(" "+e), inner, style)
			continue
		}
		lines[i] = " " + dimStyle.Render(e)
	}
	start := max(m.commit-(m.commitsHeight()-2)+1, 0)
	return lines[start:]
}

func (m Model) fileLines() []string {
	if m.loading {
		return []string{dimStyle.Render(" loading…")}
	}
	if len(m.files) == 0 {
		return []string{dimStyle.Render(" no changes")}
	}
	inner := m.leftWidth() - 2
	var lines []string
	selected, lastDir := 0, ""
	for i, f := range m.files {
		dir, name, indent := path.Dir(f.Path), f.Path, " "
		if dir != "." {
			name, indent = path.Base(f.Path), "   "
			if dir != lastDir {
				lines = append(lines, " "+dimStyle.Render(dir+"/"))
			}
		}
		lastDir = dir
		base := lipgloss.NewStyle()
		if i == m.fileCursor {
			base = base.Background(colorCursorline)
			selected = len(lines)
		}
		mark := " "
		if m.viewed[f.Path] {
			mark = "✓"
		}
		left := base.Render(indent) + statusStyle(f.Status).Inherit(base).Render(statusLetter(f.Status)) + base.Foreground(colorForeground).Render(" "+name)
		right := base.Foreground(colorAdd).Render(mark + " ")
		gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
		lines = append(lines, fit(left+base.Render(strings.Repeat(" ", max(gap, 0))), inner-lipgloss.Width(right), base)+right)
	}
	visible := max(m.panesHeight()-m.commitsHeight()-2, 1)
	return lines[max(selected-visible+1, 0):]
}

func (m Model) diffLines() []string {
	switch {
	case m.loading:
		return []string{dimStyle.Render(" loading…")}
	case m.err != nil && len(m.files) == 0:
		return []string{errorStyle.Render(" " + m.err.Error())}
	case len(m.files) == 0:
		return []string{dimStyle.Render(" no changes between " + m.base + " and " + m.svc.Branch())}
	}
	width := m.diffWidth()
	end := min(m.offset+m.diffHeight(), len(m.rows))
	lines := make([]string, 0, end-m.offset)
	from, to := -1, -1
	if m.anchor >= 0 {
		from, to = min(m.anchor, m.cursor), max(m.anchor, m.cursor)
	}
	for i := m.offset; i < end; i++ {
		r := m.rows[i]
		cursor := i == m.cursor && m.focus == focusDiff
		switch r.kind {
		case rowFile:
			lines = append(lines, renderFileHeader(m.files[r.file], m.viewed[m.files[r.file].Path], cursor, width))
		case rowHunk:
			lines = append(lines, renderHunk(m.lines[r.file][r.line].Text, width, cursor))
		case rowLine:
			selected := i >= from && i <= to && r.file == m.rows[m.cursor].file
			lines = append(lines, renderLine(m.lines[r.file][r.line], m.tokens[r.file][r.line], width, cursor, selected))
		default:
			lines = append(lines, r.text)
		}
	}
	return lines
}

func (m Model) statusLine() string {
	chip := lipgloss.NewStyle().Foreground(colorOnPrimary).Background(colorPrimary).Render(" NORMAL ")
	switch {
	case m.editing:
		chip = lipgloss.NewStyle().Foreground(colorOnPrimary).Background(colorAdd).Render(" INSERT ")
	case m.anchor >= 0:
		chip = lipgloss.NewStyle().Foreground(colorOnPrimary).Background(colorThread).Render(" VISUAL ")
	}
	message := dimStyle.Render("? shows every key")
	switch {
	case m.err != nil:
		message = errorStyle.Render(m.err.Error())
	case m.notice != "":
		message = primaryStyle.Render(m.notice)
	}
	viewed := 0
	for _, f := range m.files {
		if m.viewed[f.Path] {
			viewed++
		}
	}
	left := chip + " " + primaryStyle.Render("⎇ ") + m.svc.Branch() + "  " + message
	right := dimStyle.Render(fmt.Sprintf("%d comments · %d/%d viewed ", len(m.comments), viewed, len(m.files)))
	if gap := m.width - lipgloss.Width(left) - lipgloss.Width(right); gap > 0 {
		return left + strings.Repeat(" ", gap) + right
	}
	return ansiFit(left, m.width)
}

func (m Model) footer() string {
	switch {
	case m.editing:
		return hints(editorHints...)
	case m.anchor >= 0:
		return hints("c", "comment range", "j k", "extend", "esc", "cancel")
	}
	return hints("c", "comment", "C", "file comment", "v", "range", "r", "viewed", "dd", "delete", "S", "send", "y", "copy", "tab", "focus", "?", "help")
}

func helpLines() []string {
	keys := [][2]string{
		{"move", ""},
		{"j k", "down / up"},
		{"gg G", "top / bottom"},
		{"C-d C-u", "half page"},
		{"]f [f", "next / previous file"},
		{"} {", "next / previous hunk"},
		{"tab h l", "cycle focus, focus files, focus diff"},
		{"", ""},
		{"review", ""},
		{"v V", "visual range"},
		{"c", "comment on line, range or file header"},
		{"C", "comment on the whole file"},
		{"dd", "delete comment under cursor"},
		{"r", "mark file viewed until it changes"},
		{"S", "send every comment to Claude Code"},
		{"y", "copy every comment"},
		{"", ""},
		{"comment editor", ""},
		{"C-s", "save"},
		{"⏎", "newline"},
		{"⇥", "cycle type"},
		{"C-g", "insert a suggestion block for the lines"},
		{"esc", "cancel"},
		{"", ""},
		{"anywhere", ""},
		{"?", "this help"},
		{"q", "quit"},
	}
	lines := make([]string, len(keys))
	for i, k := range keys {
		if k[1] == "" {
			lines[i] = "  " + dimStyle.Render(k[0])
			continue
		}
		lines[i] = fmt.Sprintf("  %s%s", primaryStyle.Render(fmt.Sprintf("%-10s", k[0])), k[1])
	}
	return lines
}
