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

func TestBuildRows_Timeline(t *testing.T) {
	rows := ui.BuildRows(threeEvents(), ui.Timeline, "")

	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].Event.Seq != 1 || rows[2].Event.Seq != 3 {
		t.Error("timeline rows are not in chronological order")
	}
	for i, r := range rows {
		if r.Kind != ui.RowEvent {
			t.Errorf("row %d kind = %v, want RowEvent", i, r.Kind)
		}
	}
}

func TestBuildRows_ByFileAggregates(t *testing.T) {
	rows := ui.BuildRows(threeEvents(), ui.ByFile, "")

	// a.go header, its two edits, b.go header, its one edit.
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5", len(rows))
	}
	if rows[0].Kind != ui.RowFileHeader || rows[0].Rel != "a.go" {
		t.Fatalf("first row = %v %q, want a.go header", rows[0].Kind, rows[0].Rel)
	}
	if rows[0].Added != 5 || rows[0].Removed != 1 {
		t.Errorf("a.go = +%d -%d, want +5 -1", rows[0].Added, rows[0].Removed)
	}
	if rows[0].Edits != 2 {
		t.Errorf("a.go edits = %d, want 2", rows[0].Edits)
	}
	if rows[3].Kind != ui.RowFileHeader || rows[3].Rel != "b.go" {
		t.Errorf("row 3 = %v %q, want b.go header", rows[3].Kind, rows[3].Rel)
	}
}

// Files keep first-appearance order so the list does not reshuffle as edits stream in.
func TestBuildRows_ByFileKeepsFirstAppearanceOrder(t *testing.T) {
	s := store.Session{Events: []capture.Event{
		{Seq: 1, Rel: "z.go"},
		{Seq: 2, Rel: "a.go"},
		{Seq: 3, Rel: "z.go"},
	}}
	rows := ui.BuildRows(s, ui.ByFile, "")

	if rows[0].Rel != "z.go" {
		t.Errorf("first file = %q, want z.go (seen first)", rows[0].Rel)
	}
}

func TestBuildRows_EmptySession(t *testing.T) {
	if rows := ui.BuildRows(store.Session{}, ui.ByFile, ""); len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
	if rows := ui.BuildRows(store.Session{}, ui.Timeline, ""); len(rows) != 0 {
		t.Errorf("rows = %d, want 0", len(rows))
	}
}

// Every row in either view must resolve to an event, or the diff pane goes blank.
func TestBuildRows_HeadersCarryTheirFirstEvent(t *testing.T) {
	rows := ui.BuildRows(threeEvents(), ui.ByFile, "")
	for i, r := range rows {
		if r.Event == nil {
			t.Errorf("row %d (%v %q) has no event to display", i, r.Kind, r.Rel)
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
	rows := ui.BuildRows(mixedFiles(), ui.Timeline, "uik")

	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if rows[0].Rel != "internal/ui/keys.go" {
		t.Errorf("kept %q, want internal/ui/keys.go", rows[0].Rel)
	}
}

// The list is a timeline, so narrowing it must not reshuffle what is left.
func TestBuildRows_FilterKeepsChronologicalOrder(t *testing.T) {
	rows := ui.BuildRows(mixedFiles(), ui.Timeline, "go")

	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(rows))
	}
	if rows[0].Event.Seq != 1 || rows[1].Event.Seq != 2 || rows[2].Event.Seq != 3 {
		t.Error("filtering reordered the timeline")
	}
}

func TestBuildRows_EmptyFilterKeepsEverything(t *testing.T) {
	if got := len(ui.BuildRows(mixedFiles(), ui.Timeline, "")); got != 3 {
		t.Errorf("rows = %d, want 3", got)
	}
}

// A file whose edits are all filtered out should not leave a heading behind.
func TestBuildRows_FilterDropsEmptyFileHeadings(t *testing.T) {
	rows := ui.BuildRows(mixedFiles(), ui.ByFile, "store")

	for _, r := range rows {
		if r.Rel != "internal/store/store.go" {
			t.Errorf("row %q survived a filter it does not match", r.Rel)
		}
	}
	if len(rows) != 2 { // one heading, one edit
		t.Errorf("rows = %d, want 2", len(rows))
	}
}

// The positions are what the list highlights.
func TestBuildRows_FilterRecordsMatchPositions(t *testing.T) {
	rows := ui.BuildRows(mixedFiles(), ui.Timeline, "keys")

	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	if len(rows[0].Match) != 4 {
		t.Errorf("match positions = %v, want 4 of them", rows[0].Match)
	}
}
