package ui_test

import (
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/ui"
)

// A rendered line must be one line. Some lexers — chroma's Makefile delegates
// its recipe lines to bash — hand back a trailing newline, and a diff row that
// secretly contains one becomes two rows on screen: the layout slips, and the
// wash behind a changed line carries across the break and floods the next row.
func TestRenderDiff_EveryLineIsOneLine(t *testing.T) {
	for _, c := range []struct {
		name, file string
		lines      []string
	}{
		{"a makefile's recipe lines", "Makefile", []string{
			"+test:", "+\t-$(MAKE) test-init", "+\t-$(MAKE) test-add", " \tmkdir -p build",
		}},
		{"a bats suite", "tests/suite.bats", []string{`+  echo "v1" >f.txt`, "+}"}},
		{"go", "main.go", []string{"+func main() {", "+\tprintln(\"x\")"}},
		{"python", "app.py", []string{"+def f(x):", "+    return x"}},
	} {
		e := capture.Event{
			Rel: c.file, Tool: "Edit", Added: len(c.lines),
			Hunks: []capture.Hunk{{OldStart: 8, OldLines: 1, NewStart: 8, NewLines: len(c.lines), Lines: c.lines}},
		}
		got := ui.RenderDiffFull(e, "", 0, 100, nil)
		for i, ln := range got.Lines {
			if strings.ContainsAny(ln, "\n\r") {
				t.Errorf("%s: line %d contains a line break: %q", c.name, i, ln)
			}
		}
	}
}

// Cutting the break must not cut the code with it.
func TestRenderDiff_KeepsTheCodeOnARecipeLine(t *testing.T) {
	e := capture.Event{
		Rel: "Makefile", Tool: "Edit", Added: 1,
		Hunks: []capture.Hunk{{OldStart: 10, OldLines: 0, NewStart: 10, NewLines: 1,
			Lines: []string{"+\t-$(MAKE) test-init"}}},
	}

	out := ui.StripANSIForTest(strings.Join(ui.RenderDiffFull(e, "", 0, 100, nil).Lines, "\n"))

	if !strings.Contains(out, "-$(MAKE) test-init") {
		t.Errorf("the recipe went missing:\n%s", out)
	}
}

// The panel as a whole never emits a row it did not mean to.
func TestView_NoLineBreaksInsideRows(t *testing.T) {
	m := browsing(makefileSession())
	m.Resize(120, 30)

	for _, mode := range []string{"", "tab", "f"} {
		if mode != "" {
			m.Send(mode)
		}
		rows := strings.Split(m.View(), "\n")
		if len(rows) > 30 {
			t.Errorf("mode %q: view is %d rows tall, want at most 30", mode, len(rows))
		}
	}
}
