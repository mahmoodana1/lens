package pane

import "testing"

// A popup is drawn on a client's screen, not inside the pane it names. With
// several Claude sessions running in separate tmux sessions, a turn ending in
// one of them threw a popup over whichever project the reader was watching —
// mid-turn, showing another project's panel, which reads as empty or wrong.
//
// So the popup is only opened when a client is actually looking at the pane
// that finished. Verified against tmux: a popup targeted at a pane in an
// unattached session still runs, drawn over the session the client was on.
func TestClientOnPane(t *testing.T) {
	for _, c := range []struct {
		name    string
		listing string
		target  string
		want    bool
	}{
		{"the reader is on the pane that finished", "%2\n", "%2", true},
		{"the reader is watching another project", "%2\n", "%10", false},
		{"one of several clients is on it", "%2\n%10\n%7\n", "%10", true},
		{"no client is on it", "%2\n%7\n", "%10", false},

		// Nobody attached at all: there is no screen to draw on, and opening
		// would only leave a popup nobody asked for on the next attach.
		{"nothing attached", "", "%2", false},
		{"blank lines and padding", "\n  %2  \n\n", "%2", true},

		// Without a target there is nothing to compare, and refusing is safer
		// than throwing a popup at a guess.
		{"no target pane", "%2\n", "", false},
	} {
		if got := clientOnPane(c.listing, c.target); got != c.want {
			t.Errorf("%s: clientOnPane(%q, %q) = %v, want %v",
				c.name, c.listing, c.target, got, c.want)
		}
	}
}
