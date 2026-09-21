// Package hooks wires lens into Claude Code's settings, and takes it back out.
//
// The settings file belongs to whoever owns it: other tools keep their hooks,
// unrelated keys are left as they are, and a file that cannot be parsed is not
// touched at all. Only entries that run lens itself are ever added or removed.
package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// marker identifies a hook entry as lens's own. It is the command shape rather
// than a path, so an install that has moved is still recognised as lens.
const marker = "lens hook "

// wanted is every hook lens needs, and the tools each one watches.
//
// Bash is matched as well as the editing tools: Claude often writes files with
// heredocs and sed rather than the Write tool, and those changes are found by
// looking at the project after the command runs.
var wanted = []struct {
	Event   string
	Matcher string
	Verb    string
}{
	{"PostToolUse", "Edit|Write|MultiEdit|NotebookEdit|Bash", "post-tool-use"},
	{"UserPromptSubmit", "", "prompt"},
	{"SessionStart", "", "session-start"},
	{"SessionEnd", "", "session-end"},
	{"Stop", "", "stop"},
}

// Events is the hook events lens registers, in the order it reports them.
func Events() []string {
	out := make([]string, 0, len(wanted))
	for _, w := range wanted {
		out = append(out, w.Event)
	}
	return out
}

// State is what is wired for one event, as the doctor reports it.
type State struct {
	Event     string
	Installed bool
	Binary    string // the lens the hook runs, when one is wired
}

// Install adds lens's hooks, replacing any it put there before — which is what
// makes running the installer again an upgrade rather than a duplication.
func Install(path, binary string) error {
	settings, err := load(path)
	if err != nil {
		return err
	}

	byEvent := hookMap(settings)
	for _, w := range wanted {
		entries := dropLens(entriesFor(byEvent, w.Event))

		entry := map[string]any{
			"hooks": []any{map[string]any{
				"type":    "command",
				"command": binary + " hook " + w.Verb,
			}},
		}
		if w.Matcher != "" {
			entry["matcher"] = w.Matcher
		}
		byEvent[w.Event] = append(entries, entry)
	}

	return save(path, settings)
}

// Remove takes lens's hooks out and reports how many it found, leaving every
// other tool's hooks where they are.
func Remove(path string) (int, error) {
	settings, err := load(path)
	if err != nil {
		return 0, err
	}

	byEvent := hookMap(settings)
	removed := 0
	for _, w := range wanted {
		entries := entriesFor(byEvent, w.Event)
		kept := dropLens(entries)
		removed += countLens(entries)

		if len(kept) == 0 {
			delete(byEvent, w.Event)
			continue
		}
		byEvent[w.Event] = kept
	}
	if len(byEvent) == 0 {
		delete(settings, "hooks")
	}

	if removed == 0 {
		return 0, nil // nothing to say, so nothing is rewritten
	}
	return removed, save(path, settings)
}

// Status reports which of lens's hooks are wired, and to which binary. Absent
// settings are an answer — nothing is installed — not a failure.
func Status(path string) ([]State, error) {
	settings, err := load(path)
	if err != nil {
		return nil, err
	}
	byEvent := hookMap(settings)

	out := make([]State, 0, len(wanted))
	for _, w := range wanted {
		state := State{Event: w.Event}
		for _, cmd := range commands(entriesFor(byEvent, w.Event)) {
			if !strings.Contains(cmd, marker) {
				continue
			}
			// The command is "<binary> hook <verb>", so the binary is what
			// comes before the verb rather than before the word "lens".
			state.Installed = true
			state.Binary, _, _ = strings.Cut(cmd, " hook ")
			state.Binary = strings.TrimSpace(state.Binary)
			break
		}
		out = append(out, state)
	}
	return out, nil
}

// load reads the settings, treating a missing file as an empty one. A file that
// cannot be parsed is an error: rewriting a broken config from a guess would
// lose whatever the owner meant to put in it.
func load(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]any{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hooks: reading %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(b))) == 0 {
		return map[string]any{}, nil
	}

	var settings map[string]any
	if err := json.Unmarshal(b, &settings); err != nil {
		return nil, fmt.Errorf("hooks: %s is not valid JSON, so it has been left alone: %w", path, err)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	return settings, nil
}

// save writes the settings out through a temporary file, so an interrupted
// write cannot leave the owner without a settings file at all.
// BackupPath is where the settings file as it was found is kept.
//
// One copy, overwritten: installing again is how an upgrade works, so a
// timestamped name would leave a drawer full of near-identical files. What a
// reader wants is the file as it was just before lens last touched it.
func BackupPath(path string) string { return path + ".lens-backup" }

// backup sets aside the file about to be rewritten. This file is full of
// settings lens does not own — another tool's hooks, the reader's own model
// choice — so a bug in the merging below should be recoverable rather than
// final. Nothing there yet is nothing to keep, which is not an error.
func backup(path string) error {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("hooks: reading %s: %w", path, err)
	}
	if err := os.WriteFile(BackupPath(path), b, 0o600); err != nil {
		return fmt.Errorf("hooks: keeping a copy of %s: %w", path, err)
	}
	return nil
}

func save(path string, settings map[string]any) error {
	if err := backup(path); err != nil {
		return err
	}

	b, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("hooks: %w", err)
	}
	b = append(b, '\n')

	if dir := filepath.Dir(path); dir != "" {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("hooks: %w", err)
		}
	}
	tmp := path + ".lens-tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return fmt.Errorf("hooks: writing %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("hooks: replacing %s: %w", path, err)
	}
	return nil
}

// hookMap is the settings' hooks object, created if it is missing.
func hookMap(settings map[string]any) map[string]any {
	byEvent, ok := settings["hooks"].(map[string]any)
	if !ok {
		byEvent = map[string]any{}
		settings["hooks"] = byEvent
	}
	return byEvent
}

func entriesFor(byEvent map[string]any, event string) []any {
	entries, _ := byEvent[event].([]any)
	return entries
}

// dropLens returns the entries that are not lens's, and drops any entry left
// with no hooks in it.
func dropLens(entries []any) []any {
	kept := make([]any, 0, len(entries))
	for _, e := range entries {
		entry, ok := e.(map[string]any)
		if !ok {
			kept = append(kept, e)
			continue
		}
		list, _ := entry["hooks"].([]any)

		mine := make([]any, 0, len(list))
		for _, h := range list {
			if !isLens(h) {
				mine = append(mine, h)
			}
		}
		if len(mine) == 0 {
			continue
		}
		entry["hooks"] = mine
		kept = append(kept, entry)
	}
	return kept
}

func countLens(entries []any) int {
	n := 0
	for _, h := range commands(entries) {
		if strings.Contains(h, marker) {
			n++
		}
	}
	return n
}

func commands(entries []any) []string {
	var out []string
	for _, e := range entries {
		entry, ok := e.(map[string]any)
		if !ok {
			continue
		}
		list, _ := entry["hooks"].([]any)
		for _, h := range list {
			hook, ok := h.(map[string]any)
			if !ok {
				continue
			}
			if cmd, ok := hook["command"].(string); ok {
				out = append(out, cmd)
			}
		}
	}
	return out
}

func isLens(h any) bool {
	hook, ok := h.(map[string]any)
	if !ok {
		return false
	}
	cmd, _ := hook["command"].(string)
	return strings.Contains(cmd, marker)
}
