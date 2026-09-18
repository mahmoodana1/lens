package ui_test

import (
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/ui"
)

func joined(lines []string) string { return strings.Join(lines, "\n") }

func TestRenderDiff_ShowsIntentLine(t *testing.T) {
	e := capture.Event{Rel: "main.go", Hunks: []capture.Hunk{
		{OldStart: 1, OldLines: 1, Lines: []string{"-a", "+b"}},
	}}

	out := joined(ui.RenderDiff(e, "add a parser", 0, 80))
	if !strings.Contains(out, "add a parser") {
		t.Error("the prompt that caused the edit is not shown")
	}
	if !strings.Contains(out, "main.go") {
		t.Error("file name not shown")
	}
}

func TestRenderDiff_OmitsIntentLineWhenUnknown(t *testing.T) {
	e := capture.Event{Rel: "main.go", Hunks: []capture.Hunk{{OldStart: 1, Lines: []string{"+b"}}}}
	for _, line := range ui.RenderDiff(e, "", 0, 80) {
		if strings.Contains(line, "❯") {
			t.Error("intent marker rendered with no prompt available")
		}
	}
}

func TestRenderDiff_ContextWidensAndClamps(t *testing.T) {
	h := capture.Hunk{
		OldStart: 10, OldLines: 1,
		Lines: []string{"-old", "+new"},
		Above: []string{"a1", "a2"},
		Below: []string{"b1"},
	}
	e := capture.Event{Rel: "main.go", Hunks: []capture.Hunk{h}}

	at0 := joined(ui.RenderDiff(e, "", 0, 80))
	if strings.Contains(at0, "a2") || strings.Contains(at0, "b1") {
		t.Error("context shown at width 0")
	}

	at1 := joined(ui.RenderDiff(e, "", 1, 80))
	if !strings.Contains(at1, "a2") {
		t.Error("ctx=1 should show the line immediately above the hunk")
	}
	if strings.Contains(at1, "a1") {
		t.Error("ctx=1 showed two lines above")
	}
	if !strings.Contains(at1, "b1") {
		t.Error("ctx=1 should show the line immediately below")
	}

	at99 := joined(ui.RenderDiff(e, "", 99, 80))
	if !strings.Contains(at99, "a1") || !strings.Contains(at99, "b1") {
		t.Error("context beyond what was captured should clamp, not truncate away")
	}
}

// "\ No newline at end of file" is diff bookkeeping, not code.
func TestRenderDiff_NoNewlineMarkerNotRendered(t *testing.T) {
	e := capture.Event{Rel: "x.txt", Hunks: []capture.Hunk{
		{OldStart: 1, OldLines: 1, Lines: []string{"+alpha", "\\ No newline at end of file"}},
	}}

	if out := joined(ui.RenderDiff(e, "", 0, 80)); strings.Contains(out, "No newline") {
		t.Error("diff bookkeeping leaked into the rendered output")
	}
}

func TestRenderDiff_EmptyHunksStillRendersHeader(t *testing.T) {
	e := capture.Event{Rel: "main.go", Added: 0, Removed: 0}
	out := ui.RenderDiff(e, "", 0, 80)
	if len(out) == 0 {
		t.Fatal("no output at all for an event with no hunks")
	}
	if !strings.Contains(joined(out), "main.go") {
		t.Error("header missing")
	}
}

// Long lines must not wrap and break the two-pane layout.
func TestRenderDiff_TruncatesToWidth(t *testing.T) {
	long := strings.Repeat("x", 300)
	e := capture.Event{Rel: "main.go", Hunks: []capture.Hunk{
		{OldStart: 1, OldLines: 1, Lines: []string{"+" + long}},
	}}

	for i, line := range ui.RenderDiff(e, "", 0, 40) {
		if got := ui.VisibleWidth(line); got > 40 {
			t.Errorf("line %d is %d cells wide, want <= 40", i, got)
		}
	}
}

func TestRenderDiff_ShowsLineNumbers(t *testing.T) {
	e := capture.Event{Rel: "main.go", Hunks: []capture.Hunk{
		{OldStart: 40, OldLines: 2, NewStart: 40, NewLines: 2, Lines: []string{" keep", "-old", "+new"}},
	}}

	out := joined(ui.RenderDiff(e, "", 0, 100))
	if !strings.Contains(out, "40") {
		t.Error("line numbers not shown; you cannot tell where the change lands")
	}
}

// Removed lines carry no number: they are gone from the file being read.
func TestRenderDiff_RemovedLinesHaveNoLineNumber(t *testing.T) {
	e := capture.Event{Rel: "main.go", Hunks: []capture.Hunk{
		{OldStart: 29, OldLines: 2, NewStart: 34, NewLines: 2,
			Lines: []string{" keep", "-gone", "+fresh"}},
	}}

	for _, line := range ui.RenderDiff(e, "", 0, 100) {
		plain := ui.StripANSIForTest(line)
		if strings.Contains(plain, "-") && strings.Contains(plain, "gone") {
			if strings.ContainsAny(strings.TrimSpace(plain[:6]), "0123456789") {
				t.Errorf("removed line carries a number: %q", plain)
			}
		}
	}
}

// New and context lines are numbered by the file's current numbering.
func TestRenderDiff_ContextUsesNewFileNumbering(t *testing.T) {
	e := capture.Event{Rel: "main.go", Hunks: []capture.Hunk{
		{OldStart: 29, OldLines: 2, NewStart: 34, NewLines: 2,
			Lines: []string{" keep", "-gone", "+fresh"}},
	}}

	out := joined(ui.RenderDiff(e, "", 0, 100))
	if !strings.Contains(out, "34") {
		t.Error("context line not numbered from the new file")
	}
	if strings.Contains(out, "  29 ") {
		t.Error("old-file numbering leaked into the gutter")
	}
}
