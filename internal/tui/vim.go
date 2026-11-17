package tui

import (
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/jfmainville/revunk/internal/review"
)

type vimState struct {
	normal  bool
	pending string
}

func (v vimState) mode() string {
	if v.normal {
		return "normal"
	}
	return "insert"
}

func (v *vimState) update(a *textarea.Model, msg tea.KeyPressMsg, k keyMap) tea.Cmd {
	switch key := msg.String(); {
	case v.normal:
		v.normalKey(a, key, k)
		*a, _ = a.Update(nil)
		return nil
	case is(key, k.NormalMode):
		v.normal, v.pending = true, ""
		return nil
	}
	return v.insertKey(a, msg, k)
}

func (v *vimState) insertKey(a *textarea.Model, msg tea.KeyPressMsg, k keyMap) tea.Cmd {
	key := msg.String()
	if is(key, k.LeaveInsert) && v.pending == key {
		sendKey(a, tea.KeyBackspace, 0)
		v.normal, v.pending = true, ""
		return nil
	}
	v.pending = ""
	if is(key, k.LeaveInsert) {
		v.pending = key
	}
	var cmd tea.Cmd
	*a, cmd = a.Update(msg)
	return cmd
}

func sendKey(a *textarea.Model, code rune, mod tea.KeyMod) {
	*a, _ = a.Update(tea.KeyPressMsg{Code: code, Mod: mod})
}

func (v *vimState) normalKey(a *textarea.Model, key string, k keyMap) {
	seq := key
	if v.pending != "" {
		seq, v.pending = v.pending+" "+key, ""
	}
	lines := strings.Split(a.Value(), "\n")
	switch {
	case is(seq, k.Top):
		a.MoveToBegin()
	case is(seq, k.Delete):
		row := a.Line()
		lines = slices.Delete(lines, row, row+1)
		a.SetValue(strings.Join(lines, "\n"))
		a.MoveToBegin()
		for a.Line() < min(row, len(lines)-1) {
			a.CursorDown()
		}
	case seq != key:
	case is(key, k.Down):
		a.CursorDown()
	case is(key, k.Up):
		a.CursorUp()
	case is(key, k.Bottom):
		a.MoveToEnd()
	case is(key, k.HalfDown):
		for range max(a.Height()/2, 1) {
			a.CursorDown()
		}
	case is(key, k.HalfUp):
		for range max(a.Height()/2, 1) {
			a.CursorUp()
		}
	case is(key, k.CharLeft):
		if a.Column() > 0 {
			sendKey(a, tea.KeyLeft, 0)
		}
	case is(key, k.CharRight):
		if a.Column() < len([]rune(lines[a.Line()])) {
			sendKey(a, tea.KeyRight, 0)
		}
	case is(key, k.WordNext):
		line, i := []rune(lines[a.Line()]), a.Column()
		for i < len(line) && line[i] != ' ' {
			i++
		}
		for i < len(line) && line[i] == ' ' {
			i++
		}
		if i == len(line) && a.Line() < len(lines)-1 {
			a.CursorDown()
			a.CursorStart()
		} else {
			a.SetCursorColumn(i)
		}
	case is(key, k.WordPrev):
		line, i := []rune(lines[a.Line()]), a.Column()
		for i > 0 && line[i-1] == ' ' {
			i--
		}
		for i > 0 && line[i-1] != ' ' {
			i--
		}
		if a.Column() == 0 && a.Line() > 0 {
			a.CursorUp()
			a.CursorEnd()
		} else {
			a.SetCursorColumn(i)
		}
	case is(key, k.LineStart):
		a.CursorStart()
	case is(key, k.LineEnd):
		a.CursorEnd()
	case is(key, k.DeleteUnder):
		if a.Column() < len([]rune(lines[a.Line()])) {
			sendKey(a, tea.KeyDelete, 0)
		}
	case is(key, k.Insert):
		v.normal = false
	case is(key, k.Append):
		if a.Column() < len([]rune(lines[a.Line()])) {
			sendKey(a, tea.KeyRight, 0)
		}
		v.normal = false
	case is(key, k.InsertStart):
		a.CursorStart()
		v.normal = false
	case is(key, k.AppendEnd):
		a.CursorEnd()
		v.normal = false
	case is(key, k.OpenBelow):
		a.CursorEnd()
		a.InsertRune('\n')
		v.normal = false
	case is(key, k.OpenAbove):
		a.CursorStart()
		a.InsertRune('\n')
		a.CursorUp()
		v.normal = false
	default:
		if _, ok := rest(k.Top, key); ok {
			v.pending = key
		} else if _, ok := rest(k.Delete, key); ok {
			v.pending = key
		}
	}
}

