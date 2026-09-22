package glyph_test

import (
	"testing"
	"unicode"

	"github.com/mahmood/lens/internal/glyph"
)

// A terminal with no UTF-8 locale draws every one of these as a stray "_".
// Since lens is mostly furniture, that reads as a broken tool rather than a
// plain one — which is how this was found, in a debian:stable-slim container.
func TestUTF8Locale(t *testing.T) {
	for _, c := range []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"nothing set at all", nil, false},
		{"the C locale", map[string]string{"LANG": "C"}, false},
		{"POSIX", map[string]string{"LANG": "POSIX"}, false},
		{"a UTF-8 language", map[string]string{"LANG": "en_US.UTF-8"}, true},
		{"C.UTF-8, as a container sets it", map[string]string{"LANG": "C.UTF-8"}, true},
		{"spelled without the dash", map[string]string{"LANG": "en_GB.utf8"}, true},
		{"LC_ALL overrides a good LANG", map[string]string{"LANG": "en_US.UTF-8", "LC_ALL": "C"}, false},
		{"LC_ALL rescues a bare LANG", map[string]string{"LANG": "C", "LC_ALL": "en_US.UTF-8"}, true},
		{"LC_CTYPE beats LANG", map[string]string{"LANG": "C", "LC_CTYPE": "en_US.UTF-8"}, true},
		{"LC_ALL beats LC_CTYPE", map[string]string{"LC_CTYPE": "en_US.UTF-8", "LC_ALL": "C"}, false},
	} {
		if got := glyph.UTF8Locale(func(k string) string { return c.env[k] }); got != c.want {
			t.Errorf("%s: UTF8Locale = %v, want %v", c.name, got, c.want)
		}
	}
}

// Pick is what startup calls, so it deserves its own look.
func TestPick(t *testing.T) {
	utf8 := func(string) string { return "en_US.UTF-8" }
	if glyph.Pick(utf8) != glyph.Unicode() {
		t.Error("a UTF-8 locale should get the unicode set")
	}
	if glyph.Pick(func(string) string { return "" }) != glyph.ASCII() {
		t.Error("an environment that promises nothing should get the plain set")
	}
}

// The fallback is only worth having if it is actually ASCII.
func TestASCII_IsASCII(t *testing.T) {
	for name, s := range glyph.ASCII().Each() {
		for _, r := range s {
			if r > unicode.MaxASCII {
				t.Errorf("%s = %q, which is not ASCII", name, s)
			}
		}
	}
}

// A glyph left unset renders as nothing at all, which is worse than either set.
func TestNoneAreEmpty(t *testing.T) {
	for _, s := range []glyph.Set{glyph.Unicode(), glyph.ASCII()} {
		for name, v := range s.Each() {
			if v == "" {
				t.Errorf("%s is empty", name)
			}
		}
	}
}

// The panel lays its markers out in fixed columns, so the plain set has to keep
// the single-cell ones single-cell or the rows stop lining up.
func TestASCII_MarkersStayOneCell(t *testing.T) {
	a := glyph.ASCII()
	for name, s := range map[string]string{
		"border": a.Border, "dot": a.Dot, "caret": a.Caret,
		"back": a.Back, "block": a.Block, "sep": a.Sep,
	} {
		if len(s) != 1 {
			t.Errorf("%s = %q, which is %d cells; the columns assume one", name, s, len(s))
		}
	}
}
