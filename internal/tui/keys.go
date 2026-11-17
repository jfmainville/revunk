package tui

import (
	"reflect"
	"slices"
	"strings"

	"charm.land/bubbles/v2/key"
)

type keyMap struct {
	Quit        key.Binding
	ForceQuit   key.Binding
	Help        key.Binding
	CloseHelp   key.Binding
	NextFocus   key.Binding
	PrevFocus   key.Binding
	FocusDiff   key.Binding
	Down        key.Binding
	Up          key.Binding
	Top         key.Binding
	Bottom      key.Binding
	HalfDown    key.Binding
	HalfUp      key.Binding
	PageDown    key.Binding
	PageUp      key.Binding
	NextFile    key.Binding
	PrevFile    key.Binding
	NextChange  key.Binding
	PrevChange  key.Binding
	Open        key.Binding
	Select      key.Binding
	Cancel      key.Binding
	Comment     key.Binding
	FileComment key.Binding
	Delete      key.Binding
	DeleteAll   key.Binding
	Viewed      key.Binding
	Comments    key.Binding
	Explorer    key.Binding
	Send        key.Binding
	Copy        key.Binding
	Save        key.Binding
	Newline     key.Binding
	NextKind    key.Binding
	PrevKind    key.Binding
	Search      key.Binding
	NextMatch   key.Binding
	PrevMatch   key.Binding
	DeleteChar  key.Binding
	OpenFile    key.Binding
	AgentTab    key.Binding
	PullsTab    key.Binding
	NextFilter  key.Binding
	EditPR      key.Binding
	OpenDiff    key.Binding
	Submit      key.Binding
	Assignees   key.Binding
	Reviewers   key.Binding
	PickDown    key.Binding
	PickUp      key.Binding
	Toggle      key.Binding
	Reply       key.Binding
	React       key.Binding
	Resolve     key.Binding
	NormalMode  key.Binding
	LeaveInsert key.Binding
	Insert      key.Binding
	Append      key.Binding
	InsertStart key.Binding
	AppendEnd   key.Binding
	OpenBelow   key.Binding
	OpenAbove   key.Binding
	CharLeft    key.Binding
	CharRight   key.Binding
	WordNext    key.Binding
	WordPrev    key.Binding
	LineStart   key.Binding
	LineEnd     key.Binding
	DeleteUnder key.Binding
	WindowLeft  key.Binding
	WindowDown  key.Binding
	WindowUp    key.Binding
	WindowRight key.Binding
}

func bind(keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...))
}

func defaultKeys() keyMap {
	return keyMap{
		Quit:        bind("q"),
		ForceQuit:   bind("ctrl+c"),
		Help:        bind("?"),
		CloseHelp:   bind("esc", "?", "q"),
		NextFocus:   bind("tab"),
		PrevFocus:   bind("shift+tab"),
		FocusDiff:   bind("l"),
		Down:        bind("j", "down"),
		Up:          bind("k", "up"),
		Top:         bind("g g"),
		Bottom:      bind("G"),
		HalfDown:    bind("ctrl+d"),
		HalfUp:      bind("ctrl+u"),
		PageDown:    bind("pgdown"),
		PageUp:      bind("pgup", "b"),
		NextFile:    bind("f", "] f"),
		PrevFile:    bind("F", "[ f"),
		NextChange:  bind("h", "}"),
		PrevChange:  bind("H", "{"),
		Open:        bind("enter"),
		Select:      bind("v", "V"),
		Cancel:      bind("esc"),
		Comment:     bind("c"),
		FileComment: bind("C"),
		Delete:      bind("d d"),
		DeleteAll:   bind("D D"),
		Viewed:      bind("-"),
		Comments:    bind("t"),
		Explorer:    bind("e"),
		Send:        bind("S"),
		Copy:        bind("y"),
		Save:        bind("ctrl+s"),
		Newline:     bind("enter"),
		NextKind:    bind("tab"),
		PrevKind:    bind("shift+tab"),
		Search:      bind("/"),
		NextMatch:   bind("n", "ctrl+n"),
		PrevMatch:   bind("N", "ctrl+p"),
		DeleteChar:  bind("backspace"),
		OpenFile:    bind("g f"),
		AgentTab:    bind("1"),
		PullsTab:    bind("2"),
		NextFilter:  bind("s"),
		EditPR:      bind("g e"),
		OpenDiff:    bind("g d"),
		Submit:      bind("g s"),
		Assignees:   bind("A"),
		Reviewers:   bind("R"),
		PickDown:    bind("down", "ctrl+n"),
		PickUp:      bind("up", "ctrl+p"),
		Toggle:      bind("space"),
		Reply:       bind("r"),
		React:       bind("+"),
		Resolve:     bind("x"),
		NormalMode:  bind("esc"),
		LeaveInsert: bind("j"),
		Insert:      bind("i"),
		Append:      bind("a"),
		InsertStart: bind("I"),
		AppendEnd:   bind("A"),
		OpenBelow:   bind("o"),
		OpenAbove:   bind("O"),
		CharLeft:    bind("h", "left"),
		CharRight:   bind("l", "right"),
		WordNext:    bind("w"),
		WordPrev:    bind("b"),
		LineStart:   bind("0"),
		LineEnd:     bind("$"),
		DeleteUnder: bind("x"),
		WindowLeft:  bind("ctrl+h"),
		WindowDown:  bind("ctrl+j"),
		WindowUp:    bind("ctrl+k"),
		WindowRight: bind("ctrl+l"),
	}
}

