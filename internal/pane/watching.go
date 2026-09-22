package pane

import (
	"os/exec"
	"strings"
)

// Watching reports whether anyone is looking at the pane a popup would cover.
//
// tmux draws a popup on a client's screen rather than inside the pane it names,
// so a popup opened for one pane appears over whatever its client is currently
// showing. With several Claude sessions running in separate tmux sessions, a
// turn ending in one of them put a popup over the project the reader was
// working in — mid-turn, showing another project's panel, which reads as empty
// or simply wrong.
//
// A panel is worth opening where its reader is. Where they are not, the capture
// stays on disk and the hotkey still brings it up.
func Watching(target string) bool {
	// #{pane_id} for a client is the pane that client is on right now, which is
	// the question being asked — not which panes exist.
	out, err := exec.Command("tmux", "list-clients", "-F", "#{pane_id}").Output()
	if err != nil {
		// tmux could not be asked. Opening is the old behaviour and the panel
		// is the point, so a failure here does not suppress it.
		return true
	}
	return clientOnPane(string(out), target)
}

// WatchingHere reports whether anyone is looking at the pane this process is
// running in — the pane holding the Claude session whose turn just ended.
//
// A target that cannot be worked out is not treated as absence: pane.Open will
// fail with a reason worth recording, and suppressing the panel silently here
// would hide that.
func WatchingHere() bool {
	dest, err := target()
	if err != nil {
		return true
	}
	return Watching(dest)
}

// clientOnPane reports whether the listing of what each client is looking at
// includes this pane.
func clientOnPane(listing, target string) bool {
	if strings.TrimSpace(target) == "" {
		return false
	}
	for _, line := range strings.Split(listing, "\n") {
		if strings.TrimSpace(line) == strings.TrimSpace(target) {
			return true
		}
	}
	return false
}
