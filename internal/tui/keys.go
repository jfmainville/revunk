package tui

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
)

type keyMap struct {
	Quit        key.Binding
	ForceQuit   key.Binding
	Help        key.Binding
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
	Fold        key.Binding
	Comments    key.Binding
	Edit        key.Binding
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
	OpenURL     key.Binding
	CreatePR    key.Binding
	Actions     key.Binding
	CommitInfo  key.Binding
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
	Ops         operatorKeys
	WindowLeft  key.Binding
	WindowDown  key.Binding
	WindowUp    key.Binding
	WindowRight key.Binding
}

// operatorKeys stay out of the global sequences so c and d remain single keys outside text boxes.
type operatorKeys struct {
	InnerChange, WordChange, InnerDelete, WordDelete, ChangeToEnd, DeleteToEnd key.Binding
}

func (o operatorKeys) list() []key.Binding {
	return []key.Binding{o.InnerChange, o.WordChange, o.InnerDelete, o.WordDelete, o.ChangeToEnd, o.DeleteToEnd}
}

func bind(keys ...string) key.Binding {
	return key.NewBinding(key.WithKeys(keys...))
}

func defaultKeys(leader string) keyMap {
	return keyMap{
		Quit:        bind("q"),
		ForceQuit:   bind("ctrl+c"),
		Help:        bind("?"),
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
		Fold:        bind("z a"),
		Comments:    bind("t"),
		Edit:        bind("e"),
		Explorer:    bind(leader + " e"),
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
		OpenURL:     bind("g u"),
		CreatePR:    bind("g c"),
		Actions:     bind("A"),
		CommitInfo:  bind("K"),
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
		Ops: operatorKeys{
			InnerChange: bind("c i w"),
			WordChange:  bind("c w"),
			InnerDelete: bind("d i w"),
			WordDelete:  bind("d w"),
			ChangeToEnd: bind("c $"),
			DeleteToEnd: bind("d $"),
		},
		WindowLeft:  bind("ctrl+w h"),
		WindowDown:  bind("ctrl+w j"),
		WindowUp:    bind("ctrl+w k"),
		WindowRight: bind("ctrl+w l"),
	}
}

func is(k string, b key.Binding) bool {
	return b.Enabled() && slices.Contains(b.Keys(), k)
}

func (k *keyMap) override(keys map[string][]string) error {
	bindings := map[string]*key.Binding{}
	collectBindings(reflect.ValueOf(k).Elem(), "", bindings)
	for _, name := range slices.Sorted(maps.Keys(keys)) {
		b, ok := bindings[name]
		if !ok {
			return fmt.Errorf("unknown key binding %q", name)
		}
		if len(keys[name]) == 0 {
			b.SetEnabled(false)
			continue
		}
		b.SetKeys(keys[name]...)
	}
	return nil
}

func collectBindings(v reflect.Value, prefix string, out map[string]*key.Binding) {
	t := v.Type()
	for i := range t.NumField() {
		field := v.Field(i)
		name := prefix + configName(t.Field(i).Name)
		if b, ok := field.Addr().Interface().(*key.Binding); ok {
			out[name] = b
			continue
		}
		collectBindings(field, name+".", out)
	}
}

