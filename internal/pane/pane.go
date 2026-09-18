// Package pane opens the panel in a tmux pane beside the Claude session.
package pane

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mahmood/lens/internal/store"
)

// Ensure opens the panel for a session exactly once.
//
// It is a no-op outside tmux, and a no-op once a pane exists. Claude runs tools
// in parallel, so several hooks can race here; the pane file is created with
// O_EXCL and whichever process creates it is the one that spawns.
func Ensure(sessionID, self string) error {
	if os.Getenv("TMUX") == "" {
		return nil
	}

	lock := filepath.Join(store.Dir(sessionID), store.PaneFile)
	f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return nil // another hook already opened the panel
		}
		return fmt.Errorf("pane: claim spawn lock: %w", err)
	}
	defer f.Close()

	args := []string{"split-window", "-h", "-d", "-l", "45%", "-P", "-F", "#{pane_id}"}
	if target := os.Getenv("TMUX_PANE"); target != "" {
		args = append(args, "-t", target)
	}
	args = append(args, self, "--session", sessionID)

	out, err := exec.Command("tmux", args...).Output()
	if err != nil {
		// Leave no lock behind, so a later edit can try again.
		os.Remove(lock)
		return fmt.Errorf("pane: split-window: %w", err)
	}

	_, err = f.WriteString(strings.TrimSpace(string(out)) + "\n")
	return err
}

// Close kills the panel's pane, if one is recorded.
func Close(sessionID string) {
	b, err := os.ReadFile(filepath.Join(store.Dir(sessionID), store.PaneFile))
	if err != nil {
		return
	}
	id := strings.TrimSpace(string(b))
	if id == "" {
		return
	}
	exec.Command("tmux", "kill-pane", "-t", id).Run()
}
