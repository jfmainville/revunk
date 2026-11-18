package tui

import (
	"charm.land/bubbles/v2/key"
	"cmp"
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
	"github.com/charmbracelet/x/ansi"

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

var mergeMethods = []struct {
	method review.MergeMethod
	label  string
}{
	{review.MergeCommit, "Create a merge commit"},
	{review.MergeSquash, "Squash and merge"},
	{review.MergeRebase, "Rebase and merge"},
}

var (
	pullsPollEvery = 10 * time.Second
	now            = time.Now
)

type pullsState struct {
	list     []review.PullRequest
	cursor   int
	filter   int
	failures int
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
	modalURL
	modalMerge
	modalLabels
	modalActions
	modalClose
	modalCreate
)

type modalState struct {
	open     bool
	kind     modalKind
	title    string
	warning  string
	pr       review.PullRequest
	thread   review.Thread
	note     review.Note
	labels   []string
	fields   []textinput.Model
	area     textarea.Model
	headline textarea.Model
	hasArea  bool
	titled   bool
	drops    []drop
	focus    int
	vimState
	expanded bool
	options  []string
	picked   []string
	cursor   int
	loading  bool
	failed   error
}

type drop struct {
	name    string
	choices []string
	choice  int
}

func (d drop) value() string {
	return d.choices[d.choice]
}

type pickerMsg struct {
	repo  string
	items []string
	err   error
}

type pullsMsg struct {
	filter int
	list   []review.PullRequest
	err    error
}

type openPRMsg struct {
	pr  review.PullRequest
	err error
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

func (m Model) pullsTick() tea.Cmd {
	return tea.Tick(backoff(pullsPollEvery, m.pulls.failures), func(time.Time) tea.Msg { return pullsTickMsg{} })
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
	return m.startPulls()
}

func (m *Model) startPulls() tea.Cmd {
	if m.host == nil || m.pulls.started || m.pr.Number > 0 {
		return nil
	}
	m.pulls.started, m.pulls.loading = true, true
	return tea.Batch(m.fetchPulls(), m.pullsTick())
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

func (m Model) fetchBranchPR() tea.Cmd {
	host := m.host
	return func() tea.Msg {
		pr, err := host.BranchPullRequest(context.Background())
		return openPRMsg{pr: pr, err: err}
	}
}

func (m Model) fetchPR(repo string, number int) tea.Cmd {
	host := m.host
	return func() tea.Msg {
		pr, err := host.PullRequest(context.Background(), repo, number)
		if err != nil {
			err = fmt.Errorf("open %s#%d: %w", repo, number, err)
		}
		return openPRMsg{pr: pr, err: err}
	}
}

func (m *Model) openPR(msg openPRMsg) tea.Cmd {
	if msg.err != nil && m.tab == tabPulls {
		m.pulls.err = msg.err
		return nil
	}
	if msg.err != nil {
		m.err = msg.err
		return nil
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
		m.pulls.failures = min(m.pulls.failures+1, maxBackoff)
		return nil
	}
	m.pulls.failures = 0
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
	if m.tab != tabPulls || m.host == nil || m.diff != nil || m.blurred {
		return m.pullsTick()
	}
	cmds := []tea.Cmd{m.pullsTick(), m.fetchPulls()}
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
		if !d.typing() && !d.help && d.prefix == "" && d.anchor < 0 && d.query == "" && is(key, k.Cancel) {
			m.diff = nil
			return m, nil
		}
		return m.updateNested(msg)
	}
	if m.help {
		if is(key, k.Cancel) {
			m.help = false
		}
		return m, nil
	}
	pr, ok := m.selectedPR()
	if m.prefix != "" {
		seq := m.prefix + " " + key
		m.prefix = ""
		switch {
		case is(seq, k.OpenURL) && m.host != nil:
			return m, m.openModal(modalURL, review.PullRequest{})
		case is(seq, k.CreatePR) && m.host != nil:
			return m, m.openModal(modalCreate, review.PullRequest{})
		case is(seq, k.Top):
			if m.pulls.sidebar {
				m.pulls.scroll, m.pulls.item = 0, 0
				return m, nil
			}
			return m, m.movePull(-len(m.pulls.list))
		case !ok:
		case is(seq, k.WindowLeft):
			m.pulls.sidebar = false
		case is(seq, k.WindowRight):
			m.pulls.sidebar = true
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
	case is(key, k.Actions) && m.host != nil:
		return m, m.openModal(modalActions, pr)
	case !ok:
		return m, nil
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
		m.warning = "no comments yet, add one from the actions with " + label(k.Actions)
	case is(key, k.Reply):
		return m, m.openNoteModal(modalReply, pr, items[m.pulls.item])
	case is(key, k.React):
		return m, m.openNoteModal(modalReact, pr, items[m.pulls.item])
	case is(key, k.Resolve):
		t := items[m.pulls.item].thread
		if t.ID == "" {
			m.warning = "only review threads on code can be resolved"
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
	m.keys.Resolve.SetEnabled(false)
	m.keys.Reply.SetEnabled(false)
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
	f.SetVirtualCursor(false)
	f.SetValue(value)
	f.SetWidth(width)
	return f
}

func (m Model) modalWidth(kind modalKind) int {
	if kind == modalEdit || kind == modalCreate {
		return max(min(m.width-6, 130), 20)
	}
	return max(min(m.width-6, 90), 20)
}

const gutter = 13

func (m *Model) openModal(kind modalKind, pr review.PullRequest) tea.Cmd {
	w := m.modalWidth(kind) - 4
	d := modalState{open: true, kind: kind, pr: pr}
	switch kind {
	case modalEdit:
		d.title, d.hasArea, d.titled = fmt.Sprintf("edit #%d", pr.Number), true, true
	case modalAssignees, modalReviewers, modalLabels:
		d.title, d.picked = fmt.Sprintf("assignees of #%d", pr.Number), slices.Clone(pr.Assignees)
		switch kind {
		case modalReviewers:
			d.title, d.picked = fmt.Sprintf("reviewers of #%d", pr.Number), reviewerLogins(pr)
		case modalLabels:
			d.title, d.picked = fmt.Sprintf("labels of #%d", pr.Number), slices.Clone(pr.Labels)
		}
		d.labels, d.fields = []string{"search"}, []textinput.Model{newField("", w-12)}
		d.options, d.loading = slices.Clone(d.picked), true
	case modalComment:
		d.title, d.hasArea = fmt.Sprintf("comment on #%d", pr.Number), true
	case modalURL:
		d.title, d.labels = "review a pull request from its GitHub URL", []string{"url"}
		d.fields = []textinput.Model{newField("", w-12)}
		d.fields[0].Placeholder = "https://github.com/owner/repo/pull/123"
	case modalMerge:
		d.title, d.warning = fmt.Sprintf("merge #%d into %s", pr.Number, pr.Base), mergeWarning(pr.MergeState)
		d.hasArea, d.titled = true, true
		methods := drop{name: "method"}
		for _, mm := range mergeMethods {
			if slices.Contains(pr.MergeMethods, mm.method) {
				methods.choices = append(methods.choices, mm.label)
			}
		}
		d.drops = []drop{methods}
	case modalSubmit:
		d.title, d.hasArea = fmt.Sprintf("submit your review of #%d", pr.Number), true
		events := drop{name: "review"}
		for _, e := range reviewEvents {
			events.choices = append(events.choices, e.label)
		}
		d.drops = []drop{events}
	case modalClose:
		d.title, d.hasArea = fmt.Sprintf("close #%d without merging", pr.Number), true
	case modalCreate:
		d.title, d.hasArea, d.titled = "create a pull request", true, true
		current := ""
		if m.svc != nil {
			current = m.svc.Branch()
		}
		d.drops = []drop{{name: "branch", choices: []string{current}}, {name: "into", choices: []string{""}}}
	case modalActions:
		d.title, d.labels = "actions", []string{"search"}
		if pr.Number > 0 {
			d.title = fmt.Sprintf("actions for #%d", pr.Number)
		}
		d.fields = []textinput.Model{newField("", w-12)}
		d.fields[0].Placeholder = "type to filter"
	}
	if d.hasArea {
		d.area = textarea.New()
		d.area.ShowLineNumbers = false
		d.area.Prompt = ""
		d.area.KeyMap.InsertNewline = m.keys.Newline
		d.area.Placeholder = "markdown works"
		switch kind {
		case modalSubmit:
			d.area.Placeholder = "optional message, markdown works"
		case modalClose:
			d.area.Placeholder = "optional comment, markdown works"
		}
		d.area.SetWidth(w)
		rows := 2 * len(d.drops)
		d.area.SetHeight(max(min(m.panesHeight()-8-len(d.fields)-rows, 12), 1))
		if d.titled && kind != modalMerge {
			d.area.SetHeight(max(m.panesHeight()-10-rows, 1))
		}
		if d.titled {
			d.area.SetWidth(w - gutter)
			d.area.Placeholder = "the description, markdown works"
			d.headline = textarea.New()
			d.headline.ShowLineNumbers, d.headline.Prompt, d.headline.Placeholder = false, "", "the title"
			d.headline.KeyMap.InsertNewline = key.NewBinding(key.WithDisabled())
			d.headline.SetWidth(w - gutter)
			d.headline.SetHeight(1)
		}
		switch kind {
		case modalMerge:
			d.fillMerge()
		case modalEdit:
			d.headline.SetValue(pr.Title)
			d.area.SetValue(pr.Body)
			d.headline.MoveToBegin()
			d.area.MoveToBegin()
			if pr.Base != "" {
				d.drops = []drop{{name: "into", choices: []string{pr.Base}}}
			}
		}
	}
	m.modal = d
	m.modal.normal = kind == modalEdit || kind == modalCreate || kind == modalMerge
	focus := 0
	if kind == modalEdit {
		focus = len(d.drops)
	}
	cmd := m.modal.setFocus(focus)
	if len(d.drops) > 0 && (kind == modalEdit || kind == modalCreate) {
		host, repo := m.host, pr.Repo
		cmd = tea.Batch(cmd, func() tea.Msg {
			list, def, err := host.Branches(context.Background(), repo)
			return branchesMsg{repo: repo, list: list, def: def, err: err}
		})
	}
	if d.loading {
		host := m.host
		cmd = tea.Batch(cmd, func() tea.Msg {
			load := host.Assignable
			if kind == modalLabels {
				load = host.Labels
			}
			items, err := load(context.Background(), pr.Repo)
			return pickerMsg{repo: pr.Repo, items: items, err: err}
		})
	}
	return cmd
}

type action struct {
	icon, title string
	key         key.Binding
	run         func(m *Model, pr review.PullRequest) tea.Cmd
}

func modal(kind modalKind) func(*Model, review.PullRequest) tea.Cmd {
	return func(m *Model, pr review.PullRequest) tea.Cmd { return m.openModal(kind, pr) }
}

func (m Model) actions() []action {
	k := m.keys
	var list []action
	if m.modal.pr.Number > 0 {
		list = append(list, action{"✎", "Edit the title and description", k.EditPR, modal(modalEdit)})
		if m.pr.Number == 0 {
			list = append(list, action{"±", "Review the diff", k.OpenDiff, func(m *Model, pr review.PullRequest) tea.Cmd { return m.openDiff(pr) }})
		}
		list = append(list,
			action{"❝", "Comment", key.Binding{}, modal(modalComment)},
			action{"✓", "Submit a review", key.Binding{}, modal(modalSubmit)},
			action{"⎇", "Merge", key.Binding{}, func(m *Model, pr review.PullRequest) tea.Cmd { return m.openMerge(pr) }},
			action{"✗", "Close without merging", key.Binding{}, func(m *Model, pr review.PullRequest) tea.Cmd {
				if pr.State != "OPEN" {
					m.warning = fmt.Sprintf("#%d is already %s", pr.Number, strings.ToLower(pr.State))
					return nil
				}
				return m.openModal(modalClose, pr)
			}},
			action{"◉", "Assignees", key.Binding{}, modal(modalAssignees)},
			action{"◎", "Reviewers", key.Binding{}, modal(modalReviewers)},
			action{"◆", "Labels", key.Binding{}, modal(modalLabels)},
			action{"✦", "Request a review from Copilot", key.Binding{}, func(m *Model, pr review.PullRequest) tea.Cmd {
				host := m.host
				return m.hostAction(pr, fmt.Sprintf("requested a review of #%d from Copilot", pr.Number), func(ctx context.Context) error { return host.RequestCopilotReview(ctx, pr) })
			}},
		)
	}
	if m.pr.Number == 0 {
		list = append(list,
			action{"↗", "Review a pull request from its URL", k.OpenURL, func(m *Model, _ review.PullRequest) tea.Cmd { return m.openModal(modalURL, review.PullRequest{}) }},
			action{"+", "Create a pull request", k.CreatePR, func(m *Model, _ review.PullRequest) tea.Cmd { return m.openModal(modalCreate, review.PullRequest{}) }},
		)
	}
	return list
}

type branchesMsg struct {
	repo, def string
	list      []string
	err       error
}

func first(value string, list []string) []string {
	out := []string{value}
	for _, b := range list {
		if b != value {
			out = append(out, b)
		}
	}
	return out
}

func (m *Model) applyBranches(msg branchesMsg) {
	d := &m.modal
	if !d.open || (d.kind != modalEdit && d.kind != modalCreate) || d.pr.Repo != msg.repo || len(d.drops) == 0 {
		return
	}
	if msg.err != nil {
		d.warning = "couldn't list the branches, " + msg.err.Error()
		return
	}
	into := &d.drops[len(d.drops)-1]
	if d.kind == modalCreate {
		d.drops[0].choices = first(d.drops[0].value(), msg.list)
		into.choices = first(msg.def, msg.list)
		return
	}
	into.choices = first(into.value(), msg.list)
}

type createdMsg struct {
	pr   review.PullRequest
	head string
	err  error
}

func (m Model) createPR(head, base, title, body string) tea.Cmd {
	host := m.host
	return func() tea.Msg {
		pr, err := host.CreatePullRequest(context.Background(), head, base, title, body)
		return createdMsg{pr: pr, head: head, err: err}
	}
}

func (m *Model) applyCreated(msg createdMsg) tea.Cmd {
	if msg.err != nil {
		m.pulls.err = fmt.Errorf("create a pull request from %s: %w", msg.head, msg.err)
		return nil
	}
	m.applyDetail(detailMsg{key: prKey(msg.pr), pr: msg.pr, notice: fmt.Sprintf("created #%d from %s into %s", msg.pr.Number, msg.head, msg.pr.Base)})
	return m.fetchPulls()
}

func (m Model) matchingActions() []action {
	query := strings.ToLower(strings.TrimSpace(m.modal.fields[0].Value()))
	var out []action
	for _, a := range m.actions() {
		if strings.Contains(strings.ToLower(a.title), query) {
			out = append(out, a)
		}
	}
	return out
}

func (m Model) updateActions(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	key, k, d := msg.String(), m.keys, &m.modal
	matches := m.matchingActions()
	if ok, cmd := d.listKey(key, k, len(matches)); ok {
		return m, cmd
	}
	switch {
	case is(key, k.Open):
		if d.cursor >= len(matches) {
			return m, nil
		}
		pr := d.pr
		m.modal = modalState{}
		cmd := matches[d.cursor].run(&m, pr)
		return m, cmd
	case !d.normal:
		var cmd tea.Cmd
		d.fields[0], cmd = d.fields[0].Update(msg)
		d.cursor = 0
		return m, cmd
	}
	return m, nil
}

func (d *modalState) listKey(key string, k keyMap, n int) (bool, tea.Cmd) {
	switch {
	case is(key, k.PickDown), d.normal && is(key, k.Down):
		d.cursor = min(d.cursor+1, max(n-1, 0))
	case is(key, k.PickUp), d.normal && is(key, k.Up):
		d.cursor = max(d.cursor-1, 0)
	case !d.normal && is(key, k.Cancel):
		d.normal = true
		d.fields[0].Blur()
	case d.normal && (is(key, k.Search) || is(key, k.Insert)):
		d.normal = false
		return true, d.fields[0].Focus()
	default:
		return false, nil
	}
	return true, nil
}

func (m *Model) openMerge(pr review.PullRequest) tea.Cmd {
	switch {
	case !pr.Detailed:
		m.warning = fmt.Sprintf("wait for #%d to load before merging", pr.Number)
	case pr.State != "OPEN":
		m.warning = fmt.Sprintf("#%d is %s, only open pull requests merge", pr.Number, strings.ToLower(pr.State))
	case pr.Draft:
		m.warning = fmt.Sprintf("#%d is a draft, mark it ready for review on GitHub before merging", pr.Number)
	case len(pr.MergeMethods) == 0:
		m.warning = pr.Repo + " allows no merge method, check its settings on GitHub"
	default:
		return m.openModal(modalMerge, pr)
	}
	return nil
}

func (d *modalState) mergeMethod() review.MergeMethod {
	for _, mm := range mergeMethods {
		if mm.label == d.drops[0].value() {
			return mm.method
		}
	}
	return ""
}

func (d *modalState) fillMerge() {
	message := d.pr.MergeMessages[d.mergeMethod()]
	d.headline.SetValue(message.Headline)
	d.area.SetValue(message.Body)
	d.headline.Placeholder, d.area.Placeholder = "the commit title", "the commit description"
	if d.mergeMethod() == review.MergeRebase {
		d.headline.SetValue("")
		d.area.SetValue("")
		d.headline.Placeholder, d.area.Placeholder = "rebasing keeps the message of each commit", ""
	}
	d.headline.MoveToBegin()
	d.area.MoveToBegin()
}

func mergeWarning(state string) string {
	switch state {
	case "DIRTY":
		return "it conflicts with the base branch, resolve the conflicts before merging"
	case "BEHIND":
		return "the branch is behind its base, update it before merging"
	case "BLOCKED":
		return "branch protection blocks it until the required reviews and checks pass"
	case "UNSTABLE":
		return "some checks are failing"
	case "UNKNOWN":
		return "GitHub is still checking whether it can merge"
	}
	return ""
}

const dropdownRows = 8

func (d drop) width() int {
	width := len("default branch") + 4
	for _, c := range d.choices {
		width = max(width, len(c)+4)
	}
	return width
}

func (m Model) dropdownLines() []string {
	d := m.modal
	var lines []string
	for n, dr := range d.drops {
		labelStyle, value := dimStyle, lipgloss.NewStyle().Foreground(colorOnPrimary).Background(colorRule)
		if d.focus == n {
			labelStyle, value = primaryStyle, lipgloss.NewStyle().Foreground(colorOnPrimary).Background(colorPrimary)
		}
		arrow := "▾"
		if d.expanded && d.focus == n {
			arrow = "▴"
		}
		shown := cmp.Or(dr.value(), "default branch")
		if n > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, " "+labelStyle.Render(fmt.Sprintf("%-10s", dr.name))+" "+value.Render(fmt.Sprintf(" %-*s%s ", dr.width()-2, shown, arrow)))
	}
	return append(lines, "")
}

func (m Model) dropdownList() string {
	d := m.modal
	if !d.expanded || !d.onChoices() {
		return ""
	}
	dr := d.drops[d.focus]
	var items []string
	top := max(dr.choice-dropdownRows+1, 0)
	for i, c := range dr.choices[top:min(top+dropdownRows, len(dr.choices))] {
		style, mark := lipgloss.NewStyle().Foreground(colorDim).Background(colorPanel), "  "
		if i+top == dr.choice {
			style, mark = lipgloss.NewStyle().Foreground(colorOnPrimary).Background(colorPrimary), "▸ "
		}
		items = append(items, style.Render(fmt.Sprintf("%-*s", dr.width(), mark+c)))
	}
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colorPrimary).BorderBackground(colorPanel).Render(strings.Join(items, "\n"))
}

func (m Model) modalHints() []hint {
	if m.modal.picker() {
		return m.keys.pickerHints(m.modal.normal)
	}
	if m.modal.kind == modalActions {
		return m.keys.actionsHints(m.modal.normal)
	}
	return m.keys.modalHints(m.modal.parts() > 1, len(m.modal.drops) > 0, m.modal.mode())
}

func reviewerLogins(pr review.PullRequest) []string {
	var out []string
	for _, r := range pr.Reviewers {
		out = append(out, r.Login)
	}
	return out
}

func (d *modalState) picker() bool {
	return d.kind == modalAssignees || d.kind == modalReviewers || d.kind == modalLabels
}

func (d *modalState) applyPicker(msg pickerMsg) {
	if !d.open || !d.picker() || msg.repo != d.pr.Repo {
		return
	}
	d.loading, d.failed = false, msg.err
	for _, l := range msg.items {
		if !slices.Contains(d.options, l) && !(d.kind == modalReviewers && l == d.pr.Author) {
			d.options = append(d.options, l)
		}
	}
}

func (d *modalState) matches() []string {
	query := strings.ToLower(strings.TrimSpace(d.fields[0].Value()))
	var out []string
	for _, p := range d.options {
		if strings.Contains(strings.ToLower(p), query) {
			out = append(out, p)
		}
	}
	return out
}

func (d *modalState) pickKey(msg tea.KeyPressMsg, k keyMap) tea.Cmd {
	key, matches := msg.String(), d.matches()
	if ok, cmd := d.listKey(key, k, len(matches)); ok {
		return cmd
	}
	switch {
	case is(key, k.Toggle):
		if d.cursor < len(matches) {
			p := matches[d.cursor]
			if i := slices.Index(d.picked, p); i >= 0 {
				d.picked = slices.Delete(d.picked, i, i+1)
			} else {
				d.picked = append(d.picked, p)
			}
		}
	case !d.normal:
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
		var choices []string
		for _, r := range reactions {
			choices = append(choices, r.emoji+" "+strings.ToLower(strings.ReplaceAll(r.content, "_", " ")))
		}
		m.modal.drops = []drop{{choices: choices}}
		return m.modal.setFocus(0)
	}
	return cmd
}

func (d *modalState) parts() int {
	n := len(d.fields) + len(d.drops)
	if d.titled {
		n++
	}
	if d.hasArea {
		n++
	}
	return n
}

func (d *modalState) onChoices() bool {
	return d.focus < len(d.drops)
}

func (d *modalState) field() int {
	i := d.focus - len(d.drops)
	if i >= 0 && i < len(d.fields) {
		return i
	}
	return -1
}

func (d *modalState) dropdown() bool {
	return d.hasArea && len(d.drops) > 0
}

func (d *modalState) setFocus(i int) tea.Cmd {
	d.focus, d.expanded = i, false
	for j := range d.fields {
		d.fields[j].Blur()
	}
	d.area.Blur()
	d.headline.Blur()
	switch f := d.field(); {
	case d.onChoices():
		return nil
	case f >= 0:
		return d.fields[f].Focus()
	case d.onHeadline():
		return d.headline.Focus()
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
	case is(key, k.Cancel) && d.expanded:
		d.expanded = false
	case is(key, k.Cancel) && !d.normal && (d.kind == modalActions || d.picker()):
		d.listKey(key, k, 0)
	case is(key, k.Cancel) && (d.normal || d.text() == nil):
		m.modal = modalState{}
	case d.kind == modalActions:
		return m.updateActions(msg)
	case is(key, k.Save):
		return m.saveModal()
	case is(key, k.NextFocus), is(key, k.PrevFocus):
		step := 1
		if is(key, k.PrevFocus) {
			step = d.parts() - 1
		}
		cmd = d.setFocus((d.focus + step) % max(d.parts(), 1))
	case d.onChoices():
		dr := &d.drops[d.focus]
		opens, picked := is(key, k.Open) || is(key, k.Toggle), dr.choice
		switch {
		case d.dropdown() && !d.expanded && (opens || is(key, k.Down) || is(key, k.Up)):
			d.expanded = true
		case d.dropdown() && opens:
			d.expanded = false
		case is(key, k.Down):
			dr.choice = min(dr.choice+1, len(dr.choices)-1)
		case is(key, k.Up):
			dr.choice = max(dr.choice-1, 0)
		}
		if d.kind == modalMerge && dr.choice != picked {
			d.fillMerge()
		}
	case d.picker():
		cmd = d.pickKey(msg, k)
	case d.field() >= 0:
		d.fields[d.field()], cmd = d.fields[d.field()].Update(msg)
	case d.text() != nil:
		cmd = d.update(d.text(), msg, k)
		d.oneLineTitle()
	}
	return m, cmd
}

func (d *modalState) oneLineTitle() {
	if v := d.headline.Value(); strings.Contains(v, "\n") {
		d.headline.SetValue(strings.ReplaceAll(v, "\n", " "))
	}
}

func (m Model) pasteModal(msg tea.PasteMsg) (tea.Model, tea.Cmd) {
	d := &m.modal
	var cmd tea.Cmd
	switch f := d.field(); {
	case f >= 0:
		d.fields[f], cmd = d.fields[f].Update(msg)
		d.cursor = 0
	case d.text() != nil:
		*d.text(), cmd = d.text().Update(msg)
		d.oneLineTitle()
	}
	return m, cmd
}

func (d *modalState) onHeadline() bool {
	return d.titled && d.focus == len(d.drops)+len(d.fields)
}

func (d *modalState) onArea() bool {
	return d.hasArea && !d.onChoices() && d.field() < 0 && !d.onHeadline()
}

func (d *modalState) text() *textarea.Model {
	switch {
	case d.onHeadline():
		return &d.headline
	case d.onArea():
		return &d.area
	}
	return nil
}

func (d *modalState) mode() string {
	if d.text() == nil {
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
	if d.hasArea && body == "" && !d.titled && d.kind != modalSubmit && d.kind != modalClose {
		m.warning = "write something before saving"
		return m, nil
	}
	pr := d.pr
	var notice string
	var action func(context.Context) error
	switch d.kind {
	case modalCreate:
		title, description := strings.TrimSpace(d.headline.Value()), body
		if title == "" {
			m.warning = "the title can't be empty"
			return m, nil
		}
		m.modal = modalState{}
		return m, m.createPR(d.drops[0].value(), d.drops[1].value(), title, description)
	case modalURL:
		repo, number, err := review.ParsePullRequestURL(d.fields[0].Value())
		if err != nil {
			m.warning = err.Error()
			return m, nil
		}
		m.modal = modalState{}
		return m, m.fetchPR(repo, number)
	case modalEdit:
		title, description := strings.TrimSpace(d.headline.Value()), body
		if title == "" {
			m.warning = "the title can't be empty"
			return m, nil
		}
		base := pr.Base
		if len(d.drops) > 0 {
			base = d.drops[0].value()
		}
		notice = fmt.Sprintf("updated #%d", pr.Number)
		action = func(ctx context.Context) error { return host.EditPullRequest(ctx, pr, title, description, base) }
	case modalLabels:
		notice = fmt.Sprintf("updated the labels of #%d", pr.Number)
		action = func(ctx context.Context) error { return host.SetLabels(ctx, pr, d.picked) }
	case modalClose:
		notice = fmt.Sprintf("closed #%d", pr.Number)
		action = func(ctx context.Context) error { return host.Close(ctx, pr, body) }
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
		r := reactions[d.drops[0].choice]
		add := !slices.ContainsFunc(d.note.Reactions, func(x review.Reaction) bool { return x.Content == r.content && x.Mine })
		notice = "reacted " + r.emoji
		if !add {
			notice = "removed " + r.emoji
		}
		action = func(ctx context.Context) error { return host.React(ctx, d.note.ID, r.content, add) }
	case modalMerge:
		method, message := d.mergeMethod(), review.MergeMessage{}
		message.Headline, message.Body = strings.TrimSpace(d.headline.Value()), body
		if method != review.MergeRebase && message.Headline == "" {
			m.warning = "the commit title can't be empty"
			return m, nil
		}
		notice = fmt.Sprintf("merged #%d into %s", pr.Number, pr.Base)
		action = func(ctx context.Context) error {
			if err := host.Merge(ctx, pr, method, message); err != nil {
				return fmt.Errorf("merge #%d: %w", pr.Number, err)
			}
			return nil
		}
	case modalSubmit:
		e := reviewEvents[d.drops[0].choice]
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
	return m.width * 60 / 100
}

func (m Model) detailHeight() int {
	return max(m.panesHeight()-4, 1)
}

func (m Model) renderPulls() string {
	var panes string
	switch {
	case m.help:
		panes = box("help · "+label(m.keys.Cancel)+" closes", "", m.helpLines(), m.width, m.panesHeight(), colorPrimary)
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
		if m.modal.open {
			panes = m.renderModal(panes)
		}
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
	return statusBar(chip, m.statusMessage(m.width-lipgloss.Width(chip)-4), " ", m.width)
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

func (m Model) pullRows() int {
	return max((m.panesHeight()-3)/2, 1)
}

func (m Model) firstPull() int {
	return max(m.pulls.cursor-m.pullRows()+1, 0)
}

func cell(text string, width int, right bool, style lipgloss.Style) string {
	text = ansi.Truncate(text, width, "…")
	gap := strings.Repeat(" ", max(width-lipgloss.Width(text), 0))
	if right {
		return style.Render(gap + text)
	}
	return style.Render(text + gap)
}

func (m Model) pullLines() []string {
	switch {
	case m.pulls.loading:
		return []string{dimStyle.Render(" loading…")}
	case len(m.pulls.list) == 0 && m.pulls.err == nil:
		return []string{dimStyle.Render(" no open pull requests, " + label(m.keys.NextFilter) + " changes the filter")}
	}
	first := m.firstPull()
	visible := m.pulls.list[first:min(first+m.pullRows(), len(m.pulls.list))]
	idW, addW, delW, authorW, openedW, updatedW := 1, 1, 1, len("author"), len("opened"), len("updated")
	for _, pr := range visible {
		idW = max(idW, len(fmt.Sprint("#", pr.Number)))
		addW = max(addW, len(fmt.Sprint("+", pr.Additions)))
		delW = max(delW, len(fmt.Sprint("-", pr.Deletions)))
		authorW = min(max(authorW, len(pr.Author)+1), 16)
		openedW = max(openedW, len(age(pr.CreatedAt)))
		updatedW = max(updatedW, len(age(pr.UpdatedAt)))
	}
	titleW := max(m.pullsWidth()-2-(3+idW+2+2+addW+1+delW+2+authorW+2+openedW+2+updatedW+1), 8)
	columns := func(base lipgloss.Style, lead, id, title, add, del, author, opened, updated string, title2 lipgloss.Style) string {
		dim, gap := base.Foreground(colorDim), base.Render("  ")
		return lead + cell(id, idW, false, dim) + gap + cell(title, titleW, false, title2) + gap +
			cell(add, addW, true, base.Foreground(colorAdd)) + base.Render(" ") + cell(del, delW, true, base.Foreground(colorDel)) + gap +
			cell(author, authorW, false, dim) + gap + cell(opened, openedW, true, dim) + gap + cell(updated, updatedW, true, dim) + base.Render(" ")
	}
	header := lipgloss.NewStyle()
	lines := []string{columns(header, dimStyle.Render("   "), "#", "title", "+", "-", "author", "opened", "updated", dimStyle)}
	for i, pr := range visible {
		base := lipgloss.NewStyle()
		bar, titleColor := base.Render(" "), colorForeground
		if first+i == m.pulls.cursor {
			base = base.Background(colorCursorline)
			bar, titleColor = base.Foreground(colorPrimary).Render("▌"), colorPrimary
		}
		checks := pr.Checks
		if d, ok := m.pulls.details[prKey(pr)]; ok {
			checks = d.Checks
		}
		icon := checksIcon(checks)
		if pr.Draft {
			icon = dimStyle.Render("◌")
		}
		lines = append(lines,
			columns(base, bar+base.Render(icon)+base.Render(" "), fmt.Sprint("#", pr.Number), pr.Title,
				fmt.Sprint("+", pr.Additions), fmt.Sprint("-", pr.Deletions), "@"+pr.Author, age(pr.CreatedAt), age(pr.UpdatedAt), base.Foreground(titleColor)),
			bar+base.Render("  "+strings.Repeat(" ", idW+2))+fit(base.Foreground(colorDim).Render(pr.Repo), m.pullsWidth()-2-3-idW-2, base),
		)
	}
	return lines
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
		state := " " + lipgloss.NewStyle().Foreground(colorOnPrimary).Background(colorPrimary).Render(" "+pr.State+" ")
		if pr.Draft {
			state += dimStyle.Render(" · draft")
		}
		lines = append(lines,
			" "+lipgloss.NewStyle().Bold(true).Foreground(colorForeground).Render(pr.Title),
			state,
			" "+counts(pr.Additions, pr.Deletions, lipgloss.NewStyle())+dimStyle.Render(fmt.Sprintf(" · %d files · %d commits", pr.ChangedFiles, len(pr.Commits))),
			"",
			field("assignees", mentions(pr.Assignees)),
			"",
			field("reviewers", people(pr)),
			"",
			field("labels", strings.Join(pr.Labels, ", ")),
			"",
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

func (m Model) renderModal(background string) string {
	d := m.modal
	var lines []string
	if d.warning != "" {
		lines = append(lines, " "+errorStyle.Render(d.warning), "")
	}
	dropRow := len(lines) + 2*d.focus + 1
	if d.dropdown() {
		lines = append(lines, m.dropdownLines()...)
	} else if len(d.drops) > 0 {
		for i, c := range d.drops[0].choices {
			style, mark := dimStyle, "  "
			if i == d.drops[0].choice {
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
		lines = append(lines, " "+style.Render(fmt.Sprintf("%-10s", d.labels[i]))+" "+fieldView(f))
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
			if d.kind != modalLabels {
				p = "@" + p
			}
			lines = append(lines, " "+style.Render(mark+p))
		}
		none, unmatched := "no one to pick", "no one matches"
		if d.kind == modalLabels {
			none, unmatched = "the repository has no labels", "no label matches"
		}
		switch {
		case d.failed != nil:
			lines = append(lines, " "+errorStyle.Render(d.failed.Error()))
		case d.loading:
			lines = append(lines, " "+dimStyle.Render("loading…"))
		case len(d.options) == 0:
			lines = append(lines, " "+dimStyle.Render(none))
		case len(matches) == 0:
			lines = append(lines, " "+dimStyle.Render(unmatched))
		}
	}
	if d.kind == modalActions {
		lines = append(lines, "")
		matches := m.matchingActions()
		width := m.modalWidth(d.kind) - 6
		for i, a := range matches {
			shortcut := label(a.key)
			gap := strings.Repeat(" ", max(width-lipgloss.Width(a.icon)-2-lipgloss.Width(a.title)-len(shortcut), 1))
			if i == d.cursor {
				selected := lipgloss.NewStyle().Background(colorPrimary).Foreground(colorOnPrimary)
				lines = append(lines, " "+selected.Render(a.icon+"  "+a.title+gap+shortcut))
				continue
			}
			lines = append(lines, " "+primaryStyle.Render(a.icon)+"  "+lipgloss.NewStyle().Foreground(colorForeground).Render(a.title)+gap+dimStyle.Render(shortcut))
		}
		if len(matches) == 0 {
			lines = append(lines, " "+dimStyle.Render("no action matches"))
		}
	}
	if d.titled {
		label := func(name string, on bool) string {
			style := dimStyle
			if on {
				style = primaryStyle
			}
			return " " + style.Render(fmt.Sprintf("%-*s", gutter-1, name))
		}
		lines = append(lines, label("title", d.onHeadline())+markdownView(d.headline, d.onHeadline() && d.mode() == "insert"), "")
		for i, l := range strings.Split(markdownView(d.area, d.onArea() && d.mode() == "insert"), "\n") {
			name := ""
			if i == 0 {
				name = "description"
			}
			lines = append(lines, label(name, i == 0 && d.onArea())+l)
		}
	} else if d.hasArea {
		for l := range strings.SplitSeq(markdownView(d.area, d.mode() == "insert"), "\n") {
			lines = append(lines, " "+l)
		}
	}
	lines = append(lines, "", " "+hints(m.modalHints()))
	modal := box(d.title, "", lines, m.modalWidth(d.kind), min(len(lines)+2, m.panesHeight()), colorPrimary)
	x, y := max((m.width-lipgloss.Width(modal))/2, 0), max((m.panesHeight()-lipgloss.Height(modal))/2, 0)
	layers := []*lipgloss.Layer{lipgloss.NewLayer(background), lipgloss.NewLayer(modal).X(x).Y(y).Z(1)}
	if list := m.dropdownList(); list != "" {
		layers = append(layers, lipgloss.NewLayer(list).X(x+13).Y(y+1+dropRow).Z(2))
	}
	return lipgloss.NewCompositor(layers...).Render()
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
		row := msg.Y - 3
		if i := m.firstPull() + row/2; !m.pulls.sidebar && row >= 0 && row/2 < m.pullRows() && i < len(m.pulls.list) {
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
