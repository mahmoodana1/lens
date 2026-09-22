package ui

import "github.com/mahmood/lens/internal/glyph"

// gl is the set the panel draws its furniture with, chosen from the locale by
// the glyph package. It is a variable rather than a call so tests can pin it
// instead of depending on the LANG of whichever machine runs them.
var gl = glyph.Current
