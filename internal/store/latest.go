package store

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/mahmood/lens/internal/capture"
)

// SessionInfo is what is known about a session without reading its log: enough
// to choose between them.
type SessionInfo struct {
	ID       string
	CWD      string // the project it ran in, empty until its meta is written
	Ended    bool
	Modified time.Time
}

// Sessions lists what is on disk, newest first.
func Sessions() ([]SessionInfo, error) {
	entries, err := os.ReadDir(Root())
	if err != nil {
		return nil, err
	}

	out := make([]SessionInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue // the auto-open list lives here too
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		s := SessionInfo{ID: e.Name(), Modified: info.ModTime()}

		var m Meta
		if err := readJSON(filepath.Join(Root(), e.Name(), metaFile), &m); err == nil {
			s.CWD, s.Ended = m.CWD, m.Ended
		}
		out = append(out, s)
	}
	return out, nil
}

// Latest is the session to show for a project.
//
// Recency alone is the wrong question. Every session's directory is touched as
// its panel is read, so the session most recently written is often whichever
// one was last looked at — another project's, in another window. A panel opened
// over a project has to be that project's.
//
// So a session whose project contains the directory asked for wins, the closest
// one where several do, and the most recently touched among equals. Only when
// no session knows the directory at all does recency alone decide, which is
// what `lens` in a bare terminal relies on.
//
// Whether a session says it has "ended" does not come into it: Claude Code
// fires the SessionEnd hook on a clear or a compaction too, so the flag reads
// true for sessions still in use.
func Latest(project string) (string, error) {
	all, err := Sessions()
	if err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "", errors.New("no sessions")
	}

	if project != "" {
		if id, ok := bestForProject(all, project); ok {
			return id, nil
		}
	}

	best := all[0]
	for _, s := range all[1:] {
		if s.Modified.After(best.Modified) {
			best = s
		}
	}
	return best.ID, nil
}

// bestForProject picks the session whose project owns a directory.
func bestForProject(all []SessionInfo, project string) (string, bool) {
	var best SessionInfo
	bestDepth := -1

	for _, s := range all {
		if s.CWD == "" || !capture.Inside(s.CWD, project) {
			continue
		}
		// A longer project path is a closer one: /p/proj beats /p for /p/proj/src.
		depth := len(filepath.Clean(s.CWD))
		switch {
		case depth > bestDepth:
		case depth < bestDepth:
			continue
		case !s.Modified.After(best.Modified):
			continue
		}
		best, bestDepth = s, depth
	}
	return best.ID, bestDepth >= 0
}
