package capture_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
)

func TestSliceContext_BoundedBothSides(t *testing.T) {
	var b strings.Builder
	for i := 1; i <= 200; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}

	h := capture.Hunk{OldStart: 100, OldLines: 2}
	capture.SliceContext(b.String(), &h, 50)

	if len(h.Above) != 50 {
		t.Errorf("above = %d lines, want 50", len(h.Above))
	}
	if got := h.Above[len(h.Above)-1]; got != "line 99" {
		t.Errorf("above ends at %q, want %q", got, "line 99")
	}
	if len(h.Below) != 50 {
		t.Errorf("below = %d lines, want 50", len(h.Below))
	}
	if got := h.Below[0]; got != "line 102" {
		t.Errorf("below starts at %q, want %q", got, "line 102")
	}
}

func TestSliceContext_ClampsAtFileEdges(t *testing.T) {
	h := capture.Hunk{OldStart: 1, OldLines: 1}
	capture.SliceContext("a\nb\nc\n", &h, 50)

	if len(h.Above) != 0 {
		t.Errorf("above = %d, want 0 at the top of the file", len(h.Above))
	}
	if len(h.Below) != 2 {
		t.Errorf("below = %d, want 2", len(h.Below))
	}
	if h.Below[0] != "b" || h.Below[1] != "c" {
		t.Errorf("below = %q, want [b c]", h.Below)
	}
}

func TestSliceContext_EmptyOriginal(t *testing.T) {
	h := capture.Hunk{OldStart: 1, OldLines: 0}
	capture.SliceContext("", &h, 50)

	if len(h.Above) != 0 || len(h.Below) != 0 {
		t.Errorf("want empty context for a new file, got above=%d below=%d", len(h.Above), len(h.Below))
	}
}

func TestSliceContext_ZeroMaxStoresNothing(t *testing.T) {
	h := capture.Hunk{OldStart: 2, OldLines: 1}
	capture.SliceContext("a\nb\nc\n", &h, 0)

	if len(h.Above) != 0 || len(h.Below) != 0 {
		t.Errorf("max=0 should store no context, got above=%d below=%d", len(h.Above), len(h.Below))
	}
}

func TestSliceContext_OutOfRangeStartDoesNotPanic(t *testing.T) {
	h := capture.Hunk{OldStart: 999, OldLines: 3}
	capture.SliceContext("a\nb\n", &h, 50)

	if len(h.Below) != 0 {
		t.Errorf("below = %d, want 0 when the hunk starts past EOF", len(h.Below))
	}
}
