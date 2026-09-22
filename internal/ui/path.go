package ui

import (
	"os"
	"path"
	"strings"
)

// SplitDir separates a path into the directory the list groups it under and the
// name it is listed by. The directory keeps its trailing slash so it reads as
// one, and a file at the project root is grouped under "./".
func SplitDir(p string) (dir, name string) {
	name = path.Base(p)
	dir = path.Dir(p)
	if dir == "." {
		return "./", name
	}
	if home := os.Getenv("HOME"); home != "" && home != "/" {
		if rest, ok := strings.CutPrefix(dir, strings.TrimRight(home, "/")+"/"); ok {
			dir = "~/" + rest
		} else if dir == strings.TrimRight(home, "/") {
			dir = "~"
		}
	}
	return dir + "/", name
}

// ElidePath cuts a directory down to width by dropping components out of its
// middle, which is the part that says least: the first says which tree the file
// is in and the last says where in that tree it sits.
//
// Where not even those two fit, the head goes first and then the tail is cut
// from its front, so what survives is always the end of the path.
func ElidePath(dir string, width int) string {
	if width <= 0 {
		return ""
	}
	if VisibleWidth(dir) <= width {
		return dir
	}

	parts := strings.Split(strings.TrimSuffix(dir, "/"), "/")
	// Drop from the middle outwards, keeping the first and last components,
	// until what is left fits.
	for lo, hi := 1, len(parts)-1; lo < hi; lo++ {
		kept := append(append([]string{}, parts[:lo]...), gl.Ellipsis)
		kept = append(kept, parts[hi:]...)
		if s := strings.Join(kept, "/") + "/"; VisibleWidth(s) <= width {
			return s
		}
	}

	// Only the last component has a chance now, and then only its end.
	if tail := gl.Ellipsis + "/" + parts[len(parts)-1] + "/"; VisibleWidth(tail) <= width {
		return tail
	}
	cut, _ := elideLeft(dir, width)
	return cut
}
