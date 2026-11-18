package tui

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jfmainville/revunk/internal/review"
)

type fakeHost struct {
	lists     map[string][]review.PullRequest
	details   map[string]review.PullRequest
	calls     []string
	err       error
	listErr   error
	detailErr error
	users     []string
	usersErr  error
	labels    []string
	branches  []string
}

func (h *fakeHost) Assignable(_ context.Context, repo string) ([]string, error) {
	h.calls = append(h.calls, "assignable "+repo)
	return h.users, h.usersErr
}

func (h *fakeHost) PullRequests(_ context.Context, filter string) ([]review.PullRequest, error) {
	h.calls = append(h.calls, "list "+filter)
	return h.lists[filter], h.listErr
}

func (h *fakeHost) PullRequest(_ context.Context, repo string, number int) (review.PullRequest, error) {
	key := fmt.Sprintf("%s#%d", repo, number)
	h.calls = append(h.calls, "detail "+key)
	return h.details[key], h.detailErr
}

func (h *fakeHost) BranchPullRequest(context.Context) (review.PullRequest, error) {
	h.calls = append(h.calls, "branch")
	return h.details["o/r#7"], h.detailErr
}

func (h *fakeHost) record(call string) error {
	h.calls = append(h.calls, call)
	return h.err
}

func (h *fakeHost) EditPullRequest(_ context.Context, pr review.PullRequest, title, body, base string) error {
	call := fmt.Sprintf("edit %s %s|%s", prKey(pr), title, body)
	if base != pr.Base {
		call += " into " + base
	}
	return h.record(call)
}

func (h *fakeHost) SetPeople(_ context.Context, pr review.PullRequest, assignees, reviewers []string) error {
	return h.record(fmt.Sprintf("people %s %s %s", prKey(pr), strings.Join(assignees, ","), strings.Join(reviewers, ",")))
}

func (h *fakeHost) Comment(_ context.Context, pr review.PullRequest, body string) error {
	return h.record("comment " + prKey(pr) + " " + body)
}

func (h *fakeHost) Reply(_ context.Context, threadID, body string) error {
	return h.record("reply " + threadID + " " + body)
}

func (h *fakeHost) React(_ context.Context, subjectID, content string, add bool) error {
	return h.record(fmt.Sprintf("react %s %s %v", subjectID, content, add))
}

func (h *fakeHost) Resolve(_ context.Context, threadID string, resolved bool) error {
	return h.record(fmt.Sprintf("resolve %s %v", threadID, resolved))
}

func (h *fakeHost) SubmitReview(_ context.Context, pr review.PullRequest, event review.ReviewEvent, body string) error {
	return h.record(fmt.Sprintf("submit %s %s %s %s", prKey(pr), pr.PendingReview, event, body))
}

func (h *fakeHost) Labels(_ context.Context, repo string) ([]string, error) {
	h.calls = append(h.calls, "labels "+repo)
	return h.labels, h.usersErr
}

func (h *fakeHost) SetLabels(_ context.Context, pr review.PullRequest, labels []string) error {
	return h.record(fmt.Sprintf("set labels %s %s", prKey(pr), strings.Join(labels, ",")))
}

func (h *fakeHost) RequestCopilotReview(_ context.Context, pr review.PullRequest) error {
	return h.record("copilot " + prKey(pr))
}

func (h *fakeHost) Close(_ context.Context, pr review.PullRequest, comment string) error {
	return h.record("close " + prKey(pr) + " " + comment)
}

func (h *fakeHost) Branches(_ context.Context, repo string) ([]string, string, error) {
	h.calls = append(h.calls, "branches "+repo)
	return h.branches, "main", h.usersErr
}

func (h *fakeHost) CreatePullRequest(_ context.Context, head, base, title, body string) (review.PullRequest, error) {
	if err := h.record(fmt.Sprintf("create %s %s %s|%s", head, base, title, body)); err != nil {
		return review.PullRequest{}, err
	}
	pr := review.PullRequest{Repo: "o/r", Number: 12, Title: title, Head: head, Base: cmp.Or(base, "main"), State: "OPEN", Detailed: true}
	h.details["o/r#12"] = pr
	return pr, nil
}

func (h *fakeHost) Merge(_ context.Context, pr review.PullRequest, method review.MergeMethod, message review.MergeMessage) error {
	if err := h.record(fmt.Sprintf("merge %s %s %s|%s", prKey(pr), method, message.Headline, message.Body)); err != nil {
		return err
	}
	d := h.details[prKey(pr)]
	d.State = "MERGED"
	h.details[prKey(pr)] = d
	return nil
}

func (h *fakeHost) last() string {
	if len(h.calls) == 0 {
		return ""
	}
	return h.calls[len(h.calls)-1]
}

func (h *fakeHost) has(call string) bool {
	for _, c := range h.calls {
		if c == call {
			return true
		}
	}
	return false
}

var clock = time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)

func samplePulls() (review.PullRequest, review.PullRequest) {
	short := review.PullRequest{Repo: "o/r", Number: 7, Title: "Add login", Author: "ana", UpdatedAt: clock.Add(-2 * time.Hour)}
	detail := short
	detail.ID, detail.Body, detail.State, detail.Head, detail.Base = "PR_7", "Adds **login** support", "OPEN", "login", "main"
	detail.Additions, detail.Deletions, detail.ChangedFiles, detail.Detailed = 10, 2, 1, true
	detail.Assignees = []string{"me"}
	detail.MergeState, detail.MergeMethods = "CLEAN", []review.MergeMethod{review.MergeCommit, review.MergeSquash, review.MergeRebase}
	detail.MergeMessages = map[review.MergeMethod]review.MergeMessage{
		review.MergeCommit: {Headline: "Merge pull request #7 from o/login", Body: "Add login"},
		review.MergeSquash: {Headline: "Add login (#7)", Body: "* add login"},
	}
	detail.Reviewers = []review.Reviewer{{Login: "bob", State: "APPROVED"}, {Login: "cy", State: "PENDING"}}
	detail.Labels = []string{"auth"}
	detail.Checks = []review.Check{{Name: "build", State: "SUCCESS"}, {Name: "test", State: "FAILURE"}, {Name: "lint", State: "IN_PROGRESS"}}
	detail.Commits = []review.Commit{{SHA: "abc1234567", Subject: "add login"}}
	detail.Files = []review.File{{Path: "a.go", Status: review.StatusModified, Additions: 8, Deletions: 2}}
	detail.Threads = []review.Thread{
		{Notes: []review.Note{{ID: "IC_1", Author: "ana", Body: "Ready for review", CreatedAt: clock.Add(-3 * time.Hour), Reactions: []review.Reaction{{Content: "THUMBS_UP", Count: 2, Mine: true}}}}},
		{Notes: []review.Note{{ID: "PRR_1", Author: "bob", State: "APPROVED", CreatedAt: clock.Add(-150 * time.Minute)}}},
		{ID: "T_1", Path: "a.go", Side: review.SideRight, Start: 3, End: 4, Notes: []review.Note{
			{ID: "C_1", Author: "bob", Body: "Rename this", CreatedAt: clock.Add(-140 * time.Minute)},
			{ID: "C_2", Author: "ana", Body: "Done", CreatedAt: clock.Add(-130 * time.Minute)},
		}},
	}
	return short, detail
}

func newHost() *fakeHost {
	short, detail := samplePulls()
	other := review.PullRequest{Repo: "o/s", Number: 8, Title: "Fix logout", Author: "bob", Draft: true, UpdatedAt: clock.Add(-49 * time.Hour)}
	otherDetail := other
	otherDetail.Detailed, otherDetail.State = true, "OPEN"
	return &fakeHost{
		lists: map[string][]review.PullRequest{
			"assignee:@me":         {short, other},
			"review-requested:@me": {other},
		},
		details: map[string]review.PullRequest{"o/r#7": detail, "o/s#8": otherDetail},
	}
}

func drainCmd(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	if cmd == nil {
		return m
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	var msg tea.Msg
	select {
	case msg = <-done:
	case <-time.After(100 * time.Millisecond):
		return m
	}
	switch msg := msg.(type) {
	case nil, pullsTickMsg, tickMsg, statusTickMsg:
		return m
	case tea.BatchMsg:
		for _, c := range msg {
			m = drainCmd(t, m, c)
		}
		return m
	case nestedMsg:
		if _, ok := msg.msg.(tickMsg); ok || msg.msg == (statusTickMsg{}) {
			return m
		}
		updated, next := m.Update(msg)
		return drainCmd(t, updated.(Model), next)
	default:
		updated, next := m.Update(msg)
		return drainCmd(t, updated.(Model), next)
	}
}

func pressRun(t *testing.T, m Model, keys ...string) Model {
	t.Helper()
	for _, k := range keys {
		var cmd tea.Cmd
		m, cmd = press(t, m, k)
		m = drainCmd(t, m, cmd)
	}
	return m
}

func pullsModel(t *testing.T) (Model, *fakeHost) {
	t.Helper()
	old, oldNow, oldReview, oldStatus, oldSpin := pullsPollEvery, now, reviewPollEvery, statusEvery, spinEvery
	pullsPollEvery, reviewPollEvery, statusEvery, spinEvery, now = time.Millisecond, time.Millisecond, time.Millisecond, time.Millisecond, func() time.Time { return clock }
	t.Cleanup(func() {
		pullsPollEvery, now, reviewPollEvery, statusEvery, spinEvery = old, oldNow, oldReview, oldStatus, oldSpin
	})
	host := newHost()
	m, _ := loaded(t)
	m = m.WithHost(host, func(pr review.PullRequest) *review.Service {
		return review.NewService(sample, &fakeStore{viewed: map[string]string{}}, nil, prKey(pr))
	})
	return pressRun(t, m, "2"), host
}

func TestPullsTabListsPullRequestsWithDetails(t *testing.T) {
	m, host := pullsModel(t)
	if m.tab != tabPulls || !host.has("list assignee:@me") || !host.has("detail o/r#7") || host.has("detail o/s#8") {
		t.Fatalf("tab=%d calls=%q", m.tab, host.calls)
	}
	out := screen(m)
	for _, want := range []string{"Agent", "Pull Request", "pull requests", "assigned · 2", "#7", "Add login", "o/r", "2h", "#8", "Fix logout", "o/s", "2d", "author", "updated",
		"#7 o/r", "Overview", "Activity", "Comments", "Commits", "Files", " OPEN ", "+10 -2 · 1 files · 1 commits",
		"assignees  @me", "reviewers   @bob   @cy", "labels     auth", "checks     ✓ 1  ● 1  ✗ 1", "✓ build success", "✗ test failure", "description", "Adds login support"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen missing %q", want)
		}
	}
	if strings.Contains(out, "wants to merge") {
		t.Fatal("the overview should not repeat the branches")
	}
	pr, _ := m.selectedPR()
	var plain []string
	detail, _ := m.detailLines(pr, 80)
	for _, l := range detail {
		plain = append(plain, strings.TrimSpace(ansi.Strip(l)))
	}
	chip := lipgloss.NewStyle().Foreground(colorOnPrimary).Background(colorAdd).Render(" \uf407 OPEN ")
	if !strings.Contains(detail[1], chip) {
		t.Fatalf("an open state should be a green chip: %q", detail[1])
	}
	for _, name := range []string{"assignees", "reviewers", "labels", "checks", "description"} {
		i := slices.IndexFunc(plain, func(l string) bool { return strings.HasPrefix(l, name) })
		if i < 1 || plain[i-1] != "" {
			t.Errorf("%s should follow a blank line: %q", name, plain)
		}
	}
	if !strings.Contains(out, "▌\uf407 ✗ #7") || !strings.Contains(out, "   #8") {
		t.Fatalf("the list should show the failing checks, and the draft icon in place of the state icon:\n%s", out)
	}
	m = pressRun(t, m, "1")
	if m.tab != tabAgent || !strings.Contains(screen(m), "continuous diff") {
		t.Fatal("1 should go back to the Agent tab")
	}
	calls := len(host.calls)
	if m = pressRun(t, m, "2"); len(host.calls) != calls {
		t.Fatal("coming back should not reload the list")
	}
}

func TestPullsTabWithoutHostOrWithErrors(t *testing.T) {
	m, _ := loaded(t)
	m = pressRun(t, m, "2")
	if !strings.Contains(screen(m), "pull requests need the GitHub CLI") {
		t.Fatalf("missing host error:\n%s", screen(m))
	}
	if m = pressRun(t, m, "s", "j", "A"); m.modal.open || m.pulls.filter != 0 {
		t.Fatal("keys should do nothing without a host")
	}

	m, host := pullsModel(t)
	host.listErr = errors.New("gh search: HTTP 401")
	m = pressRun(t, m, "s")
	if !strings.Contains(screen(m), "HTTP 401") {
		t.Fatalf("list error should show:\n%s", screen(m))
	}
	host.listErr, host.err = nil, errors.New("gh pr: not allowed")
	m = pressRun(t, m, "s", "s", "s")
	m = runAction(t, m, "comment")
	m = typeText(t, m, "hi")
	m = pressRun(t, m, "ctrl+s")
	if !strings.Contains(screen(m), "not allowed") || m.modal.open {
		t.Fatalf("action error should show:\n%s", screen(m))
	}
}

