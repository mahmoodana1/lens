// Package pane opens the panel in a tmux popup over the Claude session.
package pane

import (
	"fmt"
	"os/exec"
	"syscall"
)

// The panel is a reading surface, so the popup takes most of the window while
// staying an overlay: the session it covers is still visible around the edges.
const (
	popupWidth  = "90%"
	popupHeight = "85%"
	popupTitle  = " lens "
)

// Open shows the panel for a session in a tmux popup.
//
// A popup holds the client's keyboard for as long as it lives, which is what
// makes the panel scrollable the moment it appears — no pane to switch to
// first. tmux does not return until the popup is dismissed, so the spawn is
// detached: a hook must come back promptly however long the reader stays.
func Open(sessionID, self string) error {
	dest, err := target()
	if err != nil {
		return err
	}

	cmd := exec.Command("tmux",
		"display-popup", "-E",
		"-w", popupWidth, "-h", popupHeight,
		"-T", popupTitle, "-t", dest,
		self, "--session", sessionID)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("pane: display-popup: %w", err)
	}
	return cmd.Process.Release()
}
