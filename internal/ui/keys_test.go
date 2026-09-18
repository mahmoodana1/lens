package ui_test

import (
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
	"github.com/mahmood/lens/internal/ui"
)

func TestKeys_JKMoveSelectionAndClampAtEnds(t *testing.T) {
	m := ui.NewForTest(threeEvents())

	if got := m.Send("j").Cursor(); got != 1 {
		t.Errorf("after j, cursor = %d, want 1", got)
	}
	if got := m.Send("k").Send("k").Cursor(); got != 0 {
		t.Errorf("k past the top should clamp at 0, got %d", got)
	}
	if got := m.Send("G").Cursor(); got != 2 {
		t.Errorf("G should land on the last row, got %d", got)
	}
	if got := m.Send("j").Cursor(); got != 2 {
		t.Errorf("j past the bottom should clamp, got %d", got)
	}
	if got := m.Send("g").Send("g").Cursor(); got != 0 {
		t.Errorf("gg should land on the first row, got %d", got)
	}
}

// A lone g is a pending chord, not a jump.
func TestKeys_SingleGDoesNotJump(t *testing.T) {
	m := ui.NewForTest(threeEvents()).Send("G")
	if got := m.Send("g").Cursor(); got != 2 {
		t.Errorf("single g moved the cursor to %d; it should wait for the second g", got)
	}
}

func TestKeys_ToggleSwitchesView(t *testing.T) {
	m := ui.NewForTest(threeEvents())

	before := m.RowCount()
	m.Send("t")
	if m.RowCount() == before {
		t.Error("t did not change the row layout")
	}
	m.Send("t")
	if m.RowCount() != before {
		t.Error("toggling twice did not return to the original view")
	}
}

// Toggling views must keep you on the same edit, not reset to the top.
func TestKeys_ToggleKeepsSelectedEvent(t *testing.T) {
	m := ui.NewForTest(threeEvents())
	m.Send("G") // the third event, a.go seq 3

	want := m.SelectedSeq()
	m.Send("t")
	if got := m.SelectedSeq(); got != want {
		t.Errorf("after toggling, selected seq = %d, want %d", got, want)
	}
}

func TestKeys_ContextWidthNeverNegative(t *testing.T) {
	m := ui.NewForTest(threeEvents())

	if got := m.Send("-").Send("-").Send("-").Ctx(); got != 0 {
		t.Errorf("ctx = %d, want 0", got)
	}
	if got := m.Send("+").Ctx(); got <= 0 {
		t.Errorf("ctx = %d, want a positive width after +", got)
	}
}

func TestKeys_ContextClampsAtCapturedMaximum(t *testing.T) {
	m := ui.NewForTest(threeEvents())
	for i := 0; i < 40; i++ {
		m.Send("+")
	}
	if got := m.Ctx(); got > capture.MaxContext {
		t.Errorf("ctx = %d, want no more than the %d lines captured", got, capture.MaxContext)
	}
}

// J and K move between files, skipping the other edits in between.
func TestKeys_ShiftJKMovesBetweenFiles(t *testing.T) {
	m := ui.NewForTest(threeEvents()) // a.go, b.go, a.go

	m.Send("J")
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("after J, on %q, want b.go", got)
	}
	m.Send("J")
	if got := m.SelectedRel(); got != "a.go" {
		t.Errorf("after a second J, on %q, want a.go", got)
	}
	m.Send("K")
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("after K, on %q, want b.go", got)
	}
}

func TestKeys_TabSwitchesFocus(t *testing.T) {
	m := ui.NewForTest(threeEvents())
	if !m.ListFocused() {
		t.Fatal("the list should have focus initially")
	}
	if m.Send("tab").ListFocused() {
		t.Error("tab did not move focus to the diff pane")
	}
	if !m.Send("tab").ListFocused() {
		t.Error("tab did not move focus back to the list")
	}
}

// With the diff focused, j/k scroll the diff instead of changing the selection.
func TestKeys_JScrollsDiffWhenFocused(t *testing.T) {
	m := ui.NewForTest(manyHunks())
	m.Resize(80, 12) // short enough that the diff overflows and can scroll
	m.Send("tab")

	before := m.Cursor()
	m.Send("j")
	if m.Cursor() != before {
		t.Error("j changed the list selection while the diff was focused")
	}
	if m.DiffTop() == 0 {
		t.Error("j did not scroll the diff")
	}
}