func TestFilterCyclesAndIgnoresStaleResults(t *testing.T) {
	m, host := pullsModel(t)
	m, cmd := press(t, m, "s")
	if m.pulls.filter != 1 || len(m.pulls.list) != 0 || !strings.Contains(screen(m), "loading…") {
		t.Fatal("s should switch filter and clear the list")
	}
	m = update(t, m, pullsMsg{filter: 0, list: host.lists["assignee:@me"]})
	if len(m.pulls.list) != 0 {
		t.Fatal("results for an old filter should be ignored")
	}
	m = drainCmd(t, m, cmd)
	if host.last() != "detail o/s#8" || len(m.pulls.list) != 1 || !strings.Contains(screen(m), "review requested · 1") {
		t.Fatalf("calls=%q screen:\n%s", host.calls, screen(m))
	}
	m = pressRun(t, m, "s")
	if !strings.Contains(screen(m), "no open pull requests") {
		t.Fatalf("empty filter should say so:\n%s", screen(m))
	}
	if m = pressRun(t, m, "s", "s"); m.pulls.filter != 0 {
		t.Fatal("filters should wrap around")
	}
}

func TestMovingSelectionFetchesDetailsOnce(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "j")
	if m.pulls.cursor != 1 || host.last() != "detail o/s#8" || !strings.Contains(screen(m), "#8 o/s") {
		t.Fatalf("j should select and load #8: %q", host.calls)
	}
	calls := len(host.calls)
	m = pressRun(t, m, "k", "j", "j", "g", "g")
	if m.pulls.cursor != 0 || len(host.calls) != calls {
		t.Fatalf("cached details should not be fetched again: %q", host.calls[calls:])
	}
	if m = pressRun(t, m, "G"); m.pulls.cursor != 1 {
		t.Fatal("G should select the last pull request")
	}
	m = update(t, m, pullsMsg{list: []review.PullRequest{host.lists["assignee:@me"][1], host.lists["assignee:@me"][0]}})
	if m.pulls.cursor != 0 || m.pulls.list[0].Number != 8 {
		t.Fatal("a refresh should keep the same pull request selected")
	}
}

func TestDetailTabsShowEachSection(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "enter")
	if !m.pulls.sidebar || !strings.Contains(screen(m), "next tab") {
		t.Fatal("enter should focus the details sidebar")
	}
	want := [][]string{
		{"3h    @ana commented", "2h    @bob approved", "2h    @bob commented on a.go", "2h    @ana replied on a.go"},
		{"conversation", "@ana · 3h", "Ready for review", "👍 2", "a.go L3-4", "@bob · 2h", "Rename this", "Done"},
		{"abc1234 add login"},
		{"M a.go  +8 -2"},
		{"assignees"},
	}
	for i, lines := range want {
		m = pressRun(t, m, "tab")
		out := screen(m)
		for _, w := range lines {
			if !strings.Contains(out, w) {
				t.Errorf("tab %d missing %q:\n%s", i+1, w, out)
			}
		}
	}
	if strings.Contains(screen(m), "review · approved") {
		t.Fatal("a review without a body is not a comment")
	}
	if m = pressRun(t, m, "shift+tab"); m.pulls.section != 4 {
		t.Fatal("shift+tab should go back a tab")
	}
	if m = pressRun(t, m, "esc"); m.pulls.sidebar {
		t.Fatal("esc should go back to the list")
	}
	if m = pressRun(t, m, "tab"); !m.pulls.sidebar {
		t.Fatal("tab from the list should focus the sidebar")
	}
}

func TestDetailsScrollAndRevealTheSelectedComment(t *testing.T) {
	m, host := pullsModel(t)
	pr := host.details["o/r#7"]
	pr.Body = strings.Repeat("line\n\n", 40)
	for i := range 12 {
		pr.Threads = append(pr.Threads, review.Thread{Notes: []review.Note{{ID: fmt.Sprint(i), Author: "zed", Body: fmt.Sprintf("note %d", i), CreatedAt: clock}}})
	}
	m = update(t, m, detailMsg{key: "o/r#7", pr: pr})
	m = update(t, m, tea.WindowSizeMsg{Width: 100, Height: 20})
	m = pressRun(t, m, "enter", "j", "j")
	if m.pulls.scroll != 2 {
		t.Fatalf("j should scroll the overview, scroll=%d", m.pulls.scroll)
	}
	m = pressRun(t, m, "ctrl+d")
	if m.pulls.scroll <= 2 {
		t.Fatal("ctrl+d should scroll half a page")
	}
	m = pressRun(t, m, "G")
	bottom := m.pulls.scroll
	if m = pressRun(t, m, "j"); m.pulls.scroll != bottom || bottom == 0 {
		t.Fatal("scrolling should stop at the bottom")
	}
	if m = pressRun(t, m, "ctrl+u", "k", "g", "g"); m.pulls.scroll != 0 {
		t.Fatal("gg should go back to the top")
	}
	m = pressRun(t, m, "tab", "tab")
	for range 14 {
		m = pressRun(t, m, "j")
	}
	if m.pulls.item != 14 || m.pulls.scroll == 0 || !strings.Contains(screen(m), "note 11") {
		t.Fatalf("the selected comment should stay visible: item=%d scroll=%d\n%s", m.pulls.item, m.pulls.scroll, screen(m))
	}
	for range 20 {
		m = pressRun(t, m, "k")
	}
	if m.pulls.item != 0 || m.pulls.scroll != 0 {
		t.Fatalf("moving up should scroll back: item=%d scroll=%d", m.pulls.item, m.pulls.scroll)
	}
}

func TestCommentModalPostsToGitHub(t *testing.T) {
	m, host := pullsModel(t)
	if m = pressRun(t, m, "c"); m.modal.open {
		t.Fatal("c no longer comments on the list, the actions do")
	}
	m = runAction(t, m, "comment")
	if !m.modal.open || !strings.Contains(screen(m), "comment on #7") || !strings.Contains(screen(m), "INSERT") || !strings.Contains(screen(m), "C-s save") {
		t.Fatalf("c should open the comment modal:\n%s", screen(m))
	}
	if m = pressRun(t, m, "ctrl+s"); !m.modal.open || m.warning != "write something before saving" {
		t.Fatal("an empty comment should not be sent")
	}
	m = typeText(t, m, "Looks **good**")
	m = pressRun(t, m, "enter")
	m = typeText(t, m, "ok")
	m = pressRun(t, m, "ctrl+s")
	if m.modal.open || !host.has("comment o/r#7 Looks **good**\nok") || host.last() != "detail o/r#7" || m.notice != "commented on #7" {
		t.Fatalf("calls=%q notice=%q", host.calls, m.notice)
	}
	m = runAction(t, m, "comment")
	m = typeText(t, m, "nope")
	if m = pressRun(t, m, "esc", "esc"); m.modal.open || host.has("comment o/r#7 nope") {
		t.Fatal("esc twice should cancel without sending")
	}
}

func runAction(t *testing.T, m Model, query string) Model {
	t.Helper()
	m = pressRun(t, m, "A")
	m = typeText(t, m, query)
	return pressRun(t, m, "enter")
}

func noMerge(host *fakeHost) bool {
	return !slices.ContainsFunc(host.calls, func(c string) bool { return strings.HasPrefix(c, "merge") })
}

func TestActionsListEveryPullRequestAction(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "A")
	out := screen(m)
	for _, want := range []string{"actions for #7", "  Edit the title and description", "ge", "  Review the diff", "gd", "  Review a pull request from its URL", "gu",
		"  Comment", "  Submit a review", "  Merge", "  Close without merging", "  Assignees", "  Reviewers", "  Labels", "  Request a review from Copilot", "⏎ run  esc list"} {
		if !strings.Contains(out, want) {
			t.Fatalf("the actions should list %q:\n%s", want, out)
		}
	}
	raw := m.View().Content
	for i, a := range m.actions() {
		if i > 0 && !strings.Contains(raw, primaryStyle.Render(a.icon)) {
			t.Fatalf("icon %q of %q should use the accent color", a.icon, a.title)
		}
		if w := lipgloss.Width(a.icon); w != 1 {
			t.Fatalf("icon %q of %q is %d cells wide, icons should be one cell like the rest of the interface", a.icon, a.title, w)
		}
	}
	m = typeText(t, m, "REVIEW")
	if got := len(m.matchingActions()); got != 5 || m.modal.cursor != 0 {
		t.Fatalf("the search should filter without case, got %d matches", got)
	}
	m = pressRun(t, m, "down", "down", "down", "down", "up")
	if m.modal.cursor != 3 {
		t.Fatalf("down and up should move within the matches, cursor=%d", m.modal.cursor)
	}
	m = typeText(t, m, "zzz")
	if out := screen(m); !strings.Contains(out, "no action matches") || m.modal.cursor != 0 {
		t.Fatalf("an empty search should say so:\n%s", out)
	}
	if m = pressRun(t, m, "enter"); !m.modal.open || m.modal.kind != modalActions {
		t.Fatal("enter with no match should do nothing")
	}
	if m = pressRun(t, m, "esc", "esc"); m.modal.open {
		t.Fatal("esc should close the actions")
	}
	if m = runAction(t, m, "edit"); !m.modal.open || m.modal.kind != modalEdit {
		t.Fatal("enter should run the selected action")
	}
	m = pressRun(t, m, "esc", "esc")
	if m = runAction(t, m, "review the diff"); m.diff == nil {
		t.Fatal("the review diff action should open the diff")
	}
	m = pressRun(t, m, "A")
	if got := ansi.Strip(screen(m)); strings.Contains(got, "Review the diff") || strings.Contains(got, "from its URL") || !strings.Contains(got, "Merge") {
		t.Fatalf("the review diff should list only the actions that make sense there:\n%s", got)
	}
	m = pressRun(t, m, "esc", "esc")
	if footer := ansi.Strip(m.diff.footer()); !strings.Contains(footer, "A actions") {
		t.Fatalf("the review diff footer should show the actions key: %s", footer)
	}
	agent, _ := loaded(t)
	if footer := ansi.Strip(agent.footer()); strings.Contains(footer, "actions") {
		t.Fatal("the Agent tab has no pull request actions")
	}
	if agent, _ = press(t, agent, "A"); agent.modal.open {
		t.Fatal("A does nothing on the Agent tab")
	}
}

func TestCopilotAndCloseActions(t *testing.T) {
	m, host := pullsModel(t)
	if m = runAction(t, m, "copilot"); !host.has("copilot o/r#7") || host.last() != "detail o/r#7" || m.notice != "requested a review of #7 from Copilot" {
		t.Fatalf("calls=%q notice=%q", host.calls, m.notice)
	}
	m = runAction(t, m, "close")
	if out := screen(m); !m.modal.open || !strings.Contains(out, "close #7 without merging") || !strings.Contains(out, "optional comment") {
		t.Fatalf("close should ask for confirmation with an optional comment:\n%s", out)
	}
	if m = pressRun(t, m, "esc", "esc"); m.modal.open || slices.ContainsFunc(host.calls, func(c string) bool { return strings.HasPrefix(c, "close") }) {
		t.Fatal("esc should cancel without closing")
	}
	m = runAction(t, m, "close")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("close o/r#7 ") || m.notice != "closed #7" {
		t.Fatalf("the comment should be optional, calls=%q", host.calls)
	}
	m = runAction(t, m, "close")
	m = typeText(t, m, "Superseded by #9")
	if m = pressRun(t, m, "ctrl+s"); !host.has("close o/r#7 Superseded by #9") {
		t.Fatalf("calls=%q", host.calls)
	}
	detail := host.details["o/r#7"]
	detail.State = "CLOSED"
	m.pulls.details["o/r#7"] = detail
	if m = runAction(t, m, "close"); m.modal.open || m.warning != "#7 is already closed" {
		t.Fatalf("a closed pull request should not close again, notice=%q", m.notice)
	}
}

