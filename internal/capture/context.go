package capture

import "strings"

// MaxContext caps how many unchanged lines are stored on each side of a hunk.
// Storing a window rather than the whole file keeps a long session's log small.
const MaxContext = 50

// wholeFileHunk represents a newly created file as one hunk of added lines.
func wholeFileHunk(content string) Hunk {
	lines := strings.Split(content, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	out := make([]string, 0, len(lines))
	for _, ln := range lines {
		out = append(out, "+"+ln)
	}
	return Hunk{OldStart: 0, OldLines: 0, NewStart: 1, NewLines: len(out), Lines: out}
}

// SliceContext fills h.Above and h.Below with up to max unchanged lines taken
// from the pre-edit file. Line numbers in a patch are 1-based; slice indices
// are not, which is the only subtlety here.
func SliceContext(original string, h *Hunk, max int) {
	h.Above, h.Below = nil, nil
	if max <= 0 || original == "" {
		return
	}

	lines := strings.Split(original, "\n")
	// A trailing newline yields a final empty element that is not a real line.
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	if len(lines) == 0 {
		return
	}

	start := h.OldStart - 1 // first line of the hunk, 0-based
	if start < 0 {
		start = 0
	}

	if start > 0 {
		from := start - max
		if from < 0 {
			from = 0
		}
		if start <= len(lines) {
			h.Above = append([]string(nil), lines[from:start]...)
		}
	}

	after := start + h.OldLines
	if after < len(lines) {
		to := after + max
		if to > len(lines) {
			to = len(lines)
		}
		h.Below = append([]string(nil), lines[after:to]...)
	}
}
