package tui

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jfmainville/revunk/internal/review"
)

const (
	tabAgent = iota
	tabPulls
)

var pullFilters = []struct{ name, query string }{
	{"assigned", "assignee:@me"},
	{"review requested", "review-requested:@me"},
	{"authored", "author:@me"},
	{"involved", "involves:@me"},
}

var detailTabs = []string{"Overview", "Activity", "Comments", "Commits", "Files"}

const tabComments = 2

var reactions = []struct{ content, emoji string }{
	{"THUMBS_UP", "👍"}, {"THUMBS_DOWN", "👎"}, {"LAUGH", "😄"}, {"HOORAY", "🎉"},
	{"CONFUSED", "😕"}, {"HEART", "💖"}, {"ROCKET", "🚀"}, {"EYES", "👀"},
}

var reviewEvents = []struct {
	event review.ReviewEvent
	label string
}{
	{review.EventComment, "Comment"},
	{review.EventApprove, "Approve"},
	{review.EventRequestChanges, "Request changes"},
}

var (
	pullsPollEvery = 10 * time.Second
	now            = time.Now
)

type pullsState struct {
	list     []review.PullRequest
	cursor   int
	filter   int
	loading  bool
	started  bool
	details  map[string]review.PullRequest
	markdown map[string][]string
	sidebar  bool
	section  int
	item     int
	scroll   int
	err      error
}

type modalKind int

const (
	modalEdit modalKind = iota
	modalAssignees
	modalReviewers
	modalComment
	modalReply
	modalReact
	modalSubmit
)

type modalState struct {
	open    bool
	kind    modalKind
	title   string
	pr      review.PullRequest
	thread  review.Thread
	note    review.Note
	labels  []string
	fields  []textinput.Model
	area    textarea.Model
	hasArea bool
	choices []string
	choice  int
	focus   int
	vimState
	people  []string
	picked  []string
	cursor  int
	loading bool
	failed  error
}

type assignableMsg struct {
	repo   string
	logins []string
	err    error
}

type pullsMsg struct {
	filter int
	list   []review.PullRequest
	err    error
}

type branchPRMsg struct {
	pr   review.PullRequest
	diff bool
	err  error
}

type detailMsg struct {
	key    string
	pr     review.PullRequest
	notice string
	err    error
}

type pullsTickMsg struct{}

type nestedMsg struct {
	gen int
	msg tea.Msg
}

type commentItem struct {
	thread review.Thread
	note   review.Note
}

func (m Model) WithHost(host review.Host, open func(review.PullRequest) *review.Service) Model {
	m.host, m.openReview = host, open
	return m
}

func prKey(pr review.PullRequest) string {
	return fmt.Sprintf("%s#%d", pr.Repo, pr.Number)
}

func pullsTick() tea.Cmd {
	return tea.Tick(pullsPollEvery, func(time.Time) tea.Msg { return pullsTickMsg{} })
}

func nest(cmd tea.Cmd, gen int) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		switch msg := cmd().(type) {
		case nil:
			return nil
		case tea.BatchMsg:
			for i := range msg {
				msg[i] = nest(msg[i], gen)
			}
			return msg
		default:
			if reflect.TypeOf(msg).PkgPath() == reflect.TypeFor[nestedMsg]().PkgPath() {
				return nestedMsg{gen: gen, msg: msg}
			}
			return msg
		}
	}
}

func (m *Model) showPulls() tea.Cmd {
	m.tab = tabPulls
	if m.host == nil {
		m.pulls.err = errors.New("pull requests need the GitHub CLI, gh")
		return nil
	}
	if m.pulls.started {
		return nil
	}
	m.pulls.started, m.pulls.loading = true, true
	return tea.Batch(m.fetchPulls(), pullsTick())
}

func (m Model) fetchPulls() tea.Cmd {
	host, filter := m.host, m.pulls.filter
	return func() tea.Msg {
		list, err := host.PullRequests(context.Background(), pullFilters[filter].query)
		return pullsMsg{filter: filter, list: list, err: err}
	}
}

func (m Model) fetchDetail(pr review.PullRequest) tea.Cmd {
	return m.hostAction(pr, "", nil)
}

func (m Model) hostAction(pr review.PullRequest, notice string, action func(context.Context) error) tea.Cmd {
	host := m.host
	return func() tea.Msg {
		ctx := context.Background()
		if action != nil {
			if err := action(ctx); err != nil {
				return detailMsg{key: prKey(pr), err: err}
			}
		}
		detail, err := host.PullRequest(ctx, pr.Repo, pr.Number)
		return detailMsg{key: prKey(pr), pr: detail, notice: notice, err: err}
	}
}

func (m Model) fetchBranchPR(diff bool) tea.Cmd {
	host := m.host
	return func() tea.Msg {
		pr, err := host.BranchPullRequest(context.Background())
		return branchPRMsg{pr: pr, diff: diff, err: err}
	}
}

func (m *Model) openBranchPR(msg branchPRMsg) tea.Cmd {
	if msg.err != nil {
		m.err = msg.err
		return nil
	}
	if !msg.diff {
		return m.openModal(modalEdit, msg.pr)
	}
	cmd := m.showPulls()
	return tea.Batch(cmd, m.openDiff(msg.pr))
}

