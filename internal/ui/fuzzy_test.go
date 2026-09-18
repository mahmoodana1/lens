package ui_test

import (
	"reflect"
	"testing"

	"github.com/mahmood/lens/internal/ui"
)

func TestFuzzy_MatchesASubsequence(t *testing.T) {
	if _, _, ok := ui.Match("uik", "internal/ui/keys.go"); !ok {
		t.Error("uik should match internal/ui/keys.go")
	}
}

func TestFuzzy_IgnoresCase(t *testing.T) {
	if _, _, ok := ui.Match("UIK", "internal/ui/keys.go"); !ok {
		t.Error("the query should not be case sensitive")
	}
}

// Characters out of order are not a match: the query is a subsequence, not a bag.
func TestFuzzy_RejectsCharactersOutOfOrder(t *testing.T) {
	if _, _, ok := ui.Match("kiu", "internal/ui/keys.go"); ok {
		t.Error("kiu is not in order, so it should not match")
	}
}

func TestFuzzy_RejectsMissingCharacters(t *testing.T) {
	if _, _, ok := ui.Match("zz", "internal/ui/keys.go"); ok {
		t.Error("zz should not match")
	}
}

// An empty query matches everything, so clearing the prompt shows the whole list.
func TestFuzzy_EmptyQueryMatchesEverything(t *testing.T) {
	pos, _, ok := ui.Match("", "anything")
	if !ok {
		t.Error("an empty query should match")
	}
	if len(pos) != 0 {
		t.Errorf("an empty query highlights nothing, got %v", pos)
	}
}

// The positions drive the highlighting in the list.
func TestFuzzy_ReportsMatchedPositions(t *testing.T) {
	pos, _, ok := ui.Match("key", "ui/keys.go")
	if !ok {
		t.Fatal("key should match ui/keys.go")
	}
	if want := []int{3, 4, 5}; !reflect.DeepEqual(pos, want) {
		t.Errorf("positions = %v, want %v", pos, want)
	}
}

// A file whose name contains the query in one run is the one you meant.
func TestFuzzy_ScoresConsecutiveRunsHigher(t *testing.T) {
	_, run, _ := ui.Match("keys", "internal/ui/keys.go")
	_, scattered, _ := ui.Match("keys", "internal/k/e/y/s.go")

	if run <= scattered {
		t.Errorf("consecutive match scored %d, scattered scored %d; the run should win", run, scattered)
	}
}

// Matching the start of a path segment beats matching in the middle of a word.
func TestFuzzy_ScoresSegmentStartsHigher(t *testing.T) {
	_, atStart, _ := ui.Match("st", "internal/store.go")
	_, midWord, _ := ui.Match("st", "internal/fastest.go")

	if atStart <= midWord {
		t.Errorf("segment start scored %d, mid-word scored %d; the segment start should win", atStart, midWord)
	}
}
