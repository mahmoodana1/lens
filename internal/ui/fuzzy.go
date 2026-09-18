package ui

import "strings"

// Scoring weights. A file you meant to find usually contains the query as one
// run, or at the start of a path segment, so both are worth far more than the
// bare fact that the characters appear in order.
const (
	scoreMatch       = 1  // every matched character counts for something
	scoreConsecutive = 10 // …and much more when it follows the previous one
	scoreSegment     = 6  // a match at the start of a path segment or word
	penaltyGap       = 1  // per character skipped between matches
)

// segmentBreaks are the characters a new word or path segment starts after.
const segmentBreaks = "/._- "

// Match reports whether query appears in s as a case-insensitive subsequence.
//
// It returns the byte positions of the matched characters, for highlighting,
// and a score ranking this candidate against others for the same query. An
// empty query matches everything, so clearing the prompt restores the list.
func Match(query, s string) (positions []int, score int, ok bool) {
	if query == "" {
		return nil, 0, true
	}

	q := []rune(strings.ToLower(query))
	hay := []rune(s)
	lower := []rune(strings.ToLower(s))

	positions = make([]int, 0, len(q))
	qi, last := 0, -1

	for i := 0; i < len(lower) && qi < len(q); i++ {
		if lower[i] != q[qi] {
			continue
		}

		score += scoreMatch
		switch {
		case last == i-1:
			score += scoreConsecutive
		case last >= 0:
			score -= (i - last - 1) * penaltyGap
		}
		if i == 0 || strings.ContainsRune(segmentBreaks, hay[i-1]) {
			score += scoreSegment
		}

		positions = append(positions, i)
		last = i
		qi++
	}

	if qi < len(q) {
		return nil, 0, false
	}
	return positions, score, true
}