func TestMergeConfirmsThenMergesOnGitHub(t *testing.T) {
	m, host := pullsModel(t)
	m = runAction(t, m, "merge")
	out := screen(m)
	if !m.modal.open || !strings.Contains(out, "merge #7 into main") || !strings.Contains(out, "method      Create a merge commit") ||
		!strings.Contains(out, "title       Merge pull request #7 from o/login") || !strings.Contains(out, "description Add login") {
		t.Fatalf("merge should open the confirmation with GitHub's commit message:\n%s", out)
	}
	if m = pressRun(t, m, "esc"); m.modal.open || !noMerge(host) {
		t.Fatalf("esc should cancel without merging, calls=%q", host.calls)
	}
	m = runAction(t, m, "merge")
	m = pressRun(t, m, "enter")
	if !m.modal.expanded || !noMerge(host) {
		t.Fatal("enter should open the method dropdown, not merge")
	}
	m = pressRun(t, m, "j", "enter")
	if title, body := m.modal.headline.Value(), m.modal.area.Value(); title != "Add login (#7)" || body != "* add login" {
		t.Fatalf("picking squash should fill its commit message, got %q and %q", title, body)
	}
	m = pressRun(t, m, "j", "j", "enter")
	if out := screen(m); m.modal.area.Value() != "" || m.modal.headline.Value() != "" || !strings.Contains(out, "rebasing keeps the message of each commit") {
		t.Fatalf("rebase has no commit message:\n%s", out)
	}
	m = pressRun(t, m, "esc", "esc")

	host.err = errors.New("Pull request is not mergeable")
	m = runAction(t, m, "merge")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("merge o/r#7 MERGE Merge pull request #7 from o/login|Add login") || m.modal.open || m.notice == "merged #7 into main" || !strings.Contains(screen(m), "merge #7: Pull request is not mergeable") {
		t.Fatalf("a refused merge should show GitHub's reason without reporting success, calls=%q:\n%s", host.calls, screen(m))
	}
	if pr, _ := m.selectedPR(); pr.State != "OPEN" {
		t.Fatal("a refused merge should leave the pull request open")
	}

	host.err, m.pulls.err = nil, nil
	m = runAction(t, m, "merge")
	m = pressRun(t, m, "enter", "j", "enter", "tab")
	m.modal.headline.SetValue("Add login everywhere (#7)")
	m.modal.area.SetValue("Squashed")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("merge o/r#7 SQUASH Add login everywhere (#7)|Squashed") || host.last() != "detail o/r#7" || m.notice != "merged #7 into main" {
		t.Fatalf("ctrl+s should merge with the edited commit message and reload, calls=%q notice=%q", host.calls, m.notice)
	}
	if pr, _ := m.selectedPR(); pr.State != "MERGED" || !strings.Contains(screen(m), "MERGED") {
		t.Fatalf("the merged state should show after a merge:\n%s", screen(m))
	}
	if m = runAction(t, m, "merge"); m.modal.open || m.warning != "#7 is merged, only open pull requests merge" {
		t.Fatalf("a merged pull request should not merge again, notice=%q", m.notice)
	}
}

func TestMergeFollowsRepositorySettingsAndState(t *testing.T) {
	m, host := pullsModel(t)
	detail := host.details["o/r#7"]
	detail.MergeState, detail.MergeMethods = "BLOCKED", []review.MergeMethod{review.MergeRebase, review.MergeSquash}
	m.pulls.details["o/r#7"] = detail
	m = runAction(t, m, "merge")
	out := screen(m)
	if !strings.Contains(out, "branch protection blocks it") || !strings.Contains(out, "Squash and merge") || strings.Contains(out, "Create a merge commit") {
		t.Fatalf("the modal should warn about the merge state and offer only allowed methods:\n%s", out)
	}
	m = pressRun(t, m, "tab")
	m.modal.headline.SetValue(" ")
	if m = pressRun(t, m, "ctrl+s"); !m.modal.open || m.warning != "the commit title can't be empty" || !noMerge(host) {
		t.Fatal("an empty commit title should not merge")
	}
	m = pressRun(t, m, "esc", "esc")
	detail.MergeMethods = []review.MergeMethod{review.MergeRebase}
	m.pulls.details["o/r#7"] = detail
	m = runAction(t, m, "merge")
	if m = pressRun(t, m, "ctrl+s"); !host.has("merge o/r#7 REBASE |") {
		t.Fatalf("a rebase merges without a commit message, calls=%q", host.calls)
	}

	for _, c := range []struct {
		pr     review.PullRequest
		notice string
	}{
		{review.PullRequest{Repo: "o/r", Number: 9}, "wait for #9 to load before merging"},
		{review.PullRequest{Repo: "o/r", Number: 9, Detailed: true, State: "CLOSED"}, "#9 is closed, only open pull requests merge"},
		{review.PullRequest{Repo: "o/r", Number: 9, Detailed: true, State: "OPEN", Draft: true, MergeMethods: detail.MergeMethods}, "#9 is a draft, mark it ready for review on GitHub before merging"},
		{review.PullRequest{Repo: "o/r", Number: 9, Detailed: true, State: "OPEN"}, "o/r allows no merge method, check its settings on GitHub"},
	} {
		if cmd := m.openMerge(c.pr); cmd != nil || m.modal.kind == modalMerge && m.modal.pr.Number == 9 || m.warning != c.notice {
			t.Fatalf("openMerge(%+v) warning = %q, want %q", c.pr, m.warning, c.notice)
		}
	}
	for state, want := range map[string]string{"DIRTY": "conflicts", "BEHIND": "behind its base", "UNSTABLE": "checks are failing", "UNKNOWN": "still checking", "CLEAN": "", "HAS_HOOKS": ""} {
		if got := mergeWarning(state); want == "" && got != "" || !strings.Contains(got, want) {
			t.Fatalf("mergeWarning(%s) = %q", state, got)
		}
	}
}

func TestReviewDiffMergesTheReviewedPullRequest(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "g", "d")
	m = runAction(t, m, "merge")
	if !m.diff.modal.open || !strings.Contains(screen(m), "merge #7 into main") {
		t.Fatalf("the merge action should open the confirmation in the review diff:\n%s", screen(m))
	}
	m = pressRun(t, m, "ctrl+s")
	if !host.has("merge o/r#7 MERGE Merge pull request #7 from o/login|Add login") || m.diff.notice != "merged #7 into main" || m.diff.pr.State != "MERGED" {
		t.Fatalf("calls=%q notice=%q", host.calls, m.diff.notice)
	}
}

func TestReplyReactResolveTargetTheSelectedComment(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "enter", "tab", "tab", "x")
	if m.warning != "only review threads on code can be resolved" {
		t.Fatalf("resolve on a conversation comment: %q", m.notice)
	}
	m = pressRun(t, m, "r")
	if !strings.Contains(screen(m), "reply to @ana") {
		t.Fatalf("missing reply title:\n%s", screen(m))
	}
	m = typeText(t, m, "thanks")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("comment o/r#7 > @ana wrote\n>\n> Ready for review\n\nthanks") {
		t.Fatalf("a reply to a conversation comment should quote it: %q", host.calls)
	}
	m = pressRun(t, m, "j", "r")
	m = typeText(t, m, "will do")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("reply T_1 will do") || m.notice != "replied to @bob" {
		t.Fatalf("a reply to a thread should use the thread: %q", host.calls)
	}

	m = pressRun(t, m, "+")
	if !strings.Contains(screen(m), " 👍 thumbs up") || !strings.Contains(screen(m), "j k pick") {
		t.Fatalf("missing reaction picker:\n%s", screen(m))
	}
	m = pressRun(t, m, "j", "j", "k", "enter", "ctrl+s")
	if !host.has("react C_1 THUMBS_DOWN true") || m.notice != "reacted 👎" {
		t.Fatalf("calls=%q notice=%q", host.calls, m.notice)
	}
	m = pressRun(t, m, "k", "+", "k", "ctrl+s")
	if !host.has("react IC_1 THUMBS_UP false") || m.notice != "removed 👍" {
		t.Fatalf("picking your own reaction should remove it: %q", host.calls)
	}

	m = pressRun(t, m, "j", "x")
	if !host.has("resolve T_1 true") || m.notice != "resolved the thread" {
		t.Fatalf("calls=%q", host.calls)
	}
	pr := host.details["o/r#7"]
	pr.Threads[2].Resolved = true
	m = update(t, m, detailMsg{key: "o/r#7", pr: pr})
	if m = pressRun(t, m, "x"); !host.has("resolve T_1 false") || m.notice != "reopened the thread" {
		t.Fatal("x on a resolved thread should reopen it")
	}
	if !strings.Contains(screen(m), "a.go L3-4 · resolved") {
		t.Fatalf("resolved threads should say so:\n%s", screen(m))
	}

	pr.Threads = nil
	m = update(t, m, detailMsg{key: "o/r#7", pr: pr})
	if m = pressRun(t, m, "r"); m.modal.open || !strings.Contains(m.warning, "no comments yet") || m.pulls.item != 0 {
		t.Fatal("with no comments there is nothing to reply to")
	}
	if out := screen(m); !strings.Contains(out, "no comments yet") || strings.Contains(out, "to add one") {
		t.Fatalf("the empty Comments tab should not point to c:\n%s", out)
	}
}

func TestEditModalUpdatesTitleAndDescription(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "g", "e")
	out := screen(m)
	if !m.modal.open || !strings.Contains(out, "edit #7") || !strings.Contains(out, "title       Add login") || !strings.Contains(out, "description Adds **login** support") {
		t.Fatalf("ge should open the title and description boxes:\n%s", out)
	}
	if !m.modal.normal || !strings.Contains(out, "NORMAL") || !m.modal.onHeadline() {
		t.Fatal("the edit modal should start on the title in normal mode")
	}
	if m.modal.headline.Value() != "Add login" || m.modal.area.Value() != "Adds **login** support" {
		t.Fatalf("the title and description should fill their own boxes, %q and %q", m.modal.headline.Value(), m.modal.area.Value())
	}
	m = pressRun(t, m, "A")
	m = typeText(t, m, " v2")
	m = pressRun(t, m, "esc", "tab")
	if !m.modal.onArea() || m.modal.mode() != "normal" {
		t.Fatal("tab should move from the title to the description, keeping the vim mode")
	}
	m = pressRun(t, m, "I")
	m = typeText(t, m, "> ")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("edit o/r#7 Add login v2|> Adds **login** support") || m.notice != "updated #7" {
		t.Fatalf("the title and description should save from their boxes, calls=%q", host.calls)
	}
	m = pressRun(t, m, "g", "e")
	m.modal.headline.SetValue("  ")
	if m = pressRun(t, m, "ctrl+s"); !m.modal.open || m.warning != "the title can't be empty" {
		t.Fatal("an empty title should not be saved")
	}
	m = pressRun(t, m, "esc")
	host.details["o/r#7"] = review.PullRequest{Repo: "o/r", Number: 7, Title: "Renamed", Detailed: true}
	m = update(t, m, detailMsg{key: "o/r#7", pr: host.details["o/r#7"]})
	if m.pulls.list[0].Title != "Renamed" {
		t.Fatal("a new title should show in the list")
	}
}

func TestPickersSetAssigneesAndReviewers(t *testing.T) {
	m, host := pullsModel(t)
	host.users = []string{"ana", "me", "dan", "dave"}
	m = runAction(t, m, "assignees")
	out := screen(m)
	if !host.has("assignable o/r") || !strings.Contains(out, "assignees of #7") || !strings.Contains(out, "[x] @me") || !strings.Contains(out, "[ ] @dan") || !strings.Contains(out, "[ ] @ana") {
		t.Fatalf("the assignees action should open the picker with the current ones selected:\n%s", out)
	}
	m = typeText(t, m, "DA")
	if out = screen(m); !slices.Equal(m.modal.matches(), []string{"dan", "dave"}) || !strings.Contains(out, "[ ] @dave") {
		t.Fatalf("the search should filter without case:\n%s", out)
	}
	m = pressRun(t, m, "down", "down", "up", "space", "ctrl+n", "space", "space", "ctrl+s")
	if !host.has("people o/r#7 me,dan bob,cy") || m.notice != "updated the assignees of #7" {
		t.Fatalf("calls=%q notice=%q", host.calls, m.notice)
	}
	m = runAction(t, m, "reviewers")
	out = screen(m)
	if !strings.Contains(out, "reviewers of #7") || !strings.Contains(out, "[x] @bob") || !strings.Contains(out, "[x] @cy") || slices.Contains(m.modal.options, "ana") {
		t.Fatalf("the reviewers action should open the picker without the author:\n%s", out)
	}
	m = pressRun(t, m, "space", "ctrl+s")
	if !host.has("people o/r#7 me cy") || m.notice != "updated the reviewers of #7" {
		t.Fatalf("calls=%q", host.calls)
	}
	m = runAction(t, m, "reviewers")
	m = typeText(t, m, "zz")
	if out = screen(m); !strings.Contains(out, "no one matches") {
		t.Fatalf("an empty search should say so:\n%s", out)
	}
	if m = pressRun(t, m, "space", "esc", "esc"); m.modal.open || host.has("people o/r#7 me bob,cy,") {
		t.Fatal("esc should close the picker without saving")
	}
	m = runAction(t, m, "reviewers")
	if m.modal.options = nil; !strings.Contains(screen(m), "no one to pick") {
		t.Fatal("a repository with no one else to pick should say so")
	}
	m = pressRun(t, m, "space", "esc", "esc")
	host.usersErr = errors.New("forbidden")
	if m = runAction(t, m, "assignees"); !strings.Contains(screen(m), "forbidden") {
		t.Fatal("a failed load should show the error")
	}
	m = pressRun(t, m, "esc", "esc")
	m.modal.applyPicker(pickerMsg{repo: "o/r", items: []string{"late"}})
	if m.modal.open || m.modal.options != nil {
		t.Fatal("users that load after the picker closes should be dropped")
	}
}

func TestLabelsPickerSetsLabels(t *testing.T) {
	m, host := pullsModel(t)
	host.labels = []string{"auth", "bug", "docs"}
	m = runAction(t, m, "labels")
	out := screen(m)
	if !host.has("labels o/r") || !strings.Contains(out, "labels of #7") || !strings.Contains(out, "[x] auth") || !strings.Contains(out, "[ ] bug") || strings.Contains(out, "@bug") {
		t.Fatalf("the labels action should open a picker of the repository's labels with the current ones selected:\n%s", out)
	}
	m = pressRun(t, m, "down", "space", "up", "space", "ctrl+s")
	if !host.has("set labels o/r#7 bug") || m.notice != "updated the labels of #7" {
		t.Fatalf("calls=%q notice=%q", host.calls, m.notice)
	}
	m = runAction(t, m, "labels")
	m = typeText(t, m, "zz")
	if out := screen(m); !strings.Contains(out, "no label matches") {
		t.Fatalf("an empty search should say so:\n%s", out)
	}
	if m.modal.options = nil; !strings.Contains(screen(m), "the repository has no labels") {
		t.Fatal("a repository without labels should say so")
	}
}

