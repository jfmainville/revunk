package tui

import (
	"fmt"
	"image/color"
	"regexp"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"

	"github.com/jfmainville/revunk/internal/review"
)

var (
	colorPrimary    = lipgloss.Color("#ebaf39")
	colorForeground = lipgloss.Color("#e7e0d7")
	colorPanel      = lipgloss.Color("#221a16")
	colorRule       = lipgloss.Color("#41352f")
	colorDim        = lipgloss.Color("#c0ad9f")
	colorOnPrimary  = lipgloss.Color("#201308")
	colorAdd        = lipgloss.Color("#aec76c")
	colorDel        = lipgloss.Color("#f6987b")
	colorThread     = lipgloss.Color("#7dc9b6")
	colorCursorline = lipgloss.Color("#322618")
	colorVisual     = lipgloss.Color("#18443a")
	colorAddBg      = lipgloss.Color("#292519")
	colorDelBg      = lipgloss.Color("#34231d")
	colorError      = lipgloss.Color("#fdae95")
	colorNote       = lipgloss.Color("#8fb8de")

	dimStyle     = lipgloss.NewStyle().Foreground(colorDim)
	primaryStyle = lipgloss.NewStyle().Foreground(colorPrimary)
	errorStyle   = lipgloss.NewStyle().Foreground(colorError)

	syntax = chroma.MustNewStyle("revunk", chroma.StyleEntries{
		chroma.Text:              "#e7e0d7",
		chroma.Keyword:           "#ebaf39",
		chroma.KeywordType:       "#ebaf39",
		chroma.NameBuiltin:       "#ebaf39",
		chroma.NameTag:           "#ebaf39",
		chroma.NameDecorator:     "#7dc9b6",
		chroma.Comment:           "italic #c0ad9f",
		chroma.LiteralString:     "#aec76c",
		chroma.LiteralNumber:     "#c5a3cd",
		chroma.GenericHeading:    "bold #ebaf39",
		chroma.GenericSubheading: "bold #ebaf39",
		chroma.GenericStrong:     "bold #e7e0d7",
		chroma.GenericEmph:       "italic #e7e0d7",
	})
)

const (
	gutterWidth = 9
	lineGutter  = 10
)

type token struct {
	text  string
	style lipgloss.Style
}

var markdownLexer = lexers.Get("markdown")

func lexerFor(path string) chroma.Lexer {
	if lexer := lexers.Match(path); lexer != nil {
		return lexer
	}
	return lexers.Fallback
}

func highlight(lexer chroma.Lexer, lines []review.Line) [][]token {
	texts := make([]string, len(lines))
	for i, l := range lines {
		if l.Kind != review.LineHunk {
			texts[i] = strings.ReplaceAll(l.Text, "\t", "    ")
		}
	}
	plain := func() [][]token {
		out := make([][]token, len(lines))
		for i, t := range texts {
			out[i] = []token{{text: t, style: lipgloss.NewStyle().Foreground(colorForeground)}}
		}
		return out
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, strings.Join(texts, "\n")+"\n")
	if err != nil {
		return plain()
	}
	split := chroma.SplitTokensIntoLines(it.Tokens())
	if len(split) < len(lines) {
		return plain()
	}
	out := make([][]token, len(lines))
	for i := range lines {
		for _, t := range split[i] {
			text := strings.TrimSuffix(t.Value, "\n")
			if text == "" {
				continue
			}
			entry := syntax.Get(t.Type)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(entry.Colour.String()))
			if entry.Italic == chroma.Yes {
				style = style.Italic(true)
			}
			if entry.Bold == chroma.Yes {
				style = style.Bold(true)
			}
			out[i] = append(out[i], token{text: text, style: style})
		}
	}
	return out
}

func fit(s string, width int, bg lipgloss.Style) string {
	s = ansi.Truncate(s, width, "")
	if pad := width - lipgloss.Width(s); pad > 0 {
		s += bg.Render(strings.Repeat(" ", pad))
	}
	return s
}