func is(k string, b key.Binding) bool {
	return b.Enabled() && slices.Contains(b.Keys(), k)
}

func (k keyMap) all() []key.Binding {
	v := reflect.ValueOf(k)
	out := make([]key.Binding, v.NumField())
	for i := range out {
		out[i] = v.Field(i).Interface().(key.Binding)
	}
	return out
}

func (k keyMap) isPrefix(s string) bool {
	for _, b := range k.all() {
		if _, ok := rest(b, s); ok {
			return true
		}
	}
	return false
}

func rest(b key.Binding, first string) (string, bool) {
	for _, seq := range b.Keys() {
		if after, ok := strings.CutPrefix(seq, first+" "); ok && b.Enabled() {
			return after, true
		}
	}
	return "", false
}

type hint struct {
	desc  string
	keys  []key.Binding
	label string
}

var keyNames = strings.NewReplacer(" ", "", "enter", "⏎", "ctrl+", "C-", "shift+", "S-", "down", "↓", "up", "↑", "pgdown", "pgdn")

func (h hint) text(all bool) string {
	if h.label != "" {
		return h.label
	}
	var labels []string
	for _, b := range h.keys {
		if !b.Enabled() {
			continue
		}
		keys := b.Keys()
		if !all {
			keys = keys[:min(len(keys), 1)]
		}
		for _, k := range keys {
			labels = append(labels, keyNames.Replace(k))
		}
	}
	return strings.Join(labels, " ")
}

func label(b key.Binding) string {
	return hint{keys: []key.Binding{b}}.text(false)
}

type helpSection struct {
	title string
	hints []hint
}