func (m Model) selectedPR() (review.PullRequest, bool) {
	if m.pr.Number > 0 {
		return m.pr, true
	}
	if len(m.pulls.list) == 0 {
		return review.PullRequest{}, false
	}
	pr := m.pulls.list[m.pulls.cursor]
	if d, ok := m.pulls.details[prKey(pr)]; ok {
		return d, true
	}
	return pr, true
}

func (m *Model) applyPulls(msg pullsMsg) tea.Cmd {
	if msg.filter != m.pulls.filter {
		return nil
	}
	m.pulls.loading, m.pulls.err = false, msg.err
	if msg.err != nil {
		return nil
	}
	selected := ""
	if pr, ok := m.selectedPR(); ok {
		selected = prKey(pr)
	}
	m.pulls.list, m.pulls.cursor = msg.list, 0
	for i, pr := range msg.list {
		if prKey(pr) == selected {
			m.pulls.cursor = i
		}
	}
	if pr, ok := m.selectedPR(); ok && !pr.Detailed {
		return m.fetchDetail(pr)
	}
	return nil
}

func (m *Model) applyDetail(msg detailMsg) tea.Cmd {
	if m.pr.Number > 0 {
		m.err = msg.err
		if msg.err != nil {
			return nil
		}
		m.pr, m.notice = msg.pr, msg.notice
		return m.poll()
	}
	m.pulls.err = msg.err
	if msg.err != nil {
		return nil
	}
	if msg.notice != "" {
		m.notice = msg.notice
	}
	if m.pulls.details == nil {
		m.pulls.details = map[string]review.PullRequest{}
	}
	m.pulls.details[msg.key] = msg.pr
	for i, pr := range m.pulls.list {
		if prKey(pr) == msg.key {
			m.pulls.list[i].Title = msg.pr.Title
		}
	}
	if pr, ok := m.selectedPR(); ok && prKey(pr) == msg.key {
		m.pulls.item = min(m.pulls.item, max(len(commentItems(pr))-1, 0))
	}
	return nil
}

func (m Model) pullsTicked() tea.Cmd {
	if m.tab != tabPulls || m.host == nil {
		return pullsTick()
	}
	cmds := []tea.Cmd{pullsTick(), m.fetchPulls()}
	if pr, ok := m.selectedPR(); ok {
		cmds = append(cmds, m.fetchDetail(pr))
	}
	return tea.Batch(cmds...)
}

func (m Model) updateNested(msg tea.Msg) (Model, tea.Cmd) {
	updated, cmd := m.diff.Update(msg)
	d := updated.(Model)
	m.diff = &d
	return m, nest(cmd, m.diffGen)
}

func (m Model) typing() bool {
	return m.editing || m.searching || m.modal.open
}

func (m Model) updatePullsKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key, k := msg.String(), m.keys
	if m.diff != nil {
		d := *m.diff
		if !d.typing() && !d.help && d.prefix == "" && is(key, k.Quit) {
			m.diff = nil
			return m, nil
		}
		return m.updateNested(msg)
	}
	if m.help {
		if is(key, k.CloseHelp) {
			m.help = false
		}
		return m, nil
	}
	pr, ok := m.selectedPR()
	if m.prefix != "" {
		seq := m.prefix + " " + key
		m.prefix = ""
		switch {
		case is(seq, k.Top):
			if m.pulls.sidebar {
				m.pulls.scroll, m.pulls.item = 0, 0
				return m, nil
			}
			return m, m.movePull(-len(m.pulls.list))
		case !ok:
		case is(seq, k.EditPR):
			return m, m.openModal(modalEdit, pr)
		case is(seq, k.OpenDiff):
			return m, m.openDiff(pr)
		}
		return m, nil
	}
	if k.isPrefix(key) {
		m.prefix = key
		return m, nil
	}
	switch {
	case is(key, k.Quit), is(key, k.ForceQuit):
		return m, tea.Quit
	case is(key, k.Help):
		m.help = true
		return m, nil
	case is(key, k.NextFilter) && m.host != nil:
		m.pulls.filter = (m.pulls.filter + 1) % len(pullFilters)
		m.pulls.list, m.pulls.cursor, m.pulls.loading, m.pulls.sidebar = nil, 0, true, false
		return m, m.fetchPulls()
	case !ok:
		return m, nil
	case is(key, k.Assignees):
		return m, m.openModal(modalAssignees, pr)
	case is(key, k.Reviewers):
		return m, m.openModal(modalReviewers, pr)
	case is(key, k.Comment):
		return m, m.openModal(modalComment, pr)
	}
	if !m.pulls.sidebar {
		switch {
		case is(key, k.Down):
			return m, m.movePull(1)
		case is(key, k.Up):
			return m, m.movePull(-1)
		case is(key, k.Bottom):
			return m, m.movePull(len(m.pulls.list))
		case is(key, k.Open), is(key, k.NextFocus):
			m.pulls.sidebar = true
		}
		return m, nil
	}
	items := commentItems(pr)
	page := max(m.panesHeight()-4, 1)
	switch {
	case is(key, k.Cancel):
		m.pulls.sidebar = false
	case is(key, k.NextFocus), is(key, k.PrevFocus):
		step := 1
		if is(key, k.PrevFocus) {
			step = len(detailTabs) - 1
		}
		m.pulls.section = (m.pulls.section + step) % len(detailTabs)
		m.pulls.scroll, m.pulls.item = 0, 0
	case is(key, k.Down) && m.pulls.section == tabComments:
		m.pulls.item = min(m.pulls.item+1, max(len(items)-1, 0))
		m.revealItem(pr)
	case is(key, k.Up) && m.pulls.section == tabComments:
		m.pulls.item = max(m.pulls.item-1, 0)
		m.revealItem(pr)
	case is(key, k.Down):
		m.scrollDetails(pr, 1)
	case is(key, k.Up):
		m.scrollDetails(pr, -1)
	case is(key, k.HalfDown):
		m.scrollDetails(pr, page/2)
	case is(key, k.HalfUp):
		m.scrollDetails(pr, -page/2)
	case is(key, k.Bottom):
		m.scrollDetails(pr, 1<<20)
	case m.pulls.section != tabComments:
	case len(items) == 0:
		m.notice = "no comments yet, press " + label(k.Comment) + " to add one"
	case is(key, k.Reply):
		return m, m.openNoteModal(modalReply, pr, items[m.pulls.item])
	case is(key, k.React):
		return m, m.openNoteModal(modalReact, pr, items[m.pulls.item])
	case is(key, k.Resolve):
		t := items[m.pulls.item].thread
		if t.ID == "" {
			m.notice = "only review threads on code can be resolved"
			return m, nil
		}
		notice := "resolved the thread"
		if t.Resolved {
			notice = "reopened the thread"
		}
		host := m.host
		return m, m.hostAction(pr, notice, func(ctx context.Context) error { return host.Resolve(ctx, t.ID, !t.Resolved) })
	}
	return m, nil
}

