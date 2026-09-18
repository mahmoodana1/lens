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
	Edits   int // file headers only
}

// BuildRows lays a session out for the given view.
func BuildRows(s store.Session, v View) []Row {
	if len(s.Events) == 0 {
		return nil
	}
	if v == ByFile {
		return byFileRows(s)
	}

	rows := make([]Row, 0, len(s.Events))
	for i := range s.Events {
		e := &s.Events[i]
		rows = append(rows, Row{
			Kind: RowEvent, Event: e, Rel: e.Rel,
			Added: e.Added, Removed: e.Removed,
		})
	}
	return rows
}

func byFileRows(s store.Session) []Row {
	type group struct {
		added, removed int
		events         []*capture.Event
	}
	order := []string{}
	groups := map[string]*group{}

	for i := range s.Events {
		e := &s.Events[i]
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

	rows := make([]Row, 0, len(s.Events)+len(order))
	for _, rel := range order {
		g := groups[rel]
		rows = append(rows, Row{
			Kind: RowFileHeader, Rel: rel, Event: g.events[0],
			Added: g.added, Removed: g.removed, Edits: len(g.events),
		})
		for _, e := range g.events {
			rows = append(rows, Row{
				Kind: RowEvent, Event: e, Rel: e.Rel,
				Added: e.Added, Removed: e.Removed,
			})
		}
	}
	return rows
}