func (k keyMap) helpSections() []helpSection {
	return []helpSection{
		{"move", []hint{
			{desc: "pane to the left / below / above / right", keys: []key.Binding{k.WindowLeft, k.WindowDown, k.WindowUp, k.WindowRight}},
			{desc: "down / up", keys: []key.Binding{k.Down, k.Up}},
			{desc: "top / bottom", keys: []key.Binding{k.Top, k.Bottom}},
			{desc: "half page", keys: []key.Binding{k.HalfDown, k.HalfUp}},
			{desc: "page", keys: []key.Binding{k.PageDown, k.PageUp}},
			{desc: "next / previous file", keys: []key.Binding{k.NextFile, k.PrevFile}},
			{desc: "next / previous block of changes", keys: []key.Binding{k.NextChange, k.PrevChange}},
			{desc: "cycle focus, focus diff", keys: []key.Binding{k.NextFocus, k.FocusDiff}},
			{desc: "search every file in the diff", keys: []key.Binding{k.Search}},
			{desc: "next / previous match", keys: []key.Binding{k.NextMatch, k.PrevMatch}},
			{desc: "focus a pane and select the row", label: "click"},
			{desc: "focus and scroll the pane under the pointer", label: "wheel"},
		}},
		{"review", []hint{
			{desc: "visual range", keys: []key.Binding{k.Select}},
			{desc: "comment on line, range or file header", keys: []key.Binding{k.Comment}},
			{desc: "comment on the whole file", keys: []key.Binding{k.FileComment}},
			{desc: "delete or resolve comment under cursor", keys: []key.Binding{k.Delete}},
			{desc: "show or hide the comments pane", keys: []key.Binding{k.Comments}},
			{desc: "show or hide the commits and files panes", keys: []key.Binding{k.Explorer}},
			{desc: "delete every comment", keys: []key.Binding{k.DeleteAll}},
			{desc: "mark file viewed until it changes", keys: []key.Binding{k.Viewed}},
			{desc: "send unsent comments to the agent", keys: []key.Binding{k.Send}},
			{desc: "copy unsent comments", keys: []key.Binding{k.Copy}},
			{desc: "open the file at the cursor in $EDITOR", keys: []key.Binding{k.OpenFile}},
			{desc: "edit the branch's pull request", keys: []key.Binding{k.EditPR}},
			{desc: "review the branch's pull request", keys: []key.Binding{k.OpenDiff}},
		}},
		{"pull requests", []hint{
			{desc: "next filter: assigned, review requested, authored, involved", keys: []key.Binding{k.NextFilter}},
			{desc: "focus the details sidebar, back to the list", keys: []key.Binding{k.Open, k.Cancel}},
			{desc: "next / previous details tab", keys: []key.Binding{k.NextFocus, k.PrevFocus}},
			{desc: "edit the title and description", keys: []key.Binding{k.EditPR}},
			{desc: "review the diff with GitHub comments", keys: []key.Binding{k.OpenDiff}},
			{desc: "submit the review: comment, approve, request changes", keys: []key.Binding{k.Submit}},
			{desc: "pick assignees, reviewers", keys: []key.Binding{k.Assignees, k.Reviewers}},
			{desc: "in a picker, move, select, save", keys: []key.Binding{k.PickDown, k.PickUp, k.Toggle, k.Save}},
			{desc: "comment on the pull request", keys: []key.Binding{k.Comment}},
			{desc: "reply to the selected comment", keys: []key.Binding{k.Reply}},
			{desc: "react to the selected comment", keys: []key.Binding{k.React}},
			{desc: "resolve or unresolve the selected thread", keys: []key.Binding{k.Resolve}},
		}},
		{"comment editor", []hint{
			{desc: "save", keys: []key.Binding{k.Save}},
			{desc: "newline", keys: []key.Binding{k.Newline}},
			{desc: "cycle type, suggestion adds the lines as a code block", keys: []key.Binding{k.NextKind}},
			{desc: "cancel, from normal mode", keys: []key.Binding{k.Cancel}},
		}},
		{"comment and modal text, vim style", []hint{
			{desc: "normal mode, from insert", label: k.leaveLabel(), keys: []key.Binding{k.NormalMode, k.LeaveInsert}},
			{desc: "insert before, after the cursor", keys: []key.Binding{k.Insert, k.Append}},
			{desc: "insert at the line start, end", keys: []key.Binding{k.InsertStart, k.AppendEnd}},
			{desc: "open a line below, above", keys: []key.Binding{k.OpenBelow, k.OpenAbove}},
			{desc: "left / right", keys: []key.Binding{k.CharLeft, k.CharRight}},
			{desc: "next / previous word", keys: []key.Binding{k.WordNext, k.WordPrev}},
			{desc: "line start / end", keys: []key.Binding{k.LineStart, k.LineEnd}},
			{desc: "delete the character, the line", keys: []key.Binding{k.DeleteUnder, k.Delete}},
			{desc: "cancel, from normal", keys: []key.Binding{k.Cancel}},
		}},
		{"anywhere", []hint{
			{desc: "Agent / Pull Request tab", keys: []key.Binding{k.AgentTab, k.PullsTab}},
			{desc: "this help", keys: []key.Binding{k.Help}},
			{desc: "quit", keys: []key.Binding{k.Quit}},
		}},
	}
}

func (k keyMap) editorHints(mode string) []hint {
	return append([]hint{
		{desc: "save", keys: []key.Binding{k.Save}},
		{desc: "newline", keys: []key.Binding{k.Newline}},
		{desc: "type", keys: []key.Binding{k.NextKind}},
	}, k.vimHints(mode)...)
}

func (k keyMap) vimHints(mode string) []hint {
	switch mode {
	case "insert":
		return []hint{{desc: "normal", label: k.leaveLabel()}}
	case "normal":
		return []hint{{desc: "insert", keys: []key.Binding{k.Insert}}, {desc: "cancel", keys: []key.Binding{k.Cancel}}}
	}
	return []hint{{desc: "cancel", keys: []key.Binding{k.Cancel}}}
}