func (m *Model) movePull(delta int) tea.Cmd {
	if len(m.pulls.list) == 0 {
		return nil
	}
	next := min(max(m.pulls.cursor+delta, 0), len(m.pulls.list)-1)
	if next == m.pulls.cursor {
		return nil
	}
	m.pulls.cursor, m.pulls.scroll, m.pulls.item = next, 0, 0
	if pr, _ := m.selectedPR(); !pr.Detailed {
		return m.fetchDetail(pr)
	}
	return nil
}

func (m *Model) scrollDetails(pr review.PullRequest, delta int) {
	lines, _ := m.detailLines(pr, m.sidebarWidth()-2)
	m.pulls.scroll = min(max(m.pulls.scroll+delta, 0), max(len(lines)-m.detailHeight(), 0))
}

func (m *Model) revealItem(pr review.PullRequest) {
	lines, starts := m.detailLines(pr, m.sidebarWidth()-2)
	if m.pulls.item >= len(starts) {
		return
	}
	start, end, h := starts[m.pulls.item], len(lines)-1, m.detailHeight()
	if m.pulls.item+1 < len(starts) {
		end = starts[m.pulls.item+1] - 1
	}
	if end >= m.pulls.scroll+h {
		m.pulls.scroll = end - h + 1
	}
	if start < m.pulls.scroll {
		m.pulls.scroll = start
	}
}

func commentItems(pr review.PullRequest) []commentItem {
	var items []commentItem
	for _, t := range pr.Threads {
		for _, n := range t.Notes {
			if strings.TrimSpace(n.Body) != "" {
				items = append(items, commentItem{thread: t, note: n})
			}
		}
	}
	return items
}

func (m *Model) reviewPullRequest(host review.Host, pr review.PullRequest) {
	m.host, m.pr, m.tab, m.every = host, pr, tabPulls, reviewPollEvery
	m.keys.Send.SetEnabled(false)
	m.keys.Copy.SetEnabled(false)
	m.keys.DeleteAll.SetEnabled(false)
	m.keys.Submit.SetEnabled(true)
}

func (m *Model) openDiff(pr review.PullRequest) tea.Cmd {
	if m.openReview == nil {
		return nil
	}
	d := New(m.openReview(pr), m.root, pr.Base)
	d.reviewPullRequest(m.host, pr)
	m.diffGen++
	m.diff = &d
	updated, _ := m.updateNested(tea.WindowSizeMsg{Width: m.width, Height: m.height})
	*m = updated
	return nest(d.Init(), m.diffGen)
}

func newField(value string, width int) textinput.Model {
	f := textinput.New()
	f.Prompt = ""
	f.SetValue(value)
	f.SetWidth(width)
	return f
}

func (m Model) modalWidth() int {
	return max(min(m.width-6, 90), 20)
}

