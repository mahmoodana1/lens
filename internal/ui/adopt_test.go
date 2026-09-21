package ui

import (
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/mahmood/lens/internal/editor"
)

// With no editor to ask, the panel looks exactly as it did before: the built-in
// palette is the fallback, not a stand-in for a missing feature.
func TestAdopt_NothingChangesWithoutAnEditor(t *testing.T) {
	before := syntaxStyle().Get(chroma.Keyword).Colour.String()
	beforeAdd := bgAdd

	Adopt(editor.Colours{})

	if got := syntaxStyle().Get(chroma.Keyword).Colour.String(); got != before {
		t.Errorf("keyword = %s, want the built-in %s", got, before)
	}
	if bgAdd != beforeAdd {
		t.Errorf("bgAdd = %s, want the built-in %s", bgAdd, beforeAdd)
	}
}

// Given an editor's colours, the diff wears them.
func TestAdopt_TakesTheEditorsColours(t *testing.T) {
	t.Cleanup(func() { Adopt(editor.Colours{}) })

	Adopt(editor.Colours{
		Fg: "#010101", Bg: "#020202",
		Comment: "#030303", Keyword: "#040404", String: "#050505",
		Number: "#060606", Function: "#070707", Type: "#080808",
		DiffAdd: "#090909", DiffDelete: "#0a0a0a", CursorLine: "#0b0b0b",
		LineNr: "#0c0c0c", CommentItalic: true,
	})

	st := syntaxStyle()
	for _, c := range []struct {
		token chroma.TokenType
		want  string
	}{
		{chroma.Comment, "#030303"},
		{chroma.Keyword, "#040404"},
		{chroma.LiteralString, "#050505"},
		{chroma.LiteralNumber, "#060606"},
		{chroma.NameFunction, "#070707"},
		{chroma.NameClass, "#080808"},
	} {
		if got := st.Get(c.token).Colour.String(); got != c.want {
			t.Errorf("%v = %s, want %s", c.token, got, c.want)
		}
	}
	if bgAdd != "#090909" || bgDel != "#0a0a0a" || bgCursor != "#0b0b0b" {
		t.Errorf("washes = %s / %s / %s, want the editor's", bgAdd, bgDel, bgCursor)
	}
}

// A colour the editor does not define keeps the built-in one, so a sparse
// colourscheme cannot leave the panel with holes in it.
func TestAdopt_KeepsTheBuiltInWhereTheEditorIsSilent(t *testing.T) {
	t.Cleanup(func() { Adopt(editor.Colours{}) })
	want := syntaxStyle().Get(chroma.Operator).Colour.String()

	Adopt(editor.Colours{Keyword: "#040404"}) // says nothing about operators

	if got := syntaxStyle().Get(chroma.Operator).Colour.String(); got != want {
		t.Errorf("operator = %s, want the built-in %s", got, want)
	}
}

// Comments are italic if the editor draws them that way, and upright if not.
func TestAdopt_FollowsTheEditorOnItalics(t *testing.T) {
	t.Cleanup(func() { Adopt(editor.Colours{}) })

	Adopt(editor.Colours{Comment: "#030303", CommentItalic: false})
	if got := syntaxStyle().Get(chroma.Comment).Italic; got == chroma.Yes {
		t.Error("comments are italic though the editor draws them upright")
	}

	Adopt(editor.Colours{Comment: "#030303", CommentItalic: true})
	if got := syntaxStyle().Get(chroma.Comment).Italic; got != chroma.Yes {
		t.Error("comments are upright though the editor italicises them")
	}
}