func (k keyMap) searchHints() []hint {
	return []hint{
		{desc: "confirm, then n N to step", keys: []key.Binding{k.Open}},
		{desc: "cancel", keys: []key.Binding{k.Cancel}},
	}
}

func (k keyMap) rangeHints() []hint {
	return []hint{
		{desc: "comment range", keys: []key.Binding{k.Comment}},
		{desc: "extend", keys: []key.Binding{k.Down, k.Up}},
		{desc: "cancel", keys: []key.Binding{k.Cancel}},
	}
}

func (k keyMap) listHints() []hint {
	return []hint{
		{desc: "move", keys: []key.Binding{k.Down, k.Up}},
		{desc: "go to", keys: []key.Binding{k.Open}},
		{desc: "delete", keys: []key.Binding{k.Delete}},
		{desc: "focus", keys: []key.Binding{k.NextFocus}},
		{desc: "close", keys: []key.Binding{k.Comments}},
	}
}

func (k keyMap) mainHints() []hint {
	return []hint{
		{desc: "file", keys: []key.Binding{k.NextFile, k.PrevFile}},
		{desc: "change", keys: []key.Binding{k.NextChange, k.PrevChange}},
		{desc: "search", keys: []key.Binding{k.Search}},
		{desc: "comments", keys: []key.Binding{k.Comments}},
		{desc: "explorer", keys: []key.Binding{k.Explorer}},
		{desc: "comment", keys: []key.Binding{k.Comment}},
		{desc: "file comment", keys: []key.Binding{k.FileComment}},
		{desc: "range", keys: []key.Binding{k.Select}},
		{desc: "viewed", keys: []key.Binding{k.Viewed}},
		{desc: "delete", keys: []key.Binding{k.Delete}},
		{desc: "send", keys: []key.Binding{k.Send}},
		{desc: "copy", keys: []key.Binding{k.Copy}},
		{desc: "submit review", keys: []key.Binding{k.Submit}},
		{desc: "focus", keys: []key.Binding{k.NextFocus}},
		{desc: "help", keys: []key.Binding{k.Help}},
	}
}

func (k keyMap) pullsHints() []hint {
	return []hint{
		{desc: "move", keys: []key.Binding{k.Down, k.Up}},
		{desc: "details", keys: []key.Binding{k.Open}},
		{desc: "filter", keys: []key.Binding{k.NextFilter}},
		{desc: "diff", keys: []key.Binding{k.OpenDiff}},
		{desc: "edit", keys: []key.Binding{k.EditPR}},
		{desc: "assignees", keys: []key.Binding{k.Assignees}},
		{desc: "reviewers", keys: []key.Binding{k.Reviewers}},
		{desc: "comment", keys: []key.Binding{k.Comment}},
		{desc: "agent", keys: []key.Binding{k.AgentTab}},
		{desc: "help", keys: []key.Binding{k.Help}},
	}
}

func (k keyMap) detailsHints() []hint {
	return []hint{
		{desc: "next tab", keys: []key.Binding{k.NextFocus}},
		{desc: "move", keys: []key.Binding{k.Down, k.Up}},
		{desc: "comment", keys: []key.Binding{k.Comment}},
		{desc: "reply", keys: []key.Binding{k.Reply}},
		{desc: "react", keys: []key.Binding{k.React}},
		{desc: "resolve", keys: []key.Binding{k.Resolve}},
		{desc: "diff", keys: []key.Binding{k.OpenDiff}},
		{desc: "edit", keys: []key.Binding{k.EditPR}},
		{desc: "list", keys: []key.Binding{k.Cancel}},
	}
}

func (k keyMap) leaveLabel() string {
	return label(k.NormalMode) + " " + label(k.LeaveInsert) + label(k.LeaveInsert)
}

func (k keyMap) pickerHints() []hint {
	return []hint{
		{desc: "move", keys: []key.Binding{k.PickDown, k.PickUp}},
		{desc: "select", keys: []key.Binding{k.Toggle}},
		{desc: "save", keys: []key.Binding{k.Save}},
		{desc: "cancel", keys: []key.Binding{k.Cancel}},
	}
}

func (k keyMap) modalHints(choices bool, mode string) []hint {
	list := []hint{
		{desc: "save", keys: []key.Binding{k.Save}},
		{desc: "next field", keys: []key.Binding{k.NextFocus}},
	}
	list = append(list, k.vimHints(mode)...)
	if choices {
		list = append(list, hint{desc: "pick", keys: []key.Binding{k.Down, k.Up}})
	}
	return list
}