func number(n int) string {
	if n == 0 {
		return ""
	}
	return fmt.Sprint(n)
}

func wrapTokens(tokens []token, width int) [][]token {
	width = max(width-lineGutter, 1)
	bytes, cells := 0, 0
	for _, t := range tokens {
		bytes += len(t.text)
	}
	if bytes > width {
		for _, t := range tokens {
			cells += ansi.StringWidth(t.text)
		}
	}
	if cells <= width {
		return [][]token{tokens}
	}
	parts := [][]token{nil}
	used := 0
	for _, t := range tokens {
		var b strings.Builder
		for _, c := range t.text {
			w := ansi.StringWidth(string(c))
			if used+w > width && used > 0 {
				if b.Len() > 0 {
					parts[len(parts)-1] = append(parts[len(parts)-1], token{text: b.String(), style: t.style})
					b.Reset()
				}
				parts, used = append(parts, nil), 0
			}
			b.WriteRune(c)
			used += w
		}
		if b.Len() > 0 {
			parts[len(parts)-1] = append(parts[len(parts)-1], token{text: b.String(), style: t.style})
		}
	}
	return parts
}

func renderLine(l review.Line, tokens []token, width, part int, cursor, selected bool) string {
	var bg color.Color
	marker, markerColor := " ", colorDim
	switch l.Kind {
	case review.LineAdded:
		bg, marker, markerColor = colorAddBg, "+", colorAdd
	case review.LineRemoved:
		bg, marker, markerColor = colorDelBg, "-", colorDel
	}
	if selected {
		bg = colorVisual
	}
	if cursor {
		bg = colorCursorline
	}
	base := lipgloss.NewStyle()
	if bg != nil {
		base = base.Background(bg)
	}
	numStyle := base.Foreground(colorDim)
	bar := base.Render(" ")
	if cursor {
		numStyle = base.Foreground(colorPrimary)
		bar = base.Foreground(colorPrimary).Render("▌")
	}
	num := number(l.New)
	if l.Kind == review.LineRemoved {
		num = number(l.Old)
	}
	if part > 0 {
		num, marker = "", " "
	}
	var b strings.Builder
	b.WriteString(bar)
	b.WriteString(numStyle.Render(fmt.Sprintf(" %4s ", num)))
	b.WriteString(base.Foreground(markerColor).Render(marker + "  "))
	for _, t := range wrapTokens(tokens, width)[part] {
		b.WriteString(t.style.Inherit(base).Render(t.text))
	}
	return fit(b.String(), width, base)
}

func renderFileHeader(f review.File, viewed, cursor bool, width int) string {
	base := lipgloss.NewStyle().Background(colorPanel)
	arrow := "▾"
	state := "unseen"
	if viewed {
		arrow, state = "▸", "viewed"
	}
	bar := base.Render(" ")
	if cursor {
		bar = base.Foreground(colorPrimary).Render("▌")
	}
	left := bar + base.Foreground(colorDim).Render(arrow+" ") + statusStyle(f.Status).Inherit(base).Render(statusLetter(f.Status)) + base.Foreground(colorForeground).Bold(true).Render("  "+f.Path)
	right := counts(f.Additions, f.Deletions, base) + base.Foreground(colorDim).Render("   "+state+" ")
	gap := width - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		return fit(left, width, base)
	}
	return left + base.Render(strings.Repeat(" ", gap)) + right
}

func renderHunk(text string, width int, cursor bool) string {
	base := lipgloss.NewStyle()
	bar := " "
	if cursor {
		base = base.Background(colorCursorline)
		bar = base.Foreground(colorPrimary).Render("▌")
	}
	return fit(bar+base.Foreground(colorThread).Render(text), width, base)
}

func counts(additions, deletions int, base lipgloss.Style) string {
	return base.Foreground(colorAdd).Render(fmt.Sprintf("+%d ", additions)) + base.Foreground(colorDel).Render(fmt.Sprintf("-%d", deletions))
}