func (m *Model) openModal(kind modalKind, pr review.PullRequest) tea.Cmd {
	w := m.modalWidth() - 4
	d := modalState{open: true, kind: kind, pr: pr}
	switch kind {
	case modalEdit:
		d.title, d.labels = fmt.Sprintf("edit #%d", pr.Number), []string{"title"}
		d.fields = []textinput.Model{newField(pr.Title, w-12)}
		d.hasArea = true
	case modalAssignees, modalReviewers:
		d.title, d.picked = fmt.Sprintf("assignees of #%d", pr.Number), slices.Clone(pr.Assignees)
		if kind == modalReviewers {
			d.title, d.picked = fmt.Sprintf("reviewers of #%d", pr.Number), reviewerLogins(pr)
		}
		d.labels, d.fields = []string{"search"}, []textinput.Model{newField("", w-12)}
		d.people, d.loading = slices.Clone(d.picked), true
	case modalComment:
		d.title, d.hasArea = fmt.Sprintf("comment on #%d", pr.Number), true
	case modalSubmit:
		d.title, d.hasArea = fmt.Sprintf("submit your review of #%d", pr.Number), true
		for _, e := range reviewEvents {
			d.choices = append(d.choices, e.label)
		}
	}
	if d.hasArea {
		d.area = textarea.New()
		d.area.ShowLineNumbers = false
		d.area.Prompt = ""
		d.area.KeyMap.InsertNewline = m.keys.Newline
		d.area.Placeholder = "markdown works"
		d.area.SetWidth(w)
		d.area.SetHeight(max(min(m.panesHeight()-8-len(d.fields)-len(d.choices), 12), 1))
		if kind == modalEdit {
			d.area.SetValue(pr.Body)
			d.area.MoveToBegin()
		}
	}
	m.modal = d
	cmd := m.modal.setFocus(0)
	if d.loading {
		host := m.host
		cmd = tea.Batch(cmd, func() tea.Msg {
			logins, err := host.Assignable(context.Background(), pr.Repo)
			return assignableMsg{repo: pr.Repo, logins: logins, err: err}
		})
	}
	return cmd
}

func (m Model) modalHints() []hint {
	if m.modal.picker() {
		return m.keys.pickerHints()
	}
	return m.keys.modalHints(len(m.modal.choices) > 0, m.modal.mode())
}

func reviewerLogins(pr review.PullRequest) []string {
	var out []string
	for _, r := range pr.Reviewers {
		out = append(out, r.Login)
	}
	return out
}

func (d *modalState) picker() bool {
	return d.kind == modalAssignees || d.kind == modalReviewers
}

func (d *modalState) applyAssignable(msg assignableMsg) {
	if !d.open || !d.picker() || msg.repo != d.pr.Repo {
		return
	}
	d.loading, d.failed = false, msg.err
	for _, l := range msg.logins {
		if !slices.Contains(d.people, l) && !(d.kind == modalReviewers && l == d.pr.Author) {
			d.people = append(d.people, l)
		}
	}
}

func (d *modalState) matches() []string {
	query := strings.ToLower(strings.TrimSpace(d.fields[0].Value()))
	var out []string
	for _, p := range d.people {
		if strings.Contains(strings.ToLower(p), query) {
			out = append(out, p)
		}
	}
	return out
}

func (d *modalState) pickKey(msg tea.KeyPressMsg, k keyMap) tea.Cmd {
	key, matches := msg.String(), d.matches()
	switch {
	case is(key, k.PickDown):
		d.cursor = min(d.cursor+1, max(len(matches)-1, 0))
	case is(key, k.PickUp):
		d.cursor = max(d.cursor-1, 0)
	case is(key, k.Toggle):
		if d.cursor < len(matches) {
			p := matches[d.cursor]
			if i := slices.Index(d.picked, p); i >= 0 {
				d.picked = slices.Delete(d.picked, i, i+1)
			} else {
				d.picked = append(d.picked, p)
			}
		}
	default:
		var cmd tea.Cmd
		d.fields[0], cmd = d.fields[0].Update(msg)
		d.cursor = 0
		return cmd
	}
	return nil
}

func (m *Model) openNoteModal(kind modalKind, pr review.PullRequest, item commentItem) tea.Cmd {
	cmd := m.openModal(modalComment, pr)
	m.modal.kind, m.modal.thread, m.modal.note = kind, item.thread, item.note
	m.modal.title = "reply to @" + item.note.Author
	if kind == modalReact {
		m.modal.title, m.modal.hasArea = "react to @"+item.note.Author+", picking one you gave removes it", false
		for _, r := range reactions {
			m.modal.choices = append(m.modal.choices, r.emoji+" "+strings.ToLower(strings.ReplaceAll(r.content, "_", " ")))
		}
		return m.modal.setFocus(0)
	}
	return cmd
}

func (d *modalState) parts() int {
	n := len(d.fields)
	if len(d.choices) > 0 {
		n++
	}
	if d.hasArea {
		n++
	}
	return n
}

func (d *modalState) onChoices() bool {
	return len(d.choices) > 0 && d.focus == 0
}

func (d *modalState) field() int {
	i := d.focus
	if len(d.choices) > 0 {
		i--
	}
	if i < len(d.fields) {
		return i
	}
	return -1
}

func (d *modalState) setFocus(i int) tea.Cmd {
	d.focus = i
	for j := range d.fields {
		d.fields[j].Blur()
	}
	d.area.Blur()
	switch f := d.field(); {
	case d.onChoices():
		return nil
	case f >= 0:
		return d.fields[f].Focus()
	case d.hasArea:
		return d.area.Focus()
	}
	return nil
}

