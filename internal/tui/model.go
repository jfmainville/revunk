package tui

import (
	"context"
	"fmt"
	"path"
	"reflect"
	"strings"
	"time"

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
	focusComments
)

var commentKinds = []string{"comment", "note", "suggestion"}

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
	part int
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
	notice   string
	err      error
}

type actionMsg struct {
	notice    string
	clipboard string
	err       error
}

type tickMsg struct{}

type polledMsg struct {
	commits  []review.Commit
	commit   string
	files    []review.File
	viewed   map[string]bool
	comments []review.Comment
	err      error
}

var (
	writeClipboard = clipboard.WriteAll
	pollEvery      = 2 * time.Second
)

func tick() tea.Cmd {
	return tea.Tick(pollEvery, func(time.Time) tea.Msg { return tickMsg{} })
}

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
	showList   bool
	hideTree   bool
	listCursor int
	pendingID  string
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
		return m, tick()
	case tickMsg:
		return m, m.poll()
	case polledMsg:
		if msg.err == nil {
			m.applyPoll(msg)
		}
		return m, tick()
	case filesMsg:
		if msg.commit != m.commit {
			return m, nil
		}
		m.err = msg.err
		if msg.err == nil {
			m.setFiles(msg.files)
		}
		for _, c := range m.comments {
			if m.pendingID != "" && c.ID == m.pendingID {
				m.jumpTo(c)
			}
		}
		m.pendingID = ""
	case viewedMsg:
		m.err = msg.err
		if msg.err == nil {
			m.viewed = msg.viewed
			m.relayout()
		}
	case commentsMsg:
		m.err = msg.err
		if msg.err == nil {
			m.notice = msg.notice
			m.comments = msg.comments
			m.rendered = map[int][]string{}
			m.relayout()
		}
	case actionMsg:
		m.err, m.notice = msg.err, msg.notice
		if msg.clipboard != "" {
			return m, tea.SetClipboard(msg.clipboard)
		}
	case tea.MouseClickMsg:
		if msg.Button == tea.MouseLeft && !m.editing && !m.help {
			m.notice, m.prefix = "", ""
			return m, m.click(msg.X, msg.Y)
		}
	case tea.MouseWheelMsg:
		if !m.editing && !m.help {
			step := map[tea.MouseButton]int{tea.MouseWheelDown: 1, tea.MouseWheelUp: -1}[msg.Button]
			return m, m.scroll(msg.X, msg.Y, step)
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

func (m Model) poll() tea.Cmd {
	svc, sha := m.svc, m.commitSHA()
	return func() tea.Msg {
		ctx := context.Background()
		msg := polledMsg{commit: sha}
		if msg.commits, msg.err = svc.Commits(ctx); msg.err != nil {
			return msg
		}
		if msg.files, msg.err = svc.Files(ctx, sha); msg.err != nil {
			return msg
		}
		if msg.viewed, msg.err = svc.Viewed(ctx); msg.err != nil {
			return msg
		}
		msg.comments, msg.err = svc.Comments(ctx)
		return msg
	}
}

func (m *Model) applyPoll(msg polledMsg) {
	sha := m.commitSHA()
	m.commits, m.commit = msg.commits, 0
	for i, c := range msg.commits {
		if c.SHA == sha {
			m.commit = i + 1
		}
	}
	changed := !reflect.DeepEqual(msg.viewed, m.viewed) || !reflect.DeepEqual(msg.comments, m.comments)
	m.viewed, m.comments = msg.viewed, msg.comments
	if m.commitSHA() != msg.commit {
		return
	}
	if !reflect.DeepEqual(msg.files, m.files) {
		m.refreshFiles(msg.files)
		return
	}
	if changed {
		m.rendered = map[int][]string{}
		m.relayout()
	}
}

func (m *Model) refreshFiles(files []review.File) {
	var path, selected string
	cur := row{line: -1}
	if len(m.rows) > 0 {
		cur = m.rows[m.cursor]
		path = m.files[cur.file].Path
	}
	if len(m.files) > 0 {
		selected = m.files[m.fileCursor].Path
	}
	screenRow := m.cursor - m.offset
	m.files, m.anchor, m.rendered = files, -1, map[int][]string{}
	m.lines = make([][]review.Line, len(files))
	m.tokens = make([][][]token, len(files))
	m.fileCursor = 0
	for i, f := range files {
		m.lines[i] = review.ParseLines(f.Patch)
		m.tokens[i] = highlight(f.Path, m.lines[i])
		if f.Path == selected {
			m.fileCursor = i
		}
	}
	m.layout()
	m.cursor = min(m.cursor, max(len(m.rows)-1, 0))
	for i, r := range m.rows {
		if r.kind == rowFile && files[r.file].Path == path {
			m.cursor = i
		}
	}
	for i, r := range m.rows {
		if r.kind == cur.kind && r.line == cur.line && files[r.file].Path == path {
			m.cursor = i
		}
	}
	m.offset = max(m.cursor-screenRow, 0)
	if len(m.rows) > 0 && m.focus == focusDiff {
		m.syncFile()
	}
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
		case "DD":
			return m, m.deleteAllComments()
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
	case "D":
		if len(m.comments) == 0 {
			m.notice = "no comments to delete"
			return m, nil
		}
		m.prefix = key
		m.notice = fmt.Sprintf("press D again to delete all %d comments", len(m.comments))
	case "tab":
		m.cycleFocus(1)
	case "shift+tab":
		m.cycleFocus(-1)
	case "e":
		m.hideTree = !m.hideTree
		if m.hideTree && m.focus < focusDiff {
			m.focus = focusDiff
		}
		m.rendered = map[int][]string{}
		m.relayout()
	case "n":
		m.showList = !m.showList
		m.focus = focusComments
		if !m.showList {
			m.focus = focusDiff
		}
		m.relayout()
	case "f":
		m.jump(rowFile, 1)
	case "F":
		m.jump(rowFile, -1)
	case "h":
		m.jump(rowHunk, 1)
	case "H":
		m.jump(rowHunk, -1)
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
	case focusComments:
		switch key {
		case "j", "down":
			m.listCursor = min(m.listCursor+1, max(len(m.comments)-1, 0))
		case "k", "up":
			m.listCursor = max(m.listCursor-1, 0)
		case "enter":
			if c, ok := m.selectedComment(); ok {
				return m, m.goToComment(c)
			}
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
		case "pgdown":
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
	case "tab", "shift+tab":
		step := 1
		if msg.String() == "shift+tab" {
			step = len(commentKinds) - 1
		}
		m.editor.kind = (m.editor.kind + step) % len(commentKinds)
		if commentKinds[m.editor.kind] == "suggestion" {
			m.injectSuggestion()
		}
	default:
		m.editor.area, cmd = m.editor.area.Update(msg)
	}
	m.editor.area.SetHeight(min(max(m.editor.area.LineCount(), 3), 12))
	m.relayout()
	m.revealEditor()
	return m, cmd
}

func (m *Model) injectSuggestion() {
	t, body := m.editor.target, m.editor.area.Value()
	switch {
	case t.FileLevel() || t.Side == review.SideLeft:
		m.notice = "suggestions apply to added or unchanged lines"
		return
	case strings.Contains(body, "```suggestion"):
		return
	case body != "":
		body += "\n\n"
	}
	m.editor.area.SetValue(body + "```suggestion\n" + strings.Join(m.original(t), "\n") + "\n```")
}

func (m Model) areaAt(x, y int) (focusArea, int) {
	top := 1
	switch {
	case y >= top+m.panesHeight():
		return focusComments, y - top - m.panesHeight() - 1
	case x >= m.leftWidth():
		return focusDiff, y - top - 1
	case y < top+m.commitsHeight():
		return focusCommits, y - top - 1
	}
	return focusFiles, y - top - m.commitsHeight() - 1
}

func (m *Model) click(x, y int) tea.Cmd {
	area, i := m.areaAt(x, y)
	if y < 1 || (area == focusComments && !m.showList) {
		return nil
	}
	inner := map[focusArea]int{focusCommits: m.commitsHeight(), focusFiles: m.panesHeight() - m.commitsHeight(), focusDiff: m.panesHeight(), focusComments: m.listHeight()}[area] - 2
	if i > inner {
		return nil
	}
	m.focus = area
	if i < 0 || i == inner {
		return nil
	}
	switch area {
	case focusCommits:
		if next := max(m.commit-(m.commitsHeight()-2)+1, 0) + i; next <= len(m.commits) {
			return m.selectCommit(next - m.commit)
		}
	case focusFiles:
		if _, files := m.fileLines(); i < len(files) && files[i] >= 0 {
			m.selectFile(files[i])
		}
	case focusDiff:
		r := m.offset + i
		if r < len(m.rows) {
			r -= m.rows[r].part
		}
		if r < len(m.rows) && m.rows[r].kind != rowText {
			m.cursor = r
			m.syncFile()
		}
	case focusComments:
		if c := max(min(m.listCursor, len(m.comments)-1)-(m.listHeight()-2)+1, 0) + i; c < len(m.comments) {
			m.listCursor = c
			return m.goToComment(m.comments[c])
		}
	}
	return nil
}

func (m *Model) scroll(x, y, step int) tea.Cmd {
	area, _ := m.areaAt(x, y)
	switch area {
	case focusCommits:
		return m.selectCommit(step)
	case focusFiles:
		m.selectFile(m.fileCursor + step)
	case focusDiff:
		m.move(3 * step)
	case focusComments:
		m.listCursor = min(max(m.listCursor+step, 0), max(len(m.comments)-1, 0))
	}
	return nil
}

func (m *Model) cycleFocus(step focusArea) {
	for {
		m.focus = (m.focus + step + m.focusCount()) % m.focusCount()
		if !m.hideTree || m.focus >= focusDiff {
			return
		}
	}
}

func (m Model) focusCount() focusArea {
	if m.showList {
		return 4
	}
	return 3
}

func (m Model) selectedComment() (review.Comment, bool) {
	if len(m.comments) == 0 {
		return review.Comment{}, false
	}
	return m.comments[min(m.listCursor, len(m.comments)-1)], true
}

func (m *Model) goToComment(c review.Comment) tea.Cmd {
	if c.Commit == m.commitSHA() {
		m.jumpTo(c)
		return nil
	}
	target := 0
	for i, cm := range m.commits {
		if cm.SHA == c.Commit {
			target = i + 1
		}
	}
	if c.Commit != "" && target == 0 {
		m.notice = "that comment's commit is no longer on the branch"
		return nil
	}
	m.pendingID = c.ID
	return m.selectCommit(target - m.commit)
}

func (m *Model) jumpTo(c review.Comment) {
	target := -1
	for i, r := range m.rows {
		if len(m.files) == 0 || m.files[r.file].Path != c.Path {
			continue
		}
		if r.kind == rowFile && target < 0 {
			target = i
		}
		if r.kind == rowLine && !c.FileLevel() && anchoredAt(c, m.lines[r.file][r.line]) {
			target = i
			break
		}
	}
	if target < 0 {
		m.notice = c.Path + " is not in this diff"
		return
	}
	m.focus, m.cursor, m.offset = focusDiff, target, max(target-2, 0)
	m.syncFile()
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
	last := m.cursor
	for last+1 < len(m.rows) && m.rows[last+1].part > 0 {
		last++
	}
	if last >= m.offset+h {
		m.offset = max(last-h+1, 0)
	}
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
}

func (m *Model) revealEditor() {
	last := -1
	for i, r := range m.rows {
		if r.kind == rowText && r.text == "" && r.part == 0 {
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
	if m.focus == focusComments {
		c, ok := m.selectedComment()
		if !ok {
			m.notice = "no comment here to delete"
			return nil
		}
		return m.deleteByID(c.ID)
	}
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
	return m.deleteByID(m.comments[index].ID)
}

func (m Model) deleteByID(id string) tea.Cmd {
	svc := m.svc
	return func() tea.Msg {
		if err := svc.DeleteComment(context.Background(), id); err != nil {
			return commentsMsg{err: err}
		}
		comments, err := svc.Comments(context.Background())
		return commentsMsg{comments: comments, err: err}
	}
}

func (m Model) unsent() int {
	n := 0
	for _, c := range m.comments {
		if !c.Sent {
			n++
		}
	}
	return n
}

func (m *Model) sendComments() tea.Cmd {
	if m.unsent() == 0 {
		m.notice = "no unsent comments to send"
		return nil
	}
	svc, n := m.svc, m.unsent()
	m.notice = fmt.Sprintf("sending %d comments…", n)
	return func() tea.Msg {
		ctx := context.Background()
		agent, err := svc.SendComments(ctx)
		if err != nil {
			return commentsMsg{err: err}
		}
		comments, err := svc.Comments(ctx)
		return commentsMsg{comments: comments, notice: fmt.Sprintf("sent %d comments to %s, replies show under each one", n, agent), err: err}
	}
}

func (m Model) deleteAllComments() tea.Cmd {
	svc, n := m.svc, len(m.comments)
	return func() tea.Msg {
		ctx := context.Background()
		if err := svc.DeleteAllComments(ctx); err != nil {
			return commentsMsg{err: err}
		}
		comments, err := svc.Comments(ctx)
		return commentsMsg{comments: comments, notice: fmt.Sprintf("deleted %d comments", n), err: err}
	}
}

func (m *Model) copyComments() tea.Cmd {
	if m.unsent() == 0 {
		m.notice = "no unsent comments to copy"
		return nil
	}
	svc, n := m.svc, m.unsent()
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
			for part := range len(wrapTokens(m.tokens[fi][li], m.diffWidth())) - 1 {
				m.rows = append(m.rows, row{kind: rowText, file: fi, line: li, part: part + 1})
			}
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
	for _, l := range commentBox("comment", lineLabel(m.editor.target), lines, m.diffWidth()) {
		m.rows = append(m.rows, row{kind: rowText, file: file, text: l})
	}
	m.rows = append(m.rows, row{kind: rowText, file: file})
}

var editorHints = []string{"^s", "save", "⏎", "newline", "⇥", "type", "esc", "cancel"}

func hints(pairs ...string) string {
	parts := make([]string, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		parts = append(parts, primaryStyle.Render(pairs[i])+" "+dimStyle.Render(pairs[i+1]))
	}
	return strings.Join(parts, "  ")
}

func (m Model) leftWidth() int {
	if m.hideTree {
		return 0
	}
	return min(36, m.width/3)
}

func (m Model) diffWidth() int {
	return max(m.width-m.leftWidth()-2, 0)
}

func (m Model) panesHeight() int {
	return max(m.height-3-m.listHeight(), 0)
}

func (m Model) listHeight() int {
	if !m.showList {
		return 0
	}
	return min(max(len(m.comments), 1), 8, max(m.height/3-2, 1)) + 2
}

func (m Model) listLines() []string {
	if len(m.comments) == 0 {
		return []string{dimStyle.Render(" no comments, press c on a line to add one")}
	}
	cursor := min(m.listCursor, len(m.comments)-1)
	lines := make([]string, len(m.comments))
	for i, c := range m.comments {
		where := c.Path + " " + lineLabel(c)
		if c.Commit != "" {
			where += " @" + review.Commit{SHA: c.Commit}.Short()
		}
		first, _, _ := strings.Cut(strings.TrimSpace(c.Body), "\n")
		base := lipgloss.NewStyle()
		if i == cursor {
			base = base.Background(colorCursorline)
		}
		line := base.Foreground(colorPrimary).Render(fmt.Sprintf(" ◆ %-10s ", c.Kind)) + base.Foreground(colorForeground).Render(where) + base.Foreground(colorDim).Render("  "+commentState(c)+"  "+first)
		lines[i] = fit(line, m.width-2, base)
	}
	return lines[max(cursor-(m.listHeight()-2)+1, 0):]
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
	v.MouseMode = tea.MouseModeCellMotion
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
		panes = box("continuous diff", m.rangeLabel(), m.diffLines(), m.width-m.leftWidth(), m.panesHeight(), m.focus == focusDiff)
		if !m.hideTree {
			files, _ := m.fileLines()
			left := lipgloss.JoinVertical(lipgloss.Left,
				box("commits", fmt.Sprintf("%d", len(m.commits)), m.commitLines(), m.leftWidth(), m.commitsHeight(), m.focus == focusCommits),
				box("files", m.filesCounter(), files, m.leftWidth(), m.panesHeight()-m.commitsHeight(), m.focus == focusFiles),
			)
			panes = lipgloss.JoinHorizontal(lipgloss.Top, left, panes)
		}
	}
	parts := []string{m.tabBar(), panes}
	if m.showList && !m.help {
		parts = append(parts, box("comments", fmt.Sprintf("%d", len(m.comments)), m.listLines(), m.width, m.listHeight(), m.focus == focusComments))
	}
	return lipgloss.JoinVertical(lipgloss.Left, append(parts, m.statusLine(), ansiFit(m.footer(), m.width))...)
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
	additions, deletions := 0, 0
	for _, f := range m.files {
		additions += f.Additions
		deletions += f.Deletions
	}
	return counts(additions, deletions, lipgloss.NewStyle()) + dimStyle.Render(fmt.Sprintf(" · %d/%d", m.fileCursor+1, len(m.files)))
}

func (m Model) commitLines() []string {
	inner := m.leftWidth() - 2
	entries := []string{"all changes"}
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

func (m Model) fileLines() ([]string, []int) {
	if m.loading {
		return []string{dimStyle.Render(" loading…")}, nil
	}
	if len(m.files) == 0 {
		return []string{dimStyle.Render(" no changes")}, nil
	}
	inner := m.leftWidth() - 2
	var lines []string
	var files []int
	selected, lastDir := 0, ""
	for i, f := range m.files {
		dir, name, indent := path.Dir(f.Path), f.Path, " "
		if dir != "." {
			name, indent = path.Base(f.Path), "   "
			if dir != lastDir {
				lines = append(lines, " "+dimStyle.Render(dir+"/"))
				files = append(files, -1)
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
		right := base.Render(" ") + counts(f.Additions, f.Deletions, base) + base.Foreground(colorAdd).Render(" "+mark+" ")
		gap := inner - lipgloss.Width(left) - lipgloss.Width(right)
		lines = append(lines, fit(left+base.Render(strings.Repeat(" ", max(gap, 0))), inner-lipgloss.Width(right), base)+right)
		files = append(files, i)
	}
	start := max(selected-max(m.panesHeight()-m.commitsHeight()-2, 1)+1, 0)
	return lines[start:], files[start:]
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
		if r.part > 0 {
			line := i - r.part
			selected := line >= from && line <= to && r.file == m.rows[m.cursor].file
			lines = append(lines, renderLine(m.lines[r.file][r.line], m.tokens[r.file][r.line], width, r.part, line == m.cursor && m.focus == focusDiff, selected))
			continue
		}
		switch r.kind {
		case rowFile:
			lines = append(lines, renderFileHeader(m.files[r.file], m.viewed[m.files[r.file].Path], cursor, width))
		case rowHunk:
			lines = append(lines, renderHunk(m.lines[r.file][r.line].Text, width, cursor))
		case rowLine:
			selected := i >= from && i <= to && r.file == m.rows[m.cursor].file
			lines = append(lines, renderLine(m.lines[r.file][r.line], m.tokens[r.file][r.line], width, 0, cursor, selected))
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
	case m.focus == focusComments:
		return hints("j k", "move", "⏎", "go to", "dd", "delete", "tab", "focus", "n", "close")
	}
	return hints("f F", "file", "h H", "hunk", "n", "comments", "e", "explorer", "c", "comment", "C", "file comment", "v", "range", "r", "viewed", "dd", "delete", "S", "send", "y", "copy", "tab", "focus", "?", "help")
}

func helpLines() []string {
	keys := [][2]string{
		{"move", ""},
		{"j k", "down / up"},
		{"gg G", "top / bottom"},
		{"C-d C-u", "half page"},
		{"f F  ]f [f", "next / previous file"},
		{"h H  } {", "next / previous hunk"},
		{"tab l", "cycle focus, focus diff"},
		{"click", "focus a pane and select the row"},
		{"wheel", "scroll the pane under the pointer"},
		{"", ""},
		{"review", ""},
		{"v V", "visual range"},
		{"c", "comment on line, range or file header"},
		{"C", "comment on the whole file"},
		{"dd", "delete or resolve comment under cursor"},
		{"n", "show or hide the comments pane"},
		{"e", "show or hide the commits and files panes"},
		{"DD", "delete every comment"},
		{"r", "mark file viewed until it changes"},
		{"S", "send unsent comments to the agent"},
		{"y", "copy unsent comments"},
		{"", ""},
		{"comment editor", ""},
		{"C-s", "save"},
		{"⏎", "newline"},
		{"⇥", "cycle type, suggestion adds the lines as a code block"},
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
