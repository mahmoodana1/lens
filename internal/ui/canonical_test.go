package ui_test

import (
	"strings"
	"testing"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
	"github.com/mahmood/lens/internal/ui"
)

// A log written before the session root was used as the yardstick holds two
// names for one file. The panel knows the root and the absolute path, so it
// names the file once rather than listing it twice.
func cdSession() store.Session {
	return store.Session{
		Meta: store.Meta{CWD: "/home/me/test"},
		Events: []capture.Event{
			{Seq: 1, Rel: "hangman/main.py", Path: "/home/me/test/hangman/main.py", Added: 33},
			{Seq: 2, Rel: "main.py", Path: "/home/me/test/hangman/main.py", Added: 14, Removed: 4},
			{Seq: 3, Rel: "game/ui.py", Path: "/home/me/test/hangman/game/ui.py", Added: 15},
			{Seq: 4, Rel: "hangman/game/ui.py", Path: "/home/me/test/hangman/game/ui.py", Added: 29},
		},
		Prompts: map[string]string{},
	}
}

func TestCanonical_OneFileIsListedOnce(t *testing.T) {
	m := browsing(cdSession())

	seen := map[string]int{}
	for _, r := range rowsOf(m) {
		if r.Kind == ui.RowFile {
			seen[r.Rel]++
		}
	}
	if len(seen) != 2 {
		t.Errorf("files = %v, want main.py and ui.py once each", seen)
	}
	for rel, n := range seen {
		if n > 1 {
			t.Errorf("%q is listed %d times", rel, n)
		}
	}
}

// The name it settles on is the one measured from the session root.
func TestCanonical_NamesFilesFromTheSessionRoot(t *testing.T) {
	m := browsing(cdSession())

	var rels []string
	for _, r := range rowsOf(m) {
		if r.Kind == ui.RowFile {
			rels = append(rels, r.Rel)
		}
	}
	want := map[string]bool{"hangman/main.py": true, "hangman/game/ui.py": true}
	for _, rel := range rels {
		if !want[rel] {
			t.Errorf("file named %q, want one of %v", rel, want)
		}
	}
}

// Both edits belong to the one file, so opening it shows all of them.
func TestCanonical_TheFileHoldsEveryEditToIt(t *testing.T) {
	m := browsing(cdSession())
	m.Resize(90, 16)

	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if !strings.Contains(head, "2 files") {
		t.Errorf("status line = %q, want 2 files", head)
	}

	m.Send("l") // into the first file
	edits := 0
	for _, r := range rowsOf(m) {
		if r.Kind == ui.RowEdit {
			edits++
		}
	}
	if edits != 2 {
		t.Errorf("edits inside the file = %d, want both", edits)
	}
}

// A session with no root recorded, or an event with no absolute path, keeps
// whatever name it was given: there is nothing better to measure against.
func TestCanonical_FallsBackToTheRecordedName(t *testing.T) {
	m := browsing(store.Session{
		Events: []capture.Event{{Seq: 1, Rel: "a.go", Added: 1}},
	})
	if got := m.SelectedRel(); got != "a.go" {
		t.Errorf("rel = %q, want a.go left alone", got)
	}

	m = browsing(store.Session{
		Meta:   store.Meta{CWD: "/root"},
		Events: []capture.Event{{Seq: 1, Rel: "kept.go", Added: 1}},
	})
	if got := m.SelectedRel(); got != "kept.go" {
		t.Errorf("rel = %q, want kept.go: there is no absolute path to re-measure", got)
	}
}

// A file outside the project keeps its absolute path rather than a ../.. climb.
func TestCanonical_FilesOutsideTheProjectKeepTheirFullPath(t *testing.T) {
	m := browsing(store.Session{
		Meta:   store.Meta{CWD: "/home/me/test"},
		Events: []capture.Event{{Seq: 1, Rel: "x", Path: "/etc/hosts", Added: 1}},
	})
	if got := m.SelectedRel(); got != "/etc/hosts" {
		t.Errorf("rel = %q, want the absolute path", got)
	}
}

