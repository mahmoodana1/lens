package editor_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mahmood/lens/internal/editor"
)

// The whole claim of the panel is that a diff here reads like the file open in
// the editor. That can only be true if the colours come from the editor rather
// than from a palette written down once, on one machine.
func TestPalette_ComesFromTheRunningEditor(t *testing.T) {
	proj := t.TempDir()
	sock := startNvimWith(t, proj, `
		hi clear
		hi Normal   guifg=#111111 guibg=#222222
		hi Comment  guifg=#333333 gui=italic
		hi String   guifg=#444444
		hi Keyword  guifg=#555555
		hi DiffAdd  guibg=#666666
	`)

	p, err := editor.Palette(editor.Server{Addr: sock})
	if err != nil {
		t.Fatal(err)
	}
	if p.Fg == "" {
		t.Skip("this editor reports no GUI colours, so there is no palette to read")
	}

	for _, c := range []struct {
		group, want string
		got         func() string
	}{
		{"Normal fg", "#111111", func() string { return p.Fg }},
		{"Normal bg", "#222222", func() string { return p.Bg }},
		{"Comment", "#333333", func() string { return p.Comment }},
		{"String", "#444444", func() string { return p.String }},
		{"Keyword", "#555555", func() string { return p.Keyword }},
		{"DiffAdd", "#666666", func() string { return p.DiffAdd }},
	} {
		if got := c.got(); got != c.want {
			t.Errorf("%s = %q, want %q", c.group, got, c.want)
		}
	}
	if !p.CommentItalic {
		t.Error("the editor italicises comments and the palette did not notice")
	}
}

// A group the colourscheme gives no colour must come back empty, not as a
// guess — the caller keeps its own value for it. (Most groups have a built-in
// default in the editor, so "no colour" has to be said outright.)
func TestPalette_LeavesUnsetGroupsEmpty(t *testing.T) {
	sock := startNvimWith(t, t.TempDir(), `
		hi clear
		hi Normal guifg=#abcdef guibg=#123456
		hi @property guifg=NONE guibg=NONE
	`)

	p, err := editor.Palette(editor.Server{Addr: sock})
	if err != nil {
		t.Fatal(err)
	}

	if p.Fg == "" {
		t.Skip("this editor reports no GUI colours, so there is no palette to read")
	}
	if p.Fg != "#abcdef" {
		t.Errorf("Fg = %q", p.Fg)
	}
	if p.Property != "" {
		t.Errorf("Property = %q, want empty: the scheme gives it no colour", p.Property)
	}
}

// Whatever the editor answers, nothing that is not a colour reaches the panel.
func TestPalette_AnswersAreAlwaysColoursOrNothing(t *testing.T) {
	sock := startNvimWith(t, t.TempDir(), "hi clear\nhi Normal guifg=#abcdef guibg=#123456")

	p, err := editor.Palette(editor.Server{Addr: sock})
	if err != nil {
		t.Fatal(err)
	}

	for name, got := range map[string]string{
		"Fg": p.Fg, "Bg": p.Bg, "Comment": p.Comment, "Keyword": p.Keyword,
		"Function": p.Function, "Type": p.Type, "String": p.String,
		"Number": p.Number, "Operator": p.Operator, "Punct": p.Punct,
		"Property": p.Property, "LineNr": p.LineNr, "CursorLine": p.CursorLine,
		"DiffAdd": p.DiffAdd, "DiffDelete": p.DiffDelete,
	} {
		if got == "" {
			continue
		}
		if len(got) != 7 || got[0] != '#' {
			t.Errorf("%s = %q, want a #rrggbb colour or nothing at all", name, got)
		}
	}
}

// A dead socket is an ordinary answer — use the built-in palette — not a crash.
func TestPalette_FromAnEditorThatIsNotThere(t *testing.T) {
	dead := filepath.Join(t.TempDir(), "stale.sock")
	if err := os.WriteFile(dead, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := editor.Palette(editor.Server{Addr: dead}); err == nil {
		t.Error("expected an error from a socket nothing is listening on")
	}
}

// startNvimWith is startNvim with a colourscheme of its own, so the test knows
// exactly which values it should read back.
func startNvimWith(t *testing.T, dir, vimrc string) string {
	t.Helper()
	rc := filepath.Join(t.TempDir(), "init.vim")
	if err := os.WriteFile(rc, []byte("set termguicolors\n"+vimrc), 0o600); err != nil {
		t.Fatal(err)
	}
	return startNvimRC(t, dir, rc)
}
