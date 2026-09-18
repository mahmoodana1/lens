package ui

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
)

func TestBlend_MixesTowardsTheBackground(t *testing.T) {
	if got := blend("#ffffff", "#000000", 0); got != "#000000" {
		t.Errorf("t=0 should be all background, got %s", got)
	}
	if got := blend("#ffffff", "#000000", 1); got != "#ffffff" {
		t.Errorf("t=1 should be all foreground, got %s", got)
	}
	if got := blend("#ffffff", "#000000", 0.5); got != "#808080" {
		t.Errorf("half and half = %s, want #808080", got)
	}
}

// A wash has to survive the resets that the syntax colouring emits mid-line, or
// the tint stops at the first token boundary.
func TestWithBackground_ReassertsAfterEveryReset(t *testing.T) {
	line := "\x1b[38;2;1;2;3mfoo\x1b[0m\x1b[38;2;4;5;6mbar\x1b[0m"

	got := paintBackground(line, "#222436")

	set := bgEscape("#222436")
	if !strings.HasPrefix(got, set) {
		t.Error("the wash should open the line")
	}
	if n := strings.Count(got, set); n != 3 {
		t.Errorf("the wash appears %d times, want 3 (open, plus after each reset)", n)
	}
}

// The cursor band has to beat the added/removed wash already on the line.
func TestStripBackground_RemovesOnlyBackgrounds(t *testing.T) {
	line := bgEscape("#112233") + "\x1b[38;2;1;2;3mcode\x1b[0m"

	got := stripBackground(line)

	if strings.Contains(got, "48;2;") {
		t.Errorf("background survived: %q", got)
	}
	if !strings.Contains(got, "38;2;1;2;3") {
		t.Errorf("foreground was lost: %q", got)
	}
}

// The diff wears the same colours as the editor beside it.
func TestSyntaxStyle_UsesTheEditorPalette(t *testing.T) {
	st := syntaxStyle()

	for _, c := range []struct {
		token chroma.TokenType
		want  string
	}{
		{chroma.Keyword, tnMagenta},
		{chroma.LiteralString, tnGreen},
		{chroma.LiteralNumber, tnOrange},
		{chroma.Comment, tnComment},
		{chroma.NameFunction, tnBlue},
	} {
		if got := st.Get(c.token).Colour.String(); !strings.EqualFold(got, c.want) {
			t.Errorf("%v = %s, want %s", c.token, got, c.want)
		}
	}
}

// The cursor band beats the added and removed washes, and there is no band at
// all until the diff has focus.
func TestDiffBackground_CursorBandBeatsTheDiffWash(t *testing.T) {
	for _, c := range []struct {
		name     string
		kind     LineKind
		onCursor bool
		focused  bool
		want     string
	}{
		{"unchanged line", LinePlain, false, false, ""},
		{"added line", LineAdd, false, false, bgAdd},
		{"removed line", LineDel, false, false, bgDel},
		{"cursor, diff unfocused", LinePlain, true, false, ""},
		{"cursor, diff focused", LinePlain, true, true, bgCursor},
		{"cursor on an added line", LineAdd, true, true, bgCursor},
	} {
		if got := diffBackground(c.kind, c.onCursor, c.focused); got != c.want {
			t.Errorf("%s = %q, want %q", c.name, got, c.want)
		}
	}
}
