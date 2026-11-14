package terminal

import (
	"slices"
	"strings"
	"unicode"

	"clara-code/internal/session"
)

func matchInputHistory(entries []session.InputHistoryEntry, query string) []int {
	query = normalizeHistoryMatch(query)
	if query == "" {
		matches := make([]int, len(entries))
		for i := range matches {
			matches[i] = len(entries) - 1 - i
		}
		return matches
	}
	queryRunes := []rune(query)
	var consecutive, subsequence []int
	for i, entry := range slices.Backward(entries) {
		candidate := normalizeHistoryMatch(entry.Text)
		if strings.Contains(candidate, query) {
			consecutive = append(consecutive, i)
		} else if historySubsequence(candidate, queryRunes) {
			subsequence = append(subsequence, i)
		}
	}
	return append(consecutive, subsequence...)
}

func normalizeHistoryMatch(text string) string {
	space := true
	folded := strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			if space {
				return -1
			}
			space = true
			return ' '
		}
		space = false
		if r <= unicode.MaxASCII {
			return unicode.ToUpper(r)
		}
		// SimpleFold cycles through the equivalent runes in ascending order.
		for {
			next := unicode.SimpleFold(r)
			if next <= r {
				return next
			}
			r = next
		}
	}, text)
	return strings.TrimSuffix(folded, " ")
}

func historySubsequence(text string, query []rune) bool {
	at := 0
	for _, r := range text {
		if r == query[at] {
			at++
			if at == len(query) {
				return true
			}
		}
	}
	return false
}