func TestPollingRefreshesOnlyOnThePullsTab(t *testing.T) {
	m, host := pullsModel(t)
	calls := len(host.calls)
	m = drainCmd(t, m, m.pullsTicked())
	if host.calls[calls] != "list assignee:@me" || host.last() != "detail o/r#7" {
		t.Fatalf("a tick should refresh the list and the selected details: %q", host.calls[calls:])
	}
	m = pressRun(t, m, "1")
	calls = len(host.calls)
	if msg := m.pullsTicked()(); msg != (pullsTickMsg{}) || len(host.calls) != calls {
		t.Fatal("ticks on the Agent tab should only schedule the next one")
	}
	if _, cmd := m.Update(pullsTickMsg{}); cmd == nil {
		t.Fatal("ticks should keep going")
	}
}

func TestReviewDiffOpensAndSubmitsToGitHub(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "g", "d")
	if m.diff == nil || m.diff.pr.Number != 7 || m.diff.tab != tabPulls {
		t.Fatal("gd should open the review diff")
	}
	out := screen(m)
	if footer := ansi.Strip(m.diff.footer()); !strings.Contains(out, "continuous diff") || !strings.Contains(out, "vs main") || !strings.Contains(footer, "A actions") || strings.Contains(footer, "S send") || strings.Contains(footer, "y copy") || strings.Contains(footer, "x resolve") || strings.Contains(footer, "r reply") {
		t.Fatalf("review diff screen:\n%s", out)
	}
	if m = pressRun(t, m, "S"); m.diff.notice != "" {
		t.Fatal("sending to an agent is off in the review diff")
	}
	if m = pressRun(t, m, "j"); m.diff.fileCursor != 1 {
		t.Fatal("keys should drive the review diff")
	}
	gen := m.diffGen
	m = update(t, m, nestedMsg{gen: gen - 1, msg: filesMsg{commit: 0}})
	if m.diff.fileCursor != 1 {
		t.Fatal("messages from an older diff should be ignored")
	}

	m = runAction(t, m, "submit")
	if out := screen(m); !m.diff.modal.open || !strings.Contains(out, "submit your review of #7") || !strings.Contains(out, "review      Comment") || strings.Contains(out, "Approve") || !strings.Contains(out, "optional message") {
		t.Fatalf("the submit action should open the modal with a closed dropdown:\n%s", out)
	}
	if m = pressRun(t, m, "1"); m.tab != tabPulls {
		t.Fatal("tab keys are text while the modal is open")
	}
	detail := host.details["o/r#7"]
	detail.PendingReview = "PRR_9"
	host.details["o/r#7"] = detail
	if m = pressRun(t, m, "ctrl+s"); !host.has("submit o/r#7 PRR_9 COMMENT ") || m.diff.modal.open {
		t.Fatalf("the message should be optional, calls=%q", host.calls)
	}
	m = runAction(t, m, "submit")
	m = pressRun(t, m, "enter")
	if out := screen(m); !m.diff.modal.expanded || !strings.Contains(out, " Comment") || !strings.Contains(out, "Request changes") {
		t.Fatalf("enter should open the dropdown:\n%s", out)
	}
	m = pressRun(t, m, "j", "enter")
	if out := screen(m); m.diff.modal.expanded || !strings.Contains(out, "review      Approve") || strings.Contains(out, "Request changes") {
		t.Fatalf("enter should pick and close the dropdown:\n%s", out)
	}
	m = pressRun(t, m, "ctrl+s")
	if !host.has("submit o/r#7 PRR_9 APPROVE ") || m.diff.notice != "submitted approve on #7" || m.diff.modal.open {
		t.Fatalf("calls=%q notice=%q", host.calls, m.diff.notice)
	}
	host.err = errors.New("can't approve your own pull request")
	m = runAction(t, m, "submit")
	m = pressRun(t, m, "j", "j", "j", "esc")
	if !m.diff.modal.open || m.diff.modal.expanded {
		t.Fatal("esc should close the dropdown and keep the modal")
	}
	m = pressRun(t, m, "space", "tab")
	if m.diff.modal.expanded {
		t.Fatal("tab should close the dropdown")
	}
	m = typeText(t, m, "Please fix")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("submit o/r#7 PRR_9 REQUEST_CHANGES Please fix") || !strings.Contains(statusText(m), "can't approve your own") {
		t.Fatalf("errors should show in the review diff:\n%s", screen(m))
	}

	m = pressRun(t, m, "1")
	if m.tab != tabAgent || m.diff == nil {
		t.Fatal("1 should show the Agent tab and keep the review diff")
	}
	if m = pressRun(t, m, "2"); !strings.Contains(screen(m), "vs main") {
		t.Fatal("2 should come back to the review diff")
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 90, Height: 30})
	if m.diff.width != 90 {
		t.Fatal("resizes should reach the review diff")
	}
	m = pressRun(t, m, "/")
	m = typeText(t, m, "func")
	if m = pressRun(t, m, "enter", "esc"); m.diff == nil {
		t.Fatal("esc should clear the search before closing the review diff")
	}
	if m = pressRun(t, m, "esc"); m.diff != nil || m.tab != tabPulls {
		t.Fatal("esc should close the review diff")
	}
	m.openReview = nil
	if m = pressRun(t, m, "g", "d"); m.diff != nil {
		t.Fatal("no review diff without a way to open one")
	}
}

func TestReviewDiffForwardsEditorResultsAndMouse(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "g", "d")
	m = update(t, m, editedMsg{path: "main.go"})
	if m.diff.notice != "edited main.go" {
		t.Fatal("editor results should reach the review diff")
	}
	m, _ = click(t, m, 60, 5)
	if m.diff.focus != focusDiff {
		t.Fatal("clicks should reach the review diff")
	}
}

func TestNestWrapsOnlyModelMessages(t *testing.T) {
	if nest(nil, 1) != nil {
		t.Fatal("nil commands stay nil")
	}
	if msg := nest(func() tea.Msg { return tickMsg{} }, 3)(); msg != (nestedMsg{gen: 3, msg: tickMsg{}}) {
		t.Fatalf("model messages should be wrapped: %#v", msg)
	}
	if msg := nest(tea.Quit, 3)(); msg != (tea.QuitMsg{}) {
		t.Fatalf("runtime messages should pass through: %#v", msg)
	}
	if msg := nest(func() tea.Msg { return nil }, 3)(); msg != nil {
		t.Fatal("nil messages stay nil")
	}
	batch := nest(tea.Batch(func() tea.Msg { return tickMsg{} }, func() tea.Msg { return pullsTickMsg{} }), 2)().(tea.BatchMsg)
	if len(batch) != 2 || batch[0]() != (nestedMsg{gen: 2, msg: tickMsg{}}) || batch[1]() != (nestedMsg{gen: 2, msg: pullsTickMsg{}}) {
		t.Fatal("batches should wrap each command")
	}
}

func TestPullsMouseSelectsAndScrolls(t *testing.T) {
	m, host := pullsModel(t)
	m, cmd := click(t, m, 5, 6)
	m = drainCmd(t, m, cmd)
	if m.pulls.cursor != 1 || host.last() != "detail o/s#8" {
		t.Fatal("clicking a row should select it")
	}
	m, _ = click(t, m, 5, 30)
	if m.pulls.cursor != 1 || m.pulls.sidebar {
		t.Fatal("clicking below the rows should only focus the list")
	}
	m, _ = click(t, m, 100, 5)
	if !m.pulls.sidebar {
		t.Fatal("clicking the sidebar should focus it")
	}
	m, _ = wheel(t, m, 5, 5, tea.MouseWheelUp)
	if m.pulls.cursor != 0 {
		t.Fatal("the wheel over the list should move the selection")
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 120, Height: 12})
	m, _ = wheel(t, m, 100, 5, tea.MouseWheelDown)
	if m.pulls.scroll != 3 {
		t.Fatalf("the wheel over the sidebar should scroll it, scroll=%d", m.pulls.scroll)
	}
	m = pressRun(t, m, "A")
	if m, _ = click(t, m, 5, 3); m.pulls.cursor != 0 {
		t.Fatal("clicks are ignored while a modal is open")
	}
	if m, _ = wheel(t, m, 5, 5, tea.MouseWheelDown); m.pulls.cursor != 0 {
		t.Fatal("the wheel is ignored while a modal is open")
	}
}

func TestPullsHelpAndQuit(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "?")
	out := screen(m)
	for _, want := range []string{"pull requests", "gd", "review the diff with GitHub comments", "1 2", "Agent / Pull Request tab"} {
		if !strings.Contains(out, want) {
			t.Errorf("help missing %q", want)
		}
	}
	if strings.Contains(out, "gs ") {
		t.Fatal("submitting only works in the review diff, so its key is hidden here")
	}
	if m = pressRun(t, m, "2", "esc"); m.help {
		t.Fatal("esc should close the help")
	}
	if _, cmd := press(t, m, "q"); cmd == nil || cmd() != (tea.QuitMsg{}) {
		t.Fatal("q should quit from the list")
	}
}

func TestPullsViewFitsTerminal(t *testing.T) {
	m, _ := pullsModel(t)
	for _, state := range []func(Model) Model{
		func(m Model) Model { return m },
		func(m Model) Model { return pressRun(t, m, "enter", "tab", "tab") },
		func(m Model) Model { return pressRun(t, m, "g", "e") },
	} {
		s := state(m)
		for _, size := range []tea.WindowSizeMsg{{Width: 80, Height: 24}, {Width: 160, Height: 50}, {Width: 50, Height: 14}} {
			s = update(t, s, size)
			out := s.render()
			if h := lipgloss.Height(out); h != size.Height {
				t.Errorf("%dx%d: view height %d", size.Width, size.Height, h)
			}
			if w := lipgloss.Width(out); w > size.Width {
				t.Errorf("%dx%d: view width %d overflows", size.Width, size.Height, w)
			}
		}
	}
}

func TestDetailsLoadingAndEmptyStates(t *testing.T) {
	m, host := pullsModel(t)
	host.detailErr = errors.New("rate limited")
	m = pressRun(t, m, "j")
	out := screen(m)
	if !strings.Contains(out, "rate limited") || !strings.Contains(out, "loading…") {
		t.Fatalf("detail errors and loading should show:\n%s", out)
	}
	bare := review.PullRequest{Repo: "o/r", Number: 7, Title: "x", Detailed: true}
	m = update(t, m, detailMsg{key: "o/r#7", pr: bare})
	m = pressRun(t, m, "k")
	out = screen(m)
	if !strings.Contains(out, "assignees  none") || !strings.Contains(out, "no description, ge adds one") {
		t.Fatalf("empty fields should say none:\n%s", out)
	}
	m = pressRun(t, m, "enter", "tab")
	if !strings.Contains(screen(m), "no activity yet") {
		t.Fatal("empty activity should say so")
	}
	if m = pressRun(t, m, "tab", "x"); !strings.Contains(screen(m), "no comments yet") {
		t.Fatal("empty comments should say so")
	}
}

func TestPullsHelpers(t *testing.T) {
	old := now
	now = func() time.Time { return clock }
	defer func() { now = old }()
	for d, want := range map[time.Duration]string{10 * time.Second: "now", 5 * time.Minute: "5m", 3 * time.Hour: "3h", 72 * time.Hour: "3d", 40 * 24 * time.Hour: "2026-10-07"} {
		if got := age(clock.Add(-d)); got != want {
			t.Errorf("age(%v) = %q, want %q", d, got, want)
		}
	}
	if age(time.Time{}) != "" {
		t.Fatal("zero time has no age")
	}
	for _, c := range []struct {
		checks []string
		want   string
	}{{nil, " "}, {[]string{"SUCCESS", "SKIPPED"}, "✓"}, {[]string{"SUCCESS", "QUEUED"}, "●"}, {[]string{"FAILURE", "IN_PROGRESS"}, "✗"}, {[]string{"PENDING", "TIMED_OUT"}, "✗"}, {[]string{"NEUTRAL"}, "✓"}} {
		var checks []review.Check
		for _, s := range c.checks {
			checks = append(checks, review.Check{State: s})
		}
		if got := ansi.Strip(checksIcon(checks)); got != c.want {
			t.Errorf("checksIcon(%v) = %q, want %q", c.checks, got, c.want)
		}
	}
	if got := quote(review.Note{Author: "ana", Body: "a\nb\n"}); got != "> @ana wrote\n>\n> a\n> b\n\n" {
		t.Fatalf("quote = %q", got)
	}
	file := review.Thread{ID: "T", Path: "x.go", Outdated: true, Notes: []review.Note{{ID: "1"}, {ID: "2"}}}
	titles := []struct {
		it   commentItem
		want string
	}{
		{commentItem{thread: file, note: file.Notes[0]}, "x.go file · outdated"},
		{commentItem{note: review.Note{State: "CHANGES_REQUESTED"}}, "review · changes requested"},
		{commentItem{note: review.Note{}}, "conversation"},
	}
	for _, c := range titles {
		if got := threadTitle(c.it); got != c.want {
			t.Errorf("threadTitle = %q, want %q", got, c.want)
		}
	}
	events := []struct {
		it   commentItem
		want string
	}{
		{commentItem{thread: file, note: file.Notes[1]}, "replied on x.go"},
		{commentItem{note: review.Note{State: "CHANGES_REQUESTED"}}, "requested changes"},
		{commentItem{note: review.Note{State: "COMMENTED"}}, "reviewed"},
		{commentItem{note: review.Note{State: "DISMISSED"}}, "had a review dismissed"},
	}
	for _, c := range events {
		if got := activity(c.it); got != c.want {
			t.Errorf("activity = %q, want %q", got, c.want)
		}
	}
	if got := ansi.Strip(reactionLine([]review.Reaction{{Content: "HEART", Count: 1}, {Content: "UNKNOWN", Count: 3}, {Content: "ROCKET", Count: 2, Mine: true}})); got != "💖 1  🚀 2" {
		t.Fatalf("reactionLine = %q", got)
	}
	if got := people(review.PullRequest{Reviewers: []review.Reviewer{{Login: "a", State: "CHANGES_REQUESTED"}, {Login: "b", State: "DISMISSED"}}}); got != " @a  @b" {
		t.Fatalf("people = %q", got)
	}
}

