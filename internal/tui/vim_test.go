package tui

import "testing"

func TestDeleteWord(t *testing.T) {
	tests := []struct {
		name string
		line string
		col  int
		span wordSpan
		want string
		at   int
	}{
		{"inner word", "foo bar", 5, wordInner, "foo ", 4},
		{"inner punctuation", "a.b", 1, wordInner, "ab", 1},
		{"inner blanks", "a  b", 1, wordInner, "ab", 1},
		{"end of word", "abcdef baz", 2, wordEnd, "ab baz", 2},
		{"next word", "foo bar baz", 0, wordNext, "bar baz", 0},
		{"next word from inside a word", "foo  bar", 2, wordNext, "fobar", 2},
		{"next word on the last word", "foo", 0, wordNext, "", 0},
		{"empty line", "", 0, wordInner, "", 0},
		{"column past the end", "ab", 2, wordInner, "ab", 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, at := deleteWord([]rune(tt.line), tt.col, tt.span)
			if string(got) != tt.want || at != tt.at {
				t.Fatalf("deleteWord(%q, %d) = %q, %d, want %q, %d", tt.line, tt.col, string(got), at, tt.want, tt.at)
			}
		})
	}
}

func TestURLFieldChangesAndDeletesWords(t *testing.T) {
	m, _ := pullsModel(t)
	m = pressRun(t, m, "g", "u")
	m = typeText(t, m, "hello world")
	m = pressRun(t, m, "esc", "0", "c", "i", "w")
	if m.modal.mode() != "insert" {
		t.Fatal("ciw should leave the URL field in insert mode")
	}
	m = typeText(t, m, "bye")
	if got := m.modal.fields[0].Value(); got != "bye world" {
		t.Fatalf("ciw should replace the word, value=%q", got)
	}
	m = pressRun(t, m, "esc", "0", "d", "w")
	if got := m.modal.fields[0].Value(); got != "world" || m.modal.mode() != "normal" {
		t.Fatalf("dw should delete the word and the blanks after it, value=%q mode=%q", got, m.modal.mode())
	}
}

func TestCommentEditorChangesAndDeletesWords(t *testing.T) {
	m, _ := loaded(t)
	m, _ = press(t, m, "l")
	m.cursor = rowOf(t, m, rowLine, 1, 3)
	m, _ = press(t, m, "c")
	m = typeText(t, m, "hello world")
	m = pressRun(t, m, "esc", "0", "c", "i", "w")
	m = typeText(t, m, "bye")
	if got := m.editor.area.Value(); got != "bye world" {
		t.Fatalf("ciw should replace the word in the comment, value=%q", got)
	}
	m = pressRun(t, m, "esc", "0", "d", "i", "w")
	if got := m.editor.area.Value(); got != " world" {
		t.Fatalf("diw should delete the word in the comment, value=%q", got)
	}
}

func TestPendingOperatorsOnlyStartInTextBoxes(t *testing.T) {
	k := defaultKeys()
	for _, seq := range []string{"c", "c i", "d", "d i", "g"} {
		if !k.pendingOp(seq) {
			t.Errorf("%q should wait for the rest of a text edit", seq)
		}
	}
	for _, seq := range []string{"c x", "c c", "j", "i"} {
		if k.pendingOp(seq) {
			t.Errorf("%q should not wait for anything", seq)
		}
	}
}