func statusLetter(s review.FileStatus) string {
	switch s {
	case review.StatusAdded:
		return "A"
	case review.StatusRemoved:
		return "D"
	case review.StatusRenamed:
		return "R"
	}
	return "M"
}

func statusStyle(s review.FileStatus) lipgloss.Style {
	switch s {
	case review.StatusAdded:
		return lipgloss.NewStyle().Foreground(colorAdd)
	case review.StatusRemoved:
		return lipgloss.NewStyle().Foreground(colorDel)
	}
	return lipgloss.NewStyle().Foreground(colorPrimary)
}

var suggestionBlock = regexp.MustCompile("(?s)```suggestion[^\n]*\n(.*?)\n?```")

func expandSuggestions(body string, original []string) string {
	return suggestionBlock.ReplaceAllStringFunc(body, func(block string) string {
		suggested := suggestionBlock.FindStringSubmatch(block)[1]
		var b strings.Builder
		b.WriteString("**Suggested change**\n\n```diff\n")
		for _, l := range original {
			b.WriteString("- " + l + "\n")
		}
		if suggested != "" {
			for l := range strings.SplitSeq(suggested, "\n") {
				b.WriteString("+ " + l + "\n")
			}
		}
		b.WriteString("```")
		return b.String()
	})
}

func lineLabel(c review.Comment) string {
	switch {
	case c.FileLevel():
		return "file"
	case c.Start == c.End:
		return fmt.Sprintf("L%d", c.End)
	}
	return fmt.Sprintf("L%d-%d", c.Start, c.End)
}

func renderMarkdown(body string, width int) []string {
	r, err := glamour.NewTermRenderer(glamour.WithStandardStyle("dark"), glamour.WithWordWrap(width))
	if err == nil {
		if out, err := r.Render(body); err == nil {
			body = out
		}
	}
	lines := strings.Split(strings.TrimRight(body, "\n "), "\n")
	for len(lines) > 0 && strings.TrimSpace(ansi.Strip(lines[0])) == "" {
		lines = lines[1:]
	}
	return lines
}

func commentWidth(width int) int {
	return width - gutterWidth + 1
}

func kindColor(kind string) color.Color {
	switch kind {
	case "note":
		return colorNote
	case "suggestion":
		return colorAdd
	}
	return colorPrimary
}

func commentBox(kind, right string, lines []string, width int) []string {
	padded := make([]string, len(lines))
	for i, l := range lines {
		padded[i] = " " + l
	}
	indent := strings.Repeat(" ", gutterWidth-2)
	out := strings.Split(box("◆ "+kind, right, padded, commentWidth(width), len(lines)+2, kindColor(kind)), "\n")
	for i, l := range out {
		out[i] = indent + l
	}
	return out
}

func renderComment(c review.Comment, original []string, width int) []string {
	wrap := max(commentWidth(width)-4, 10)
	lines := renderMarkdown(expandSuggestions(c.Body, original), wrap)
	reply := "↳ agent reply"
	if c.Author != "" {
		reply = "↳ reply"
	}
	for _, r := range c.Replies {
		label := reply
		if body, ok := strings.CutPrefix(r, review.YourReply); ok && c.Author == "" {
			label, r = "↳ your reply", body
		}
		lines = append(lines, "", lipgloss.NewStyle().Foreground(colorThread).Render(label))
		lines = append(lines, renderMarkdown(r, wrap)...)
	}
	return commentBox(c.Kind, lineLabel(c)+" · "+commentState(c), lines, width)
}

func commentState(c review.Comment) string {
	switch {
	case c.Resolved:
		return "resolved"
	case c.Author != "" && !c.Sent:
		return "pending"
	case c.Author != "":
		return "@" + c.Author
	case c.Answered():
		return "replied"
	case c.Sent:
		return "sent"
	}
	return "draft"
}
