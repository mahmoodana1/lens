// Command lens shows what Claude Code changes in your code, as it happens.
//
// It wears two hats. As a hook it records edits:
//
//	lens hook session-start | post-tool-use | prompt | stop | session-end
//
// As a panel it displays them, in a tmux popup over the session:
//
//	lens [--session ID]   the panel itself, as the popup runs it
//	lens popup            open that popup over the current tmux pane, for
//	                      the project that pane is working in
//	lens auto [on|off]    whether the popup opens by itself, for this project
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mahmood/lens/internal/editor"
	"github.com/mahmood/lens/internal/hook"
	"github.com/mahmood/lens/internal/pane"
	"github.com/mahmood/lens/internal/store"
	"github.com/mahmood/lens/internal/ui"
)

const version = "0.1.0"

func main() {
	// Hooks run on every edit of every session. They must never take a session
	// down, so they always exit 0 whatever happens inside.
	if len(os.Args) > 2 && os.Args[1] == "hook" {
		runHook(os.Args[2])
		os.Exit(0)
	}

	// `lens auto` turns the automatic popup on and off. It prints where the
	// setting landed, which is what the tmux binding shows.
	if len(os.Args) > 1 && os.Args[1] == "auto" {
		if err := runAuto(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "lens:", err)
			os.Exit(1)
		}
		return
	}

	// `lens popup` re-opens the panel over the session it belongs to. It is what
	// the tmux binding runs, so it returns as soon as the popup is up.
	if len(os.Args) > 1 && os.Args[1] == "popup" {
		if err := runPopup(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "lens:", err)
			os.Exit(1)
		}
		return
	}

	fs := flag.NewFlagSet("lens", flag.ContinueOnError)
	session := fs.String("session", "", "session to show (default: this project's)")
	project := fs.String("project", "", "project to show a session for (default: the working directory)")
	showVersion := fs.Bool("version", false, "print the version and exit")
	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if *showVersion {
		fmt.Println("lens", version)
		return
	}

	// Sessions whose panel was never opened would otherwise linger.
	store.Sweep(24 * time.Hour)

	id, err := resolveSession(*session, *project)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lens:", err)
		os.Exit(1)
	}

	m, err := ui.New(id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lens:", err)
		os.Exit(1)
	}

	// Opening a change where it lives is the panel's one reach outside itself.
	sessionProject := m.Project()
	m.OnOpen(func(file string, line int) {
		err := editor.Show(editor.Request{
			Project: sessionProject, File: file, Line: line,
			Sockets: editor.Sockets(),
			Find:    pane.EditorPane,
			Reveal:  pane.Reveal,
			Start:   pane.NewWindow,
		})
		if err != nil {
			fmt.Fprintln(os.Stderr, "lens:", err)
		}
	})

	// The panel lives in a tmux popup, which is already its own screen.
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "lens:", err)
		os.Exit(1)
	}
	// Closing the panel only closes the popup: the capture stays on disk so the
	// tmux binding can bring it back. SessionEnd and Sweep clear it up.
}

// runAuto reads or flips whether the panel opens by itself for a project.
//
// The project defaults to the working directory, which is why the tmux binding
// runs it from the pane's own path: a key binding's command otherwise inherits
// the tmux server's directory, not the one you are working in.
func runAuto(args []string) error {
	want, dir, err := parseAutoArgs(args)
	if err != nil {
		return err
	}

	var on bool
	switch want {
	case "toggle":
		on, err = store.ToggleAutoOpen(dir)
	case "on":
		on, err = true, store.SetAutoOpen(dir, true)
	case "off":
		on, err = false, store.SetAutoOpen(dir, false)
	case "status":
		on = store.AutoOpen(dir)
	}
	if err != nil {
		return err
	}

	state := "off"
	if on {
		state = "on"
	}
	// The whole path, not its last element: two checkouts of the same repo are
	// two projects, and "auto-open off for lens" would not say which.
	fmt.Printf("lens: auto-open %s for %s\n", state, dir)
	return nil
}

