package tui

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/jfmainville/revunk/internal/review"
)

func TestConfigNameIsKebabCase(t *testing.T) {
	for field, want := range map[string]string{"OpenURL": "open-url", "EditPR": "edit-pr", "NextFocus": "next-focus", "InnerChange": "inner-change"} {
		if got := configName(field); got != want {
			t.Errorf("configName(%q) = %q, want %q", field, got, want)
		}
	}
}

func TestOverrideReplacesKeysAndDisablesOnEmptyList(t *testing.T) {
	k := defaultKeys("space")
	if err := k.override(map[string][]string{"next-focus": {"ctrl+right"}, "quit": {}}); err != nil {
		t.Fatalf("override() error = %v", err)
	}
	if !is("ctrl+right", k.NextFocus) || is("tab", k.NextFocus) {
		t.Fatal("override should replace the default keys")
	}
	if is("q", k.Quit) || k.Quit.Enabled() {
		t.Fatal("an empty list should disable the binding")
	}
}

func TestOverrideReachesOperatorKeys(t *testing.T) {
	k := defaultKeys("space")
	if err := k.override(map[string][]string{"ops.inner-change": {"c p"}}); err != nil {
		t.Fatalf("override() error = %v", err)
	}
	if !is("c p", k.Ops.InnerChange) || is("c i w", k.Ops.InnerChange) {
		t.Fatal("override should reach operator bindings")
	}
}

func TestOverrideRejectsUnknownBinding(t *testing.T) {
	k := defaultKeys("space")
	if err := k.override(map[string][]string{"nope": {"x"}}); err == nil || !strings.Contains(err.Error(), "nope") {
		t.Fatalf("override() error = %v, want unknown binding nope", err)
	}
}

func TestWithKeysAppliesOverridesToModel(t *testing.T) {
	m, err := New(nil, ".", "main").WithKeys(map[string][]string{"quit": {"x"}})
	if err != nil || !is("x", m.keys.Quit) || is("q", m.keys.Quit) {
		t.Fatalf("WithKeys() = %v, quit keys %v", err, m.keys.Quit.Keys())
	}
	if _, err := New(nil, ".", "main").WithKeys(map[string][]string{"nope": {"x"}}); err == nil {
		t.Fatal("WithKeys() should reject an unknown binding")
	}
}

func TestIsMatchesOnlyEnabledBindings(t *testing.T) {
	b := bind("j", "down")
	if !is("j", b) || !is("down", b) || is("k", b) {
		t.Fatal("is should match exactly the bound keys")
	}
	b.SetEnabled(false)
	if is("j", b) {
		t.Fatal("disabled bindings should not match")
	}
}

func TestPrefixAndRest(t *testing.T) {
	k := defaultKeys("space")
	for _, p := range []string{"g", "d", "D", "]", "["} {
		if !k.isPrefix(p) {
			t.Errorf("%q should start a sequence", p)
		}
	}
	for _, p := range []string{"j", "c", "gg", ""} {
		if k.isPrefix(p) {
			t.Errorf("%q should not start a sequence", p)
		}
	}
	if again, ok := rest(k.DeleteAll, "D"); !ok || again != "D" {
		t.Fatalf("rest = %q %v", again, ok)
	}
	if _, ok := rest(k.DeleteAll, "d"); ok {
		t.Fatal("d does not start DD")
	}
	off := bind("x y")
	off.SetEnabled(false)
	if _, ok := rest(off, "x"); ok {
		t.Fatal("disabled sequences should not count as prefixes")
	}
}

func TestHintText(t *testing.T) {
	k := defaultKeys("space")
	pair := hint{keys: []key.Binding{k.Down, k.Up}}
	if got := pair.text(false); got != "j k" {
		t.Fatalf("short = %q", got)
	}
	if got := pair.text(true); got != "j ↓ k ↑" {
		t.Fatalf("all = %q", got)
	}
	for b, want := range map[*key.Binding]string{&k.Top: "gg", &k.Save: "C-s", &k.Open: "⏎", &k.PrevFocus: "S-tab", &k.PageDown: "pgdn"} {
		if got := label(*b); got != want {
			t.Errorf("label = %q, want %q", got, want)
		}
	}
	if got := (hint{label: "click"}).text(true); got != "click" {
		t.Fatalf("explicit label = %q", got)
	}
	if got := (hint{keys: []key.Binding{bind()}}).text(false); got != "" {
		t.Fatalf("unbound = %q", got)
	}
}

func TestEveryBindingIsDocumented(t *testing.T) {
	k := defaultKeys("space")
	documented := map[string]bool{}
	for _, sec := range k.helpSections() {
		for _, h := range sec.hints {
			for _, b := range h.keys {
				documented[strings.Join(b.Keys(), ",")] = true
			}
		}
	}
	for _, list := range [][]hint{k.listHints(), k.mainHints(), k.rangeHints(), k.editorHints("insert")} {
		for _, h := range list {
			for _, b := range h.keys {
				documented[strings.Join(b.Keys(), ",")] = true
			}
		}
	}
	hidden := map[string]bool{"ctrl+c": true, "esc,?,q": true, "shift+tab": true, "backspace": true}
	for _, b := range k.all() {
		if keys := strings.Join(b.Keys(), ","); !documented[keys] && !hidden[keys] {
			t.Errorf("binding %q is missing from the help or hints", keys)
		}
	}
}

func TestReboundKeysDriveBehaviorAndLabels(t *testing.T) {
	m, st := loaded(t)
	m.keys.Down = bind("J")
	m.keys.Help = bind("H1")
	m.keys.DeleteAll = bind("z z")
	m, _ = press(t, m, "l")

	if m, _ = press(t, m, "j"); m.cursor != 0 {
		t.Fatal("the old key should do nothing once rebound")
	}
	if m, _ = press(t, m, "J"); m.cursor != 1 {
		t.Fatalf("the new key should move, cursor=%d", m.cursor)
	}
	out := screen(m)
	if !strings.Contains(out, "H1 help") {
		t.Fatalf("labels should follow the binding:\n%s", out)
	}

	st.comments = []review.Comment{{ID: "a", Path: "README.md", Body: "x"}}
	m.comments = st.comments
	m, _ = press(t, m, "z")
	if !strings.Contains(m.warning, "press z again") {
		t.Fatalf("warning = %q", m.warning)
	}
	m, cmd := press(t, m, "z")
	if m = run(t, m, cmd); len(st.comments) != 0 {
		t.Fatal("the rebound sequence should delete every comment")
	}
	if m, _ = press(t, m, "D", "D"); m.prefix != "" || m.notice != "" {
		t.Fatal("the old sequence should no longer start a prefix")
	}

	m.keys.Newline = bind("ctrl+j")
	m, _ = press(t, m, "J", "c")
	if !m.editing {
		t.Fatal("editor should open")
	}
	m = typeText(t, m, "a")
	m, _ = press(t, m, "enter")
	if m.editor.area.Value() != "a" {
		t.Fatalf("enter should no longer insert a newline: %q", m.editor.area.Value())
	}
	m = update(t, m, tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if m.editor.area.Value() != "a\n" {
		t.Fatalf("the rebound newline key should insert one: %q", m.editor.area.Value())
	}
}

func TestExplorerLabelShowsTheLeader(t *testing.T) {
	if got := label(defaultKeys("space").Explorer); got != "SPC e" {
		t.Fatalf("label = %q", got)
	}
	if got := label(defaultKeys(",").Explorer); got != ",e" {
		t.Fatalf("label = %q", got)
	}
}
