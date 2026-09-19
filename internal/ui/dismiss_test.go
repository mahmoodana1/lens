package ui_test

import (
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
	"github.com/mahmood/lens/internal/ui"
)

// twoFilesWithHunks: a.go edited twice with two hunks then one, b.go once.
func twoFilesWithHunks() store.Session {
	h := func(line int, first string) capture.Hunk {
		return capture.Hunk{
			OldStart: line, OldLines: 1, NewStart: line, NewLines: 2,
			Lines: []string{" context", first, " more"},
		}
	}
	return store.Session{
		Events: []capture.Event{
			{Seq: 1, Rel: "a.go", Tool: "Edit", Added: 2, Hunks: []capture.Hunk{h(42, "+first"), h(88, "+second")}},
			{Seq: 2, Rel: "a.go", Tool: "Edit", Added: 1, Hunks: []capture.Hunk{h(12, "+third")}},
			{Seq: 3, Rel: "b.go", Tool: "Write", Added: 1, Hunks: []capture.Hunk{h(1, "+lonely")}},
		},
		Prompts: map[string]string{},
	}
}

// dd clears a file out of the list. The file on disk is not touched: this is
// about what you still have left to read.
func TestDismiss_DDRemovesAFileFromTheList(t *testing.T) {
	m := browsing(twoFilesWithHunks())

	m.Send("d").Send("d")

	for _, r := range rowsOf(m) {
		if r.Rel == "a.go" {
			t.Errorf("a.go is still listed as %v", r.Kind)
		}
	}
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("after dd the cursor is on %q, want the next file", got)
	}
}

// A lone d waits for the second one, the way a lone g does.
func TestDismiss_ASingleDDeletesNothing(t *testing.T) {
	m := browsing(twoFilesWithHunks()).Send("d")

	if fileCount(m) != 2 {
		t.Error("a single d removed something")
	}
	if got := m.SelectedRel(); got != "a.go" {
		t.Errorf("d moved the cursor to %q", got)
	}
}

func TestDismiss_DDRemovesAHunkFromTheListAndTheDiff(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Resize(90, 16)
	m.Send("l") // inside a.go
	m.Send("j") // its first hunk, "+ first"
	before := diffText(m)
	if !strings.Contains(before, "+ first") {
		t.Fatalf("setup: the diff does not show the hunk:\n%s", before)
	}

	m.Send("d").Send("d")

	for _, r := range rowsOf(m) {
		if strings.Contains(r.Label, "+ first") {
			t.Error("the hunk is still listed")
		}
	}
	if after := diffText(m); strings.Contains(after, "+ first") {
		t.Errorf("the hunk is still in the diff:\n%s", after)
	}
	if !strings.Contains(diffText(m), "+ second") {
		t.Error("dismissing one hunk took the other with it")
	}
}

func TestDismiss_DDRemovesAnEditAndItsHunks(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Send("l") // inside a.go, on its first edit

	m.Send("d").Send("d")

	for _, r := range rowsOf(m) {
		if r.Event != nil && r.Event.Seq == 1 {
			t.Errorf("edit 1 is still listed as %v %q", r.Kind, r.Label)
		}
	}
	if len(rowsOf(m)) == 0 {
		t.Error("dismissing one edit emptied the file")
	}
}

// Dismissing everything a file holds takes the file with it.
func TestDismiss_AFileGoesWhenItsLastEditDoes(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Send("j")           // b.go, which has one edit
	m.Send("l")           // inside it
	m.Send("d").Send("d") // its only edit

	if got := m.OpenFile(); got != "" {
		t.Errorf("still inside %q, which has nothing left to show", got)
	}
	for _, r := range rowsOf(m) {
		if r.Rel == "b.go" {
			t.Error("b.go is still listed with all its edits dismissed")
		}
	}
}

// u walks back one dd at a time, so you can return to any earlier point.
func TestDismiss_UndoRestoresOneDismissalAtATime(t *testing.T) {
	m := browsing(twoFilesWithHunks())

	m.Send("d").Send("d") // a.go
	m.Send("d").Send("d") // b.go
	if fileCount(m) != 0 {
		t.Fatalf("setup: %d files left, want 0", fileCount(m))
	}

	m.Send("u")
	if got := fileCount(m); got != 1 {
		t.Fatalf("after one undo, %d files, want 1", got)
	}
	if got := m.SelectedRel(); got != "b.go" {
		t.Errorf("undo brought back %q, want b.go — the last one dismissed", got)
	}
	m.Send("u")
	if got := fileCount(m); got != 2 {
		t.Errorf("after a second undo, %d files, want 2", got)
	}
}

