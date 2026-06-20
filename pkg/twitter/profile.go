package twitter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// User represents a Twitter/X user from the v2 API.
type User struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Username        string        `json:"username"` // handle without @
	Description     string        `json:"description"`
	ProfileImageURL string        `json:"profile_image_url"`
	PublicMetrics   PublicMetrics `json:"public_metrics"`
	Verified        bool          `json:"verified"`
	VerifiedType    string        `json:"verified_type"` // "blue", "business", "government"
}

// PublicMetrics contains public follower/following/tweet counts.
type PublicMetrics struct {
	FollowersCount int64 `json:"followers_count"`
	FollowingCount int64 `json:"following_count"`
	TweetCount     int64 `json:"tweet_count"`
	ListedCount    int64 `json:"listed_count"`
}

type meResponse struct {
	Data User `json:"data"`
}

// GetMe returns the authenticated user's own profile.
// accessToken must have users.read and tweet.read scopes.
func GetMe(accessToken string) (*User, error) {
	url := APIURL + "/users/me?user.fields=description,profile_image_url,public_metrics,verified,verified_type"

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("twitter: failed to build /me request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("twitter: /me request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("twitter: /users/me returned %d: %s", resp.StatusCode, string(body))
	}

	var r meResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, fmt.Errorf("twitter: failed to parse /me response: %w", err)
	}
	return &r.Data, nil
}
