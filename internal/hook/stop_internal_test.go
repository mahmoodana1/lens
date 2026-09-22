package hook

import (
	"testing"

	"github.com/mahmood/lens/internal/capture"
)

const testRoot = "/home/me/project"

func events(seqs ...int) []capture.Event {
	out := make([]capture.Event, 0, len(seqs))
	for _, s := range seqs {
		out = append(out, capture.Event{Seq: s, Rel: "a.go", Tool: "Edit", Path: testRoot + "/a.go"})
	}
	return out
}

// strayed is what a shell command sweeps up outside the project: recorded, and
// never drawn.
func strayed(seqs ...int) []capture.Event {
	out := make([]capture.Event, 0, len(seqs))
	for _, s := range seqs {
		out = append(out, capture.Event{Seq: s, Tool: "Bash", Path: "/tmp/scratch/x.go"})
	}
	return out
}

func TestShouldOpen(t *testing.T) {
	for _, c := range []struct {
		name      string
		events    []capture.Event
		announced int
		autoOpen  bool
		wantSeq   int
		wantOpen  bool
	}{
		{"nothing has ever been edited", nil, 0, true, 0, false},
		{"the first edits of a session", events(1, 2, 3), 0, true, 3, true},
		{"a turn that changed nothing", events(1, 2, 3), 3, true, 0, false},
		{"a turn that changed something new", events(1, 2, 3, 4), 3, true, 4, true},
		{"auto-open turned off", events(1, 2, 3), 0, false, 0, false},
		{"auto-open off, nothing new either", events(1), 1, false, 0, false},
	} {
		seq, open := shouldOpen(testRoot, c.events, c.announced, c.autoOpen)
		if open != c.wantOpen || seq != c.wantSeq {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", c.name, seq, open, c.wantSeq, c.wantOpen)
		}
	}
}

// The bug behind an empty popup.
//
// The decision counted every event on disk while the panel drew only some of
// them, so a turn whose writes the panel hides opened a popup with nothing new
// in it. It is now asked of the events the panel would actually draw.
func TestShouldOpen_IgnoresWhatThePanelWouldHide(t *testing.T) {
	for _, c := range []struct {
		name      string
		events    []capture.Event
		announced int
		wantSeq   int
		wantOpen  bool
	}{
		{"a turn that only swept up scratch files", strayed(1, 2, 3), 0, 0, false},
		{"real edits, then a turn of nothing but scratch files",
			append(events(1, 2), strayed(3, 4)...), 2, 0, false},
		{"scratch files, then something real",
			append(strayed(1, 2), events(3)...), 0, 3, true},
		{"the marker is the last edit worth drawing, not the last recorded",
			append(events(1, 2, 3), strayed(4, 5)...), 0, 3, true},
	} {
		seq, open := shouldOpen(testRoot, c.events, c.announced, true)
		if open != c.wantOpen || seq != c.wantSeq {
			t.Errorf("%s: shouldOpen = (%d, %v), want (%d, %v)",
				c.name, seq, open, c.wantSeq, c.wantOpen)
		}
	}
}

// Hidden files are machinery, and a turn that only touched them is not worth
// taking the keyboard for.
func TestShouldOpen_IgnoresHiddenFiles(t *testing.T) {
	hidden := []capture.Event{{Seq: 1, Tool: "Write", Path: testRoot + "/.git/index"}}
	if seq, open := shouldOpen(testRoot, hidden, 0, true); open {
		t.Errorf("shouldOpen = (%d, true) for a hidden-only turn, want no popup", seq)
	}
}
