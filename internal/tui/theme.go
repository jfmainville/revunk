package tui

import (
	"fmt"
	"image/color"
	"math"

	"charm.land/glamour/v2/ansi"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/alecthomas/chroma/v2"
)

type palette struct {
	accent, panel, activeRow, selection, surfaceDim, overlay, text, subtext, mauve, green, red, teal, peach color.RGBA
}

var (
	colorPrimary    color.Color
	colorForeground color.Color
	colorPanel      color.Color
	colorRule       color.Color
	colorDim        color.Color
	colorOnPrimary  color.Color
	colorAdd        color.Color
	colorDel        color.Color
	colorThread     color.Color
	colorCursorline color.Color
	colorVisual     color.Color
	colorAddBg      color.Color
	colorDelBg      color.Color
	colorAddWord    color.Color
	colorDelWord    color.Color
	colorError      color.Color
	colorNote       color.Color
	colorIssue      color.Color
	colorMerged     color.Color

	dimStyle     lipgloss.Style
	primaryStyle lipgloss.Style
	errorStyle   lipgloss.Style

	syntax         *chroma.Style
	markdownConfig ansi.StyleConfig
)

func init() {
	applyTheme(themes["catppuccin"])
}

func rgb(v uint32) color.RGBA {
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 255}
}

func (m Model) WithTheme(name string) (Model, error) {
	p, ok := themes[name]
	if !ok {
		return m, fmt.Errorf("unknown theme %q", name)
	}
	applyTheme(p)
	return m, nil
}

func applyTheme(p palette) {
	colorPrimary = p.accent
	colorForeground = p.text
	colorPanel = p.panel
	colorRule = p.surfaceDim
	colorDim = p.overlay
	colorOnPrimary = p.panel
	colorAdd = p.green
	colorDel = p.red
	colorThread = p.teal
	colorCursorline = p.activeRow
	colorVisual = p.selection
	colorAddBg = mix(p.panel, p.green, 0.15)
	colorDelBg = mix(p.panel, p.red, 0.15)
	colorAddWord = mix(p.panel, p.green, 0.35)
	colorDelWord = mix(p.panel, p.red, 0.35)
	colorError = p.peach
	colorNote = p.teal
	colorIssue = p.red
	colorMerged = p.mauve

	dimStyle = lipgloss.NewStyle().Foreground(colorDim)
	primaryStyle = lipgloss.NewStyle().Foreground(colorPrimary)
	errorStyle = lipgloss.NewStyle().Foreground(colorError)

	syntax = chroma.MustNewStyle("revunk", chroma.StyleEntries{
		chroma.Text:              hex(p.text),
		chroma.Keyword:           hex(p.accent),
		chroma.KeywordType:       hex(p.accent),
		chroma.NameBuiltin:       hex(p.accent),
		chroma.NameTag:           hex(p.accent),
		chroma.NameDecorator:     hex(p.teal),
		chroma.Comment:           "italic " + hex(p.overlay),
		chroma.LiteralString:     hex(p.green),
		chroma.LiteralNumber:     hex(p.mauve),
		chroma.GenericHeading:    "bold " + hex(p.accent),
		chroma.GenericSubheading: "bold " + hex(p.accent),
		chroma.GenericStrong:     "bold " + hex(p.text),
		chroma.GenericEmph:       "italic " + hex(p.text),
	})
	markdownConfig = glamourStyle(p)
}