func TestKeys_NextHunkJumps(t *testing.T) {
	m := ui.NewForTest(manyHunks())

	m.Send("n")
	first := m.DiffTop()
	if first == 0 {
		t.Fatal("n did not move to the next hunk")
	}
	m.Send("n")
	if m.DiffTop() <= first {
		t.Error("a second n did not advance further")
	}
	m.Send("N")
	if m.DiffTop() != first {
		t.Errorf("N should return to the previous hunk at %d, got %d", first, m.DiffTop())
	}
}

func TestKeys_HelpToggles(t *testing.T) {
	m := ui.NewForTest(threeEvents())
	// A phrase unique to the overlay: the footer also mentions "toggle view".
	const overlayOnly = "quit and clear this session"

	if strings.Contains(m.View(), overlayOnly) {
		t.Error("help shown before it was asked for")
	}
	if !strings.Contains(m.Send("?").View(), overlayOnly) {
		t.Error("? did not show help")
	}
	if strings.Contains(m.Send("?").View(), overlayOnly) {
		t.Error("? did not hide help again")
	}
}

func TestView_EmptySessionExplainsItself(t *testing.T) {
	m := ui.NewForTest(store.Session{})
	out := m.View()
	if !strings.Contains(strings.ToLower(out), "waiting") {
		t.Errorf("an empty panel should say it is waiting for edits, got:\n%s", out)
	}
}

func TestView_RendersWithinTerminalBounds(t *testing.T) {
	m := ui.NewForTest(threeEvents())
	m.Resize(80, 24)

	lines := strings.Split(m.View(), "\n")
	if len(lines) > 24 {
		t.Errorf("view is %d lines tall, want at most 24", len(lines))
	}
	for i, ln := range lines {
		if got := ui.VisibleWidth(ln); got > 80 {
			t.Errorf("line %d is %d cells wide, want at most 80", i, got)
		}
	}
}

func TestTail_PicksUpAppendedEvents(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("live")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go", Added: 1})

	m, err := ui.New("live")
	if err != nil {
		t.Fatal(err)
	}
	if got := m.RowCount(); got != 1 {
		t.Fatalf("rows = %d, want 1", got)
	}

	s.Append(capture.Event{Rel: "b.go", Added: 2})
	m.Reload()

	if got := m.RowCount(); got != 2 {
		t.Errorf("rows after reload = %d, want 2", got)
	}
}

// New events must not yank the selection out from under the reader.
func TestTail_KeepsSelectionWhileStreaming(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("live2")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go"})
	s.Append(capture.Event{Rel: "b.go"})

	m, _ := ui.New("live2")
	m.Send("j") // sitting on b.go, seq 2
	want := m.SelectedSeq()

	s.Append(capture.Event{Rel: "c.go"})
	m.Reload()

	if got := m.SelectedSeq(); got != want {
		t.Errorf("selection jumped to seq %d, want %d", got, want)
	}
}

func manyHunks() store.Session {
	hunk := func(start int) capture.Hunk {
		return capture.Hunk{
			OldStart: start, OldLines: 2, NewStart: start, NewLines: 2,
			Lines: []string{" keep", "-old", "+new"},
			Above: []string{"x", "y"}, Below: []string{"z"},
		}
	}
	return store.Session{
		Events: []capture.Event{{
			Seq: 1, Rel: "main.go", Added: 3, Removed: 3,
			Hunks: []capture.Hunk{hunk(10), hunk(40), hunk(80)},
		}},
		Prompts: map[string]string{},
	}
}

// Sitting on a file header, a reload must not slide the cursor onto the edit
// below it: a header and its first edit share a sequence number.
func TestReload_KeepsCursorOnFileHeader(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("hdr")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go", Added: 1})
	s.Append(capture.Event{Rel: "b.go", Added: 1})

	m, _ := ui.New("hdr")
	m.Send("t") // by file
	m.Send("g").Send("g")
	if !m.OnFileHeader() {
		t.Fatalf("expected to start on a file header, cursor=%d", m.Cursor())
	}

	want := m.Cursor()
	for i := 0; i < 3; i++ {
		m.Reload()
	}
	if got := m.Cursor(); got != want {
		t.Errorf("cursor drifted from %d to %d across reloads", want, got)
	}
	if !m.OnFileHeader() {
		t.Error("cursor slid off the file header")
	}
}

// J in by-file view lands on the next file's header, not its first edit.
func TestKeys_JumpFileLandsOnHeaderInByFileView(t *testing.T) {
	m := ui.NewForTest(threeEvents())
	m.Send("t").Send("g").Send("g")

	m.Send("J")
	if !m.OnFileHeader() {
		t.Errorf("J landed on row %d, which is not a file header", m.Cursor())
	}
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("J landed on %q, want b.go", got)
	}
}
