package ui

import (
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
)

// A file type the highlighter does not know renders as a wall of plain text,
// which is the difference between reading a diff and squinting at one. Where a
// name is not enough, something close has to stand in for it.
func TestLexerFor(t *testing.T) {
	for _, c := range []struct {
		name string
		path string
		code string
		want string
	}{
		{"a known extension", "a.py", "", "Python"},
		{"a known name", "Makefile", "", "Makefile"},
		{"bats is bash with a test harness", "suite.bats", "", "Bash"},
		{"a tmux config", "theme.tmux", "", "Bash"},
		{"a justfile recipe", "build.just", "", "Makefile"},
		{"an ini-shaped conf", "app.conf", "", "INI"},
		{"an env file", ".envrc", "", "Bash"},

		// No name to go on, so the code itself has to say.
		{"a shebang in the body", "runme", "#!/bin/bash\necho hi\n", "Bash"},
		{"nothing to go on", "mystery.zzz", "qqq wibble\n", ""},
	} {
		lx := lexerFor(c.path, c.code)
		got := ""
		if lx != nil {
			got = lx.Config().Name
		}
		if got != c.want {
			t.Errorf("%s: lexerFor(%q) = %q, want %q", c.name, c.path, got, c.want)
		}
	}
}

// The end of it: a .bats diff comes out coloured rather than flat.
func TestHighlighter_ColoursAFileTypeChromaDoesNotKnow(t *testing.T) {
	hl := newHighlighter("suite.bats", "")

	if hl.lexer == nil {
		t.Fatal("no lexer for a .bats file, so its diff renders plain")
	}
	// The comment and the string must come back as different token types.
	if got := tokenColours(t, hl, `  # a comment`); got == "" {
		t.Error("a comment was not recognised")
	}
}

func tokenColours(t *testing.T, hl *highlighter, code string) string {
	t.Helper()
	it, err := hl.lexer.Tokenise(nil, code)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, tok := range it.Tokens() {
		if hl.style.Get(tok.Type).Colour.IsSet() && strings.TrimSpace(tok.Value) != "" {
			b.WriteString(tok.Type.String() + " ")
		}
	}
	return b.String()
}

// The sample handed to the guesser is the code the edit touched, without the
// diff markers that would confuse it.
func TestCodeSample_StripsDiffMarkers(t *testing.T) {
	e := capture.Event{Hunks: []capture.Hunk{{Lines: []string{
		"+#!/bin/bash", "-old line", " context", `+echo "hi"`,
	}}}}

	got := codeSample(e)

	if strings.Contains(got, "+#!") || strings.Contains(got, "-old") {
		t.Errorf("the markers came through: %q", got)
	}
	if !strings.HasPrefix(got, "#!/bin/bash") {
		t.Errorf("sample = %q, want it to start with the shebang", got)
	}
	if !strings.Contains(got, `echo "hi"`) {
		t.Errorf("sample = %q, want the added code in it", got)
	}
}
