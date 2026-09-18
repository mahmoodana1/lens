// Package capture turns Claude Code hook payloads into the events the panel
// renders. Claude Code computes the diff itself and hands it to the PostToolUse
// hook, so nothing here re-diffs or snapshots files.
package capture

import "time"

// Hunk is one contiguous change within a file, as Claude Code reports it.
// Lines carry unified-diff prefixes: " " unchanged, "-" removed, "+" added,
// and "\" for the no-newline-at-end-of-file marker.
type Hunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`

	// Above and Below hold unchanged lines adjacent to the hunk, sliced from
	// the pre-edit file so the panel can widen context without re-reading it.
	Above []string `json:"above,omitempty"`
	Below []string `json:"below,omitempty"`
}

// Event is a single edit Claude made to a single file.
type Event struct {
	Seq      int       `json:"seq"`
	Time     time.Time `json:"time"`
	Tool     string    `json:"tool"`
	Path     string    `json:"path"`
	Rel      string    `json:"rel"`
	Kind     string    `json:"kind"` // "create" or "update"
	PromptID string    `json:"prompt_id,omitempty"`
	Hunks    []Hunk    `json:"hunks"`
	Added    int       `json:"added"`
	Removed  int       `json:"removed"`
}

// Record pairs an event with the session that produced it.
type Record struct {
	SessionID string
	CWD       string
	Event     Event
}
