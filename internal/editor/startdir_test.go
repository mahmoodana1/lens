package editor_test

import (
	"path/filepath"
	"testing"

	"github.com/mahmood/lens/internal/editor"
)

// An editor started for a file wants to sit a level above the folder the file
// is in — close enough to the work to reach its siblings, without rooting on
// the one directory the file happens to live in.
func TestStartDir(t *testing.T) {
	home := "/home/me"
	t.Setenv("HOME", home)

	for _, c := range []struct {
		name string
		file string
		want string
	}{
		{"a file in a subdirectory", home + "/proj/lib/util.sh", home + "/proj"},
		{"deeper still", home + "/proj/tools/reporter/main.sh", home + "/proj/tools"},
		{"the parent would be home", home + "/proj/main.sh", home + "/proj"},
		{"the file sits in home itself", home + "/notes.md", home},
		{"outside home entirely", "/srv/app/lib/x.go", "/srv/app"},
		{"the parent would be root", "/srv/x.go", "/srv"},
		{"the file sits in root", "/x.go", "/"},
	} {
		if got := editor.StartDir(c.file); got != c.want {
			t.Errorf("%s: StartDir(%q) = %q, want %q", c.name, c.file, got, c.want)
		}
	}
}

// A directory above home is no better a place to sit than home is.
func TestStartDir_RefusesToClimbPastHome(t *testing.T) {
	t.Setenv("HOME", "/home/me")

	if got := editor.StartDir("/home/me/x.go"); got != "/home/me" {
		t.Errorf("StartDir = %q, want home itself rather than /home", got)
	}
}

// A relative path still names a real place.
func TestStartDir_MakesThePathAbsolute(t *testing.T) {
	t.Setenv("HOME", "/nowhere")
	dir := t.TempDir()
	t.Chdir(dir)

	got := editor.StartDir("lib/util.sh")

	want, _ := filepath.EvalSymlinks(dir)
	if real, _ := filepath.EvalSymlinks(got); real != want {
		t.Errorf("StartDir = %q, want %q", got, want)
	}
}