func TestGitHubCommentsShowAuthorAndState(t *testing.T) {
	for c, want := range map[*review.Comment]string{
		{Author: "bob", Sent: true}:                 "@bob",
		{Author: "me"}:                              "pending",
		{Author: "bob", Sent: true, Resolved: true}: "resolved",
		{Sent: true, Replies: []string{"x"}}:        "replied",
	} {
		if got := commentState(*c); got != want {
			t.Errorf("commentState(%+v) = %q, want %q", *c, got, want)
		}
	}
	out := ansi.Strip(strings.Join(renderComment(review.Comment{Kind: "comment", Path: "a.go", End: 2, Start: 2, Body: "why", Author: "bob", Sent: true, Replies: []string{"**@ana** because"}}, nil, 80), "\n"))
	if !strings.Contains(out, "L2 · @bob") || !strings.Contains(out, " reply") || strings.Contains(out, "agent reply") || !strings.Contains(out, "@ana because") {
		t.Fatalf("GitHub comment box:\n%s", out)
	}
}

func TestHintsSkipDisabledBindings(t *testing.T) {
	k := defaultKeys()
	k.Send.SetEnabled(false)
	out := ansi.Strip(hints([]hint{{desc: "send", keys: []key.Binding{k.Send}}, {desc: "copy", keys: []key.Binding{k.Copy}}}))
	if out != "y copy" {
		t.Fatalf("hints = %q", out)
	}
}

func TestClickingTabsSwitchesThem(t *testing.T) {
	m, host := pullsModel(t)
	if m, _ = click(t, m, 3, 0); m.tab != tabAgent {
		t.Fatal("clicking Agent should open the Agent tab")
	}
	if m, _ = click(t, m, 7, 0); m.tab != tabAgent {
		t.Fatal("the gap between tabs should not switch")
	}
	m, cmd := click(t, m, 10, 0)
	if m.tab != tabPulls || cmd != nil || len(host.calls) != 2 {
		t.Fatal("clicking Pull Request should reopen the tab without reloading it")
	}
	m = pressRun(t, m, "?")
	if m, _ = click(t, m, 3, 0); m.tab != tabPulls {
		t.Fatal("tab clicks are ignored while the help is open")
	}
}

func TestCommentsTabSelectsCommentsByKeyAndClick(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "enter", "tab", "tab")
	pr, _ := m.selectedPR()
	chip := func(author string) string {
		return lipgloss.NewStyle().Background(colorPrimary).Foreground(colorOnPrimary).Render("@" + author)
	}
	marked := func(m Model) string {
		lines, _ := m.detailLines(pr, m.sidebarWidth()-2)
		return strings.Join(lines, "\n")
	}
	if out := marked(m); !strings.Contains(out, chip("ana")) || strings.Contains(out, chip("bob")) {
		t.Fatalf("only the selected comment should be marked:\n%s", out)
	}
	m = pressRun(t, m, "j")
	if out := marked(m); m.pulls.item != 1 || !strings.Contains(out, chip("bob")) || strings.Contains(out, chip("ana")) {
		t.Fatalf("j should move the mark to the next comment, item=%d", m.pulls.item)
	}
	_, starts := m.detailLines(pr, m.sidebarWidth()-2)
	if m, _ = click(t, m, m.pullsWidth()+4, 4+starts[0]-m.pulls.scroll+1); m.pulls.item != 0 {
		t.Fatalf("clicking the first comment should select it, item=%d", m.pulls.item)
	}
	m = pressRun(t, m, "r")
	if !strings.Contains(screen(m), "reply to @ana") {
		t.Fatalf("r should reply to the clicked comment:\n%s", screen(m))
	}
	m = pressRun(t, m, "esc", "esc")
	if m, _ = click(t, m, m.pullsWidth()+4, 4+starts[1]-m.pulls.scroll); m.pulls.item != 1 {
		t.Fatalf("clicking the second comment should select it, item=%d", m.pulls.item)
	}
	if m = pressRun(t, m, "+"); !strings.Contains(screen(m), "react to @bob") {
		t.Fatalf("+ should react to the clicked comment:\n%s", screen(m))
	}
	m = pressRun(t, m, "esc", "esc")
	if m, _ = click(t, m, 3, 4); m.pulls.sidebar || strings.Contains(marked(m), chip("bob")) {
		t.Fatal("the mark should only show while the sidebar has focus")
	}
}

func TestClickingDetailTabsOpensThem(t *testing.T) {
	m, _ := pullsModel(t)
	x := m.pullsWidth() + 1
	m.pulls.scroll = 4
	if m, _ = click(t, m, x+len(" Overview  Activity  C"), 2); m.pulls.section != tabComments || m.pulls.scroll != 0 || !m.pulls.sidebar {
		t.Fatalf("clicking Comments should open it, section=%d", m.pulls.section)
	}
	if m, _ = click(t, m, x, 2); m.pulls.section != 0 {
		t.Fatal("clicking Overview should open it")
	}
	if m, _ = click(t, m, x+len(" Overview  Activity  C"), 3); m.pulls.section != 0 {
		t.Fatal("clicks below the tabs should not switch them")
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 200, Height: 40})
	if m, _ = click(t, m, m.width-2, 2); m.pulls.section != 0 {
		t.Fatal("clicks past the last tab should not switch them")
	}
}

func TestTabAtFindsTheLabelUnderX(t *testing.T) {
	names := []string{"ab", "cde"}
	for x, want := range map[int]int{-1: -1, 0: 0, 3: 0, 4: -1, 5: 1, 9: 1, 10: -1} {
		if got := tabAt(x, names, 1); got != want {
			t.Errorf("tabAt(%d)=%d want %d", x, got, want)
		}
	}
}

func TestAgentTabOpensTheBranchPullRequest(t *testing.T) {
	m, host := pullsModel(t)
	if m = pressRun(t, m, "1", "g", "e"); host.has("branch") || m.modal.open {
		t.Fatalf("ge does nothing on the Agent tab, calls=%q", host.calls)
	}
	m = pressRun(t, m, "g", "d")
	if m.tab != tabPulls || m.diff == nil || m.diff.pr.Number != 7 {
		t.Fatal("gd on the Agent tab should open the branch's review diff on the Pull Request tab")
	}
	calls := len(host.calls)
	if m = pressRun(t, m, "g", "e"); slices.Contains(host.calls[calls:], "branch") || !m.diff.modal.open || !strings.Contains(screen(m), "edit #7") {
		t.Fatal("ge in the review diff should edit the reviewed pull request without looking up the branch's")
	}
	m = pressRun(t, m, "esc", "esc", "esc", "1")
	host.detailErr = errors.New("no pull requests found for branch")
	if m = pressRun(t, m, "g", "d"); m.tab != tabAgent || m.diff != nil || !strings.Contains(screen(m), "no pull requests found") {
		t.Fatal("a branch without a pull request should show the error and stay on the Agent tab")
	}
	plain, _ := loaded(t)
	if _, cmd := press(t, plain, "g", "d"); cmd != nil {
		t.Fatal("without gh, gd does nothing")
	}
}

func TestReviewDiffEditsTheReviewedPullRequest(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "g", "d", "g", "e")
	if !m.diff.modal.open || m.diff.modal.kind != modalEdit || m.diff.modal.headline.Value() != "Add login" || m.diff.modal.area.Value() != "Adds **login** support" {
		t.Fatalf("ge should open the edit modal filled with the reviewed pull request:\n%s", screen(m))
	}
	m = pressRun(t, m, "A")
	m = typeText(t, m, " v2")
	detail := host.details["o/r#7"]
	detail.Title = "Add login v2"
	host.details["o/r#7"] = detail
	m = pressRun(t, m, "ctrl+s")
	if !host.has("edit o/r#7 Add login v2|Adds **login** support") || m.diff.modal.open || m.diff.notice != "updated #7" || m.diff.pr.Title != "Add login v2" {
		t.Fatalf("saving should edit on GitHub and reload the reviewed pull request, calls=%q notice=%q", host.calls, m.diff.notice)
	}
	host.err = errors.New("HTTP 403: Resource not accessible")
	if m = pressRun(t, m, "g", "e", "ctrl+s"); !strings.Contains(statusText(m), "Resource not accessible") || m.diff == nil {
		t.Fatalf("edit failures should show in the review diff:\n%s", screen(m))
	}
}

func TestURLModalOpensTheReviewDiff(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "g", "u")
	if out := screen(m); !m.modal.open || m.modal.kind != modalURL || !strings.Contains(out, "review a pull request from its GitHub URL") {
		t.Fatalf("gu should open the URL modal:\n%s", out)
	}
	if m = pressRun(t, m, "enter"); !m.modal.open || strings.Contains(screen(m), "the pull request URL is empty") {
		t.Fatal("enter should not save, only ctrl+s does")
	}
	if m = pressRun(t, m, "ctrl+s"); !m.modal.open || !strings.Contains(screen(m), "the pull request URL is empty") {
		t.Fatalf("an empty URL should keep the modal open with an error:\n%s", screen(m))
	}
	m = typeText(t, m, "https://gitlab.com/o/s/pull/8")
	if m = pressRun(t, m, "ctrl+s"); !m.modal.open || !strings.Contains(statusText(m), "is not a GitHub pull") || host.has("detail o/s#8") {
		t.Fatalf("an unsupported URL should keep the modal open without loading anything:\n%s", screen(m))
	}
	m = pressRun(t, m, "esc", "g", "u")
	if m.modal.fields[0].Value() != "" {
		t.Fatal("the URL modal should open empty")
	}
	m = typeText(t, m, "https://github.com/o/s/pull/8/files")
	if m = pressRun(t, m, "ctrl+s"); m.modal.open || !host.has("detail o/s#8") || m.diff == nil || m.diff.pr.Number != 8 || m.diff.pr.Repo != "o/s" || m.tab != tabPulls {
		t.Fatalf("a valid URL should open its review diff, calls=%q", host.calls)
	}
	m = pressRun(t, m, "esc")

	host.detailErr = errors.New("gh api: HTTP 401: Bad credentials")
	m = pressRun(t, m, "g", "u")
	m = typeText(t, m, "github.com/o/r/pull/7")
	if m = pressRun(t, m, "ctrl+s"); m.diff != nil || !strings.Contains(screen(m), "open o/r#7: gh api: HTTP 401: Bad credentials") {
		t.Fatalf("access failures should show on the Pull Request tab:\n%s", screen(m))
	}

	m = pressRun(t, m, "1", "g", "u")
	if !m.modal.open || m.tab != tabAgent {
		t.Fatal("gu should open the URL modal on the Agent tab")
	}
	m = typeText(t, m, "https://github.com/o/r/pull/7")
	if m = pressRun(t, m, "ctrl+s"); m.diff != nil || m.tab != tabAgent || !strings.Contains(screen(m), "Bad credentials") {
		t.Fatalf("access failures should show on the Agent tab:\n%s", screen(m))
	}
	host.detailErr = nil
	m = pressRun(t, m, "g", "u")
	m = typeText(t, m, "https://github.com/o/r/pull/7")
	if m = pressRun(t, m, "ctrl+s"); m.tab != tabPulls || m.diff == nil || m.diff.pr.Number != 7 {
		t.Fatal("a valid URL on the Agent tab should open the review diff on the Pull Request tab")
	}
	plain, _ := loaded(t)
	if plain, _ = press(t, plain, "g", "u"); plain.modal.open {
		t.Fatal("without gh, gu does nothing")
	}
}

