package ui

import (
	"fmt"
	"strings"

	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
)

// RowKind is what a line in the list stands for. The list has two levels: the
// files a session touched, and — once one is opened — what happened inside it.
type RowKind int

const (
	// RowDir heads a group of files. It is a signpost, not a destination.
	RowDir RowKind = iota
	// RowFile is one file, summing every edit made to it.
	RowFile
	// RowEdit is one edit to the opened file.
	RowEdit
	// RowHunk is one hunk of one edit to the opened file.
	RowHunk
)

// Row is one line in the list pane.
type Row struct {
	Kind    RowKind
	Event   *capture.Event // the edit whose diff this row shows
	Rel     string         // the file this row belongs to
	Label   string         // what the list draws
	Added   int
	Removed int
	Edits   int   // RowFile: how many edits the file collected
	Hunk    int   // RowHunk: which hunk of Event it is
	Match   []int // positions in Label the filter matched

	dir string // RowFile: the heading it sits under, for the status line
}

// Selectable reports whether the cursor can land on a row.
func (r Row) Selectable() bool { return r.Kind != RowDir }

// BuildRows lays a session out for the list pane.
//
// With no file open it is the file list: one row per file, grouped under the
// directory it lives in, in the order the files were first touched. Opening a
// file replaces all of that with that file's own history — each edit and the
// hunks it made — so a file with a past can be read one change at a time.
//
// Narrowing the list must not reshuffle it, so the filter never reorders; the
// ranking instead decides where the cursor lands.
func BuildRows(s store.Session, openFile, filter string, hidden Hidden) []Row {
	if openFile != "" {
		return insideFile(s, openFile, hidden)
	}
	return fileList(s, filter, hidden)
}

func fileList(s store.Session, filter string, hidden Hidden) []Row {
	type file struct {
		rel            string
		added, removed int
		edits          int
		latest         *capture.Event
	}
	dirOrder := []string{}
	byDir := map[string][]*file{}
	files := map[string]*file{}

	for i := range s.Events {
		e := &s.Events[i]
		if _, _, ok := Match(filter, e.Rel); !ok {
			continue
		}
		// A file with nothing left to show is not a file the list shows.
		if hidden.Empty(e) {
			continue
		}
		f, ok := files[e.Rel]
		if !ok {
			dir, _ := SplitDir(e.Rel)
			f = &file{rel: e.Rel}
			files[e.Rel] = f
			if _, seen := byDir[dir]; !seen {
				dirOrder = append(dirOrder, dir)
			}
			byDir[dir] = append(byDir[dir], f)
		}
		f.added += e.Added
		f.removed += e.Removed
		f.edits++
		f.latest = e
	}

	rows := make([]Row, 0, len(files)+len(dirOrder))
	for _, dir := range dirOrder {
		rows = append(rows, Row{Kind: RowDir, Label: dir, dir: dir})
		for _, f := range byDir[dir] {
			_, name := SplitDir(f.rel)
			rows = append(rows, Row{
				Kind: RowFile, Rel: f.rel, Label: name,
				// The newest edit, so a file previews what just happened to it.
				Event:   f.latest,
				Added:   f.added,
				Removed: f.removed,
				Edits:   f.edits,
				Match:   nameMatches(filter, f.rel, name),
				dir:     dir,
			})
		}
	}
	return rows
}

func insideFile(s store.Session, rel string, hidden Hidden) []Row {
	var events []*capture.Event
	for i := range s.Events {
		if s.Events[i].Rel == rel && !hidden.Empty(&s.Events[i]) {
			events = append(events, &s.Events[i])
		}
	}
	if len(events) == 0 {
		return nil
	}

	rows := []Row{}
	for _, e := range events {
		// One edit needs no heading to tell it apart from the others, and an
		// edit that recorded no hunks at all is only its heading.
		if len(events) > 1 || len(e.Hunks) == 0 {
			rows = append(rows, Row{
				Kind: RowEdit, Event: e, Rel: rel, Hunk: -1,
				Label:   fmt.Sprintf("%s  %s", e.Time.Format("15:04:05"), orDash(e.Tool)),
				Added:   e.Added,
				Removed: e.Removed,
			})
		}
		for i, h := range e.Hunks {
			if hidden.Hunk(rel, e.Seq, i) {
				continue
			}
			rows = append(rows, Row{
				Kind: RowHunk, Event: e, Rel: rel, Hunk: i,
				Label: hunkLabel(h),
			})
		}
	}
	return rows
}

// hunkLabel names a hunk by where it is and what it did there, which says more
// at a glance than the line counts in an @@ header.
func hunkLabel(h capture.Hunk) string {
	line, body := h.NewStart, ""
	for _, raw := range h.Lines {
		if raw == "" || (raw[0] != '+' && raw[0] != '-') {
			continue
		}
		body = raw[:1] + " " + strings.TrimSpace(raw[1:])
		break
	}
	if body == "" {
		return fmt.Sprintf("%d", line)
	}
	return fmt.Sprintf("%d  %s", line, body)
}

// nameMatches maps the filter's hits on the full path onto the bare name the
// list actually draws, so the underlines land under the right letters.
func nameMatches(filter, rel, name string) []int {
	pos, _, _ := Match(filter, rel)
	if len(pos) == 0 {
		return nil
	}
	off := len([]rune(rel)) - len([]rune(name))
	out := pos[:0:0]
	for _, p := range pos {
		if p >= off {
			out = append(out, p-off)
		}
	}
	return out
}

// BestMatch is the index of the row the filter ranks highest, or the first
// selectable row when there is nothing to rank. It is where the cursor goes as
// you type.
func BestMatch(rows []Row, filter string) int {
	first := 0
	for i, r := range rows {
		if r.Selectable() {
			first = i
			break
		}
	}
	if filter == "" {
		return first
	}

	best, bestScore := first, 0
	for i, r := range rows {
		if r.Kind != RowFile {
			continue
		}
		_, score, ok := Match(filter, r.Rel)
		if ok && score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}
