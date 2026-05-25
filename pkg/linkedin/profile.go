package linkedin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Profile represents a LinkedIn member profile from the OpenID Connect userinfo endpoint.
// For full profile data (follower count, etc.) you need the Marketing Developer Platform.
type Profile struct {
	Sub        string `json:"sub"`        // LinkedIn member URN, e.g. "urn:li:person:abc123"
	Name       string `json:"name"`
	GivenName  string `json:"given_name"`
	FamilyName string `json:"family_name"`
	Picture    string `json:"picture"`
	Email      string `json:"email"`
	Locale     string `json:"locale"`
}

// GetMe returns the authenticated member's profile via the OIDC userinfo endpoint.
// accessToken must have openid + profile + email scopes.
func GetMe(accessToken string) (*Profile, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.linkedin.com/v2/userinfo", nil)
	if err != nil {
		return nil, fmt.Errorf("linkedin: failed to build profile request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: profile request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin: userinfo endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var p Profile
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, fmt.Errorf("linkedin: failed to parse profile response: %w", err)
	}
	return &p, nil
}

// GetFollowerCount fetches the follower count for the authenticated member.
// Requires r_organization_followers or r_1st_connections_size permission
// (available on upgraded LinkedIn partner API access).
// Returns -1 if the count cannot be determined with current scopes.
func GetFollowerCount(accessToken string) (int64, error) {
	url := APIURL + "/networkSizes/urn:li:member:~?edgeType=CompanyFollowedByMember"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return -1, fmt.Errorf("linkedin: failed to build follower count request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1, fmt.Errorf("linkedin: follower count request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Not all apps have access — return gracefully
		return -1, nil
	}

	var result struct {
		FirstDegreeSize int64 `json:"firstDegreeSize"`
	}
	body, _ := io.ReadAll(resp.Body)
	if err := json.Unmarshal(body, &result); err != nil {
		return -1, nil
	}
	return result.FirstDegreeSize, nil
}
