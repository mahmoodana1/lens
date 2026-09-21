// Package fsindex notices file changes that no tool payload describes.
//
// Claude often writes files through shell commands — heredocs, sed -i, code
// generators — which report nothing about what they touched. fsindex keeps a
// small record of the project's text files and, after each command, stat-walks
// the tree to find what moved. Content for changed files is kept so the next
// change can be shown as a real diff.
package fsindex

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mahmood/lens/internal/capture"
)

const (
	maxFileBytes = 256 * 1024 // bigger files are not readable diffs
	maxFiles     = 20000      // a guard against walking something enormous
	maxDepth     = 12
	sniffBytes   = 8192
)

// skipDirs are directories whose churn is never worth showing. Hidden ones are
// not listed: everything starting with a dot is skipped by the rule below, which
// no list of names could keep up with.
var skipDirs = map[string]bool{
	"node_modules": true, "vendor": true, "target": true,
	"build": true, "dist": true, "out": true,
	"__pycache__": true, "venv": true, "zig-cache": true,
}

// hidden reports whether an entry is one the panel never records: anything
// whose name starts with a dot, file or directory alike. A hidden directory is
// not even descended into, so a large one costs nothing to ignore.
//
// The walk begins at the project root and this asks only about names inside it,
// so a project that itself lives somewhere hidden is unaffected.
func hidden(name string) bool {
	return len(name) > 1 && name[0] == '.'
}

// Meta is what the index remembers about one file.
type Meta struct {
	ModTime time.Time `json:"mtime"`
	Size    int64     `json:"size"`
	Hash    string    `json:"hash"`
}

// Change is one observed difference in a file.
type Change struct {
	Path string // absolute
	Kind string // "create", "update" or "delete"
	Old  string
	New  string
}

// Index tracks a set of roots and the files under them.
type Index struct {
	dir       string // where the index and its blobs live
	project   string // the boundary: no root may lie outside it
	roots     []string
	files     map[string]Meta
	baselined bool // the pre-session state has been recorded
}

type persisted struct {
	Roots     []string        `json:"roots"`
	Files     map[string]Meta `json:"files"`
	Baselined bool            `json:"baselined"`
}

// Load reads the index stored in dir, or starts an empty one.
func Load(dir string) (*Index, error) {
	ix := &Index{dir: dir, files: map[string]Meta{}}
	if err := os.MkdirAll(filepath.Join(dir, "blobs"), 0o700); err != nil {
		return nil, err
	}

	b, err := os.ReadFile(filepath.Join(dir, "index.json"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ix, nil
		}
		return nil, err
	}
	var p persisted
	if err := json.Unmarshal(b, &p); err != nil {
		return ix, nil // a corrupt index costs a baseline, not a session
	}
	ix.roots = p.Roots
	ix.baselined = p.Baselined
	if p.Files != nil {
		ix.files = p.Files
	}
	return ix, nil
}

// Save persists the index for the next hook invocation.
func (ix *Index) Save() error {
	b, err := json.Marshal(persisted{Roots: ix.roots, Files: ix.files, Baselined: ix.baselined})
	if err != nil {
		return err
	}
	tmp := filepath.Join(ix.dir, "index.json.tmp")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(ix.dir, "index.json"))
}

// Roots are the directories being watched.
func (ix *Index) Roots() []string { return ix.roots }

// AddRoot starts watching a directory.
//
// A home directory or the filesystem root is refused: walking either would cost
// far more than it could ever show. Those sessions get their roots from the
// paths that commands actually name.
// Confine fixes the project the walk may not leave, and prunes anything already
// recorded outside it.
//
// The walk sweeps whole directories, so a root anywhere else fills the panel
// with other programs' scratch files — a shell command naming a path in /tmp is
// enough to acquire one. Roots are saved between calls, so an index that strayed
// before must be brought back rather than merely stopped from straying again.
func (ix *Index) Confine(project string) {
	if project == "" {
		return
	}
	abs, err := filepath.Abs(project)
	if err != nil {
		return
	}
	ix.project = filepath.Clean(abs)

	kept := ix.roots[:0]
	for _, r := range ix.roots {
		if ix.watched(r) {
			kept = append(kept, r)
		}
	}
	ix.roots = kept

	// The files remembered from those roots would otherwise sit in the index
	// forever, since nothing will visit them again to retire them.
	for path := range ix.files {
		if !ix.watched(path) {
			delete(ix.files, path)
		}
	}
}

