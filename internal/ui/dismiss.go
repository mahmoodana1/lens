package ui

import (
	"github.com/mahmood/lens/internal/capture"
	"github.com/mahmood/lens/internal/store"
)

// A dismissal is one thing cleared out of the list: a whole file, one edit to
// it, or one hunk of one edit. Nothing on disk changes — this is about what you
// still have left to read.
//
// It is the store's own type because the trail is kept with the session, so
// reopening the popup over a session finds it as the reader left it — and a
// different session finds its own.
type dismissal = store.Dismissal

func dismissFile(rel string) dismissal { return dismissal{Rel: rel, Hunk: -1} }
func dismissEdit(seq int) dismissal    { return dismissal{Seq: seq, Hunk: -1} }
func dismissHunk(seq, i int) dismissal { return dismissal{Seq: seq, Hunk: i} }

func zeroDismissal(d dismissal) bool { return d.Rel == "" && d.Seq == 0 }

// Hidden is everything dismissed so far, in a form the row and diff builders
// can ask about.
type Hidden map[dismissal]bool

// File reports whether a whole file has been dismissed.
func (h Hidden) File(rel string) bool { return h[dismissFile(rel)] }

// Edit reports whether an edit is gone, either on its own or with its file.
func (h Hidden) Edit(rel string, seq int) bool {
	return h.File(rel) || h[dismissEdit(seq)]
}

// hiddenFrom turns a trail into the set the row and diff builders ask about.
func hiddenFrom(trail []dismissal) Hidden {
	h := Hidden{}
	for _, d := range trail {
		h[d] = true
	}
	return h
}

// Hunk reports whether one hunk of an edit is gone.
func (h Hidden) Hunk(rel string, seq, i int) bool {
	return h.Edit(rel, seq) || h[dismissHunk(seq, i)]
}

// Empty reports whether an edit has nothing left to show: it was dismissed
// outright, or it had hunks and every one of them has gone.
func (h Hidden) Empty(e *capture.Event) bool {
	if h.Edit(e.Rel, e.Seq) {
		return true
	}
	for i := range e.Hunks {
		if !h.Hunk(e.Rel, e.Seq, i) {
			return false
		}
	}
	return len(e.Hunks) > 0
}

// Len is how many dismissals are in force.
func (h Hidden) Len() int { return len(h) }

// dismissSelected clears the row under the cursor out of the list. What goes
// depends on what the row stands for: a file takes all of its edits, an edit
// takes its hunks, and a hunk goes alone.
func (m *Model) dismissSelected() {
	r := m.selected()
	if r == nil {
		return
	}

	var d dismissal
	switch r.Kind {
	case RowFile:
		d = dismissFile(r.Rel)
	case RowEdit:
		d = dismissEdit(r.Event.Seq)
	case RowHunk:
		d = dismissHunk(r.Event.Seq, r.Hunk)
	default:
		return
	}
	if m.hidden[d] {
		return
	}

	if m.hidden == nil {
		m.hidden = Hidden{}
	}
	m.hidden[d] = true
	m.undone = append(m.undone, d)
	// A fresh dismissal is a new branch: what was undone is no longer ahead.
	m.redone = m.redone[:0]
	m.saveDismissals()
	m.afterDismissChange()
}

// undoDismiss puts back the most recent dismissal, one step per press, so any
// earlier point in the trail can be reached.
func (m *Model) undoDismiss() {
	if len(m.undone) == 0 {
		return
	}
	d := m.undone[len(m.undone)-1]
	m.undone = m.undone[:len(m.undone)-1]
	delete(m.hidden, d)
	m.redone = append(m.redone, d)
	m.saveDismissals()
	m.afterDismissChange()
	m.toDismissal(d)
}

// redoDismiss replays a dismissal that undo took back.
func (m *Model) redoDismiss() {
	if len(m.redone) == 0 {
		return
	}
	d := m.redone[len(m.redone)-1]
	m.redone = m.redone[:len(m.redone)-1]
	if m.hidden == nil {
		m.hidden = Hidden{}
	}
	m.hidden[d] = true
	m.undone = append(m.undone, d)
	m.saveDismissals()
	m.afterDismissChange()
}

// saveDismissals keeps the session's trail on disk. Losing it costs the reader
// their undo, not their work, so a failure here is not worth interrupting for.
func (m *Model) saveDismissals() {
	if m.store == nil {
		return
	}
	if err := m.store.SetDismissals(m.undone); err != nil {
		return
	}
}

// adoptDismissals takes on the trail the session was left with. It happens once,
// when the panel opens: the reader's own dd and u are the only things that move
// it afterwards, so a reload must not read it back over them.
func (m *Model) adoptDismissals(trail []dismissal) {
	if m.loadedTrail {
		return
	}
	m.loadedTrail = true
	m.undone = append(m.undone, trail...)
	m.hidden = hiddenFrom(m.undone)
}

// afterDismissChange rebuilds around what is left. An opened file whose last
// row has just gone has nothing to show, so the list steps back out of it.
func (m *Model) afterDismissChange() {
	m.rebuild()
	if m.openFile != "" && len(m.rows) == 0 {
		m.leaveFile()
		return
	}
	m.settle(1)
	m.onSelectionChange()
	m.markSelectedRead()
}

// toDismissal puts the cursor on whatever an undo just brought back, so the
// thing you asked for is the thing you are looking at.
func (m *Model) toDismissal(d dismissal) {
	if zeroDismissal(d) {
		return
	}
	for i, r := range m.rows {
		if !r.Selectable() {
			continue
		}
		if d.Rel != "" && r.Kind == RowFile && r.Rel == d.Rel {
			m.cursor = i
		} else if d.Rel == "" && r.Event != nil && r.Event.Seq == d.Seq && r.Hunk == d.Hunk {
			m.cursor = i
		} else {
			continue
		}
		m.onSelectionChange()
		m.markSelectedRead()
		return
	}
}
