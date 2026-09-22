// Package doctor answers "why is the panel not showing me anything".
//
// Every check here stands for a way lens has actually been found broken: hooks
// that were never wired, a project quietly muted, no tmux to put a popup in, a
// session that recorded nothing. Each one reports what it found rather than
// only whether it passed, because the finding is usually the answer.
package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mahmood/lens/internal/editor"
	"github.com/mahmood/lens/internal/glyph"
	"github.com/mahmood/lens/internal/hooks"
	"github.com/mahmood/lens/internal/store"
)

// Level is how much a finding matters.
type Level int

const (
	// OK is working as intended.
	OK Level = iota
	// Warn is working, but not as the reader probably wants.
	Warn
	// Fail is the reason nothing is happening.
	Fail
)

func (l Level) String() string {
	switch l {
	case OK:
		return "ok"
	case Warn:
		return "warn"
	default:
		return "fail"
	}
}

// Check is one finding.
type Check struct {
	Name   string
	Level  Level
	Detail string
	Fix    string // what to do about it, when there is something to do
}

// Report is everything the doctor found, in the order it looked.
type Report struct {
	Checks []Check
}

// Worst is the most serious level in the report.
func (r Report) Worst() Level {
	worst := OK
	for _, c := range r.Checks {
		if c.Level > worst {
			worst = c.Level
		}
	}
	return worst
}

// Failed reports whether anything is actually stopping the panel working.
//
// A warning is not a failure: a muted project or an editor open elsewhere is
// worth saying and still works. Only a check the panel cannot get past counts,
// so a script can tell "something to mention" from "something to fix".
func (r Report) Failed() bool { return r.Worst() == Fail }

// Options are the surroundings to examine. They are arguments rather than
// globals so the checks can be run against a fixture.
type Options struct {
	Project  string // the project to answer for
	Settings string // Claude Code's settings file
	Binary   string // the lens being run
	Version  string
}

// Run works through the checks and reports what it found.
func Run(o Options) Report {
	var r Report
	add := func(c Check) { r.Checks = append(r.Checks, c) }

	add(checkBinary(o))
	add(checkHooks(o))
	add(checkTmux())
	add(checkProject(o))
	add(checkAutoOpen(o))
	add(checkCapture(o))
	add(checkEditor(o))
	return r
}

func checkBinary(o Options) Check {
	c := Check{Name: "binary", Detail: o.Binary}
	if o.Version != "" {
		c.Detail = fmt.Sprintf("%s (%s)", o.Binary, o.Version)
	}
	if _, err := os.Stat(o.Binary); err != nil {
		c.Level, c.Fix = Fail, "reinstall with ./install.sh"
		c.Detail = fmt.Sprintf("%s is not there", o.Binary)
	}
	return c
}

// checkHooks is the first thing to look at: without them lens sees nothing at
// all, and it is the failure that looks most like the tool being broken.
func checkHooks(o Options) Check {
	c := Check{Name: "hooks"}
	state, err := hooks.Status(o.Settings)
	if err != nil {
		c.Level, c.Detail, c.Fix = Fail, err.Error(), "fix the JSON, then run ./install.sh"
		return c
	}

	var missing []string
	wired := map[string]bool{}
	for _, s := range state {
		if !s.Installed {
			missing = append(missing, s.Event)
			continue
		}
		wired[s.Binary] = true
	}

	switch {
	case len(missing) == len(state):
		c.Level = Fail
		c.Detail = "none are installed in " + short(o.Settings)
		c.Fix = "run ./install.sh, then restart Claude Code"
	case len(missing) > 0:
		c.Level = Warn
		c.Detail = fmt.Sprintf("%d of %d installed; missing %s",
			len(state)-len(missing), len(state), strings.Join(missing, ", "))
		c.Fix = "run ./install.sh to put the rest back"
	default:
		c.Detail = fmt.Sprintf("%d of %d installed", len(state), len(state))
	}

	// A hook pointing at a lens that is not this one is why an upgrade can
	// seem to have no effect.
	for path := range wired {
		if path != "" && path != o.Binary {
			c.Level = maxLevel(c.Level, Warn)
			c.Detail += fmt.Sprintf("; hooks run %s, not this binary", path)
			c.Fix = "run ./install.sh to point them here"
		}
	}
	return c
}

