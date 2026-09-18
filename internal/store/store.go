// Package store holds one Claude session's captured edits on disk.
//
// The log is append-only JSONL: hooks only ever append, the panel only ever
// reads, and neither blocks the other. Storage is per-session and ephemeral.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/mahmood/lens/internal/capture"
)

const (
	eventsFile  = "events.jsonl"
	promptsFile = "prompts.jsonl"
	metaFile    = "meta.json"
	// PaneFile records the tmux pane showing this session, and doubles as the
	// spawn lock.
	PaneFile = "pane"
)

// Meta describes the session itself.
type Meta struct {
	SessionID string    `json:"session_id"`
	CWD       string    `json:"cwd"`
	Started   time.Time `json:"started"`
	Ended     bool      `json:"ended"`
}

// Session is everything the panel needs to render.
type Session struct {
	Meta    Meta
	Events  []capture.Event
	Prompts map[string]string
}

// Store is a handle on one session's directory.
type Store struct {
	id  string
	dir string
	mu  sync.Mutex // orders appends within a process; flock orders them across processes
}

// Root is where all sessions live.
func Root() string {
	if base := os.Getenv("XDG_STATE_HOME"); base != "" {
		return filepath.Join(base, "lens")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(os.TempDir(), "lens")
	}
	return filepath.Join(home, ".local", "state", "lens")
}

// Dir is the directory holding one session.
func Dir(sessionID string) string {
	return filepath.Join(Root(), sanitize(sessionID))
}

// sanitize keeps a session id from escaping the root directory.
func sanitize(id string) string {
	if id == "" {
		return "unknown"
	}
	out := []rune(id)
	for i, r := range out {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			out[i] = '_'
		}
	}
	return string(out)
}

// Open returns a handle on a session, creating its directory.
func Open(sessionID string) (*Store, error) {
	dir := Dir(sessionID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("store: create session dir: %w", err)
	}
	return &Store{id: sanitize(sessionID), dir: dir}, nil
}

// Dir returns this session's directory.
func (s *Store) Dir() string { return s.dir }

// EnsureMeta writes meta.json the first time it is called and leaves it alone
// afterwards, so the recorded start time and project root stay put.
func (s *Store) EnsureMeta(cwd string) error {
	path := filepath.Join(s.dir, metaFile)
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	m := Meta{SessionID: s.id, CWD: cwd, Started: time.Now()}
	return writeJSON(path, m)
}

// Append adds one edit to the log, assigning its sequence number.
func (s *Store) Append(e capture.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := os.OpenFile(filepath.Join(s.dir, eventsFile), os.O_APPEND|os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return fmt.Errorf("store: open events: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("store: lock events: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	n, err := countLines(f)
	if err != nil {
		return err
	}
	e.Seq = n + 1
	if e.Time.IsZero() {
		e.Time = time.Now()
	}

	line, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("store: marshal event: %w", err)
	}
	if _, err := f.Seek(0, 2); err != nil {
		return fmt.Errorf("store: seek events: %w", err)
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("store: append event: %w", err)
	}
	return nil
}

// RecordPrompt stores the text of a prompt so edits can show what asked for them.
func (s *Store) RecordPrompt(id, text string) error {
	if id == "" {
		return nil
	}
	f, err := os.OpenFile(filepath.Join(s.dir, promptsFile), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("store: open prompts: %w", err)
	}
	defer f.Close()

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("store: lock prompts: %w", err)
	}
	defer syscall.Flock(int(f.Fd()), syscall.LOCK_UN)

	line, err := json.Marshal(struct {
		ID   string `json:"id"`
		Text string `json:"text"`
	}{id, text})
	if err != nil {
		return fmt.Errorf("store: marshal prompt: %w", err)
	}
	_, err = f.Write(append(line, '\n'))
	return err
}

// MarkEnded records that Claude's session is over. The panel keeps rendering.
func (s *Store) MarkEnded() error {
	path := filepath.Join(s.dir, metaFile)
	var m Meta
	if err := readJSON(path, &m); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	m.Ended = true
	if m.SessionID == "" {
		m.SessionID = s.id
	}
	if m.Started.IsZero() {
		m.Started = time.Now()
	}
	return writeJSON(path, m)
}

// Read loads the whole session. Lines that fail to parse are skipped, so a
// half-written tail from an interrupted hook costs at most its own event.
func (s *Store) Read() (Session, error) {
	out := Session{Prompts: map[string]string{}}

	if err := readJSON(filepath.Join(s.dir, metaFile), &out.Meta); err != nil && !errors.Is(err, os.ErrNotExist) {
		return out, err
	}

	if err := eachLine(filepath.Join(s.dir, eventsFile), func(b []byte) {
		var e capture.Event
		if json.Unmarshal(b, &e) == nil {
			out.Events = append(out.Events, e)
		}
	}); err != nil {
		return out, err
	}

	if err := eachLine(filepath.Join(s.dir, promptsFile), func(b []byte) {
		var p struct {
			ID   string `json:"id"`
			Text string `json:"text"`
		}
		if json.Unmarshal(b, &p) == nil && p.ID != "" {
			out.Prompts[p.ID] = p.Text
		}
	}); err != nil {
		return out, err
	}

	return out, nil
}

// Destroy removes the session from disk.
func (s *Store) Destroy() error {
	return os.RemoveAll(s.dir)
}

func countLines(f *os.File) (int, error) {
	if _, err := f.Seek(0, 0); err != nil {
		return 0, fmt.Errorf("store: rewind events: %w", err)
	}
	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		n++
	}
	if err := sc.Err(); err != nil {
		return 0, fmt.Errorf("store: count events: %w", err)
	}
	return n, nil
}

func eachLine(path string, fn func([]byte)) error {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("store: open %s: %w", filepath.Base(path), err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		b := sc.Bytes()
		if len(b) == 0 {
			continue
		}
		cp := make([]byte, len(b))
		copy(cp, b)
		fn(cp)
	}
	// A truncated final line surfaces here; earlier lines are already delivered.
	if err := sc.Err(); err != nil {
		return nil
	}
	return nil
}

func writeJSON(path string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("store: marshal %s: %w", filepath.Base(path), err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("store: write %s: %w", filepath.Base(path), err)
	}
	return os.Rename(tmp, path)
}

func readJSON(path string, v any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
