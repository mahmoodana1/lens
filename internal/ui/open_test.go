package ui_test

import (
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
)

func fileWithHunks() store.Session {
	h := func(line int, first string) capture.Hunk {
		return capture.Hunk{
			OldStart: line, OldLines: 1, NewStart: line, NewLines: 2,
			Lines: []string{" context", first, " more"},
		}
	}
	return store.Session{
		Meta: store.Meta{CWD: "/p"},
		Events: []capture.Event{{
			Seq: 1, Rel: "internal/ui/keys.go", Path: "/p/internal/ui/keys.go", Tool: "Edit", Added: 2,
			Hunks: []capture.Hunk{h(42, "+first"), h(88, "+second")},
		}},
		Prompts: map[string]string{},
	}
}

// On a file, the place to open is where its change begins.
func TestOpenAt_FromAFileRow(t *testing.T) {
	m := browsing(fileWithHunks())

	file, line := m.OpenAt()
	if file != "/p/internal/ui/keys.go" {
		t.Errorf("file = %q, want the absolute path: the editor is not in the panel's directory", file)
	}
	if line != 42 {
		t.Errorf("line = %d, want 42, where the first hunk starts", line)
	}
}

// On a hunk, the place to open is that hunk.
func TestOpenAt_FromAHunkRow(t *testing.T) {
	m := browsing(fileWithHunks())
	m.Send("l").Send("j") // inside the file, on its second hunk

	if _, line := m.OpenAt(); line != 88 {
		t.Errorf("line = %d, want 88", line)
	}
}

// With the diff focused, the place to open is the line under the diff cursor —
// you point at a line and ask to see it.
func TestOpenAt_FollowsTheDiffCursor(t *testing.T) {
	m := browsing(fileWithHunks())
	m.Resize(100, 24)
	m.Send("tab") // the diff takes the cursor

	m.Send("n") // the first hunk, whose code starts at 42
	if _, line := m.OpenAt(); line != 42 {
		t.Errorf("line = %d, want 42", line)
	}
	m.Send("n") // the second, at 88
	if _, line := m.OpenAt(); line != 88 {
		t.Errorf("line = %d, want 88", line)
	}
	// The header, then its context line at 88, then the added line at 89.
	m.Send("j").Send("j")
	if _, line := m.OpenAt(); line != 89 {
		t.Errorf("line = %d, want 89", line)
	}
}

// A file with no absolute path recorded is still worth opening, by the name
// there is.
func TestOpenAt_FallsBackToTheRelativePath(t *testing.T) {
	m := browsing(store.Session{
		Events:  []capture.Event{{Seq: 1, Rel: "a.go", Added: 1}},
		Prompts: map[string]string{},
	})

	if file, _ := m.OpenAt(); file != "a.go" {
		t.Errorf("file = %q, want a.go", file)
	}
}

// Nothing selected, nothing to open.
func TestOpenAt_WithAnEmptySession(t *testing.T) {
	m := browsing(store.Session{})

	if file, _ := m.OpenAt(); file != "" {
		t.Errorf("file = %q, want nothing", file)
	}
}

// o hands the panel back: the reader asked to look at the file, and a popup
// holding the keyboard would be in the way of that.
func TestOpen_ClosesThePopup(t *testing.T) {
	m := browsing(fileWithHunks())

	var got struct {
		file string
		line int
	}
	m.OnOpen(func(file string, line int) { got.file, got.line = file, line })

	if !m.Send("o").Quitting() {
		t.Error("o did not close the popup")
	}
	if got.file == "" || got.line != 42 {
		t.Errorf("asked to open %q:%d, want the selected file at 42", got.file, got.line)
	}
}