func checkTmux() Check {
	c := Check{Name: "tmux"}
	if _, err := exec.LookPath("tmux"); err != nil {
		c.Level, c.Detail = Fail, "not installed; the panel is a tmux popup"
		c.Fix = "install tmux"
		return c
	}
	out, err := exec.Command("tmux", "-V").Output()
	version := strings.TrimSpace(string(out))
	if err != nil {
		version = "installed"
	}
	if os.Getenv("TMUX") == "" {
		c.Level, c.Detail = Warn, version+", but this shell is not inside it"
		c.Fix = "run Claude Code inside tmux, or the popup has nowhere to open"
		return c
	}
	c.Detail = version
	return c
}

func checkProject(o Options) Check {
	c := Check{Name: "project", Detail: short(o.Project)}
	if o.Project == "" {
		c.Level, c.Detail = Warn, "could not tell which project this is"
		return c
	}
	if home, err := os.UserHomeDir(); err == nil && filepath.Clean(o.Project) == filepath.Clean(home) {
		c.Level = Warn
		c.Detail += " " + glyph.Current.Dash + " your home directory, which is too broad to watch"
		c.Fix = "start Claude Code inside a project instead"
	}
	return c
}

func checkAutoOpen(o Options) Check {
	c := Check{Name: "auto-open"}
	if o.Project == "" {
		c.Detail = "unknown project"
		return c
	}
	if store.AutoOpen(o.Project) {
		c.Detail = "on for " + short(o.Project)
		return c
	}
	c.Level = Warn
	c.Detail = "OFF for " + short(o.Project) + " " + glyph.Current.Dash + " capture runs, the panel stays shut"
	c.Fix = fmt.Sprintf("lens auto on --project %s", o.Project)
	return c
}

// checkCapture is the proof: whatever the configuration says, has anything
// actually been recorded for this project?
func checkCapture(o Options) Check {
	c := Check{Name: "capture"}

	id, err := store.Latest(o.Project)
	if err != nil || id == "" {
		c.Level, c.Detail = Warn, "no sessions recorded yet"
		c.Fix = "let Claude change a file, then look again"
		return c
	}
	s, err := store.Open(id)
	if err != nil {
		c.Level, c.Detail = Fail, err.Error()
		return c
	}
	sess, err := s.Read()
	if err != nil {
		c.Level, c.Detail = Fail, err.Error()
		return c
	}

	if len(sess.Events) == 0 {
		c.Level = Warn
		c.Detail = fmt.Sprintf("session %s has no edits recorded", shortID(id))
		c.Fix = "if Claude has changed files since it started, the hooks are not firing"
		return c
	}

	last := sess.Events[len(sess.Events)-1]
	c.Detail = fmt.Sprintf("%d edits, last %s ago (session %s)",
		len(sess.Events), coarse(time.Since(last.Time)), shortID(id))
	if sess.Meta.CWD == "" {
		c.Level = Warn
		c.Detail += "; this session records no project, so the panel may open another"
		c.Fix = "it fills itself in on the next prompt or tool call"
	}
	return c
}

func checkEditor(o Options) Check {
	c := Check{Name: "editor"}
	socks := editor.Sockets()
	if len(socks) == 0 {
		c.Detail = "no nvim running; o will start one"
		return c
	}
	if srv, err := editor.For(o.Project, socks); err == nil {
		c.Detail = "nvim on " + short(srv.CWD)
		return c
	}
	c.Detail = fmt.Sprintf("%d nvim running, none on this project; o will start one", len(socks))
	return c
}

func maxLevel(a, b Level) Level {
	if b > a {
		return b
	}
	return a
}

// coarse is a duration a reader can take in at a glance.
func coarse(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// short writes a path the way a prompt would, so the report stays readable.
func short(p string) string {
	if p == "" {
		return "unknown"
	}
	if home, err := os.UserHomeDir(); err == nil {
		if rest, ok := strings.CutPrefix(p, home); ok {
			return "~" + rest
		}
	}
	return p
}
