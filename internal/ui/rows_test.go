package ui_test

import (
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
	"github.com/mahmood/lens/internal/ui"
)

func threeEvents() store.Session {
	return store.Session{
		Events: []capture.Event{
			{Seq: 1, Rel: "a.go", Added: 3, Removed: 1, Tool: "Edit"},
			{Seq: 2, Rel: "b.go", Added: 1, Tool: "Write"},
			{Seq: 3, Rel: "a.go", Added: 2, Tool: "Edit"},
		},
		Prompts: map[string]string{},
	}
}

// The top level is files, one row each, under the directory they live in.
func TestBuildRows_GroupsFilesUnderTheirDirectory(t *testing.T) {
	s := store.Session{Events: []capture.Event{
		{Seq: 1, Rel: "internal/ui/keys.go", Added: 1},
		{Seq: 2, Rel: "internal/ui/view.go", Added: 1},
		{Seq: 3, Rel: "internal/store/store.go", Added: 1},
	}}
	rows := ui.BuildRows(s, "", "", nil)

	want := []struct {
		kind  ui.RowKind
		label string
	}{
		{ui.RowDir, "internal/ui/"},
		{ui.RowFile, "keys.go"},
		{ui.RowFile, "view.go"},
		{ui.RowDir, "internal/store/"},
		{ui.RowFile, "store.go"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows = %v, want %v", labels(rows), want)
	}
	for i, w := range want {
		if rows[i].Kind != w.kind || rows[i].Label != w.label {
			t.Errorf("row %d = %v %q, want %v %q", i, rows[i].Kind, rows[i].Label, w.kind, w.label)
		}
	}
}

// A file row is the whole file: every edit to it, added up.
func TestBuildRows_FileRowSumsItsEdits(t *testing.T) {
	rows := ui.BuildRows(threeEvents(), "", "", nil)

	a := rows[1] // ./ heading, then a.go
	if a.Kind != ui.RowFile || a.Rel != "a.go" {
		t.Fatalf("row 1 = %v %q, want the a.go file row", a.Kind, a.Rel)
	}
	if a.Added != 5 || a.Removed != 1 || a.Edits != 2 {
		t.Errorf("a.go = +%d -%d over %d edits, want +5 -1 over 2", a.Added, a.Removed, a.Edits)
	}
	if a.Event == nil || a.Event.Seq != 3 {
		t.Error("a file previews its latest edit")
	}
}

// Directory headings are signposts, not destinations.
func TestBuildRows_DirectoryHeadingsAreNotSelectable(t *testing.T) {
	rows := ui.BuildRows(threeEvents(), "", "", nil)

	if rows[0].Kind != ui.RowDir {
		t.Fatalf("row 0 = %v, want a directory heading", rows[0].Kind)
	}
	if rows[0].Selectable() {
		t.Error("a directory heading should not be selectable")
	}
	if !rows[1].Selectable() {
		t.Error("a file row should be selectable")
	}
}

func hunkAt(line int, first string) capture.Hunk {
	return capture.Hunk{
		OldStart: line, OldLines: 1, NewStart: line, NewLines: 2,
		Lines: []string{" context", first},
	}
}

func editedTwice() store.Session {
	return store.Session{
		Events: []capture.Event{
			{Seq: 1, Rel: "keys.go", Tool: "Edit", Added: 2, Hunks: []capture.Hunk{
				hunkAt(42, `+	case "l", "right":`), hunkAt(88, "-	m.toggleView()"),
			}},
			{Seq: 2, Rel: "keys.go", Tool: "Edit", Added: 1, Hunks: []capture.Hunk{
				hunkAt(12, "+	func fileOf() {"),
			}},
		},
		Prompts: map[string]string{},
	}
}

// Opening a file replaces the list with that file's own history.
func TestBuildRows_InsideAFileListsItsEditsAndHunks(t *testing.T) {
	rows := ui.BuildRows(editedTwice(), "keys.go", "", nil)

	want := []ui.RowKind{ui.RowEdit, ui.RowHunk, ui.RowHunk, ui.RowEdit, ui.RowHunk}
	if len(rows) != len(want) {
		t.Fatalf("rows = %v, want an edit then its hunks, twice", labels(rows))
	}
	for i, k := range want {
		if rows[i].Kind != k {
			t.Errorf("row %d = %v, want %v", i, rows[i].Kind, k)
		}
	}
	if rows[0].Event.Seq != 1 || rows[3].Event.Seq != 2 {
		t.Error("the edits should be in the order they were made")
	}
}

// Nothing else is in the list: that is what "opening a file" means.
func TestBuildRows_InsideAFileHidesTheOtherFiles(t *testing.T) {
	for _, r := range ui.BuildRows(threeEvents(), "a.go", "", nil) {
		if r.Rel != "a.go" {
			t.Errorf("row %q belongs to another file", r.Rel)
		}
	}
}

// A hunk is named by where it is and what it did, which beats "@@ -42,1 +42,2 @@".
func TestBuildRows_HunkRowsCarryTheirLineAndFirstChange(t *testing.T) {
	rows := ui.BuildRows(editedTwice(), "keys.go", "", nil)

	if got := rows[1].Label; got != `42  + case "l", "right":` {
		t.Errorf("hunk label = %q", got)
	}
	if got := rows[2].Label; got != "88  - m.toggleView()" {
		t.Errorf("second hunk label = %q", got)
	}
	if rows[1].Hunk != 0 || rows[2].Hunk != 1 {
		t.Errorf("hunk indexes = %d, %d; want 0, 1", rows[1].Hunk, rows[2].Hunk)
	}
}

// One edit needs no heading to separate it from the others.
func TestBuildRows_ASingleEditFileListsOnlyItsHunks(t *testing.T) {
	s := store.Session{Events: []capture.Event{
		{Seq: 1, Rel: "keys.go", Tool: "Write", Hunks: []capture.Hunk{hunkAt(1, "+package ui")}},
	}}
	rows := ui.BuildRows(s, "keys.go", "", nil)

	if len(rows) != 1 || rows[0].Kind != ui.RowHunk {
		t.Errorf("rows = %v, want just the one hunk", labels(rows))
	}
}

// An edit with no hunks still has to be reachable, or it vanishes.
func TestBuildRows_AnEditWithNoHunksStillGetsARow(t *testing.T) {
	s := store.Session{Events: []capture.Event{{Seq: 1, Rel: "a.go", Tool: "Write"}}}

	rows := ui.BuildRows(s, "a.go", "", nil)
	if len(rows) != 1 || rows[0].Kind != ui.RowEdit {
		t.Errorf("rows = %v, want the edit itself", labels(rows))
	}
}

// Opening a file that the session never touched leaves nothing to show.
func TestBuildRows_InsideAnUnknownFile(t *testing.T) {
	if rows := ui.BuildRows(threeEvents(), "gone.go", "", nil); len(rows) != 0 {
		t.Errorf("rows = %v, want none", labels(rows))
	}
}

// Files keep first-appearance order so the list does not reshuffle as edits
// stream in, and so does the directory they are grouped under.
func TestBuildRows_KeepsFirstAppearanceOrder(t *testing.T) {
	s := store.Session{Events: []capture.Event{
		{Seq: 1, Rel: "z/z.go"},
		{Seq: 2, Rel: "a/a.go"},
		{Seq: 3, Rel: "z/y.go"},
	}}
	rows := ui.BuildRows(s, "", "", nil)

	if got := labels(rows); got[0] != "z/" || got[3] != "a/" {
		t.Errorf("order = %v, want z/ before a/", got)
	}
}

func TestBuildRows_EmptySession(t *testing.T) {
	if rows := ui.BuildRows(store.Session{}, "", "", nil); len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

// Every selectable row must resolve to an event, or the diff pane goes blank.
func TestBuildRows_EverySelectableRowHasAnEvent(t *testing.T) {
	for _, rows := range [][]ui.Row{
		ui.BuildRows(threeEvents(), "", "", nil),
		ui.BuildRows(editedTwice(), "keys.go", "", nil),
	} {
		for i, r := range rows {
			if r.Selectable() && r.Event == nil {
				t.Errorf("row %d (%v %q) has no event to display", i, r.Kind, r.Label)
			}
		}
	}
}

func mixedFiles() store.Session {
	return store.Session{
		Events: []capture.Event{
			{Seq: 1, Rel: "internal/ui/keys.go", Added: 3, Tool: "Edit"},
			{Seq: 2, Rel: "internal/store/store.go", Added: 1, Tool: "Edit"},
			{Seq: 3, Rel: "internal/ui/view.go", Added: 2, Tool: "Edit"},
		},
		Prompts: map[string]string{},
	}
}

func TestBuildRows_FilterKeepsOnlyMatchingFiles(t *testing.T) {
	rows := ui.BuildRows(mixedFiles(), "", "uik", nil)

	if len(rows) != 2 { // the heading and the one file
		t.Fatalf("rows = %v, want one file and its heading", labels(rows))
	}
	if rows[1].Rel != "internal/ui/keys.go" {
		t.Errorf("kept %q, want internal/ui/keys.go", rows[1].Rel)
	}
}

// A directory whose files are all filtered out must not leave a heading behind.
func TestBuildRows_FilterDropsEmptyDirectories(t *testing.T) {
	for _, r := range ui.BuildRows(mixedFiles(), "", "store", nil) {
		if r.Kind == ui.RowDir && r.Label != "internal/store/" {
			t.Errorf("heading %q survived a filter nothing under it matches", r.Label)
		}
	}
}

// Narrowing the list must not reshuffle what is left.
func TestBuildRows_FilterKeepsOrder(t *testing.T) {
	rows := ui.BuildRows(mixedFiles(), "", "go", nil)

	var seqs []int
	for _, r := range rows {
		if r.Kind == ui.RowFile {
			seqs = append(seqs, r.Event.Seq)
		}
	}
	if len(seqs) != 3 || seqs[0] != 1 || seqs[1] != 3 || seqs[2] != 2 {
		t.Errorf("file order = %v, want the order they were first touched, grouped by directory", seqs)
	}
}

func TestBuildRows_EmptyFilterKeepsEverything(t *testing.T) {
	n := 0
	for _, r := range ui.BuildRows(mixedFiles(), "", "", nil) {
		if r.Kind == ui.RowFile {
			n++
		}
	}
	if n != 3 {
		t.Errorf("files = %d, want 3", n)
	}
}

// The positions are what the list highlights, and they index the label shown —
// the bare file name, not the path it was matched against.
func TestBuildRows_FilterMarksTheNameItShows(t *testing.T) {
	rows := ui.BuildRows(mixedFiles(), "", "keys", nil)

	if len(rows) != 2 {
		t.Fatalf("rows = %v, want one file and its heading", labels(rows))
	}
	for _, p := range rows[1].Match {
		if p < 0 || p >= len([]rune(rows[1].Label)) {
			t.Errorf("match position %d is outside the label %q", p, rows[1].Label)
		}
	}
	if len(rows[1].Match) != 4 {
		t.Errorf("match positions = %v, want the four letters of keys", rows[1].Match)
	}
}

func labels(rows []ui.Row) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.Label)
	}
	return out
}
