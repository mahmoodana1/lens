package ui

import (
	"os"
	"strings"
	"testing"
	"unicode"

	"github.com/mahmood/lens/internal/glyph"
)

// TestMain pins the unicode glyphs for every test in this package.
//
// The panel falls back to ASCII when the locale cannot promise UTF-8, and many
// of these tests assert on the characters themselves. Without pinning they
// would pass or fail according to the LANG of whichever machine ran them —
// which on a CI runner or in a slim container is often nothing at all.
func TestMain(m *testing.M) {
	gl = glyph.Unicode()
	os.Exit(m.Run())
}

// The help lines are built from the glyphs rather than written out, so they
// come out plain too. A separator left as a literal would be the one stray
// character on an otherwise readable screen.
func TestHelpLines_FollowTheGlyphSet(t *testing.T) {
	restore := gl
	defer func() { gl = restore }()
	gl = glyph.ASCII()

	for _, s := range []string{helpText(), insideHelpText(), panelHelpText()} {
		if s == "" {
			t.Fatal("a help line is empty")
		}
		for _, r := range s {
			if r > unicode.MaxASCII {
				t.Errorf("help line %q contains %q, which the terminal may not draw", s, r)
			}
		}
	}
}

// Cutting a label short has to respect how wide the marker is: the unicode
// ellipsis takes one cell and the ASCII one takes three, and a truncation that
// assumed one would overflow the width it was given by two.
func TestTruncateVisible_AccountsForTheMarkersWidth(t *testing.T) {
	restore := gl
	defer func() { gl = restore }()

	for _, set := range []struct {
		name string
		g    glyph.Set
	}{{"unicode", glyph.Unicode()}, {"ascii", glyph.ASCII()}} {
		gl = set.g
		for _, width := range []int{1, 2, 3, 4, 8, 20} {
			got := truncateVisible(strings.Repeat("abcdefgh", 5), width)
			if w := VisibleWidth(got); w > width {
				t.Errorf("%s: truncateVisible(..., %d) = %q, which is %d cells wide",
					set.name, width, got, w)
			}
		}
	}
}

func TestElideLeft_AccountsForTheMarkersWidth(t *testing.T) {
	restore := gl
	defer func() { gl = restore }()

	for _, set := range []struct {
		name string
		g    glyph.Set
	}{{"unicode", glyph.Unicode()}, {"ascii", glyph.ASCII()}} {
		gl = set.g
		for _, width := range []int{1, 2, 3, 4, 8, 20} {
			got, _ := elideLeft("internal/ui/some/deep/path/file.go", width)
			if w := VisibleWidth(got); w > width {
				t.Errorf("%s: elideLeft(..., %d) = %q, which is %d cells wide",
					set.name, width, got, w)
			}
		}
	}
}
