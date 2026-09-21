package pane

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/mahmood/lens/internal/capture"
)

// editors are the commands a pane may be running that count as an editor to
// show a file in. Only nvim, because only nvim can be handed one over a socket.
var editors = map[string]bool{"nvim": true}

// EditorPane is the pane running an editor on a project, if one is.
//
// Handing a file to an editor is half the job: it is usually in another window
// — the editor in one, Claude in another — so the reader has to be taken there
// or they will be looking at a file they cannot see.
func EditorPane(project string) (string, bool) {
	out, err := exec.Command("tmux", "list-panes", "-a", "-F",
		"#{pane_id}\t#{pane_current_command}\t#{pane_current_path}").Output()
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		parts := strings.SplitN(line, "\t", 3)
		if len(parts) != 3 || !editors[parts[1]] {
			continue
		}
		if capture.Inside(parts[2], project) || capture.Inside(project, parts[2]) {
			return parts[0], true
		}
	}
	return "", false
}

// Reveal brings the window holding a pane to the front.
func Reveal(paneID string) error {
	if err := exec.Command("tmux", "select-window", "-t", paneID).Run(); err != nil {
		return fmt.Errorf("pane: select-window: %w", err)
	}
	if err := exec.Command("tmux", "select-pane", "-t", paneID).Run(); err != nil {
		return fmt.Errorf("pane: select-pane: %w", err)
	}
	return nil
}

// NewWindow runs a program in a new tmux window, working in dir.
func NewWindow(dir string, argv ...string) error {
	if len(argv) == 0 {
		return fmt.Errorf("pane: nothing to run")
	}
	args := append([]string{"new-window", "-c", dir}, argv...)
	if err := exec.Command("tmux", args...).Run(); err != nil {
		return fmt.Errorf("pane: new-window: %w", err)
	}
	return nil
}