func (m Model) updateModal(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key, k := msg.String(), m.keys
	var cmd tea.Cmd
	switch d := &m.modal; {
	case is(key, k.ForceQuit):
		return m, tea.Quit
	case is(key, k.Cancel) && (d.normal || !d.onArea()):
		m.modal = modalState{}
	case is(key, k.Save):
		return m.saveModal()
	case is(key, k.NextFocus), is(key, k.PrevFocus):
		step := 1
		if is(key, k.PrevFocus) {
			step = d.parts() - 1
		}
		cmd = d.setFocus((d.focus + step) % max(d.parts(), 1))
	case d.onChoices():
		switch {
		case is(key, k.Down):
			d.choice = min(d.choice+1, len(d.choices)-1)
		case is(key, k.Up):
			d.choice = max(d.choice-1, 0)
		case is(key, k.Open) && d.parts() == 1:
			return m.saveModal()
		}
	case d.picker():
		cmd = d.pickKey(msg, k)
	case d.field() >= 0:
		d.fields[d.field()], cmd = d.fields[d.field()].Update(msg)
	default:
		cmd = d.update(&d.area, msg, k)
	}
	return m, cmd
}

func (d *modalState) onArea() bool {
	return d.hasArea && !d.onChoices() && d.field() < 0
}

func (d *modalState) mode() string {
	if !d.onArea() {
		return ""
	}
	return d.vimState.mode()
}

func quote(n review.Note) string {
	var b strings.Builder
	for l := range strings.SplitSeq(strings.TrimSpace(n.Body), "\n") {
		b.WriteString("> " + l + "\n")
	}
	return "> @" + n.Author + " wrote\n>\n" + b.String() + "\n"
}

func (m Model) saveModal() (tea.Model, tea.Cmd) {
	d, host := m.modal, m.host
	body := strings.TrimSpace(d.area.Value())
	if d.hasArea && body == "" && d.kind != modalEdit && !(d.kind == modalSubmit && d.choice == 1) {
		m.notice = "write something before saving"
		return m, nil
	}
	pr := d.pr
	var notice string
	var action func(context.Context) error
	switch d.kind {
	case modalEdit:
		title := strings.TrimSpace(d.fields[0].Value())
		if title == "" {
			m.notice = "the title can't be empty"
			return m, nil
		}
		notice = fmt.Sprintf("updated #%d", pr.Number)
		action = func(ctx context.Context) error { return host.EditPullRequest(ctx, pr, title, d.area.Value()) }
	case modalAssignees:
		notice = fmt.Sprintf("updated the assignees of #%d", pr.Number)
		action = func(ctx context.Context) error { return host.SetPeople(ctx, pr, d.picked, reviewerLogins(pr)) }
	case modalReviewers:
		notice = fmt.Sprintf("updated the reviewers of #%d", pr.Number)
		action = func(ctx context.Context) error { return host.SetPeople(ctx, pr, pr.Assignees, d.picked) }
	case modalComment:
		notice = fmt.Sprintf("commented on #%d", pr.Number)
		action = func(ctx context.Context) error { return host.Comment(ctx, pr, body) }
	case modalReply:
		notice = "replied to @" + d.note.Author
		action = func(ctx context.Context) error {
			if d.thread.ID == "" {
				return host.Comment(ctx, pr, quote(d.note)+body)
			}
			return host.Reply(ctx, d.thread.ID, body)
		}
	case modalReact:
		r := reactions[d.choice]
		add := !slices.ContainsFunc(d.note.Reactions, func(x review.Reaction) bool { return x.Content == r.content && x.Mine })
		notice = "reacted " + r.emoji
		if !add {
			notice = "removed " + r.emoji
		}
		action = func(ctx context.Context) error { return host.React(ctx, d.note.ID, r.content, add) }
	case modalSubmit:
		e := reviewEvents[d.choice]
		notice = fmt.Sprintf("submitted %s on #%d", strings.ToLower(e.label), pr.Number)
		action = func(ctx context.Context) error {
			fresh, err := host.PullRequest(ctx, pr.Repo, pr.Number)
			if err != nil {
				return err
			}
			return host.SubmitReview(ctx, fresh, e.event, body)
		}
	}
	m.modal = modalState{}
	return m, m.hostAction(pr, notice, action)
}

func (m Model) sidebarWidth() int {
	return m.width - m.pullsWidth()
}

func (m Model) pullsWidth() int {
	return m.width * 45 / 100
}

func (m Model) detailHeight() int {
	return max(m.panesHeight()-4, 1)
}

func (m Model) renderPulls() string {
	var panes string
	switch {
	case m.help:
		panes = box("help · "+label(m.keys.CloseHelp)+" closes", "", m.helpLines(), m.width, m.panesHeight(), colorPrimary)
	case m.modal.open:
		panes = m.renderModal()
	default:
		title, right := "details", ""
		var lines []string
		if pr, ok := m.selectedPR(); ok {
			title = fmt.Sprintf("#%d %s", pr.Number, pr.Repo)
			right = age(pr.UpdatedAt)
			lines = m.detailView(pr)
		}
		panes = lipgloss.JoinHorizontal(lipgloss.Top,
			box("pull requests", pullFilters[m.pulls.filter].name+" · "+fmt.Sprint(len(m.pulls.list)), m.pullLines(), m.pullsWidth(), m.panesHeight(), focusColor(!m.pulls.sidebar)),
			box(title, right, lines, m.sidebarWidth(), m.panesHeight(), focusColor(m.pulls.sidebar)),
		)
	}
	return lipgloss.JoinVertical(lipgloss.Left, m.tabBar(), panes, m.pullsStatus(), ansiFit(m.pullsFooter(), m.width))
}