// autoUsage is what an unrecognised `lens auto` invocation is answered with.
const autoUsage = "usage: lens auto [--project DIR] [on|off|toggle|status]"

// parseAutoArgs reads a `lens auto` command line, defaulting to toggling the
// working directory.
//
// The flag package stops at the first non-flag argument, so `lens auto on
// --project X` would otherwise parse as "on" with the flag silently dropped —
// and act on the wrong project. The verb is taken out first, wherever it sits,
// and the flags parsed from what is left.
func parseAutoArgs(args []string) (action, dir string, err error) {
	action = "toggle"
	seen := false
	rest := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		a := args[i]
		switch a {
		case "on", "off", "toggle", "status":
			if seen {
				return "", "", errors.New(autoUsage)
			}
			action, seen = a, true
		case "--project", "-project":
			// Its value is not a verb, so it travels with the flag.
			rest = append(rest, a)
			if i+1 < len(args) {
				i++
				rest = append(rest, args[i])
			}
		default:
			rest = append(rest, a)
		}
	}

	fs := flag.NewFlagSet("lens auto", flag.ContinueOnError)
	project := fs.String("project", "", "project directory (default: the working directory)")
	if err := fs.Parse(rest); err != nil {
		return "", "", err
	}
	if fs.NArg() > 0 {
		return "", "", errors.New(autoUsage)
	}

	dir = *project
	if dir == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return "", "", fmt.Errorf("no project directory: %w", err)
		}
		dir = cwd
	}
	// Resolved here so the confirmation names the project the setting is filed
	// under, not the "." or "../.." it was reached by.
	return action, store.ProjectKey(dir), nil
}

// runPopup opens the panel for a session in a tmux popup and returns.
func runPopup(args []string) error {
	fs := flag.NewFlagSet("lens popup", flag.ContinueOnError)
	session := fs.String("session", "", "session to show (default: this project's)")
	project := fs.String("project", "", "project to show a session for (default: the pane's directory)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	id, err := resolveSession(*session, *project)
	if err != nil {
		return err
	}
	self, err := os.Executable()
	if err != nil {
		self = "lens"
	}
	return pane.Open(id, self)
}

// resolveSession picks the session to show: the one named, or the one belonging
// to the project the panel was asked for.
//
// The project matters because every session's directory is touched as its panel
// is read, so "the most recent session" is often whichever one was last looked
// at rather than the one in front of you. A panel opened over a project has to
// be that project's.
func resolveSession(id, project string) (string, error) {
	if id != "" {
		return id, nil
	}
	latest, err := store.Latest(projectOrHere(project))
	if err != nil || latest == "" {
		return "", errors.New("no active session. It opens by itself when Claude edits a file.")
	}
	return latest, nil
}

// projectOrHere works out which project was meant: the one asked for, else the
// directory of the pane the popup covers, else the working directory.
//
// The pane comes before the working directory because a tmux key binding is run
// by the server: its command inherits the server's directory, not the reader's,
// so `lens popup` from a binding has no other way to know where it was pressed.
func projectOrHere(project string) string {
	if project != "" {
		return store.ProjectKey(project)
	}
	if path := pane.CurrentPath(); path != "" {
		return store.ProjectKey(path)
	}
	if cwd, err := os.Getwd(); err == nil {
		return store.ProjectKey(cwd)
	}
	return ""
}

func runHook(name string) {
	var err error
	switch name {
	case "post-tool-use":
		err = hook.PostToolUse(os.Stdin)
	case "prompt":
		err = hook.Prompt(os.Stdin)
	case "session-start":
		err = hook.SessionStart(os.Stdin)
	case "session-end":
		err = hook.SessionEnd(os.Stdin)
	case "stop":
		err = hook.Stop(os.Stdin)
	default:
		hook.Debugf("unknown hook %q", name)
		return
	}
	if err != nil {
		hook.Debugf("%s: %v", name, err)
	}
}