// watched reports whether a path is one the walk may touch: inside the project
// and not hidden.
//
// Hidden trees are machinery whether they are walked into or handed over as a
// root — a command naming ~/.config once cost a session thirteen thousand
// indexed files and every change it was meant to be watching. Roots are saved
// between calls, so this decides what an older index keeps as well as what a
// new one takes on.
func (ix *Index) watched(path string) bool {
	return capture.Inside(ix.project, path) && !capture.HiddenPath(ix.project, path)
}

func (ix *Index) AddRoot(dir string) {
	if dir == "" {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	abs = filepath.Clean(abs)
	if !watchable(abs) {
		return
	}
	if !ix.watched(abs) {
		return
	}
	for _, r := range ix.roots {
		if r == abs || strings.HasPrefix(abs, r+string(filepath.Separator)) {
			return // already covered by an existing root
		}
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return
	}

	// Adding a parent absorbs any roots nested inside it, so no file is ever
	// walked — and reported — twice.
	kept := ix.roots[:0]
	for _, r := range ix.roots {
		if !strings.HasPrefix(r, abs+string(filepath.Separator)) {
			kept = append(kept, r)
		}
	}
	ix.roots = append(kept, abs)
}

// watchable rejects roots too broad to walk.
func watchable(abs string) bool {
	if abs == "/" || abs == "." {
		return false
	}
	if home, err := os.UserHomeDir(); err == nil {
		if abs == filepath.Clean(home) {
			return false
		}
		// Refuse a parent of home too, e.g. /home.
		if strings.HasPrefix(filepath.Clean(home), abs+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

// SeedFromCommand adds roots named by a shell command, which is how a session
// started outside a project still finds the project it just created.
// SeedFromCommand adds roots named by a shell command that ran in dir.
//
// The paths are resolved against dir rather than this process's own directory:
// an agent working in a subdirectory writes "lib/x.sh", and that names a file
// where the command ran, not where lens happens to be. Resolving it here sent
// the walk somewhere else entirely, and the project went unwatched.
func (ix *Index) SeedFromCommand(dir, cmd string) {
	for _, tok := range pathTokens(cmd) {
		abs := tok
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(dir, abs)
		}
		abs = filepath.Clean(abs)
		fi, err := os.Stat(abs)
		if err != nil {
			// A path that does not exist may be a file about to be written;
			// its parent is the interesting directory.
			abs = filepath.Dir(abs)
			if fi, err = os.Stat(abs); err != nil {
				continue
			}
		}
		if fi.IsDir() {
			ix.AddRoot(abs)
		} else {
			ix.AddRoot(filepath.Dir(abs))
		}
	}
}

// pathTokens picks the path-looking words out of a command line.
func pathTokens(cmd string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range strings.FieldsFunc(cmd, func(r rune) bool {
		return r == ' ' || r == '\t' || r == '\n' || r == ';' || r == '|' || r == '&' ||
			r == '"' || r == '\'' || r == '(' || r == ')' || r == '<' || r == '>'
	}) {
		tok := strings.TrimSpace(raw)
		if len(tok) < 2 || strings.HasPrefix(tok, "-") {
			continue
		}
		if !strings.Contains(tok, "/") {
			continue
		}
		if strings.ContainsAny(tok, "*?$`") {
			continue
		}
		if strings.HasPrefix(tok, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				tok = filepath.Join(home, tok[2:])
			}
		}
		if seen[tok] {
			continue
		}
		seen[tok] = true
		out = append(out, tok)
	}
	return out
}

// Rescan walks the roots and reports what changed since the last scan,
// updating the index as it goes. The first scan establishes a baseline and
// reports nothing.
//
// A root added later — a project created during the session — is a special
// case: its files are unknown but not necessarily new. Only those modified at
// or after since are reported, so adopting an existing directory mid-session
// does not flood the panel with files Claude never touched.
func (ix *Index) Rescan(since time.Time) []Change {
	first := !ix.baselined
	seen := make(map[string]Meta, len(ix.files))
	var changes []Change
	budget := maxFiles

	for _, root := range ix.roots {
		ix.walk(root, root, 0, &budget, seen, &changes, first, since)
	}
	ix.baselined = true

	for path, meta := range seen {
		ix.files[path] = meta
	}

	// A file that was known and is now gone is a change in its own right — the
	// one a reader would most want to catch. Absence from this scan is not
	// enough to say so: the walk has a budget and roots come and go, so a file
	// it simply did not reach this time is still there. The disk decides.
	for path, meta := range ix.files {
		if _, ok := seen[path]; ok {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			continue // not visited, but not gone either
		}
		delete(ix.files, path) // reported once, then forgotten
		if first {
			continue // the baseline reports nothing
		}
		old, _ := ix.blob(meta.Hash)
		changes = append(changes, Change{Path: path, Kind: "delete", Old: old})
	}

	sort.Slice(changes, func(i, j int) bool { return changes[i].Path < changes[j].Path })
	return changes
}

func (ix *Index) walk(root, dir string, depth int, budget *int, seen map[string]Meta, changes *[]Change, first bool, since time.Time) {
	if depth > maxDepth || *budget <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if *budget <= 0 {
			return
		}
		name := e.Name()
		path := filepath.Join(dir, name)

		if hidden(name) {
			continue
		}
		if e.IsDir() {
			if skipDirs[name] {
				continue
			}
			if sameFile(path, ix.dir) {
				continue // never report the index's own storage
			}
			ix.walk(root, path, depth+1, budget, seen, changes, first, since)
			continue
		}
		if !e.Type().IsRegular() {
			continue
		}

		if _, done := seen[path]; done {
			continue // already visited this scan, via an overlapping root
		}

		info, err := e.Info()
		if err != nil || info.Size() > maxFileBytes {
			continue
		}
		*budget--

		prev, known := ix.files[path]
		if known && prev.Size == info.Size() && prev.ModTime.Equal(info.ModTime()) {
			seen[path] = prev
			continue
		}

		content, ok := readText(path)
		if !ok {
			continue // binary: not a readable diff
		}
		hash := hashOf(content)
		meta := Meta{ModTime: info.ModTime(), Size: info.Size(), Hash: hash}
		seen[path] = meta

		if first {
			ix.putBlob(hash, content)
			continue
		}
		if known && prev.Hash == hash {
			continue // touched but unchanged, e.g. rebuilt from identical input
		}
		if !known && info.ModTime().Before(since) {
			// Pre-existing file in a newly adopted root: record, do not report.
			ix.putBlob(hash, content)
			continue
		}

		old := ""
		kind := "create"
		if known {
			kind = "update"
			old, _ = ix.blob(prev.Hash)
		}
		ix.putBlob(hash, content)
		*changes = append(*changes, Change{Path: path, Kind: kind, Old: old, New: content})
	}
}

// readText returns a file's contents, or false when it is not text.
func readText(path string) (string, bool) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	head := b
	if len(head) > sniffBytes {
		head = head[:sniffBytes]
	}
	for _, c := range head {
		if c == 0 {
			return "", false
		}
	}
	return string(b), true
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

func (ix *Index) blobPath(hash string) string {
	return filepath.Join(ix.dir, "blobs", hash)
}

func (ix *Index) putBlob(hash, content string) {
	path := ix.blobPath(hash)
	if _, err := os.Stat(path); err == nil {
		return
	}
	os.WriteFile(path, []byte(content), 0o600)
}

func (ix *Index) blob(hash string) (string, bool) {
	b, err := os.ReadFile(ix.blobPath(hash))
	if err != nil {
		return "", false
	}
	return string(b), true
}

func sameFile(a, b string) bool {
	ai, err := os.Stat(a)
	if err != nil {
		return false
	}
	bi, err := os.Stat(b)
	if err != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

// Note records a file's current state without reporting it as a change.
//
// A change already described by a tool payload must not be rediscovered by the
// next scan, or the panel shows it twice.
func Note(dir, path string) error {
	ix, err := Load(dir)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	content, ok := readText(path)
	if !ok {
		return nil
	}
	hash := hashOf(content)
	ix.putBlob(hash, content)
	ix.files[path] = Meta{ModTime: info.ModTime(), Size: info.Size(), Hash: hash}
	return ix.Save()
}
