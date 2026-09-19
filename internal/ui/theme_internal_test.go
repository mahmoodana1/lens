package ui

import (
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
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

// The diff wears the same colours as the editor beside it: catppuccin mocha,
// mapped the way catppuccin's own treesitter groups map them.
func TestSyntaxStyle_UsesTheEditorPalette(t *testing.T) {
	st := syntaxStyle()

	for _, c := range []struct {
		token chroma.TokenType
		want  string
	}{
		{chroma.Keyword, ctpMauve},            // Keyword
		{chroma.KeywordConstant, ctpPeach},    // Boolean / Constant
		{chroma.LiteralString, ctpGreen},      // String
		{chroma.LiteralStringEscape, ctpPink}, // @string.escape
		{chroma.LiteralNumber, ctpPeach},      // Number
		{chroma.Comment, ctpOverlay2},         // Comment
		{chroma.NameFunction, ctpBlue},        // Function
		{chroma.NameClass, ctpYellow},         // Type / Structure
		{chroma.NameProperty, ctpLavender},    // @property
		{chroma.NameBuiltin, ctpPeach},        // @function.builtin
		{chroma.Operator, ctpSky},             // Operator
		{chroma.Punctuation, ctpOverlay2},     // Delimiter
		{chroma.NameLabel, ctpSapphire},       // Label
		{chroma.Error, ctpRed},                // Error
	} {
		if got := st.Get(c.token).Colour.String(); !strings.EqualFold(got, c.want) {
			t.Errorf("%v = %s, want %s", c.token, got, c.want)
		}
	}
}

// Catppuccin italicises comments, and that is half of what makes code look
// like code. The style carries it; rendering has to keep it.
func TestSyntaxStyle_ComentsAreItalic(t *testing.T) {
	if got := syntaxStyle().Get(chroma.Comment).Italic; got != chroma.Yes {
		t.Errorf("comment italic = %v, want yes", got)
	}
}

// Rendering a token used to keep only its colour, so every italic and bold in
// the style was thrown away on the way to the screen. (Asserting on escapes is
// no good here: lipgloss renders plain when nothing is attached to a terminal.)
func TestEntryStyle_KeepsMoreThanTheColour(t *testing.T) {
	st, ok := entryStyle(syntaxStyle().Get(chroma.Comment))
	if !ok {
		t.Fatal("a comment has a style; entryStyle reported none")
	}
	if !st.GetItalic() {
		t.Error("the comment's italics were dropped")
	}
	if got := st.GetForeground(); got != lipgloss.Color(ctpOverlay2) {
		t.Errorf("comment colour = %v, want %s", got, ctpOverlay2)
	}

	// A token the style says nothing about must be left exactly as it came.
	if _, ok := entryStyle(chroma.StyleEntry{}); ok {
		t.Error("an empty entry should not dress the token at all")
	}

	bold, _ := entryStyle(chroma.StyleEntry{Bold: chroma.Yes})
	if !bold.GetBold() {
		t.Error("bold was dropped")
	}
}

// The washes behind changed lines are catppuccin's own DiffAdd and DiffDelete,
// which are the flavour's green and red mixed 18%% into the base.
func TestDiffWashes_MatchTheEditor(t *testing.T) {
	if want := blend(ctpGreen, ctpBase, 0.18); bgAdd != want {
		t.Errorf("bgAdd = %s, want %s", bgAdd, want)
	}
	if want := blend(ctpRed, ctpBase, 0.18); bgDel != want {
		t.Errorf("bgDel = %s, want %s", bgDel, want)
	}
	// CursorLine, which catppuccin darkens surface0 towards the base for.
	if want := blend(ctpSurface0, ctpBase, 0.64); bgCursor != want {
		t.Errorf("bgCursor = %s, want %s", bgCursor, want)
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
