// Package glyph chooses the characters lens draws with, once, from the locale.
//
// Lens is mostly furniture: box-drawing between the list and the diff, a dot
// for an unread edit, ticks and crosses down the doctor's report. On a machine
// with no UTF-8 locale — a slim container, a bare server, a cron job with a
// stripped environment — every one of those arrives as a stray "_", and the
// result reads as a broken tool rather than a plain one.
//
// So there are two sets. The nicer one is used when the environment promises it
// can be drawn, and a plain one that is never wrong otherwise.
package glyph

import (
	"os"
	"strings"
)

// Set is every character lens draws that is not a letter.
type Set struct {
	// The panel.
	Border   string // between the list and the diff
	Dot      string // an edit not looked at yet
	Caret    string // the row under the cursor, and the prompt behind a diff
	Back     string // the heading of the file being looked inside
	Block    string // the filter's cursor
	Sep      string // between the items of a status or help line
	Ellipsis string // where a label or a path was cut short
	Enter    string // the return key, where help names it

	// The command line.
	Tick  string // a check that passed
	Cross string // a check that failed
	Arrow string // pointing at what to do about it
	Dash  string // setting a clause apart in a sentence
}

// Unicode is the set worth having when the terminal can draw it.
func Unicode() Set {
	return Set{
		Border: "│", Dot: "●", Caret: "❯", Back: "◀",
		Block: "█", Sep: "·", Ellipsis: "…", Enter: "⏎",
		Tick: "✓", Cross: "✗", Arrow: "→", Dash: "—",
	}
}

// ASCII is the same in characters every terminal has. Plainer, never wrong.
//
// The panel's markers stay one cell wide so the columns still line up; the
// ellipsis cannot, which is why anything cutting text short has to ask how wide
// this is rather than assume.
func ASCII() Set {
	return Set{
		Border: "|", Dot: "*", Caret: ">", Back: "<",
		Block: "_", Sep: "-", Ellipsis: "...", Enter: "enter",
		Tick: "+", Cross: "x", Arrow: "->", Dash: "-",
	}
}

// Each names every glyph, so a test can cover all of them rather than the ones
// it remembered.
func (s Set) Each() map[string]string {
	return map[string]string{
		"border": s.Border, "dot": s.Dot, "caret": s.Caret, "back": s.Back,
		"block": s.Block, "sep": s.Sep, "ellipsis": s.Ellipsis, "enter": s.Enter,
		"tick": s.Tick, "cross": s.Cross, "arrow": s.Arrow, "dash": s.Dash,
	}
}

// Current is the set in use, decided once at startup.
var Current = Pick(os.Getenv)

// Pick chooses a set for an environment.
func Pick(look func(string) string) Set {
	if UTF8Locale(look) {
		return Unicode()
	}
	return ASCII()
}

// UTF8Locale reports whether the environment promises a terminal that can draw
// the unicode set.
//
// The order is POSIX's: LC_ALL overrides everything, then the category itself,
// then LANG. Nothing set at all is the container case, and the answer there is
// no — being plainer than necessary costs a little elegance, while drawing
// characters the terminal cannot render costs legibility.
func UTF8Locale(look func(string) string) bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := look(name)
		if v == "" {
			continue
		}
		v = strings.ToLower(v)
		return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
	}
	return false
}
