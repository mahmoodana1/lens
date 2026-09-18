package hook

import (
	"testing"

	"github.com/mahmood/lens/internal/capture"
)

func events(seqs ...int) []capture.Event {
	out := make([]capture.Event, 0, len(seqs))
	for _, s := range seqs {
		out = append(out, capture.Event{Seq: s, Rel: "a.go"})
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
		seq, open := shouldOpen(c.events, c.announced, c.autoOpen)
		if open != c.wantOpen || seq != c.wantSeq {
			t.Errorf("%s: got (%d, %v), want (%d, %v)", c.name, seq, open, c.wantSeq, c.wantOpen)
		}
	}
}
