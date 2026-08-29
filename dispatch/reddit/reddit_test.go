package reddit

import "testing"

func TestOldLink(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "absolute reddit.com URL",
			in:   "https://www.reddit.com/r/soccer/comments/1vuw4s6/some_post/",
			want: "https://old.reddit.com/r/soccer/comments/1vuw4s6/some_post/",
		},
		{
			name: "non-reddit URL is returned unchanged",
			in:   "https://example.com/r/soccer/comments/1vuw4s6/some_post/",
			want: "https://example.com/r/soccer/comments/1vuw4s6/some_post/",
		},
		{
			name: "malformed URL is returned unchanged",
			in:   "://not a url",
			want: "://not a url",
		},
		{
			name: "already old.reddit.com is left as-is",
			in:   "https://old.reddit.com/r/soccer/comments/1vuw4s6/some_post/",
			want: "https://old.reddit.com/r/soccer/comments/1vuw4s6/some_post/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := OldLink(tc.in); got != tc.want {
				t.Errorf("OldLink(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMatchesAny(t *testing.T) {
	cases := []struct {
		name     string
		haystack string
		needles  []string
		want     bool
	}{
		{"exact substring", "This is so HYPE", []string{"hype"}, true},
		{"different casing on both sides", "totally hYpE right now", []string{"HYPE"}, true},
		{"no match", "just a normal title", []string{"hype"}, false},
		{"matches within a larger word", "I got hyped up", []string{"hype"}, true},
		{"empty needle list never matches", "anything at all", nil, false},
		{"second needle matches", "Rocket League highlights", []string{"hype", "highlights"}, true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := MatchesAny(tc.haystack, tc.needles); got != tc.want {
				t.Errorf("MatchesAny(%q, %v) = %v, want %v", tc.haystack, tc.needles, got, tc.want)
			}
		})
	}
}
