// Package editor hands a file and a line to the editor already open on a
// project, so a change read in the panel can be opened where it lives.
package editor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mahmood/lens/internal/capture"
)

// ErrNoEditor means nothing is running on the project. It is an ordinary
// answer — the caller starts one — not a failure worth reporting as one.
var ErrNoEditor = errors.New("editor: nothing open on this project")

// ask is how long an editor gets to answer before it is taken for dead. A
// running one replies immediately; a stale socket never replies at all.
const ask = 500 * time.Millisecond

// Server is an editor that can be spoken to.
type Server struct {
	Addr string // the socket it listens on
	CWD  string // what it is working in
}

// Sockets are the editors that might be running: the one this process was
// started from, and every socket nvim leaves in the runtime directory.
func Sockets() []string {
	var out []string
	if addr := os.Getenv("NVIM"); addr != "" {
		out = append(out, addr)
	}
	dir := os.Getenv("XDG_RUNTIME_DIR")
	if dir == "" {
		dir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	matches, err := filepath.Glob(filepath.Join(dir, "nvim.*"))
	if err != nil {
		return out
	}
	return append(out, matches...)
}

// For finds the editor whose own directory holds target — a file, usually —
// preferring the one rooted closest to it.
//
// The question is asked of the file rather than of the session's project,
// because the two are not the same thing: a session started in a home
// directory that goes on to create ~/bashkit has a project far wider than the
// work, and an editor rooted at ~/bashkit does not contain it. Asked that way
// nothing ever matched and every request opened another editor.
//
// Several can be running at once — one per project, typically — so the choice
// matters: handing a file to the wrong one opens it in the wrong window. A
// socket whose editor has died simply does not answer, and is passed over.
func For(target string, addrs []string) (Server, error) {
	var best Server
	bestDepth := -1

	for _, addr := range addrs {
		cwd, err := query(addr, "getcwd()")
		if err != nil || cwd == "" {
			continue
		}
		if !capture.Inside(cwd, target) {
			continue
		}
		if depth := len(filepath.Clean(cwd)); depth > bestDepth {
			best, bestDepth = Server{Addr: addr, CWD: cwd}, depth
		}
	}
	if bestDepth < 0 {
		return Server{}, ErrNoEditor
	}
	return best, nil
}

// JumpTo shows a file at a line in an editor already running.
//
// It asks for `drop`, which is the editor's own answer to "use it if it is
// already on screen, open it if not" — so reading the same file twice moves the
// cursor rather than stacking another copy of it.
func JumpTo(s Server, file string, line int) error {
	if line < 1 {
		line = 1
	}
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	expr := fmt.Sprintf(`execute("drop +%d " . fnameescape(%s))`, line, vimString(abs))
	if _, err := query(s.Addr, expr); err != nil {
		return fmt.Errorf("editor: opening %s: %w", file, err)
	}
	return nil
}

// query evaluates an expression in a running editor and returns its answer.
func query(addr, expr string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ask)
	defer cancel()

	cmd := exec.CommandContext(ctx, "nvim", "--server", addr, "--remote-expr", expr)
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// vimString quotes a path as a literal for the editor's own expression parser,
// where a single quote is escaped by doubling it.
func vimString(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "''") + "'"
}

// Request is one "show me this line" gesture. The two side effects the panel
// cannot perform itself — bringing a window forward, starting a program — are
// passed in, so the gesture can be tested without a terminal to run it in.
type Request struct {
	Project string
	File    string
	Line    int

	Sockets []string                      // editors that might be running
	Find    func(string) (string, bool)   // the pane an editor is in
	Reveal  func(string) error            // bring that pane forward
	Start   func(string, ...string) error // run a program in a new window
}

// Show puts a file in front of the reader at a line: in the editor already open
// on the project where there is one, in a new window where there is not.
func Show(r Request) error {
	if r.File == "" {
		return errors.New("editor: no file to show")
	}
	line := r.Line
	if line < 1 {
		line = 1
	}

	srv, err := For(r.File, r.Sockets)
	if err != nil {
		// Nothing to hand it to, so somewhere to hand it to is made. It is
		// rooted near the file rather than at the session's project, which may
		// be several levels up from the work.
		if r.Start == nil {
			return err
		}
		return r.Start(StartDir(r.File), "nvim", fmt.Sprintf("+%d", line), r.File)
	}

	if err := JumpTo(srv, r.File, line); err != nil {
		return err
	}
	// The editor is usually in another window; a file opened out of sight is
	// not a file shown.
	if r.Find != nil && r.Reveal != nil {
		if id, ok := r.Find(r.Project); ok {
			return r.Reveal(id)
		}
	}
	return nil
}

// StartDir is where an editor started for a file should work.
//
// A level above the folder the file is in: close enough to the work that its
// siblings are to hand, without rooting on the one directory the file happens
// to live in. Where that level would be the home directory — or anything wider
// — the file's own directory is as far up as it goes, since an editor rooted at
// home is rooted at everything.
func StartDir(file string) string {
	abs, err := filepath.Abs(file)
	if err != nil {
		abs = file
	}
	dir := filepath.Dir(abs)
	if parent := filepath.Dir(dir); !tooBroad(parent) {
		return parent
	}
	return dir
}

// tooBroad reports whether a directory is too wide to root an editor in.
func tooBroad(dir string) bool {
	if dir == "/" || dir == "." || dir == string(filepath.Separator) {
		return true
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	home = filepath.Clean(home)
	// Home itself, and anything containing it: /home, and upwards.
	return dir == home || strings.HasPrefix(home, dir+string(filepath.Separator))
}

// Probe reports what an editor is working in, for diagnosis.
func Probe(addr string) (string, error) { return query(addr, "getcwd()") }
