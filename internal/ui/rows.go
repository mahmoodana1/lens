package ui

import (
	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
)

// View is how the list is organised.
type View int

const (
	// Timeline shows every edit in the order Claude made it.
	Timeline View = iota
	// ByFile groups edits under the file they touched, with running totals.
	ByFile
)

func (v View) String() string {
	if v == ByFile {
		return "by file"
	}
	return "timeline"
}

// RowKind distinguishes a file heading from an individual edit.
type RowKind int

const (
	RowEvent RowKind = iota
	RowFileHeader
)

// Row is one selectable line in the list pane.
type Row struct {
	Kind    RowKind
	Event   *capture.Event // the edit to show in the diff pane
	Rel     string
	Added   int
	Removed int
	Edits   int   // file headers only
	Match   []int // positions in Rel matched by the filter, for highlighting
}

// BuildRows lays a session out for the given view, keeping only the files the
// filter matches. The order is the view's own — narrowing the list must not
// reshuffle a timeline — so the ranking instead decides where the cursor lands.
func BuildRows(s store.Session, v View, filter string) []Row {
	kept := make([]*capture.Event, 0, len(s.Events))
	for i := range s.Events {
		if _, _, ok := Match(filter, s.Events[i].Rel); ok {
			kept = append(kept, &s.Events[i])
		}
	}
	if len(kept) == 0 {
		return nil
	}
	if v == ByFile {
		return byFileRows(kept, filter)
	}

	rows := make([]Row, 0, len(kept))
	for _, e := range kept {
		rows = append(rows, Row{
			Kind: RowEvent, Event: e, Rel: e.Rel,
			Added: e.Added, Removed: e.Removed,
			Match: matchPositions(filter, e.Rel),
		})
	}
	return rows
}

// BestMatch is the index of the row the filter ranks highest, or 0 when there
// is nothing to rank. It is where the cursor goes as you type.
func BestMatch(rows []Row, filter string) int {
	if filter == "" {
		return 0
	}
	best, bestScore := 0, 0
	for i, r := range rows {
		_, score, ok := Match(filter, r.Rel)
		if ok && score > bestScore {
			best, bestScore = i, score
		}
	}
	return best
}

func matchPositions(filter, rel string) []int {
	pos, _, _ := Match(filter, rel)
	return pos
}

func byFileRows(events []*capture.Event, filter string) []Row {
	type group struct {
		added, removed int
		events         []*capture.Event
	}
	order := []string{}
	groups := map[string]*group{}

	for _, e := range events {
		g, ok := groups[e.Rel]
		if !ok {
			g = &group{}
			groups[e.Rel] = g
			order = append(order, e.Rel) // first appearance wins, so the list stays stable
		}
		g.added += e.Added
		g.removed += e.Removed
		g.events = append(g.events, e)
	}

	rows := make([]Row, 0, len(events)+len(order))
	for _, rel := range order {
		g := groups[rel]
		rows = append(rows, Row{
			Kind: RowFileHeader, Rel: rel, Event: g.events[0],
			Added: g.added, Removed: g.removed, Edits: len(g.events),
			Match: matchPositions(filter, rel),
		})
		for _, e := range g.events {
			rows = append(rows, Row{
				Kind: RowEvent, Event: e, Rel: e.Rel,
				Added: e.Added, Removed: e.Removed,
				Match: matchPositions(filter, e.Rel),
			})
		}
	}
	return rows
}