func TestPastingFillsTheFocusedText(t *testing.T) {
	m, host := pullsModel(t)
	if m = update(t, m, tea.PasteMsg{Content: "q"}); m.modal.open || m.diff != nil {
		t.Fatal("a paste with no text focused should do nothing")
	}
	m = pressRun(t, m, "g", "u")
	m = update(t, m, tea.PasteMsg{Content: "https://github.com/o/s/pull/8"})
	if got := m.modal.fields[0].Value(); got != "https://github.com/o/s/pull/8" {
		t.Fatalf("pasted URL = %q", got)
	}
	if m = pressRun(t, m, "ctrl+s"); m.diff == nil || m.diff.pr.Number != 8 || !host.has("detail o/s#8") {
		t.Fatal("a pasted URL should open its review diff")
	}
	m = pressRun(t, m, "g", "e")
	m = update(t, m, tea.PasteMsg{Content: "pasted\ntitle"})
	if got := m.diff.modal.headline.Value(); !strings.HasPrefix(got, "pasted title") {
		t.Fatalf("a paste on the title should stay on one line, got %q", got)
	}
	m = pressRun(t, m, "tab")
	m = update(t, m, tea.PasteMsg{Content: "pasted\nbody"})
	if got := m.diff.modal.area.Value(); !strings.HasPrefix(got, "pasted\nbody") {
		t.Fatalf("a paste in the review diff should reach its modal description, got %q", got)
	}
	m = pressRun(t, m, "esc", "esc", "esc")
	m = runAction(t, m, "assignees")
	m = update(t, m, tea.PasteMsg{Content: "bo"})
	if m.modal.fields[0].Value() != "bo" || m.modal.cursor != 0 {
		t.Fatal("a paste should fill the picker search")
	}
	m = pressRun(t, m, "esc", "esc")
	m = runAction(t, m, "comment")
	m = update(t, m, tea.PasteMsg{Content: "x"})
	if m.modal.area.Value() != "x" {
		t.Fatal("a paste should fill the comment modal")
	}
}

func TestModalTextUsesVimModes(t *testing.T) {
	m, _ := pullsModel(t)
	m = runAction(t, m, "comment")
	area := func() string { return m.modal.area.Value() }
	if m.modal.normal || !strings.Contains(screen(m), "INSERT") || !strings.Contains(screen(m), "esc jj normal") {
		t.Fatal(screen(m), "the description should start in insert mode")
	}
	m.modal.area.SetValue("one two\nthree")
	m = typeText(t, m, "jj")
	if !m.modal.normal || area() != "one two\nthree" || !strings.Contains(screen(m), "NORMAL") {
		t.Fatalf("jj should leave insert mode without typing, value=%q", area())
	}
	m = typeText(t, m, "gg0x")
	if area() != "ne two\nthree" {
		t.Fatalf("gg 0 x should delete the first character, value=%q", area())
	}
	m = typeText(t, m, "wiX")
	m = pressRun(t, m, "esc")
	if area() != "ne Xtwo\nthree" || !m.modal.open || !m.modal.normal {
		t.Fatalf("w should move to the next word and esc should only leave insert mode, value=%q", area())
	}
	m = typeText(t, m, "$aZ")
	m = pressRun(t, m, "esc")
	m = typeText(t, m, "0IA")
	m = typeText(t, m, "jjAB")
	if area() != "Ane XtwoZB\nthree" {
		t.Fatalf("a, I, and A should insert after, at the start, and at the end, value=%q", area())
	}
	m = typeText(t, m, "jjoC")
	m = typeText(t, m, "jjOD")
	if area() != "Ane XtwoZB\nD\nC\nthree" {
		t.Fatalf("o and O should open lines below and above, value=%q", area())
	}
	m = typeText(t, m, "jjjdd")
	if area() != "Ane XtwoZB\nD\nthree" {
		t.Fatalf("dd should delete the line, value=%q", area())
	}
	m = typeText(t, m, "Gdd")
	if area() != "Ane XtwoZB\nD" || m.modal.area.Line() != 1 {
		t.Fatalf("dd on the last line should keep the cursor on the new last line, value=%q", area())
	}
	m = typeText(t, m, "k$biY")
	m = typeText(t, m, "jj$wiW")
	m = typeText(t, m, "jj0biV")
	if area() != "Ane YXtwoZBV\nWD" {
		t.Fatalf("w and b should move by words and across lines, value=%q", area())
	}
	m = typeText(t, m, "jjk0hlydz")
	if area() != "Ane YXtwoZBV\nWD" {
		t.Fatalf("motions and unknown keys should not change the text, value=%q", area())
	}
	if m = pressRun(t, m, "esc"); m.modal.open {
		t.Fatal("esc in normal mode should cancel the modal")
	}
}

func TestModalHintsFollowTheFocus(t *testing.T) {
	k := defaultKeys()
	for mode, want := range map[string]string{"": "esc cancel", "insert": "esc jj normal", "normal": "i insert  esc cancel"} {
		if got := ansi.Strip(hints(k.modalHints(false, false, mode))); !strings.Contains(got, want) {
			t.Errorf("mode %q hints %q, want %q", mode, got, want)
		}
	}
}

func TestTitledModalsLabelTheTitleLine(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "g", "e")
	if got := m.modal.area.Width(); got != m.modalWidth(modalEdit)-4-gutter || m.modalWidth(modalEdit) <= m.modalWidth(modalComment) {
		t.Fatalf("the edit modal should be wider than the others and leave room for the labels, width=%d", got)
	}
	m.modal.headline.SetValue(strings.Repeat("long title ", 20))
	m.modal.area.SetValue("The description")
	lines := strings.Split(ansi.Strip(m.render()), "\n")
	row := func(text string) int {
		for i, l := range lines {
			if strings.Contains(l, text) {
				return i
			}
		}
		return -1
	}
	if title, description := row("title       long title"), row("description The description"); title < 0 || description != title+2 {
		t.Fatalf("a long title should stay on its row above an empty line and the description:\n%s", strings.Join(lines, "\n"))
	}
	m = pressRun(t, m, "esc")
	m = runAction(t, m, "comment")
	if strings.Contains(ansi.Strip(m.renderModal("")), "title  ") {
		t.Fatal("modals without a title should not label lines")
	}
}

func TestReviewDiffCommentEditorUsesVimModes(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "g", "d", "l")
	m.diff.cursor = rowOf(t, *m.diff, rowLine, 1, 3)
	m = pressRun(t, m, "c")
	m = typeText(t, m, "ajj")
	if !m.diff.editing || !m.diff.editor.normal || m.diff.editor.area.Value() != "a" {
		t.Fatalf("jj should switch the review diff editor to normal mode, value=%q", m.diff.editor.area.Value())
	}
	if m = pressRun(t, m, "esc"); m.diff.editing {
		t.Fatal("esc in normal mode should close the review diff editor")
	}
}

func TestModalsOverlayTheScreen(t *testing.T) {
	m, _ := pullsModel(t)
	m = runAction(t, m, "comment")
	if out := screen(m); !strings.Contains(out, "comment on #7") || !strings.Contains(out, "pull requests") {
		t.Fatalf("the list should stay visible behind the modal:\n%s", out)
	}
	m = pressRun(t, m, "esc", "esc", "g", "d")
	m = runAction(t, m, "submit")
	if out := screen(m); !strings.Contains(out, "submit your review of #7") || !strings.Contains(out, "files") {
		t.Fatalf("the review diff should stay visible behind the submit modal:\n%s", out)
	}
}

func TestPollingPausesWhenHiddenOrUnfocused(t *testing.T) {
	m, host := pullsModel(t)
	m.every = time.Millisecond
	if _, ok := update1(t, m, tickMsg{}).(tickMsg); !ok {
		t.Fatal("the Agent diff should not poll git while the Pull Request tab shows")
	}
	m = update(t, m, tea.BlurMsg{})
	calls := len(host.calls)
	if msg := m.pullsTicked()(); msg != (pullsTickMsg{}) || len(host.calls) != calls || !m.blurred {
		t.Fatal("an unfocused terminal should not call GitHub")
	}
	m = pressRun(t, m, "1")
	if _, ok := update1(t, m, tickMsg{}).(tickMsg); !ok {
		t.Fatal("an unfocused terminal should not poll git")
	}
	m = update(t, m, tea.FocusMsg{})
	if _, ok := update1(t, m, tickMsg{}).(polledMsg); !ok || m.blurred {
		t.Fatal("focus should resume polling")
	}
	m = pressRun(t, m, "2", "g", "d")
	m = update(t, m, tea.BlurMsg{})
	if !m.diff.blurred {
		t.Fatal("focus changes should reach the review diff")
	}
	m = update(t, m, tea.FocusMsg{})
	calls = len(host.calls)
	if msg := m.pullsTicked()(); msg != (pullsTickMsg{}) || len(host.calls) != calls {
		t.Fatal("the hidden list should not refresh while the review diff is open")
	}
	m = pressRun(t, m, "1")
	if msg, ok := update1(t, m, nestedMsg{gen: m.diffGen, msg: tickMsg{}}).(nestedMsg); !ok || msg.msg != (tickMsg{}) {
		t.Fatalf("the review diff should hold its polling while hidden, got %#v", msg)
	}
	m = pressRun(t, m, "2")
	if msg, ok := update1(t, m, nestedMsg{gen: m.diffGen, msg: tickMsg{}}).(nestedMsg); !ok || msg.msg == (tickMsg{}) {
		t.Fatalf("the visible review diff should poll, got %#v", msg)
	}
}

func TestFailuresBackOff(t *testing.T) {
	if backoff(time.Second, 0) != time.Second || backoff(time.Second, 2) != 4*time.Second || backoff(time.Second, 9) != 32*time.Second {
		t.Fatal("backoff should double per failure up to 32 times the interval")
	}
	m, host := pullsModel(t)
	host.listErr = errors.New("API rate limit exceeded")
	for range maxBackoff + 2 {
		m = update(t, m, pullsMsg{err: host.listErr})
	}
	if m.pulls.failures != maxBackoff || !strings.Contains(screen(m), "rate limit") {
		t.Fatalf("list failures should back off, failures=%d", m.pulls.failures)
	}
	if m = update(t, m, pullsMsg{}); m.pulls.failures != 0 {
		t.Fatal("a successful refresh should reset the backoff")
	}
	for range maxBackoff + 2 {
		m = update(t, m, polledMsg{err: errors.New("boom")})
	}
	if m.failures != maxBackoff {
		t.Fatalf("diff poll failures should back off, failures=%d", m.failures)
	}
	if m = update(t, m, polledMsg{svc: m.svc}); m.failures != 0 {
		t.Fatal("a successful poll should reset the backoff")
	}
}

func update1(t *testing.T, m Model, msg tea.Msg) tea.Msg {
	t.Helper()
	_, cmd := m.Update(msg)
	if cmd == nil {
		t.Fatal("expected a command")
	}
	done := make(chan tea.Msg, 1)
	go func() { done <- cmd() }()
	select {
	case got := <-done:
		return got
	case <-time.After(time.Second):
		t.Fatal("command did not finish")
	}
	return nil
}

func TestOnlyEscClosesModalsAndOnlyCtrlSSaves(t *testing.T) {
	m, host := pullsModel(t)
	m = runAction(t, m, "comment")
	m = typeText(t, m, "q")
	if !m.modal.open || m.modal.area.Value() != "q" {
		t.Fatal("q in insert mode should type")
	}
	if m = pressRun(t, m, "esc", "q"); !m.modal.open {
		t.Fatal("q in normal mode should not close the comment modal")
	}
	if m = pressRun(t, m, "esc"); m.modal.open || host.has("comment o/r#7 q") {
		t.Fatal("esc in normal mode should close the comment modal without sending")
	}
	m = pressRun(t, m, "g", "e", "tab", "tab", "+")
	if !m.modal.open || m.modal.kind != modalEdit {
		t.Fatal("tab and + are text in the edit modal")
	}
	m = pressRun(t, m, "esc", "esc", "enter", "tab", "tab", "+")
	if !m.modal.open || m.modal.kind != modalReact {
		t.Fatal("+ should open the reaction picker")
	}
	if m = pressRun(t, m, "q", "enter"); !m.modal.open || slices.ContainsFunc(host.calls, func(c string) bool { return strings.HasPrefix(c, "react") }) {
		t.Fatal("q and enter should neither close nor save a list modal")
	}
	if m = pressRun(t, m, "esc"); m.modal.open {
		t.Fatal("esc should close a list modal")
	}
	m = pressRun(t, m, "g", "d", "l")
	m.diff.cursor = rowOf(t, *m.diff, rowLine, 1, 3)
	m = pressRun(t, m, "c")
	m = typeText(t, m, "q")
	if m = pressRun(t, m, "esc", "q"); !m.diff.editing {
		t.Fatal("q in normal mode should keep the review diff comment editor open")
	}
	if m = pressRun(t, m, "esc"); m.diff == nil || m.diff.editing {
		t.Fatal("esc in normal mode should close the review diff comment editor and keep the diff open")
	}
}

func TestCtrlWMovesBetweenTheListAndSidebar(t *testing.T) {
	m, _ := pullsModel(t)
	if m = pressRun(t, m, "ctrl+w", "l"); !m.pulls.sidebar {
		t.Fatal("ctrl+w l should focus the sidebar")
	}
	if m = pressRun(t, m, "ctrl+w", "h"); m.pulls.sidebar {
		t.Fatal("ctrl+w h should focus the list")
	}
	m = pressRun(t, m, "g", "d", "ctrl+w", "l")
	if m.diff.focus != focusDiff {
		t.Fatalf("ctrl+w l should move between the review diff panes, focus=%d", m.diff.focus)
	}
	empty, _ := pullsModel(t)
	empty.pulls.list = nil
	if empty = pressRun(t, empty, "ctrl+w", "l"); empty.pulls.sidebar {
		t.Fatal("ctrl+w l should do nothing without a pull request")
	}
}

