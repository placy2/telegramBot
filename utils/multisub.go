package utils

import (
	"fmt"

	"github.com/jzelinskie/geddit"
)

// GetFromSubreddits - abstract gathering of posts from multiple subreddits,
// with error checking. Returns numPosts length list of new posts from each, all in one slice.
func GetFromSubreddits(subreddits []string, numPosts int, session *geddit.LoginSession) []*geddit.Submission {
	subOpts := geddit.ListingOptions{
		Limit: numPosts,
	}
	var submissions []*geddit.Submission
	for _, name := range subreddits {
		temp, err := session.SubredditSubmissions(name, geddit.NewSubmissions, subOpts)

		if err != nil {
			fmt.Println("Error getting subreddit submissions", err)
			return nil
		}
		submissions = append(submissions, temp...)
	}

	return submissions
}