func glamourStyle(p palette) ansi.StyleConfig {
	cfg := styles.DarkStyleConfig
	fg := func(c color.RGBA) *string {
		s := hex(c)
		return &s
	}
	cfg.Document.Color = fg(p.text)
	cfg.Heading.Color = fg(p.accent)
	cfg.H1.Color = fg(p.panel)
	cfg.H1.BackgroundColor = fg(p.accent)
	cfg.H6.Color = fg(p.subtext)
	cfg.BlockQuote.Color = fg(p.overlay)
	cfg.Link.Color = fg(p.teal)
	cfg.LinkText.Color = fg(p.accent)
	cfg.Code.Color = fg(p.peach)
	cfg.Code.BackgroundColor = fg(p.activeRow)
	cfg.HorizontalRule.Color = fg(p.overlay)

	// The dark style shares its Chroma pointer, so copy it before recoloring.
	codeTheme := *cfg.CodeBlock.Chroma
	codeTheme.Text.Color = fg(p.text)
	codeTheme.Error.Color = fg(p.red)
	codeTheme.Comment.Color = fg(p.overlay)
	codeTheme.Keyword.Color = fg(p.accent)
	codeTheme.KeywordType.Color = fg(p.accent)
	codeTheme.NameBuiltin.Color = fg(p.accent)
	codeTheme.NameDecorator.Color = fg(p.teal)
	codeTheme.LiteralString.Color = fg(p.green)
	codeTheme.LiteralNumber.Color = fg(p.mauve)
	codeTheme.NameFunction.Color = fg(p.teal)
	codeTheme.Name.Color = fg(p.text)
	codeTheme.Punctuation.Color = fg(p.text)
	cfg.CodeBlock.Chroma = &codeTheme
	return cfg
}

