package difftext_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/difftext"
)

func TestHunks_SingleLineChange(t *testing.T) {
	old := "alpha\nbeta\ngamma\n"
	new := "alpha\nBETA\ngamma\n"

	hunks := difftext.Hunks(old, new)
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(hunks))
	}

	var got []string
	for _, ln := range hunks[0].Lines {
		got = append(got, ln)
	}
	joined := strings.Join(got, "|")
	if !strings.Contains(joined, "-beta") || !strings.Contains(joined, "+BETA") {
		t.Errorf("lines = %q, want a -beta and a +BETA", joined)
	}
}

func TestHunks_CountsMatchLines(t *testing.T) {
	old := "a\nb\nc\n"
	new := "a\nb2\nb3\nc\n"

	hunks := difftext.Hunks(old, new)
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(hunks))
	}
	h := hunks[0]

	oldCount, newCount := 0, 0
	for _, ln := range h.Lines {
		switch {
		case strings.HasPrefix(ln, "-"):
			oldCount++
		case strings.HasPrefix(ln, "+"):
			newCount++
		default:
			oldCount++
			newCount++
		}
	}
	if h.OldLines != oldCount {
		t.Errorf("OldLines = %d, want %d", h.OldLines, oldCount)
	}
	if h.NewLines != newCount {
		t.Errorf("NewLines = %d, want %d", h.NewLines, newCount)
	}
}

// Distant edits belong in separate hunks, so n/N can jump between them.
func TestHunks_SeparateHunksForDistantEdits(t *testing.T) {
	var oldB, newB strings.Builder
	for i := 1; i <= 100; i++ {
		fmt.Fprintf(&oldB, "line %d\n", i)
		switch i {
		case 5:
			newB.WriteString("CHANGED near the top\n")
		case 90:
			newB.WriteString("CHANGED near the bottom\n")
		default:
			fmt.Fprintf(&newB, "line %d\n", i)
		}
	}

	hunks := difftext.Hunks(oldB.String(), newB.String())
	if len(hunks) != 2 {
		t.Fatalf("hunks = %d, want 2", len(hunks))
	}
	if hunks[0].OldStart >= hunks[1].OldStart {
		t.Error("hunks are not in file order")
	}
}

func TestHunks_NewFileIsAllAdditions(t *testing.T) {
	hunks := difftext.Hunks("", "one\ntwo\n")
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(hunks))
	}
	for _, ln := range hunks[0].Lines {
		if !strings.HasPrefix(ln, "+") {
			t.Errorf("line %q is not an addition", ln)
		}
	}
}

func TestHunks_IdenticalContentHasNoHunks(t *testing.T) {
	if hunks := difftext.Hunks("same\n", "same\n"); len(hunks) != 0 {
		t.Errorf("hunks = %d, want 0 for identical content", len(hunks))
	}
}

func TestHunks_StartLinesAreOneBased(t *testing.T) {
	hunks := difftext.Hunks("a\nb\nc\n", "A\nb\nc\n")
	if len(hunks) != 1 {
		t.Fatalf("hunks = %d, want 1", len(hunks))
	}
	if hunks[0].OldStart != 1 || hunks[0].NewStart != 1 {
		t.Errorf("starts = %d/%d, want 1/1", hunks[0].OldStart, hunks[0].NewStart)
	}
}

func TestCounts(t *testing.T) {
	hunks := difftext.Hunks("a\nb\nc\n", "a\nB\nc\nd\n")
	added, removed := difftext.Counts(hunks)
	if added != 2 || removed != 1 {
		t.Errorf("+%d -%d, want +2 -1", added, removed)
	}
}
