package twitter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

// GetOwnTweetsWithMetrics returns the authenticated user's recent original
// tweets enriched with private (non_public / organic) metrics. The impression
// count is merged from organic/non_public metrics when present, falling back to
// public_metrics.impression_count.
func GetOwnTweetsWithMetrics(accessToken, userID string, maxResults int) ([]Tweet, error) {
	q := url.Values{}
	q.Set("max_results", strconv.Itoa(maxResults))
	q.Set("tweet.fields", "created_at,public_metrics,non_public_metrics,organic_metrics")
	q.Set("exclude", "replies,retweets")
	requestURL := fmt.Sprintf("%s/users/%s/tweets?%s", APIURL, userID, q.Encode())

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build own tweets metrics request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: own tweets metrics request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter: own tweets metrics returned %d: %s", resp.StatusCode, string(body))
	}

	var r tweetsListResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse own tweets metrics response: %w", err)
	}

	tweets := make([]Tweet, 0, len(r.Data))
	for _, p := range r.Data {
		t := p.toTweet()
		// Prefer organic, then non_public, then public impression counts.
		if p.OrganicMetrics.ImpressionCount > 0 {
			t.Impressions = p.OrganicMetrics.ImpressionCount
		} else if p.NonPublicMetrics.ImpressionCount > 0 {
			t.Impressions = p.NonPublicMetrics.ImpressionCount
		}
		tweets = append(tweets, t)
	}
	return tweets, nil
}