func hex(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func mix(a, b color.RGBA, t float64) color.RGBA {
	lerp := func(x, y uint8) uint8 {
		return uint8(math.Round(float64(x) + (float64(y)-float64(x))*t))
	}
	return color.RGBA{R: lerp(a.R, b.R), G: lerp(a.G, b.G), B: lerp(a.B, b.B), A: 255}
}

// terminal is left out because its ANSI colors have no fixed RGB values to blend.
var themes = map[string]palette{
	"catppuccin": {
		accent:     rgb(0x89b4fa),
		panel:      rgb(0x181825),
		activeRow:  rgb(0x1e1e2e),
		selection:  rgb(0x313244),
		surfaceDim: rgb(0x1e1e2e),
		overlay:    rgb(0x6c7086),
		text:       rgb(0xcdd6f4),
		subtext:    rgb(0xa6adc8),
		mauve:      rgb(0xcba6f7),
		green:      rgb(0xa6e3a1),
		red:        rgb(0xf38ba8),
		teal:       rgb(0x94e2d5),
		peach:      rgb(0xfab387),
	},
	"catppuccin-latte": {
		accent:     rgb(0x1e66f5),
		panel:      rgb(0xeff1f5),
		activeRow:  rgb(0xe6e9ef),
		selection:  rgb(0xbdd0f5),
		surfaceDim: rgb(0xe6e9ef),
		overlay:    rgb(0x9ca0b0),
		text:       rgb(0x4c4f69),
		subtext:    rgb(0x6c6f85),
		mauve:      rgb(0x8839ef),
		green:      rgb(0x40a02b),
		red:        rgb(0xd20f39),
		teal:       rgb(0x179299),
		peach:      rgb(0xfe640b),
	},
	"tokyo-night": {
		accent:     rgb(0x7aa2f7),
		panel:      rgb(0x1a1b26),
		activeRow:  rgb(0x232636),
		selection:  rgb(0x2d3650),
		surfaceDim: rgb(0x1a1b26),
		overlay:    rgb(0x565f89),
		text:       rgb(0xc0caf5),
		subtext:    rgb(0xa9b1d6),
		mauve:      rgb(0xbb9af7),
		green:      rgb(0x9ece6a),
		red:        rgb(0xf7768e),
		teal:       rgb(0x7dcfff),
		peach:      rgb(0xff9e64),
	},
	"tokyo-night-day": {
		accent:     rgb(0x2e7de9),
		panel:      rgb(0xe1e2e7),
		activeRow:  rgb(0xd2d3da),
		selection:  rgb(0xb6cae7),
		surfaceDim: rgb(0xd2d3da),
		overlay:    rgb(0x8990b3),
		text:       rgb(0x3760bf),
		subtext:    rgb(0x6172b0),
		mauve:      rgb(0x7847bd),
		green:      rgb(0x587539),
		red:        rgb(0xf52a65),
		teal:       rgb(0x118c74),
		peach:      rgb(0xb15c00),
	},
	"dracula": {
		accent:     rgb(0xbd93f9),
		panel:      rgb(0x282a36),
		activeRow:  rgb(0x373c52),
		selection:  rgb(0x463f5d),
		surfaceDim: rgb(0x282a36),
		overlay:    rgb(0x6272a4),
		text:       rgb(0xf8f8f2),
		subtext:    rgb(0xd2d2dc),
		mauve:      rgb(0xff79c6),
		green:      rgb(0x50fa7b),
		red:        rgb(0xff5555),
		teal:       rgb(0x8be9fd),
		peach:      rgb(0xffb86c),
	},
	"nord": {
		accent:     rgb(0x88c0d0),
		panel:      rgb(0x2e3440),
		activeRow:  rgb(0x434c5e),
		selection:  rgb(0x40505d),
		surfaceDim: rgb(0x2e3440),
		overlay:    rgb(0x4c566a),
		text:       rgb(0xeceff4),
		subtext:    rgb(0xd8dee9),
		mauve:      rgb(0xb48ead),
		green:      rgb(0xa3be8c),
		red:        rgb(0xbf616a),
		teal:       rgb(0x8fbcbb),
		peach:      rgb(0xd08770),
	},
	"gruvbox": {
		accent:     rgb(0xd79921),
		panel:      rgb(0x282828),
		activeRow:  rgb(0x323130),
		selection:  rgb(0x4b3f27),
		surfaceDim: rgb(0x282828),
		overlay:    rgb(0x928374),
		text:       rgb(0xebdbb2),
		subtext:    rgb(0xd5c4a1),
		mauve:      rgb(0xd3869b),
		green:      rgb(0xb8bb26),
		red:        rgb(0xfb4934),
		teal:       rgb(0x8ec07c),
		peach:      rgb(0xfe8019),
	},
	"gruvbox-light": {
		accent:     rgb(0x076678),
		panel:      rgb(0xfbf1c7),
		activeRow:  rgb(0xf2e5bc),
		selection:  rgb(0xebdbb2),
		surfaceDim: rgb(0xf2e5bc),
		overlay:    rgb(0x928374),
		text:       rgb(0x3c3836),
		subtext:    rgb(0x504945),
		mauve:      rgb(0x8f3f71),
		green:      rgb(0x79740e),
		red:        rgb(0x9d0006),
		teal:       rgb(0x427b58),
		peach:      rgb(0xaf3a03),
	},
	"one-dark": {
		accent:     rgb(0x61afef),
		panel:      rgb(0x282c34),
		activeRow:  rgb(0x313640),
		selection:  rgb(0x334659),
		surfaceDim: rgb(0x282c34),
		overlay:    rgb(0x5c6370),
		text:       rgb(0xabb2bf),
		subtext:    rgb(0x969ca8),
		mauve:      rgb(0xc678dd),
		green:      rgb(0x98c379),
		red:        rgb(0xe06c75),
		teal:       rgb(0x56b6c2),
		peach:      rgb(0xd19a66),
	},
	"one-light": {
		accent:     rgb(0x4078f2),
		panel:      rgb(0xfafafa),
		activeRow:  rgb(0xd8dbe2),
		selection:  rgb(0xcddbf8),
		surfaceDim: rgb(0xf5f5f6),
		overlay:    rgb(0xa0a1a7),
		text:       rgb(0x383a42),
		subtext:    rgb(0x686b77),
		mauve:      rgb(0xa626a4),
		green:      rgb(0x50a14f),
		red:        rgb(0xe45649),
		teal:       rgb(0x0184bc),
		peach:      rgb(0x986801),
	},
	"solarized": {
		accent:     rgb(0x268bd2),
		panel:      rgb(0x002b36),
		activeRow:  rgb(0x164b57),
		selection:  rgb(0x083e55),
		surfaceDim: rgb(0x002b36),
		overlay:    rgb(0x586e75),
		text:       rgb(0x93a1a1),
		subtext:    rgb(0x839496),
		mauve:      rgb(0xd33682),
		green:      rgb(0x859900),
		red:        rgb(0xdc322f),
		teal:       rgb(0x2aa198),
		peach:      rgb(0xcb4b16),
	},
	"solarized-light": {
		accent:     rgb(0x268bd2),
		panel:      rgb(0xfdf6e3),
		activeRow:  rgb(0xeee8d5),
		selection:  rgb(0xc9dcdf),
		surfaceDim: rgb(0xeee8d5),
		overlay:    rgb(0x93a1a1),
		text:       rgb(0x657b83),
		subtext:    rgb(0x839496),
		mauve:      rgb(0xd33682),
		green:      rgb(0x859900),
		red:        rgb(0xdc322f),
		teal:       rgb(0x2aa198),
		peach:      rgb(0xcb4b16),
	},
	"kanagawa": {
		accent:     rgb(0x7e9cd8),
		panel:      rgb(0x1f1f28),
		activeRow:  rgb(0x363646),
		selection:  rgb(0x32384b),
		surfaceDim: rgb(0x1f1f28),
		overlay:    rgb(0x727169),
		text:       rgb(0xdcd7ba),
		subtext:    rgb(0xc8c3aa),
		mauve:      rgb(0x957fb8),
		green:      rgb(0x76946a),
		red:        rgb(0xc34043),
		teal:       rgb(0x7fb4ca),
		peach:      rgb(0xffa066),
	},
	"kanagawa-lotus": {
		accent:     rgb(0x4d699b),
		panel:      rgb(0xf2ecbc),
		activeRow:  rgb(0xd5cea3),
		selection:  rgb(0xdcd5ac),
		surfaceDim: rgb(0xd5cea3),
		overlay:    rgb(0xa09cac),
		text:       rgb(0x545464),
		subtext:    rgb(0x43436c),
		mauve:      rgb(0x624c83),
		green:      rgb(0x6f894e),
		red:        rgb(0xc84053),
		teal:       rgb(0x4e8ca2),
		peach:      rgb(0xcc6d00),
	},
	"rose-pine": {
		accent:     rgb(0xc4a7e7),
		panel:      rgb(0x191724),
		activeRow:  rgb(0x26233a),
		selection:  rgb(0x3b344b),
		surfaceDim: rgb(0x26233a),
		overlay:    rgb(0x6e6a86),
		text:       rgb(0xe0def4),
		subtext:    rgb(0xc8c5dc),
		mauve:      rgb(0xc4a7e7),
		green:      rgb(0x31748f),
		red:        rgb(0xeb6f92),
		teal:       rgb(0x9ccfd8),
		peach:      rgb(0xea9a97),
	},
	"rose-pine-dawn": {
		accent:     rgb(0x907aa9),
		panel:      rgb(0xfaf4ed),
		activeRow:  rgb(0xe3d9cf),
		selection:  rgb(0xf2e9e1),
		surfaceDim: rgb(0xf2e9e1),
		overlay:    rgb(0x9893a5),
		text:       rgb(0x464261),
		subtext:    rgb(0x797593),
		mauve:      rgb(0x907aa9),
		green:      rgb(0x286983),
		red:        rgb(0xb4637a),
		teal:       rgb(0x56949f),
		peach:      rgb(0xd7827e),
	},
	"vesper": {
		accent:     rgb(0xffc799),
		panel:      rgb(0x1a1a1a),
		activeRow:  rgb(0x101010),
		selection:  rgb(0x232323),
		surfaceDim: rgb(0x101010),
		overlay:    rgb(0x5c5c5c),
		text:       rgb(0xffffff),
		subtext:    rgb(0xa0a0a0),
		mauve:      rgb(0xffd1a8),
		green:      rgb(0x99ffe4),
		red:        rgb(0xff8080),
		teal:       rgb(0x66ddcc),
		peach:      rgb(0xffc799),
	},
}
