package pane

import "testing"

func TestMatchPaneByTTY(t *testing.T) {
	listing := "/dev/pts/3 %0\n/dev/pts/7 %4\n/dev/pts/9 %11\n"

	got, ok := matchPaneByTTY(listing, "/dev/pts/7")
	if !ok || got != "%4" {
		t.Errorf("got %q, %v; want %%4, true", got, ok)
	}
}

func TestMatchPaneByTTY_NoMatch(t *testing.T) {
	listing := "/dev/pts/3 %0\n"
	if got, ok := matchPaneByTTY(listing, "/dev/pts/8"); ok {
		t.Errorf("got %q; want no match", got)
	}
}

func TestMatchPaneByTTY_IgnoresMalformedRows(t *testing.T) {
	listing := "garbage\n\n/dev/pts/3 %0\n%no-tty\n"
	got, ok := matchPaneByTTY(listing, "/dev/pts/3")
	if !ok || got != "%0" {
		t.Errorf("got %q, %v; want %%0, true", got, ok)
	}
}

// tty_nr packs the device's major and minor numbers; /dev/pts uses the minor.
func TestTTYFromDevNumber(t *testing.T) {
	cases := []struct {
		name string
		nr   int
		want string
	}{
		{"pts/0", 34816, "/dev/pts/0"},  // major 136, minor 0
		{"pts/7", 34823, "/dev/pts/7"},  // major 136, minor 7
		{"pts/12", 34828, "/dev/pts/12"},
		{"no controlling terminal", 0, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ttyFromDevNumber(tc.nr); got != tc.want {
				t.Errorf("ttyFromDevNumber(%d) = %q, want %q", tc.nr, got, tc.want)
			}
		})
	}
}

func TestParseTTYNr(t *testing.T) {
	// A real /proc/<pid>/stat line; the command name contains spaces and parens.
	stat := "1234 (my (odd) proc) S 1200 1234 1234 34823 1234 4194304 0 0"
	got, ok := parseTTYNr(stat)
	if !ok || got != 34823 {
		t.Errorf("parseTTYNr = %d, %v; want 34823, true", got, ok)
	}
}

func TestParseTTYNr_Malformed(t *testing.T) {
	if _, ok := parseTTYNr("not a stat line"); ok {
		t.Error("want failure on a malformed stat line")
	}
}
