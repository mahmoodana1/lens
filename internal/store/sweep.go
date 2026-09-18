package store

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Sweep removes session directories untouched for longer than olderThan.
// Sessions are normally deleted when the panel is closed; this catches the ones
// whose panel was never opened, or was killed with the terminal.
func Sweep(olderThan time.Duration) error {
	root := Root()
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	cutoff := time.Now().Add(-olderThan)
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			os.RemoveAll(filepath.Join(root, e.Name()))
		}
	}
	return nil
}