type markdownEntry struct {
	tokens   [][]token
	rendered []string
}

var markdownCache struct {
	sync.Mutex
	entries map[string]markdownEntry
}

func markdownLines(plain []string) ([][]token, []string) {
	text := strings.Join(plain, "\n")
	c := &markdownCache
	c.Lock()
	defer c.Unlock()
	if e, ok := c.entries[text]; ok {
		return e.tokens, e.rendered
	}
	if len(c.entries) >= 4 {
		c.entries = nil
	}
	if c.entries == nil {
		c.entries = map[string]markdownEntry{}
	}
	lines := make([]review.Line, len(plain))
	for i, l := range plain {
		lines[i] = review.Line{Text: l}
	}
	e := markdownEntry{tokens: highlight(markdownLexer, lines), rendered: make([]string, len(plain))}
	for i, tokens := range e.tokens {
		var b strings.Builder
		for _, t := range tokens {
			b.WriteString(t.style.Render(t.text))
		}
		e.rendered[i] = b.String()
	}
	c.entries[text] = e
	return e.tokens, e.rendered
}

func markdownView(a textarea.Model, insert bool) string {
	if a.Value() == "" {
		return a.View()
	}
	a.SetVirtualCursor(false)
	cursor := a.Cursor()
	tokens, rendered := markdownLines(strings.Split(ansi.Strip(a.View()), "\n"))
	out := slices.Clone(rendered)
	if cursor == nil || cursor.Y < 0 || cursor.Y >= len(out) {
		return strings.Join(out, "\n")
	}
	col := cursor.X
	var b strings.Builder
	x := 0
	for _, t := range tokens[cursor.Y] {
		w := ansi.StringWidth(t.text)
		if col < x || col >= x+w {
			b.WriteString(t.style.Render(t.text))
			x += w
			continue
		}
		if before := ansi.Truncate(t.text, col-x, ""); before != "" {
			b.WriteString(t.style.Render(before))
		}
		if insert {
			b.WriteString(t.style.Render(mark(ansi.Truncate(ansi.TruncateLeft(t.text, col-x, ""), 1, ""))))
		} else {
			b.WriteString(t.style.Reverse(true).Render(ansi.Truncate(ansi.TruncateLeft(t.text, col-x, ""), 1, "")))
		}
		if after := ansi.TruncateLeft(t.text, col-x+1, ""); after != "" {
			b.WriteString(t.style.Render(after))
		}
		x += w
	}
	switch {
	case col >= x && insert:
		b.WriteString(strings.Repeat(" ", col-x) + mark(" "))
	case col >= x:
		b.WriteString(strings.Repeat(" ", col-x) + lipgloss.NewStyle().Reverse(true).Render(" "))
	}
	out[cursor.Y] = b.String()
	return strings.Join(out, "\n")
}

const cursorBase = 0xF0000

func mark(cell string) string {
	r := []rune(cell)
	if len(r) != 1 || r[0] >= 0xFFFE {
		return string(rune(cursorBase + ' '))
	}
	return string(cursorBase + r[0])
}

func barCursor(screen string) (string, *tea.Cursor) {
	i := strings.IndexFunc(screen, func(r rune) bool { return r >= cursorBase && r < cursorBase+0xFFFE })
	if i < 0 {
		return screen, nil
	}
	r, size := utf8.DecodeRuneInString(screen[i:])
	before := screen[:i]
	c := tea.NewCursor(ansi.StringWidth(before[strings.LastIndex(before, "\n")+1:]), strings.Count(before, "\n"))
	c.Shape = tea.CursorBar
	return before + string(r-cursorBase) + screen[i+size:], c
}

func fieldView(f textinput.Model) string {
	view := f.View()
	c := f.Cursor()
	if c == nil {
		return view
	}
	cell := ansi.Truncate(ansi.Strip(ansi.TruncateLeft(view, c.X, "")), 1, "")
	return ansi.Truncate(view, c.X, "") + mark(cell) + ansi.TruncateLeft(view, c.X+1, "")
}