func TestCreatePullRequestFromTheActions(t *testing.T) {
	m, host := pullsModel(t)
	host.branches = []string{"main", "feature", "login", "release"}
	m = runAction(t, m, "create")
	out := screen(m)
	if !host.has("branches ") || !m.modal.open || m.modal.kind != modalCreate || !m.modal.normal || m.modal.focus != 0 {
		t.Fatalf("create should open in normal mode on the branch dropdown, calls=%q", host.calls)
	}
	if !slices.Equal(m.modal.drops[0].choices, []string{"feature", "main", "login", "release"}) || !slices.Equal(m.modal.drops[1].choices, []string{"main", "feature", "login", "release"}) {
		t.Fatalf("the current branch and the default branch should come first, drops=%+v", m.modal.drops)
	}
	if !strings.Contains(out, "branch      feature") || !strings.Contains(out, "into        main") || !strings.Contains(out, "title") || !strings.Contains(out, "description") {
		t.Fatalf("the modal should show both dropdowns and the labeled title and description:\n%s", out)
	}
	rows := strings.Split(out, "\n")
	at := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "branch      feature") })
	if !strings.Contains(rows[at+2], "into        main") || strings.Trim(rows[at+1][strings.Index(rows[at+1], "│ "):], "│ ▌\uf407") != "" {
		t.Fatalf("an empty line should separate the source and target branches:\n%s", strings.Join(rows[at:at+3], "\n"))
	}
	if m = pressRun(t, m, "ctrl+s"); !m.modal.open || m.warning != "the title can't be empty" {
		t.Fatal("an empty title should not create anything")
	}
	m = pressRun(t, m, "enter", "j", "enter", "tab", "enter", "j", "j", "j", "enter")
	if out := screen(m); !strings.Contains(out, "branch      main") || !strings.Contains(out, "into        release") {
		t.Fatalf("each dropdown should keep its own pick:\n%s", out)
	}
	m = pressRun(t, m, "tab", "i")
	m = typeText(t, m, "Add login")
	m = pressRun(t, m, "esc", "tab", "i")
	m = typeText(t, m, "Adds login")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("create main release Add login|Adds login") || m.modal.open || m.notice != "created #12 from main into release" || m.pulls.details["o/r#12"].Head != "main" {
		t.Fatalf("ctrl+s should create from and into the picked branches, calls=%q notice=%q", host.calls, m.notice)
	}
	if i := slices.Index(host.calls, "create main release Add login|Adds login"); !slices.Contains(host.calls[i:], "list assignee:@me") {
		t.Fatalf("the list should reload after creating, calls=%q", host.calls)
	}

	host.err = errors.New("you must first push the current branch")
	m = runAction(t, m, "create")
	m.modal.headline.SetValue("Title")
	m.modal.area.SetValue("Body")
	if m = pressRun(t, m, "ctrl+s"); !strings.Contains(statusText(m), "create a pull request from feature: you must first push") {
		t.Fatalf("a failed create should show the reason, got %q", statusText(m))
	}
	host.err, m.pulls.err = nil, nil
	host.usersErr = errors.New("HTTP 404")
	m = runAction(t, m, "create")
	if !m.modal.open || !strings.Contains(screen(m), "couldn't list the branches, HTTP 404") || !strings.Contains(screen(m), "into        default branch") {
		t.Fatalf("a failed branch load should keep the modal with the default branch:\n%s", screen(m))
	}
	host.usersErr = nil
	m.modal.headline.SetValue("Title")
	m.modal.area.SetValue("Body")
	if m = pressRun(t, m, "ctrl+s"); !host.has("create feature  Title|Body") {
		t.Fatalf("saving before the branches load should leave the target to GitHub, calls=%q", host.calls)
	}
}

func TestEditModalChangesTheTargetBranch(t *testing.T) {
	m, host := pullsModel(t)
	host.branches = []string{"main", "release"}
	m = runAction(t, m, "edit")
	if !host.has("branches o/r") || !m.modal.onHeadline() || !slices.Equal(m.modal.drops[0].choices, []string{"main", "release"}) || !strings.Contains(screen(m), "into        main") {
		t.Fatalf("edit should start on the text with the current base first, calls=%q:\n%s", host.calls, screen(m))
	}
	m = pressRun(t, m, "ctrl+s")
	lastEdit := func() string {
		for i := len(host.calls) - 1; i >= 0; i-- {
			if strings.HasPrefix(host.calls[i], "edit ") {
				return host.calls[i]
			}
		}
		return ""
	}
	if last := lastEdit(); !strings.HasPrefix(last, "edit o/r#7 Add login|") || strings.Contains(last, " into ") {
		t.Fatalf("keeping the base should not change it, got %q", last)
	}
	m = runAction(t, m, "edit")
	m = pressRun(t, m, "shift+tab", "enter", "j", "enter", "ctrl+s")
	if last := lastEdit(); !strings.HasPrefix(last, "edit o/r#7 Add login|") || !strings.HasSuffix(last, " into release") {
		t.Fatalf("a new base should be sent, got %q", last)
	}
	m = runAction(t, m, "edit")
	m.applyBranches(branchesMsg{repo: "o/other", list: []string{"x"}})
	m.applyBranches(branchesMsg{repo: "o/r", list: []string{"y"}, err: errors.New("late")})
	if !slices.Equal(m.modal.drops[0].choices, []string{"main", "release"}) || !strings.Contains(m.modal.warning, "late") {
		t.Fatalf("branches of another repository should be ignored, drops=%+v", m.modal.drops)
	}
	undetailed, _ := pullsModel(t)
	undetailed.openModal(modalEdit, review.PullRequest{Repo: "o/r", Number: 7, Title: "t"})
	if len(undetailed.modal.drops) != 0 {
		t.Fatal("without a known base the edit modal should not offer to change it")
	}
}

func TestActionsWithoutASelectedPullRequest(t *testing.T) {
	m, _ := pullsModel(t)
	m.pulls.list = nil
	m = pressRun(t, m, "A")
	out := screen(m)
	if !m.modal.open || !strings.Contains(out, "actions ") || strings.Contains(out, "actions for") || len(m.matchingActions()) != 2 || !strings.Contains(out, "Create a pull request") || strings.Contains(out, "Merge") {
		t.Fatalf("without a pull request only the actions that need none should show:\n%s", out)
	}
	m, _ = pullsModel(t)
	m = pressRun(t, m, "g", "d", "A")
	if strings.Contains(screen(m), "Create a pull request") {
		t.Fatal("the review diff should not offer to create a pull request")
	}
}

func TestOpenDropdownShowsAWindowOfChoices(t *testing.T) {
	m, _ := pullsModel(t)
	m.openModal(modalCreate, review.PullRequest{})
	for i := range 20 {
		m.modal.drops[0].choices = append(m.modal.drops[0].choices, fmt.Sprintf("branch-%02d", i))
	}
	m.modal.setFocus(0)
	m.modal.expanded, m.modal.drops[0].choice = true, 13
	lines := m.dropdownList()
	if strings.Contains(lines, "branch-04") || !strings.Contains(lines, "branch-05") || !strings.Contains(lines, " branch-12") || strings.Contains(lines, "branch-13") {
		t.Fatalf("the open dropdown should show the %d rows ending at the picked one:\n%s", dropdownRows, ansi.Strip(lines))
	}
}

func TestDetailsSidebarLeavesMostRoomToTheList(t *testing.T) {
	m, _ := pullsModel(t)
	if m.sidebarWidth() >= m.pullsWidth() || m.sidebarWidth()+m.pullsWidth() != m.width {
		t.Fatalf("the sidebar (%d) should be narrower than the list (%d)", m.sidebarWidth(), m.pullsWidth())
	}
}

func statusText(m Model) string {
	if m.diff != nil {
		m = *m.diff
	}
	_, text := m.status()
	return text
}

func statusRow(m Model) string {
	rows := strings.Split(m.View().Content, "\n")
	return rows[len(rows)-2]
}

func TestStatusMessagesShowOnTheRightWithAnIconForFiveSeconds(t *testing.T) {
	m, _ := pullsModel(t)
	if row := ansi.Strip(statusRow(m)); strings.Contains(row, "shows every key") || strings.TrimSpace(row) != "NORMAL" {
		t.Fatalf("without a message the bar should only show the mode, got %q", row)
	}
	for _, c := range []struct {
		set  func(*Model)
		want string
	}{
		{func(m *Model) { m.notice = "merged #7" }, " merged #7"},
		{func(m *Model) { m.warning = "#7 is a draft" }, " #7 is a draft"},
		{func(m *Model) { m.pulls.err = errors.New("HTTP 502") }, " HTTP 502"},
	} {
		c.set(&m)
		m = update(t, m, tea.FocusMsg{})
		row := ansi.Strip(statusRow(m))
		if !strings.HasSuffix(strings.TrimRight(row, " "), c.want) || strings.Index(row, c.want) < m.width/2 {
			t.Fatalf("the message should sit on the right of the bar, got %q", row)
		}
		if m = update(t, m, statusTickMsg{}); !strings.Contains(statusRow(m), strings.SplitN(c.want, " ", 2)[1]) {
			t.Fatal("the message should stay before five seconds")
		}
		later := clock.Add(statusFor)
		now = func() time.Time { return later }
		if m = update(t, m, statusTickMsg{}); strings.Contains(ansi.Strip(statusRow(m)), c.want) || m.notice != "" || m.warning != "" || m.pulls.err != nil {
			t.Fatalf("the message should clear after five seconds, got %q", ansi.Strip(statusRow(m)))
		}
		now = func() time.Time { return clock }
	}

	m.pulls.loading = true
	m = update(t, m, tea.FocusMsg{})
	first := ansi.Strip(statusRow(m))
	if !strings.Contains(first, spinnerFrames[m.spin%len(spinnerFrames)]+" loading pull requests") || m.statusTick() == nil {
		t.Fatalf("a sync should show a spinner, got %q", first)
	}
	later := clock.Add(time.Minute)
	now = func() time.Time { return later }
	m = update(t, m, statusTickMsg{})
	if second := ansi.Strip(statusRow(m)); second == first || !strings.Contains(second, "loading pull requests") {
		t.Fatalf("the spinner should turn and never expire, %q then %q", first, second)
	}
	now = func() time.Time { return clock }
}

func TestStatusBarHasItsOwnBackground(t *testing.T) {
	m, _ := pullsModel(t)
	m.pulls.err = errors.New("boom")
	on := lipgloss.NewStyle().Background(colorRule).Foreground(colorForeground).Render(" ")
	on = on[:strings.Index(on, " ")]
	if row := statusRow(m); !strings.Contains(row, "\x1b[m"+on) || lipgloss.Width(row) != m.width {
		t.Fatalf("the bar background should carry across styled parts and span the width: %q", row)
	}
}

func TestPullListShowsTwoLineRowsInColumns(t *testing.T) {
	m, host := pullsModel(t)
	list := host.lists["assignee:@me"]
	list[0].Additions, list[0].Deletions, list[0].CreatedAt = 120, 4, clock.Add(-72*time.Hour)
	list[1].Title = strings.Repeat("A very long title ", 10)
	m.pulls.list = list
	var rows []string
	for r := range strings.SplitSeq(screen(m), "\n") {
		rows = append(rows, ansi.Truncate(r, m.pullsWidth(), ""))
	}
	find := func(s string) string {
		for _, r := range rows {
			if strings.Contains(r, s) {
				return r
			}
		}
		t.Fatalf("no row with %q:\n%s", s, screen(m))
		return ""
	}
	header, first, second := find("updated"), find("#7"), find("#8")
	col := func(row, s string) int { return ansi.StringWidth(row[:strings.Index(row, s)]) }
	if col(first, "@ana") != col(second, "@bob") || col(header, "author") != col(first, "@ana") || col(first, "Add login") != col(second, "A very") {
		t.Fatalf("the columns should line up:\n%s\n%s\n%s", header, first, second)
	}
	for _, want := range []string{"+120", "-4", "3d", "2h"} {
		if !strings.Contains(first, want) {
			t.Fatalf("the first line should show %q: %q", want, first)
		}
	}
	if !strings.Contains(second, "…") || ansi.StringWidth(second) != ansi.StringWidth(first) {
		t.Fatalf("a long title should end with … and keep the row width: %q", second)
	}
	i := slices.Index(rows, first)
	if !strings.Contains(rows[i+1], "o/r") || strings.Contains(rows[i+1], "#") {
		t.Fatalf("the second line should show the repository: %q", rows[i+1])
	}
}

func TestTitledModalsLeaveAnEmptyLineBeforeTheDescription(t *testing.T) {
	for _, query := range []string{"edit", "merge", "create"} {
		m, host := pullsModel(t)
		host.branches = []string{"main"}
		m = runAction(t, m, query)
		var title, description int
		for i, row := range strings.Split(screen(m), "\n") {
			if strings.Contains(row, "│ title ") {
				title = i
			}
			if strings.Contains(row, "│ description ") {
				description = i
			}
		}
		if title == 0 || description != title+2 {
			t.Fatalf("%s should show description two rows under title, title=%d description=%d:\n%s", query, title, description, screen(m))
		}
	}
}

