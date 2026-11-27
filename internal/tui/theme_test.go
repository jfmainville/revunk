package tui

import (
	"image/color"
	"testing"

	"charm.land/glamour/v2/styles"
)

func TestWithThemeAppliesPalette(t *testing.T) {
	t.Cleanup(func() { applyTheme(themes["catppuccin"]) })
	_, err := Model{}.WithTheme("dracula")
	if err != nil {
		t.Fatalf("WithTheme() error = %v", err)
	}
	if colorPrimary != color.Color(themes["dracula"].accent) {
		t.Fatalf("accent = %v, want dracula's accent", colorPrimary)
	}
	if colorAdd != color.Color(themes["dracula"].green) || colorNote != color.Color(themes["dracula"].teal) {
		t.Fatal("diff and comment colors should follow the theme")
	}
}

func TestWithThemeRejectsUnknownName(t *testing.T) {
	for _, name := range []string{"", "nope", "terminal"} {
		if _, err := (Model{}).WithTheme(name); err == nil {
			t.Errorf("WithTheme(%q) should fail", name)
		}
	}
}

func TestEveryThemeKeepsNotesApartFromComments(t *testing.T) {
	t.Cleanup(func() { applyTheme(themes["catppuccin"]) })
	for name := range themes {
		if _, err := (Model{}).WithTheme(name); err != nil {
			t.Fatalf("WithTheme(%q) error = %v", name, err)
		}
		if kindColor("note") == kindColor("comment") {
			t.Errorf("%s: note and comment share a color", name)
		}
	}
}

func TestRgbUnpacksHex(t *testing.T) {
	if got := rgb(0x89b4fa); got != (color.RGBA{R: 0x89, G: 0xb4, B: 0xfa, A: 255}) {
		t.Fatalf("rgb() = %v", got)
	}
}

func TestHexFormatsLowercase(t *testing.T) {
	if got := hex(color.RGBA{R: 0x0a, G: 0x0b, B: 0xff, A: 255}); got != "#0a0bff" {
		t.Fatalf("hex() = %q", got)
	}
}

func TestGlamourStyleFollowsPalette(t *testing.T) {
	p := themes["dracula"]
	cfg := glamourStyle(p)
	if *cfg.Heading.Color != hex(p.accent) || *cfg.Document.Color != hex(p.text) {
		t.Fatalf("headings and text should use the palette, got heading %q, text %q", *cfg.Heading.Color, *cfg.Document.Color)
	}
	if *cfg.CodeBlock.Chroma.Keyword.Color != hex(p.accent) {
		t.Fatalf("code keywords should use the accent, got %q", *cfg.CodeBlock.Chroma.Keyword.Color)
	}
}

func TestGlamourStyleLeavesSharedDarkStyleAlone(t *testing.T) {
	before := *styles.DarkStyleConfig.CodeBlock.Chroma.Keyword.Color
	glamourStyle(themes["dracula"])
	if *styles.DarkStyleConfig.CodeBlock.Chroma.Keyword.Color != before {
		t.Fatal("recoloring should not change glamour's shared dark style")
	}
}

func TestMixBlendsBetweenColors(t *testing.T) {
	black := color.RGBA{A: 255}
	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	cases := map[string]struct {
		t    float64
		want color.RGBA
	}{
		"start":    {0, black},
		"end":      {1, white},
		"midpoint": {0.5, color.RGBA{R: 128, G: 128, B: 128, A: 255}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if got := mix(black, white, c.t); got != c.want {
				t.Fatalf("mix(%v) = %v, want %v", c.t, got, c.want)
			}
		})
	}
}