// Logs written before hidden files were turned away still hold them, and the
// panel is the thing that has to stop showing them.
func TestCanonical_HiddenFilesAreNotListed(t *testing.T) {
	m := browsing(store.Session{
		Meta: store.Meta{CWD: "/home/me/test"},
		Events: []capture.Event{
			{Seq: 1, Rel: "main.py", Path: "/home/me/test/main.py", Added: 3},
			{Seq: 2, Rel: ".gitignore", Path: "/home/me/test/.gitignore", Added: 2},
			{Seq: 3, Rel: ".claude/settings.json", Path: "/home/me/test/.claude/settings.json", Added: 5},
			{Seq: 4, Rel: "src/app.py", Path: "/home/me/test/src/app.py", Added: 1},
		},
		Prompts: map[string]string{},
	})
	m.Resize(90, 16)

	for _, r := range rowsOf(m) {
		if strings.Contains(r.Rel, ".gitignore") || strings.Contains(r.Rel, ".claude") {
			t.Errorf("a hidden file is listed: %q", r.Rel)
		}
	}
	if got := fileCount(m); got != 2 {
		t.Errorf("files = %d, want 2", got)
	}
	head := strings.SplitN(ui.StripANSIForTest(m.View()), "\n", 2)[0]
	if !strings.Contains(head, "2 edits") {
		t.Errorf("status line = %q, want it to count only what it shows", head)
	}
}

// A project under a hidden directory still shows its own files.
func TestCanonical_AProjectLivingSomewhereHiddenStillShows(t *testing.T) {
	m := browsing(store.Session{
		Meta: store.Meta{CWD: "/home/me/.config/nvim"},
		Events: []capture.Event{
			{Seq: 1, Rel: "init.lua", Path: "/home/me/.config/nvim/init.lua", Added: 3},
			{Seq: 2, Rel: "x", Path: "/home/me/.config/nvim/.git/config", Added: 1},
		},
		Prompts: map[string]string{},
	})

	if got := fileCount(m); got != 1 {
		t.Errorf("files = %d, want just init.lua", got)
	}
	if got := m.SelectedRel(); got != "init.lua" {
		t.Errorf("showing %q, want init.lua", got)
	}
}

// A log written before the walk was confined still holds what it swept up
// elsewhere. Those came from the walk, which had no business leaving the
// project; an edit made deliberately to a file outside it is another matter.
func TestCanonical_StrayWalkedFilesAreNotListed(t *testing.T) {
	m := browsing(store.Session{
		Meta: store.Meta{CWD: "/home/me/proj"},
		Events: []capture.Event{
			{Seq: 1, Rel: "main.py", Path: "/home/me/proj/main.py", Tool: "Edit", Added: 3},
			{Seq: 2, Rel: "/tmp/claude-1000/bash-edit-diff/abc/HEAD", Path: "/tmp/claude-1000/bash-edit-diff/abc/HEAD", Tool: "Bash", Added: 1},
			{Seq: 3, Rel: "/tmp/cm-fs-preload-297136.js", Path: "/tmp/cm-fs-preload-297136.js", Tool: "Bash", Added: 1},
			{Seq: 4, Rel: "src/app.py", Path: "/home/me/proj/src/app.py", Tool: "Bash", Added: 1},
		},
		Prompts: map[string]string{},
	})
	m.Resize(90, 16)

	for _, r := range rowsOf(m) {
		if strings.Contains(r.Rel, "/tmp/") {
			t.Errorf("a file the walk strayed to is listed: %q", r.Rel)
		}
	}
	if got := fileCount(m); got != 2 {
		t.Errorf("files = %d, want main.py and src/app.py", got)
	}
}

// An edit made on purpose to a file outside the project is deliberate, and
// still worth seeing.
func TestCanonical_DeliberateEditsOutsideTheProjectAreKept(t *testing.T) {
	m := browsing(store.Session{
		Meta: store.Meta{CWD: "/home/me/proj"},
		Events: []capture.Event{
			{Seq: 1, Rel: "/etc/hosts", Path: "/etc/hosts", Tool: "Edit", Added: 1},
		},
		Prompts: map[string]string{},
	})

	if got := fileCount(m); got != 1 {
		t.Errorf("files = %d, want the deliberate edit kept", got)
	}
	if got := m.SelectedRel(); got != "/etc/hosts" {
		t.Errorf("showing %q, want /etc/hosts", got)
	}
}

// A deletion must not read like an ordinary edit that happened to remove a lot.
func TestView_ADeletedFileSaysSo(t *testing.T) {
	m := browsing(store.Session{
		Meta: store.Meta{CWD: "/p"},
		Events: []capture.Event{{
			Seq: 1, Rel: "gone.go", Path: "/p/gone.go", Tool: "Bash", Kind: "delete", Removed: 3,
			Hunks: []capture.Hunk{{OldStart: 1, OldLines: 3, NewStart: 0, Lines: []string{
				"-package main", "-", "-func main() {}",
			}}},
		}},
		Prompts: map[string]string{},
	})
	m.Resize(100, 16)

	out := ui.StripANSIForTest(m.View())
	if !strings.Contains(out, "deleted") {
		t.Errorf("nothing says the file was deleted:\n%s", out)
	}
}