func TestListsMoveWithJKOutsideTheSearch(t *testing.T) {
	m, host := pullsModel(t)
	host.labels = []string{"auth", "bug", "docs"}
	m = pressRun(t, m, "A")
	if m = typeText(t, m, "j"); m.modal.fields[0].Value() != "j" || m.modal.normal {
		t.Fatal("while searching, j should type")
	}
	m = pressRun(t, m, "backspace", "esc")
	if !m.modal.open || !m.modal.normal || m.modal.fields[0].Focused() || !strings.Contains(screen(m), "j k move") || !strings.Contains(screen(m), "/ i search") {
		t.Fatalf("esc should leave the search for the list:\n%s", screen(m))
	}
	if m = pressRun(t, m, "j", "j", "k", "j"); m.modal.cursor != 2 || m.modal.fields[0].Value() != "" {
		t.Fatalf("j and k should move the list, cursor=%d", m.modal.cursor)
	}
	if m = pressRun(t, m, "x"); m.modal.fields[0].Value() != "" {
		t.Fatal("other keys should not type outside the search")
	}
	if m = pressRun(t, m, "/"); m.modal.normal || !m.modal.fields[0].Focused() {
		t.Fatal("/ should go back to the search")
	}
	m = typeText(t, m, "labels")
	m = pressRun(t, m, "enter")
	if m.modal.kind != modalLabels || m.modal.normal {
		t.Fatal("the picked action should run")
	}
	m = pressRun(t, m, "esc", "j", "space")
	if !slices.Contains(m.modal.picked, host.labels[1]) || m.modal.cursor != 1 {
		t.Fatalf("j should move the label picker, picked=%q", m.modal.picked)
	}
	if m = pressRun(t, m, "i", "esc", "esc"); m.modal.open {
		t.Fatal("esc in the list should close the picker")
	}
}

func TestVimTextBoxesScrollWithTheCursor(t *testing.T) {
	m, _ := pullsModel(t)
	m = runAction(t, m, "edit")
	var body []string
	for i := range 80 {
		body = append(body, fmt.Sprintf("line %02d", i))
	}
	m = pressRun(t, m, "tab")
	m.modal.area.SetValue(strings.Join(body, "\n"))
	m.modal.area.MoveToBegin()
	m = pressRun(t, m, "G")
	if out := screen(m); !strings.Contains(out, "line 79") || strings.Contains(out, "line 00") {
		t.Fatalf("G should scroll to the end:\n%s", out)
	}
	m = pressRun(t, m, "g", "g")
	if out := screen(m); !strings.Contains(out, "line 00") || strings.Contains(out, "line 79") {
		t.Fatal("gg should scroll back to the top")
	}
	half := max(m.modal.area.Height()/2, 1)
	if m = pressRun(t, m, "ctrl+d"); m.modal.area.Line() != half {
		t.Fatalf("ctrl+d should move half the box, line=%d want %d", m.modal.area.Line(), half)
	}
	if m = pressRun(t, m, "ctrl+d", "ctrl+u"); m.modal.area.Line() != half {
		t.Fatalf("ctrl+u should move back half the box, line=%d", m.modal.area.Line())
	}
	m = pressRun(t, m, "G")
	for range 79 {
		m = pressRun(t, m, "k")
	}
	if out := screen(m); !strings.Contains(out, "line 00") || strings.Contains(out, "line 79") {
		t.Fatal("k should scroll up past the top of the box")
	}
}

func TestPullRequestsLoadInTheBackgroundOnceTheBranchLoads(t *testing.T) {
	host := newHost()
	m := New(review.NewService(sample, &fakeStore{viewed: map[string]string{}}, nil, "feature"), "", "main").
		WithHost(host, func(pr review.PullRequest) *review.Service {
			return review.NewService(sample, &fakeStore{viewed: map[string]string{}}, nil, prKey(pr))
		})
	m = update(t, m, m.Init()())
	if !m.pulls.started || !m.pulls.loading || m.tab == tabPulls {
		t.Fatal("the pull request list should start loading without leaving the Agent tab")
	}
	if _, text := m.status(); text != "" {
		t.Fatalf("the Agent tab should not show the pull request spinner, got %q", text)
	}
	if cmd := m.showPulls(); cmd != nil {
		t.Fatal("opening the tab should not load the list a second time")
	}
	nested := m
	nested.pr = review.PullRequest{Number: 7}
	nested.pulls.started = false
	if nested.startPulls(); nested.pulls.started {
		t.Fatal("a pull request's review diff should not load the list")
	}
}

func TestOpenDropdownOverlaysTheModal(t *testing.T) {
	m, host := pullsModel(t)
	host.branches = []string{"main", "feature", "login", "release"}
	m = runAction(t, m, "create")
	closed := strings.Split(screen(m), "\n")
	m = pressRun(t, m, "enter")
	open := strings.Split(screen(m), "\n")
	row := slices.IndexFunc(closed, func(r string) bool { return strings.Contains(r, "branch      feature") })
	if row < 0 || !strings.Contains(open[row], "branch      feature") || !strings.Contains(open[row], "") {
		t.Fatalf("the dropdown should stay on its row:\n%s", strings.Join(open, "\n"))
	}
	for _, label := range []string{"into ", "│ title", "│ description"} {
		before := slices.IndexFunc(closed, func(r string) bool { return strings.Contains(r, label) })
		after := slices.IndexFunc(open, func(r string) bool { return strings.Contains(r, label) && !strings.Contains(r, "") })
		if before < 0 || label != "into " && before != after {
			t.Fatalf("%q moved from row %d to %d when the dropdown opened", label, before, after)
		}
	}
	col := strings.Index(open[row], "feature")
	for i, want := range []string{" feature", "  main", "  login", "  release"} {
		got := open[row+2+i]
		if !strings.Contains(got, want) || ansi.StringWidth(got[:strings.Index(got, want)]) != ansi.StringWidth(open[row][:col]) {
			t.Fatalf("the list should open under the dropdown, row %d = %q", i, got)
		}
	}
	if m = pressRun(t, m, "enter"); strings.Contains(screen(m), " feature") || len(strings.Split(screen(m), "\n")) != len(closed) {
		t.Fatal("closing the dropdown should remove the list")
	}
}

func TestGCOpensTheCreateModalAndShowsInTheFooter(t *testing.T) {
	m, host := pullsModel(t)
	host.branches = []string{"main", "feature"}
	if footer := ansi.Strip(m.pullsFooter()); !strings.Contains(footer, "gc create") {
		t.Fatalf("the Pull Request footer should show gc, got %q", footer)
	}
	if m = pressRun(t, m, "g", "c"); !m.modal.open || m.modal.kind != modalCreate || !host.has("branches ") {
		t.Fatal("gc should open the create modal")
	}
	m = pressRun(t, m, "esc", "esc", "A")
	m = typeText(t, m, "create")
	if out := screen(m); !strings.Contains(out, "Create a pull request") || !strings.Contains(out, "gc") {
		t.Fatalf("the create action should list its gc shortcut:\n%s", out)
	}
	nohost, _ := pullsModel(t)
	nohost.host = nil
	if nohost = pressRun(t, nohost, "g", "c"); nohost.modal.open {
		t.Fatal("without gh, gc should do nothing")
	}
}

func TestMergeMessageOpensInNormalModeAtARegularSize(t *testing.T) {
	m, host := pullsModel(t)
	m = runAction(t, m, "merge")
	if m.modalWidth(modalMerge) >= m.modalWidth(modalEdit) || m.modal.area.Height() > 12 {
		t.Fatalf("the merge modal should be smaller than the edit modal, width=%d height=%d", m.modalWidth(modalMerge), m.modal.area.Height())
	}
	m = pressRun(t, m, "tab")
	if !m.modal.onHeadline() || m.modal.mode() != "normal" {
		t.Fatalf("the commit title should start in normal mode, mode=%q", m.modal.mode())
	}
	m = pressRun(t, m, "tab", "A")
	m = typeText(t, m, " and logout")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("merge o/r#7 MERGE Merge pull request #7 from o/login|Add login and logout") {
		t.Fatalf("j should reach the description to edit it, calls=%q", host.calls)
	}
}

func TestTitleBoxIsOneVimLine(t *testing.T) {
	m, _ := pullsModel(t)
	m = runAction(t, m, "edit")
	m = pressRun(t, m, "o")
	m = typeText(t, m, "more")
	m = pressRun(t, m, "enter")
	if v := m.modal.headline.Value(); strings.Contains(v, "\n") || !strings.Contains(v, "Add login") || !strings.Contains(v, "more") {
		t.Fatalf("o and enter should keep the title on one line, got %q", v)
	}
	if v := m.View(); v.Cursor == nil || v.Cursor.Shape != tea.CursorBar {
		t.Fatal("insert mode in the title should show a bar cursor")
	}
	m = pressRun(t, m, "esc", "0", "x")
	if v := m.modal.headline.Value(); !strings.HasPrefix(v, "dd login") {
		t.Fatalf("normal mode commands should edit the title, got %q", v)
	}
	if m.View().Cursor != nil {
		t.Fatal("normal mode should draw a block, not the bar")
	}
	if m.modal.area.Value() != "Adds **login** support" {
		t.Fatal("editing the title should leave the description alone")
	}
}

func TestMergedPullRequestTurnsPurpleThenLeavesTheList(t *testing.T) {
	m, host := pullsModel(t)
	if raw := m.View().Content; !strings.Contains(raw, lipgloss.NewStyle().Background(colorCursorline).Foreground(colorAdd).Render("\uf407")) {
		t.Fatalf("open pull requests should show a green icon:\n%s", raw)
	}
	m = runAction(t, m, "merge")
	m = pressRun(t, m, "ctrl+s")
	merged := lipgloss.NewStyle().Background(colorCursorline).Foreground(colorMerged).Render("\uf419")
	if raw := m.View().Content; !host.has("merge o/r#7 MERGE Merge pull request #7 from o/login|Add login") || !strings.Contains(raw, merged) {
		t.Fatalf("the merged pull request should turn purple right after the merge, calls=%q:\n%s", host.calls, raw)
	}
	if _, cmd := m.Update(detailMsg{key: "o/r#7", pr: host.details["o/r#7"]}); cmd != nil {
		t.Fatal("a second reload of the merged pull request should not start another timer")
	}

	other := m.pulls.list[1]
	m = update(t, m, pullsMsg{list: []review.PullRequest{other}})
	if len(m.pulls.list) != 2 || prKey(m.pulls.list[0]) != "o/r#7" || m.pulls.cursor != 0 {
		t.Fatalf("the merged pull request should stay in the list until its time is up, list=%v cursor=%d", m.pulls.list, m.pulls.cursor)
	}

	updated, cmd := m.Update(leftMsg{key: "o/r#7"})
	m = updated.(Model)
	if len(m.pulls.list) != 1 || prKey(m.pulls.list[0]) != "o/s#8" || m.pulls.cursor != 0 || cmd == nil || strings.Contains(screen(m), "Add login") {
		t.Fatalf("the merged pull request should leave the list and select the next one, list=%v cursor=%d:\n%s", m.pulls.list, m.pulls.cursor, screen(m))
	}
	if m = update(t, m, pullsMsg{list: []review.PullRequest{other}}); len(m.pulls.list) != 1 {
		t.Fatal("a removed pull request should not come back on the next reload")
	}
}

func TestClosedPullRequestTurnsRedAndLeavesWithoutMovingTheSelection(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "j")
	closed := host.details["o/r#7"]
	closed.State = "CLOSED"
	updated, cmd := m.Update(detailMsg{key: "o/r#7", pr: closed})
	m = updated.(Model)
	if raw := m.View().Content; cmd == nil || !strings.Contains(raw, lipgloss.NewStyle().Foreground(colorIssue).Render("\uf4dc")) {
		t.Fatalf("a closed pull request should show a red icon and start its timer:\n%s", raw)
	}
	if m = update(t, m, leftMsg{key: "o/r#7"}); len(m.pulls.list) != 1 || prKey(m.pulls.list[m.pulls.cursor]) != "o/s#8" {
		t.Fatalf("the selection should stay on #8, list=%v cursor=%d", m.pulls.list, m.pulls.cursor)
	}
	if m = update(t, m, leftMsg{key: "o/r#7"}); len(m.pulls.list) != 1 {
		t.Fatal("a second removal of the same pull request should change nothing")
	}
}

func TestStateOf(t *testing.T) {
	for state, want := range map[string]prState{"OPEN": {"\uf407", colorAdd}, "MERGED": {"\uf419", colorMerged}, "CLOSED": {"\uf4dc", colorIssue}, "": {"\uf407", colorDim}} {
		if got := stateOf(state); got != want {
			t.Errorf("stateOf(%q) = %v, want %v", state, got, want)
		}
	}
}

func TestDraftPullRequestKeepsItsPassingChecksMark(t *testing.T) {
	m, _ := pullsModel(t)
	m.pulls.list[1].Draft, m.pulls.list[1].State = true, "OPEN"
	m.pulls.list[1].Checks = []review.Check{{Name: "checks", State: "SUCCESS"}}
	raw := m.View().Content
	draft := lipgloss.NewStyle().Foreground(colorDim).Render(iconDraft)
	passed := lipgloss.NewStyle().Foreground(colorAdd).Render("✓")
	if !strings.Contains(raw, draft+" "+passed) {
		t.Fatalf("a draft should show the gray draft icon then the green check:\n%q", raw)
	}
}