func (m Model) pullsFooter() string {
	switch {
	case m.modal.open:
		return hints(m.modalHints())
	case m.pulls.sidebar:
		return hints(m.keys.detailsHints())
	}
	return hints(m.keys.pullsHints())
}

func (m Model) pullsStatus() string {
	chip := m.modeChip()
	message := dimStyle.Render(label(m.keys.Help) + " shows every key")
	switch {
	case m.pulls.err != nil:
		message = errorStyle.Render(m.pulls.err.Error())
	case m.notice != "":
		message = primaryStyle.Render(m.notice)
	}
	return ansiFit(chip+" "+message, m.width)
}

func checkClass(state string) int {
	switch state {
	case "SUCCESS":
		return 0
	case "FAILURE", "ERROR", "CANCELLED", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE":
		return 2
	case "SKIPPED", "NEUTRAL", "STALE":
		return 3
	}
	return 1
}

var checkIcons = []struct {
	icon  string
	color lipgloss.Style
}{
	{"✓", lipgloss.NewStyle().Foreground(colorAdd)},
	{"●", lipgloss.NewStyle().Foreground(colorPrimary)},
	{"✗", lipgloss.NewStyle().Foreground(colorDel)},
	{"○", dimStyle},
}

func checksIcon(checks []review.Check) string {
	if len(checks) == 0 {
		return " "
	}
	worst := 0
	for _, c := range checks {
		if class := checkClass(c.State); class != 3 && (class == 2 || worst == 0) {
			worst = max(worst, class)
		}
	}
	return checkIcons[worst].color.Render(checkIcons[worst].icon)
}

