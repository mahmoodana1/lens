// Package difftext computes diffs for changes lens observes itself.
//
// Edits made through Claude's Edit and Write tools arrive with a patch already
// computed. Changes made by shell commands — heredocs, sed -i, code generators —
// do not, so lens diffs the before and after contents here.
package difftext

import (
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"github.com/mahmood/lens/internal/capture"
)

// Hunks computes unified-diff hunks between two versions of a file, in the same
// shape Claude Code's own patches arrive in.
func Hunks(old, new string) []capture.Hunk {
	if old == new {
		return nil
	}

	edits := myers.ComputeEdits(span.URIFromPath("f"), old, new)
	unified := gotextdiff.ToUnified("a", "b", old, edits)

	out := make([]capture.Hunk, 0, len(unified.Hunks))
	for _, h := range unified.Hunks {
		hunk := capture.Hunk{OldStart: h.FromLine, NewStart: h.ToLine}
		for _, ln := range h.Lines {
			content := strings.TrimSuffix(ln.Content, "\n")
			switch ln.Kind {
			case gotextdiff.Delete:
				hunk.Lines = append(hunk.Lines, "-"+content)
				hunk.OldLines++
			case gotextdiff.Insert:
				hunk.Lines = append(hunk.Lines, "+"+content)
				hunk.NewLines++
			default:
				hunk.Lines = append(hunk.Lines, " "+content)
				hunk.OldLines++
				hunk.NewLines++
			}
		}
		// An empty original has no line 0; unified diffs still count from 1.
		if hunk.OldStart < 1 {
			hunk.OldStart = 1
		}
		if hunk.NewStart < 1 {
			hunk.NewStart = 1
		}
		capture.SliceContext(old, &hunk, capture.MaxContext)
		out = append(out, hunk)
	}
	return out
}

// Counts totals the added and removed lines across hunks.
func Counts(hunks []capture.Hunk) (added, removed int) {
	for _, h := range hunks {
		for _, ln := range h.Lines {
			switch {
			case strings.HasPrefix(ln, "+"):
				added++
			case strings.HasPrefix(ln, "-"):
				removed++
			}
		}
	}
	return added, removed
}
