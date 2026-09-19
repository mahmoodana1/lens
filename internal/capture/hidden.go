package capture

import (
	"path/filepath"
	"strings"
)

// HiddenPath reports whether a file is hidden, and so is not something the
// panel should record at all.
//
// Hidden means any part of the path starts with a dot: the file itself, or a
// directory on the way to it. That covers the machinery a reader never wants to
// read — .git, .venv, .cache, an editor's or an agent's own state — without a
// list of names that would always be one entry behind.
//
// The question is asked of the path relative to the project root, not of the
// path itself, so a project that lives somewhere hidden is not hidden from
// itself: in ~/.config/nvim, init.lua is an ordinary file and .git is not.
func HiddenPath(root, path string) bool {
	rel := RelativeTo(root, path)
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		// "." alone is a path saying "here", not a hidden name.
		if len(part) > 1 && part[0] == '.' && part != ".." {
			return true
		}
	}
	return false
}

// Inside reports whether a path is within the project root.
//
// The file walk must never leave the project: it sweeps whole directories, so a
// root anywhere else turns the panel into a log of other programs' scratch
// files. A root of "" means the project is unknown, and then nothing can be
// judged to be outside it.
func Inside(root, path string) bool {
	if root == "" || path == "" {
		return true
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
}