func TestDismiss_UndoWithNothingDismissedDoesNothing(t *testing.T) {
	m := browsing(twoFilesWithHunks()).Send("u")

	if got := fileCount(m); got != 2 {
		t.Errorf("files = %d, want 2 left alone", got)
	}
}

func TestDismiss_RedoPutsItBack(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Send("d").Send("d").Send("u")

	m.Send("ctrl+r")
	if got := fileCount(m); got != 1 {
		t.Errorf("files = %d after redo, want 1", got)
	}
	m.Send("ctrl+r") // nothing left to redo
	if got := fileCount(m); got != 1 {
		t.Errorf("files = %d, want redo to stop when the trail runs out", got)
	}
}

// A fresh dismissal is a new branch: what was undone is no longer ahead of you.
func TestDismiss_ANewDeleteClearsTheRedoTrail(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Send("d").Send("d") // a.go
	m.Send("u")           // back

	m.Send("d").Send("d") // a.go again, a new branch
	m.Send("ctrl+r")

	if got := fileCount(m); got != 1 {
		t.Errorf("files = %d; redo should have had nothing to replay", got)
	}
}

// New edits keep streaming in while the panel is open; a reload must not undo
// what you cleared away.
func TestDismiss_SurvivesAReload(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("dismiss")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go", Added: 1})
	s.Append(capture.Event{Rel: "b.go", Added: 1})

	m, err := ui.New("dismiss")
	if err != nil {
		t.Fatal(err)
	}
	m.Send("esc")
	m.Send("d").Send("d")

	m.Reload()

	for _, r := range rowsOf(m) {
		if r.Rel == "a.go" {
			t.Error("a reload brought back a dismissed file")
		}
	}
}

// Rows that vanish without explanation are unnerving, so the count says so.
func TestDismiss_StatusLineCountsWhatIsHidden(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Resize(90, 16)

	m.Send("d").Send("d")

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if !strings.Contains(head, "1 hidden") {
		t.Errorf("status line = %q, want it to report 1 hidden", head)
	}
	m.Send("u")
	if head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]; strings.Contains(head, "hidden") {
		t.Errorf("status line = %q, want the note gone once nothing is hidden", head)
	}
}

// diffText is the diff pane as plain text.
func diffText(m *ui.Model) string {
	var b strings.Builder
	for _, ln := range strings.Split(ui.StripANSIForTest(m.View()), "\n") {
		if i := strings.Index(ln, "│"); i >= 0 {
			ln = ln[i+len("│"):]
		}
		b.WriteString(ln)
		b.WriteString("\n")
	}
	return b.String()
}

// d and g are both chords; pressing one then the other must do neither.
func TestDismiss_DThenGIsNeitherChord(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Send("j") // b.go

	m.Send("d").Send("g").Send("g")

	if got := fileCount(m); got != 2 {
		t.Errorf("files = %d, want 2: dg deleted something", got)
	}
	if got := m.SelectedRel(); got != "a.go" {
		t.Errorf("on %q; the gg after the stray d should still reach the top", got)
	}
}

// Dismissing the last file leaves the panel saying so rather than going blank.
func TestDismiss_ClearingEverythingIsNotAnError(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Resize(90, 16)

	m.Send("d").Send("d")
	m.Send("d").Send("d")

	out := ui.StripANSIForTest(m.View())
	if strings.Contains(out, "panic") || fileCount(m) != 0 {
		t.Errorf("clearing the list broke it:\n%s", out)
	}
	if !strings.Contains(out, "2 hidden") {
		t.Errorf("the status line should say what became of them:\n%s", strings.SplitN(out, "\n", 2)[0])
	}
	m.Send("u").Send("u")
	if got := fileCount(m); got != 2 {
		t.Errorf("files = %d after undoing both, want 2", got)
	}
}

