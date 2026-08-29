package tasks

import "strings"

// matchesAny finds whether haystack contains any needles, case-insensitively.
func matchesAny(haystack string, needles []string) bool {
	lower := strings.ToLower(haystack)
	for _, n := range needles {
		if strings.Contains(lower, strings.ToLower(n)) {
			return true
		}
	}
	return false
}
