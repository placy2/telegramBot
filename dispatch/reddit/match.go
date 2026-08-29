package reddit

import "strings"

// MatchesAny reports whether haystack contains any needle, case-insensitively.
// Exported (rather than living privately in package tasks, its original home)
// so both dispatch/tasks and dispatch/poller can share it without an import
// cycle — poller needs it to filter entries before they're persisted.
func MatchesAny(haystack string, needles []string) bool {
	lower := strings.ToLower(haystack)
	for _, n := range needles {
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}
