package pane

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// ErrNoTmux means no tmux pane could be found to split. It is not a failure
// worth disturbing a session over — capture continues either way.
var ErrNoTmux = errors.New("pane: no tmux pane to split")

// target finds the tmux pane the Claude session is running in.
//
// $TMUX_PANE is the direct answer when it survives into the hook's environment.
// It does not always: hooks are not guaranteed to inherit the terminal's
// variables. The controlling terminal does survive, and tmux knows which pane
// owns which tty, so that is the reliable route.
func target() (string, error) {
	if os.Getenv("TMUX") != "" && os.Getenv("TMUX_PANE") != "" {
		return os.Getenv("TMUX_PANE"), nil
	}

	tty := controllingTTY()
	if tty == "" {
		return "", fmt.Errorf("%w: no controlling terminal", ErrNoTmux)
	}

	out, err := exec.Command("tmux", "list-panes", "-a", "-F", "#{pane_tty} #{pane_id}").Output()
	if err != nil {
		return "", fmt.Errorf("%w: listing panes: %v", ErrNoTmux, err)
	}
	id, ok := matchPaneByTTY(string(out), tty)
	if !ok {
		return "", fmt.Errorf("%w: %s is not a tmux pane", ErrNoTmux, tty)
	}
	return id, nil
}

// matchPaneByTTY picks the pane owning a terminal out of tmux's listing.
func matchPaneByTTY(listing, tty string) (string, bool) {
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if fields[0] == tty {
			return fields[1], true
		}
	}
	return "", false
}

// controllingTTY walks up the process tree for a terminal. The hook's own
// stdio is piped, but the terminal is inherited from the Claude process.
func controllingTTY() string {
	pid := os.Getpid()
	for range 12 { // far more ancestry than a hook ever has
		stat, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
		if err != nil {
			return ""
		}
		nr, ok := parseTTYNr(string(stat))
		if !ok {
			return ""
		}
		if tty := ttyFromDevNumber(nr); tty != "" {
			return tty
		}
		ppid, ok := parsePPID(string(stat))
		if !ok || ppid <= 1 {
			return ""
		}
		pid = ppid
	}
	return ""
}

// statFields returns the fields after the command name, which may itself
// contain spaces and parentheses.
func statFields(stat string) []string {
	end := strings.LastIndex(stat, ")")
	if end < 0 || end+1 >= len(stat) {
		return nil
	}
	return strings.Fields(stat[end+1:])
}

// parseTTYNr reads the tty_nr field of a /proc/<pid>/stat line.
func parseTTYNr(stat string) (int, bool) {
	fields := statFields(stat)
	if len(fields) < 5 {
		return 0, false
	}
	nr, err := strconv.Atoi(fields[4])
	if err != nil {
		return 0, false
	}
	return nr, true
}

func parsePPID(stat string) (int, bool) {
	fields := statFields(stat)
	if len(fields) < 2 {
		return 0, false
	}
	ppid, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, false
	}
	return ppid, true
}

// ttyFromDevNumber turns a packed device number into a pseudo-terminal path.
func ttyFromDevNumber(nr int) string {
	if nr == 0 {
		return ""
	}
	minor := (nr & 0xff) | ((nr >> 12) & 0xfff00)
	return fmt.Sprintf("/dev/pts/%d", minor)
}
