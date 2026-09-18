// Command lens shows what Claude Code changes in your code, as it happens.
//
// It wears two hats. As a hook it records edits:
//
//	lens hook post-tool-use | prompt | session-end
//
// As a panel it displays them:
//
//	lens [--session ID]
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mahmood/lens/internal/hook"
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

	fs := flag.NewFlagSet("lens", flag.ContinueOnError)
	session := fs.String("session", "", "session to show (default: the most recent)")
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

	id := *session
	if id == "" {
		latest, err := latestSession()
		if err != nil || latest == "" {
			fmt.Fprintln(os.Stderr, "lens: no active session. It opens by itself when Claude edits a file.")
			os.Exit(1)
		}
		id = latest
	}

	m, err := ui.New(id)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lens:", err)
		os.Exit(1)
	}

	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "lens:", err)
		os.Exit(1)
	}

	// Closing the panel ends the session's life on disk.
	if s, err := store.Open(id); err == nil {
		s.Destroy()
	}
}

func runHook(name string) {
	var err error
	switch name {
	case "post-tool-use":
		err = hook.PostToolUse(os.Stdin)
	case "prompt":
		err = hook.Prompt(os.Stdin)
	case "session-end":
		err = hook.SessionEnd(os.Stdin)
	default:
		hook.Debugf("unknown hook %q", name)
		return
	}
	if err != nil {
		hook.Debugf("%s: %v", name, err)
	}
}

// latestSession finds the most recently touched session, so `lens` in a bare
// terminal attaches to whatever Claude is doing now.
func latestSession() (string, error) {
	entries, err := os.ReadDir(store.Root())
	if err != nil {
		return "", err
	}
	var best string
	var bestTime time.Time
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(bestTime) {
			bestTime, best = info.ModTime(), e.Name()
		}
	}
	return best, nil
}