// The heading over an opened file counts what is left in it, not what was.
func TestDismiss_FileHeadingCountsWhatRemains(t *testing.T) {
	m := browsing(twoFilesWithHunks())
	m.Resize(90, 16)
	m.Send("l") // inside a.go: two edits, +3

	m.Send("d").Send("d") // the first edit, +2

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if !strings.Contains(head, "1 edit") || strings.Contains(head, "2 edits") {
		t.Errorf("heading = %q, want it to count the one edit left", head)
	}
	if !strings.Contains(head, "+1") {
		t.Errorf("heading = %q, want the totals of what remains", head)
	}
}

// Reopening the popup over a session keeps what that session cleared away.
func TestDismiss_IsRememberedAcrossPopups(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("keep")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go", Added: 1})
	s.Append(capture.Event{Rel: "b.go", Added: 1})

	first, err := ui.New("keep")
	if err != nil {
		t.Fatal(err)
	}
	first.Send("esc")
	first.Send("d").Send("d") // a.go

	again, err := ui.New("keep") // the popup closed and was opened again
	if err != nil {
		t.Fatal(err)
	}
	again.Send("esc")

	for _, r := range rowsOf(again) {
		if r.Rel == "a.go" {
			t.Error("reopening the panel brought back a dismissed file")
		}
	}
	if got := again.HiddenCount(); got != 1 {
		t.Errorf("hidden = %d, want the one dismissal carried over", got)
	}
}

// Undo works on a trail made before the popup was closed.
func TestDismiss_UndoReachesBackPastAReopen(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("trail")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go", Added: 1})
	s.Append(capture.Event{Rel: "b.go", Added: 1})

	first, _ := ui.New("trail")
	first.Send("esc")
	first.Send("d").Send("d")

	again, _ := ui.New("trail")
	again.Send("esc")
	again.Send("u")

	if got := fileCount(again); got != 2 {
		t.Errorf("files = %d after undoing, want both back", got)
	}
	if got := again.HiddenCount(); got != 0 {
		t.Errorf("hidden = %d, want 0", got)
	}
}

// One session's dismissals are its own: another session opens untouched.
func TestDismiss_DoesNotLeakIntoAnotherSession(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	for _, id := range []string{"one", "two"} {
		s, _ := store.Open(id)
		s.EnsureMeta("/p")
		s.Append(capture.Event{Rel: "a.go", Added: 1})
		s.Append(capture.Event{Rel: "b.go", Added: 1})
	}

	one, _ := ui.New("one")
	one.Send("esc")
	one.Send("d").Send("d")

	two, _ := ui.New("two")
	two.Send("esc")

	if got := two.HiddenCount(); got != 0 {
		t.Errorf("session two starts with %d hidden; it should keep its own history", got)
	}
	if got := fileCount(two); got != 2 {
		t.Errorf("session two shows %d files, want 2", got)
	}
}

// The tail reloads four times a second; it must not undo the reader's dd.
func TestDismiss_ReloadDoesNotClobberTheTrail(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())
	s, _ := store.Open("reload")
	s.EnsureMeta("/p")
	s.Append(capture.Event{Rel: "a.go", Added: 1})
	s.Append(capture.Event{Rel: "b.go", Added: 1})

	m, _ := ui.New("reload")
	m.Send("esc")
	m.Send("d").Send("d")
	for range 3 {
		m.Reload()
	}

	if got := m.HiddenCount(); got != 1 {
		t.Errorf("hidden = %d after reloads, want 1", got)
	}
	m.Send("u")
	if got := m.HiddenCount(); got != 0 {
		t.Errorf("hidden = %d after undo, want 0", got)
	}
	m.Reload()
	if got := m.HiddenCount(); got != 0 {
		t.Errorf("a reload after undo brought the dismissal back: hidden = %d", got)
	}
}

// The status line counts what the list is showing, in the plural it has earned.
func TestDismiss_StatusLineCountsWhatIsLeft(t *testing.T) {
	m := browsing(twoFilesWithHunks()) // a.go twice, b.go once
	m.Resize(90, 16)

	m.Send("d").Send("d") // a.go and both its edits

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if !strings.Contains(head, "1 edit ") {
		t.Errorf("status line = %q, want it to count the one edit still shown", head)
	}
	if !strings.Contains(head, "1 file ") {
		t.Errorf("status line = %q, want \"1 file\", not \"1 files\"", head)
	}
}
