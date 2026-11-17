package tui

import (
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

func (h *fakeHost) EditPullRequest(_ context.Context, pr review.PullRequest, title, body string) error {
	return h.record(fmt.Sprintf("edit %s %s|%s", prKey(pr), title, body))
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
	case nil, pullsTickMsg, tickMsg:
		return m
	case tea.BatchMsg:
		for _, c := range msg {
			m = drainCmd(t, m, c)
		}
		return m
	case nestedMsg:
		if _, ok := msg.msg.(tickMsg); ok {
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
	old, oldNow, oldReview := pullsPollEvery, now, reviewPollEvery
	pullsPollEvery, reviewPollEvery, now = time.Millisecond, time.Millisecond, func() time.Time { return clock }
	t.Cleanup(func() { pullsPollEvery, now, reviewPollEvery = old, oldNow, oldReview })
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
	for _, want := range []string{"Agent", "Pull Request", "pull requests", "assigned · 2", "#7", "Add login", "o/r 2h", "#8", "Fix logout", "o/s 2d",
		"#7 o/r", "Overview", "Activity", "Comments", "Commits", "Files", "OPEN · @ana wants to merge login into main", "+10 -2 · 1 files · 1 commits",
		"assignees  @me", "reviewers  ✓ @bob  ● @cy", "labels     auth", "checks     ✓ 1  ● 1  ✗ 1", "✓ build success", "✗ test failure", "description", "Adds login support"} {
		if !strings.Contains(out, want) {
			t.Errorf("screen missing %q", want)
		}
	}
	if !strings.Contains(out, "▌✗ #7") || !strings.Contains(out, "◌ #8") {
		t.Fatalf("the list should show the failing checks and the draft:\n%s", out)
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
	if m = pressRun(t, m, "s", "j", "c"); m.modal.open || m.pulls.filter != 0 {
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
	m = pressRun(t, m, "c")
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
	m = pressRun(t, m, "c")
	if !m.modal.open || !strings.Contains(screen(m), "comment on #7") || !strings.Contains(screen(m), "INSERT") || !strings.Contains(screen(m), "C-s save") {
		t.Fatalf("c should open the comment modal:\n%s", screen(m))
	}
	if m = pressRun(t, m, "ctrl+s"); !m.modal.open || m.notice != "write something before saving" {
		t.Fatal("an empty comment should not be sent")
	}
	m = typeText(t, m, "Looks **good**")
	m = pressRun(t, m, "enter")
	m = typeText(t, m, "ok")
	m = pressRun(t, m, "ctrl+s")
	if m.modal.open || !host.has("comment o/r#7 Looks **good**\nok") || host.last() != "detail o/r#7" || m.notice != "commented on #7" {
		t.Fatalf("calls=%q notice=%q", host.calls, m.notice)
	}
	m = pressRun(t, m, "c")
	m = typeText(t, m, "nope")
	if m = pressRun(t, m, "esc"); m.modal.open || host.has("comment o/r#7 nope") {
		t.Fatal("esc should cancel without sending")
	}
}

func TestReplyReactResolveTargetTheSelectedComment(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "enter", "tab", "tab", "x")
	if m.notice != "only review threads on code can be resolved" {
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
	if !strings.Contains(screen(m), "▸ 👍 thumbs up") || !strings.Contains(screen(m), "j k pick") {
		t.Fatalf("missing reaction picker:\n%s", screen(m))
	}
	m = pressRun(t, m, "j", "j", "k", "enter")
	if !host.has("react C_1 THUMBS_DOWN true") || m.notice != "reacted 👎" {
		t.Fatalf("calls=%q notice=%q", host.calls, m.notice)
	}
	m = pressRun(t, m, "k", "+", "k", "enter")
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
	if m = pressRun(t, m, "r"); m.modal.open || !strings.Contains(m.notice, "no comments yet") || m.pulls.item != 0 {
		t.Fatal("with no comments there is nothing to reply to")
	}
}

func TestEditModalUpdatesTitleAndDescription(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "g", "e")
	out := screen(m)
	if !m.modal.open || !strings.Contains(out, "edit #7") || !strings.Contains(out, "title      Add login") || !strings.Contains(out, "Adds **login** support") {
		t.Fatalf("ge should open the edit modal with the current values:\n%s", out)
	}
	m = typeText(t, m, " v2")
	m = pressRun(t, m, "tab")
	m = typeText(t, m, "> ")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("edit o/r#7 Add login v2|> Adds **login** support") || m.notice != "updated #7" {
		t.Fatalf("calls=%q", host.calls)
	}
	m = pressRun(t, m, "g", "e")
	m.modal.fields[0].SetValue("  ")
	if m = pressRun(t, m, "ctrl+s"); !m.modal.open || m.notice != "the title can't be empty" {
		t.Fatal("an empty title should not be saved")
	}
	m = pressRun(t, m, "esc")
	host.details["o/r#7"] = review.PullRequest{Repo: "o/r", Number: 7, Title: "Renamed", Detailed: true}
	m = update(t, m, detailMsg{key: "o/r#7", pr: host.details["o/r#7"]})
	if m.pulls.list[0].Title != "Renamed" {
		t.Fatal("a new title should show in the list")
	}
}

func TestPeopleModalSetsAssigneesAndReviewers(t *testing.T) {
	m, host := pullsModel(t)
	m = pressRun(t, m, "A")
	if !strings.Contains(screen(m), "assignees  me") || !strings.Contains(screen(m), "reviewers  bob, cy") {
		t.Fatalf("A should open the people modal:\n%s", screen(m))
	}
	m = typeText(t, m, ", @dan dan")
	m = pressRun(t, m, "tab", "shift+tab", "tab")
	m = typeText(t, m, ",")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("people o/r#7 me,dan bob,cy") || m.notice != "updated assignees and reviewers" {
		t.Fatalf("calls=%q", host.calls)
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
	if footer := ansi.Strip(m.diff.footer()); !strings.Contains(out, "continuous diff") || !strings.Contains(out, "main…o/r#7") || !strings.Contains(footer, "gs submit review") || strings.Contains(footer, "S send") || strings.Contains(footer, "y copy") {
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

	m = pressRun(t, m, "g", "s")
	if !m.diff.modal.open || !strings.Contains(screen(m), "submit your review of #7") || !strings.Contains(screen(m), "▸ Comment") {
		t.Fatalf("gs should open the submit modal:\n%s", screen(m))
	}
	if m = pressRun(t, m, "1"); m.tab != tabPulls {
		t.Fatal("tab keys are text while the modal is open")
	}
	if m = pressRun(t, m, "ctrl+s"); !m.diff.modal.open {
		t.Fatal("a comment review needs a body")
	}
	detail := host.details["o/r#7"]
	detail.PendingReview = "PRR_9"
	host.details["o/r#7"] = detail
	m = pressRun(t, m, "j", "ctrl+s")
	if !host.has("submit o/r#7 PRR_9 APPROVE ") || m.diff.notice != "submitted approve on #7" || m.diff.modal.open {
		t.Fatalf("calls=%q notice=%q", host.calls, m.diff.notice)
	}
	host.err = errors.New("can't approve your own pull request")
	m = pressRun(t, m, "g", "s", "j", "j", "tab")
	m = typeText(t, m, "Please fix")
	m = pressRun(t, m, "ctrl+s")
	if !host.has("submit o/r#7 PRR_9 REQUEST_CHANGES Please fix") || !strings.Contains(screen(m), "can't approve your own") {
		t.Fatalf("errors should show in the review diff:\n%s", screen(m))
	}

	m = pressRun(t, m, "1")
	if m.tab != tabAgent || m.diff == nil {
		t.Fatal("1 should show the Agent tab and keep the review diff")
	}
	if m = pressRun(t, m, "2"); !strings.Contains(screen(m), "main…o/r#7") {
		t.Fatal("2 should come back to the review diff")
	}
	m = update(t, m, tea.WindowSizeMsg{Width: 90, Height: 30})
	if m.diff.width != 90 {
		t.Fatal("resizes should reach the review diff")
	}
	if m = pressRun(t, m, "q"); m.diff != nil || m.tab != tabPulls {
		t.Fatal("q should close the review diff")
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
	m, cmd := click(t, m, 5, 3)
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
	m = pressRun(t, m, "c")
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
	if got := logins(" @ana, bob ,, ana cy"); strings.Join(got, ",") != "ana,bob,cy" {
		t.Fatalf("logins = %q", got)
	}
	if logins("  ") != nil {
		t.Fatal("no logins in blank text")
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
	if got := people(review.PullRequest{Reviewers: []review.Reviewer{{Login: "a", State: "CHANGES_REQUESTED"}, {Login: "b", State: "DISMISSED"}}}); got != "✗ @a  @b" {
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
	if !strings.Contains(out, "L2 · @bob") || !strings.Contains(out, "↳ reply") || strings.Contains(out, "agent reply") || !strings.Contains(out, "@ana because") {
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
	m = pressRun(t, m, "1", "g", "e")
	if !host.has("branch") || m.tab != tabAgent || !m.modal.open || !strings.Contains(screen(m), "edit #7") {
		t.Fatalf("ge on the Agent tab should edit the branch's pull request in place, calls=%q", host.calls)
	}
	m = pressRun(t, m, "esc", "g", "d")
	if m.tab != tabPulls || m.diff == nil || m.diff.pr.Number != 7 {
		t.Fatal("gd on the Agent tab should open the branch's review diff on the Pull Request tab")
	}
	calls := len(host.calls)
	if m = pressRun(t, m, "g", "e"); slices.Contains(host.calls[calls:], "branch") {
		t.Fatal("the review diff should not look up the branch's pull request")
	}
	m = pressRun(t, m, "q", "1")
	host.detailErr = errors.New("no pull requests found for branch")
	if m = pressRun(t, m, "g", "d"); m.tab != tabAgent || m.diff != nil || !strings.Contains(screen(m), "no pull requests found") {
		t.Fatal("a branch without a pull request should show the error and stay on the Agent tab")
	}
	plain, _ := loaded(t)
	if _, cmd := press(t, plain, "g", "e"); cmd != nil {
		t.Fatal("without gh, ge does nothing")
	}
}