func age(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	switch d := now().Sub(t); {
	case d < time.Minute:
		return "now"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
	return t.Format("2006-01-02")
}

func (m Model) pullLines() []string {
	switch {
	case m.pulls.loading:
		return []string{dimStyle.Render(" loading…")}
	case len(m.pulls.list) == 0 && m.pulls.err == nil:
		return []string{dimStyle.Render(" no open pull requests, " + label(m.keys.NextFilter) + " changes the filter")}
	}
	inner := m.pullsWidth() - 2
	lines := make([]string, len(m.pulls.list))
	for i, pr := range m.pulls.list {
		base := lipgloss.NewStyle()
		bar, titleColor := base.Render(" "), colorForeground
		if i == m.pulls.cursor {
			base = base.Background(colorCursorline)
			bar, titleColor = base.Foreground(colorPrimary).Render("▌"), colorPrimary
		}
		icon := checksIcon(m.pulls.details[prKey(pr)].Checks)
		if pr.Draft {
			icon = dimStyle.Render("◌")
		}
		left := bar + base.Render(icon) + base.Foreground(colorDim).Render(fmt.Sprintf(" #%-5d", pr.Number)) + base.Foreground(titleColor).Render(pr.Title)
		right := base.Foreground(colorDim).Render(fmt.Sprintf("  %s %s ", pr.Repo, age(pr.UpdatedAt)))
		lines[i] = fit(left, max(inner-lipgloss.Width(right), 0), base) + right
	}
	return lines[max(m.pulls.cursor-(m.panesHeight()-2)+1, 0):]
}

func (m Model) detailView(pr review.PullRequest) []string {
	width := m.sidebarWidth() - 2
	tabs := make([]string, len(detailTabs))
	for i, t := range detailTabs {
		switch {
		case i == m.pulls.section && m.pulls.sidebar:
			tabs[i] = lipgloss.NewStyle().Background(colorPrimary).Foreground(colorOnPrimary).Render(" " + t + " ")
		case i == m.pulls.section:
			tabs[i] = primaryStyle.Render(" " + t + " ")
		default:
			tabs[i] = dimStyle.Render(" " + t + " ")
		}
	}
	head := []string{strings.Join(tabs, ""), lipgloss.NewStyle().Foreground(colorRule).Render(strings.Repeat("─", width))}
	lines, _ := m.detailLines(pr, width)
	return append(head, lines[min(m.pulls.scroll, len(lines)):]...)
}

func (p pullsState) render(body string, width int) []string {
	key := fmt.Sprintf("%d\x00%s", width, body)
	if lines, ok := p.markdown[key]; ok {
		return lines
	}
	lines := renderMarkdown(body, max(width, 10))
	if p.markdown != nil {
		p.markdown[key] = lines
	}
	return lines
}

func people(pr review.PullRequest) string {
	var out []string
	for _, r := range pr.Reviewers {
		icon := map[string]string{"APPROVED": "✓", "CHANGES_REQUESTED": "✗", "PENDING": "●", "COMMENTED": "💬"}[r.State]
		out = append(out, strings.TrimSpace(icon+" @"+r.Login))
	}
	return strings.Join(out, "  ")
}

func mentions(logins []string) string {
	if len(logins) == 0 {
		return dimStyle.Render("none")
	}
	return "@" + strings.Join(logins, " @")
}

func (m Model) detailLines(pr review.PullRequest, width int) ([]string, []int) {
	field := func(name, value string) string {
		if value == "" {
			value = dimStyle.Render("none")
		}
		return " " + dimStyle.Render(fmt.Sprintf("%-10s", name)) + " " + value
	}
	if !pr.Detailed {
		return []string{" " + lipgloss.NewStyle().Bold(true).Render(pr.Title), "", dimStyle.Render(" loading…")}, nil
	}
	var lines []string
	var starts []int
	switch m.pulls.section {
	case 0:
		state := pr.State
		if pr.Draft {
			state += " · draft"
		}
		lines = append(lines,
			" "+lipgloss.NewStyle().Bold(true).Foreground(colorForeground).Render(pr.Title),
			" "+primaryStyle.Render(state)+dimStyle.Render(" · @"+pr.Author+" wants to merge "+pr.Head+" into "+pr.Base),
			" "+counts(pr.Additions, pr.Deletions, lipgloss.NewStyle())+dimStyle.Render(fmt.Sprintf(" · %d files · %d commits", pr.ChangedFiles, len(pr.Commits))),
			"",
			field("assignees", mentions(pr.Assignees)),
			field("reviewers", people(pr)),
			field("labels", strings.Join(pr.Labels, ", ")),
		)
		summary := make([]int, len(checkIcons))
		for _, c := range pr.Checks {
			summary[checkClass(c.State)]++
		}
		var parts []string
		for i, n := range summary {
			if n > 0 {
				parts = append(parts, checkIcons[i].color.Render(fmt.Sprintf("%s %d", checkIcons[i].icon, n)))
			}
		}
		lines = append(lines, field("checks", strings.Join(parts, "  ")))
		for _, c := range pr.Checks {
			ic := checkIcons[checkClass(c.State)]
			lines = append(lines, "   "+ic.color.Render(ic.icon)+" "+c.Name+dimStyle.Render(" "+strings.ToLower(c.State)))
		}
		lines = append(lines, "", " "+dimStyle.Render("description"))
		if strings.TrimSpace(pr.Body) == "" {
			lines = append(lines, dimStyle.Render(" no description, "+label(m.keys.EditPR)+" adds one"))
		}
		for _, l := range m.pulls.render(pr.Body, width-2) {
			lines = append(lines, " "+l)
		}
	case 1:
		var notes []commentItem
		for _, t := range pr.Threads {
			for _, n := range t.Notes {
				notes = append(notes, commentItem{thread: t, note: n})
			}
		}
		slices.SortStableFunc(notes, func(a, b commentItem) int { return a.note.CreatedAt.Compare(b.note.CreatedAt) })
		for _, it := range notes {
			lines = append(lines, " "+dimStyle.Render(fmt.Sprintf("%-5s", age(it.note.CreatedAt)))+" "+primaryStyle.Render("@"+it.note.Author)+" "+activity(it))
		}
		if len(lines) == 0 {
			lines = append(lines, dimStyle.Render(" no activity yet"))
		}
	case tabComments:
		items := commentItems(pr)
		if len(items) == 0 {
			lines = append(lines, dimStyle.Render(" no comments yet, press "+label(m.keys.Comment)+" to add one"))
		}
		for i, it := range items {
			starts = append(starts, len(lines))
			if i == 0 || it.thread.ID != items[i-1].thread.ID || it.thread.ID == "" {
				if i > 0 {
					lines = append(lines, "")
					starts[i]++
				}
				lines = append(lines, " "+lipgloss.NewStyle().Foreground(colorThread).Render(threadTitle(it)))
			}
			bar := " "
			if i == m.pulls.item {
				bar = primaryStyle.Render("▌")
			}
			meta := primaryStyle.Render("@"+it.note.Author) + dimStyle.Render(" · "+age(it.note.CreatedAt))
			if it.note.State == "PENDING" {
				meta += dimStyle.Render(" · pending")
			}
			lines = append(lines, bar+" "+meta)
			for _, l := range m.pulls.render(it.note.Body, width-4) {
				lines = append(lines, bar+"   "+l)
			}
			if r := reactionLine(it.note.Reactions); r != "" {
				lines = append(lines, bar+"   "+r)
			}
		}
	case 3:
		for _, c := range pr.Commits {
			lines = append(lines, " "+primaryStyle.Render(c.Short())+" "+c.Subject)
		}
	case 4:
		for _, f := range pr.Files {
			lines = append(lines, " "+statusStyle(f.Status).Render(statusLetter(f.Status))+" "+f.Path+"  "+counts(f.Additions, f.Deletions, lipgloss.NewStyle()))
		}
	}
	return lines, starts
}

func threadTitle(it commentItem) string {
	switch {
	case it.thread.ID != "":
		title := it.thread.Path + " " + lineLabel(review.Comment{Start: it.thread.Start, End: it.thread.End})
		if it.thread.Resolved {
			title += " · resolved"
		}
		if it.thread.Outdated {
			title += " · outdated"
		}
		return title
	case it.note.State != "":
		return "review · " + strings.ToLower(strings.ReplaceAll(it.note.State, "_", " "))
	}
	return "conversation"
}

func activity(it commentItem) string {
	switch {
	case it.thread.ID != "" && it.note.ID != it.thread.Notes[0].ID:
		return "replied on " + it.thread.Path
	case it.thread.ID != "":
		return "commented on " + it.thread.Path
	}
	switch it.note.State {
	case "APPROVED":
		return "approved"
	case "CHANGES_REQUESTED":
		return "requested changes"
	case "COMMENTED":
		return "reviewed"
	case "DISMISSED":
		return "had a review dismissed"
	}
	return "commented"
}

func reactionLine(list []review.Reaction) string {
	var parts []string
	for _, r := range list {
		for _, known := range reactions {
			if known.content != r.Content {
				continue
			}
			text := fmt.Sprintf("%s %d", known.emoji, r.Count)
			if r.Mine {
				text = primaryStyle.Render(text)
			}
			parts = append(parts, text)
		}
	}
	return strings.Join(parts, "  ")
}

func (m Model) renderModal() string {
	d := m.modal
	var lines []string
	if len(d.choices) > 0 {
		for i, c := range d.choices {
			style, mark := dimStyle, "  "
			if i == d.choice {
				style, mark = primaryStyle, "▸ "
				if d.onChoices() {
					style = lipgloss.NewStyle().Background(colorPrimary).Foreground(colorOnPrimary)
				}
			}
			lines = append(lines, " "+style.Render(mark+c))
		}
		lines = append(lines, "")
	}
	for i, f := range d.fields {
		style := dimStyle
		if d.field() == i {
			style = primaryStyle
		}
		lines = append(lines, " "+style.Render(fmt.Sprintf("%-10s", d.labels[i]))+" "+f.View())
	}
	if d.picker() {
		lines = append(lines, "")
		matches := d.matches()
		rows := max(m.panesHeight()-8, 1)
		top := max(d.cursor-rows+1, 0)
		for i, p := range matches[top:min(top+rows, len(matches))] {
			mark, style := "[ ] ", dimStyle
			if slices.Contains(d.picked, p) {
				mark, style = "[x] ", primaryStyle
			}
			if top+i == d.cursor {
				style = lipgloss.NewStyle().Background(colorPrimary).Foreground(colorOnPrimary)
			}
			lines = append(lines, " "+style.Render(mark+"@"+p))
		}
		switch {
		case d.failed != nil:
			lines = append(lines, " "+errorStyle.Render(d.failed.Error()))
		case d.loading:
			lines = append(lines, " "+dimStyle.Render("loading…"))
		case len(d.people) == 0:
			lines = append(lines, " "+dimStyle.Render("no one to pick"))
		case len(matches) == 0:
			lines = append(lines, " "+dimStyle.Render("no one matches"))
		}
	}
	if d.hasArea {
		if len(d.fields) > 0 {
			style := lipgloss.NewStyle().Foreground(colorRule)
			if d.onArea() {
				style = primaryStyle
			}
			name := "description "
			lines = append(lines, "", " "+style.Render(name+strings.Repeat("─", max(m.modalWidth()-4-len(name), 0))))
		}
		for _, l := range strings.Split(d.area.View(), "\n") {
			lines = append(lines, " "+l)
		}
	}
	lines = append(lines, "", " "+hints(m.modalHints()))
	modal := box(d.title, "", lines, m.modalWidth(), min(len(lines)+2, m.panesHeight()), colorPrimary)
	return lipgloss.Place(m.width, m.panesHeight(), lipgloss.Center, lipgloss.Center, modal)
}

func (m Model) pullsMouse(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.diff != nil {
		return m.updateNested(msg)
	}
	if m.help || m.modal.open {
		return m, nil
	}
	pr, _ := m.selectedPR()
	switch msg := msg.(type) {
	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft || msg.Y < 1 || msg.Y > m.panesHeight() {
			return m, nil
		}
		m.prefix = ""
		m.pulls.sidebar = msg.X >= m.pullsWidth()
		if i := tabAt(msg.X-m.pullsWidth()-1, detailTabs, 0); m.pulls.sidebar && msg.Y == 2 && i >= 0 {
			m.pulls.section, m.pulls.scroll, m.pulls.item = i, 0, 0
			return m, nil
		}
		row := msg.Y - 2
		if i := max(m.pulls.cursor-(m.panesHeight()-2)+1, 0) + row; !m.pulls.sidebar && row >= 0 && row < m.panesHeight()-2 && i < len(m.pulls.list) {
			return m, m.movePull(i - m.pulls.cursor)
		}
	case tea.MouseWheelMsg:
		step := map[tea.MouseButton]int{tea.MouseWheelDown: 1, tea.MouseWheelUp: -1}[msg.Button]
		if msg.X >= m.pullsWidth() {
			m.scrollDetails(pr, 3*step)
			return m, nil
		}
		return m, m.movePull(step)
	}
	return m, nil
}