func configName(field string) string {
	runes := []rune(field)
	var b strings.Builder
	for i, r := range runes {
		if i > 0 && unicode.IsUpper(r) && (unicode.IsLower(runes[i-1]) || (i+1 < len(runes) && unicode.IsLower(runes[i+1]))) {
			b.WriteByte('-')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

func (k keyMap) all() []key.Binding {
	v := reflect.ValueOf(k)
	var out []key.Binding
	for i := range v.NumField() {
		if b, ok := v.Field(i).Interface().(key.Binding); ok {
			out = append(out, b)
		}
	}
	return out
}

func (k keyMap) pendingOp(seq string) bool {
	for _, b := range append([]key.Binding{k.Top, k.Delete}, k.Ops.list()...) {
		if _, ok := rest(b, seq); ok {
			return true
		}
	}
	return false
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

var keyNames = strings.NewReplacer("space ", "SPC ", " ", "", "enter", "⏎", "ctrl+", "C-", "shift+", "S-", "down", "↓", "up", "↑", "pgdown", "pgdn")

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
			{desc: "edit the comment under cursor, your own only", keys: []key.Binding{k.Edit}},
			{desc: "delete comment under cursor", keys: []key.Binding{k.Delete}},
			{desc: "resolve or reopen comment, resolved ones only show in the comments pane", keys: []key.Binding{k.Resolve}},
			{desc: "reply to the agent's answer, sent with the thread by the next send", keys: []key.Binding{k.Reply}},
			{desc: "show or hide the comments pane", keys: []key.Binding{k.Comments}},
			{desc: "show or hide the commits and files panes", keys: []key.Binding{k.Explorer}},
			{desc: "delete every comment", keys: []key.Binding{k.DeleteAll}},
			{desc: "details of the selected commit: author, message, changed files", keys: []key.Binding{k.CommitInfo}},
			{desc: "mark file viewed until it changes", keys: []key.Binding{k.Viewed}},
			{desc: "expand / collapse the file under cursor", keys: []key.Binding{k.Fold}},
			{desc: "send unsent comments to the agent", keys: []key.Binding{k.Send}},
			{desc: "copy unsent comments", keys: []key.Binding{k.Copy}},
			{desc: "open the file at the cursor in $EDITOR", keys: []key.Binding{k.OpenFile}},
			{desc: "edit the reviewed pull request, in a review diff", keys: []key.Binding{k.EditPR}},
			{desc: "review the branch's pull request", keys: []key.Binding{k.OpenDiff}},
		}},
		{"pull requests", []hint{
			{desc: "next filter: this repo, assigned, review requested, authored, involved", keys: []key.Binding{k.NextFilter}},
			{desc: "focus the details sidebar, back to the list", keys: []key.Binding{k.Open, k.Cancel}},
			{desc: "next / previous details tab", keys: []key.Binding{k.NextFocus, k.PrevFocus}},
			{desc: "edit the title and description, create a pull request", keys: []key.Binding{k.EditPR, k.CreatePR}},
			{desc: "review the diff with GitHub comments", keys: []key.Binding{k.OpenDiff}},
			{desc: "close the review diff, once nothing is selected or searched", keys: []key.Binding{k.Cancel}},
			{desc: "review the diff of a pull request from its GitHub URL, on either tab", keys: []key.Binding{k.OpenURL}},
			{desc: "search the actions: comment, review, merge, close, assignees, reviewers, labels, Copilot, also in the review diff", keys: []key.Binding{k.Actions}},
			{desc: "in the actions, move, run", keys: []key.Binding{k.PickDown, k.PickUp, k.Open}},
			{desc: "in a list, leave the search, move, search again", keys: []key.Binding{k.Cancel, k.Down, k.Up, k.Search}},
			{desc: "in a picker, move, select, save", keys: []key.Binding{k.PickDown, k.PickUp, k.Toggle, k.Save}},
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
			{desc: "delete or change a character, line, or word", keys: []key.Binding{k.DeleteUnder, k.Delete, k.Ops.InnerDelete, k.Ops.WordDelete, k.Ops.InnerChange, k.Ops.WordChange}},
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
		{desc: "resolve", keys: []key.Binding{k.Resolve}},
		{desc: "reply", keys: []key.Binding{k.Reply}},
		{desc: "focus", keys: []key.Binding{k.NextFocus}},
		{desc: "close", keys: []key.Binding{k.Comments}},
	}
}

func (k keyMap) mainHints() []hint {
	return []hint{
		{desc: "comment", keys: []key.Binding{k.Comment}},
		{desc: "range", keys: []key.Binding{k.Select}},
		{desc: "comments", keys: []key.Binding{k.Comments}},
		{desc: "send", keys: []key.Binding{k.Send}},
		{desc: "search", keys: []key.Binding{k.Search}},
		{desc: "help", keys: []key.Binding{k.Help}},
	}
}

func (k keyMap) pullsHints() []hint {
	return []hint{
		{desc: "move", keys: []key.Binding{k.Down, k.Up}},
		{desc: "details", keys: []key.Binding{k.Open}},
		{desc: "actions", keys: []key.Binding{k.Actions}},
		{desc: "filter", keys: []key.Binding{k.NextFilter}},
		{desc: "diff", keys: []key.Binding{k.OpenDiff}},
		{desc: "url", keys: []key.Binding{k.OpenURL}},
		{desc: "edit", keys: []key.Binding{k.EditPR}},
		{desc: "create", keys: []key.Binding{k.CreatePR}},
		{desc: "help", keys: []key.Binding{k.Help}},
	}
}

func (k keyMap) detailsHints() []hint {
	return []hint{
		{desc: "next tab", keys: []key.Binding{k.NextFocus}},
		{desc: "move", keys: []key.Binding{k.Down, k.Up}},
		{desc: "actions", keys: []key.Binding{k.Actions}},
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

func (k keyMap) searchListHints(normal bool, act hint) []hint {
	if normal {
		return []hint{{desc: "move", keys: []key.Binding{k.Down, k.Up}}, act, {desc: "search", keys: []key.Binding{k.Search, k.Insert}}, {desc: "cancel", keys: []key.Binding{k.Cancel}}}
	}
	return []hint{{desc: "move", keys: []key.Binding{k.PickDown, k.PickUp}}, act, {desc: "list", keys: []key.Binding{k.Cancel}}}
}

func (k keyMap) actionsHints(normal bool) []hint {
	return k.searchListHints(normal, hint{desc: "run", keys: []key.Binding{k.Open}})
}

func (k keyMap) pickerHints(normal bool) []hint {
	return append(k.searchListHints(normal, hint{desc: "select", keys: []key.Binding{k.Toggle}}), hint{desc: "save", keys: []key.Binding{k.Save}})
}

func (k keyMap) modalHints(fields, choices bool, mode string) []hint {
	list := []hint{{desc: "save", keys: []key.Binding{k.Save}}}
	if fields {
		list = append(list, hint{desc: "next field", keys: []key.Binding{k.NextFocus}})
	}
	list = append(list, k.vimHints(mode)...)
	if choices {
		list = append(list, hint{desc: "pick", keys: []key.Binding{k.Down, k.Up}})
	}
	return list
}
