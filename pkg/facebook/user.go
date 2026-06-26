package facebook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

type UserProfile struct {
	Name                 string `json:"name" firestore:"name"`
	Username             string `json:"username" firestore:"username"`
	ProfilePic           string `json:"profile_pic" firestore:"profile_pic"`
	FollowerCount        int    `json:"follower_count" firestore:"follower_count"`
	IsUserFollowBusiness bool   `json:"is_user_follow_business" firestore:"is_user_follow_business"`
	IsBusinessFollowUser bool   `json:"is_business_follow_user" firestore:"is_business_follow_user"`
}

func (user UserProfile) GenerateUserDescription() string {
	description := "Address the user with there name to make the message personalized. If name is missing, address them with there username.\n-----------------\nBelow are the details about the user -\n"
	description += "Name: " + user.Name + "\n"
	description += "Username: " + user.Username + "\n"
	// description += "Profile Picture: " + user.ProfilePic + "\n"
	description += "Follower Count: " + strconv.Itoa(user.FollowerCount) + "\n"

	if user.IsUserFollowBusiness {
		description += "User follows the trendshub page.\n"
	} else {
		description += "User does not follow the trendshub page.\n"
	}

	if user.IsBusinessFollowUser {
		description += "TrendsHub follows the user.\n"
	} else {
		description += "TrendsHub follows the user.\n"
	}

	return description
}

// MessengerUserProfile is a Facebook page-scoped user (PSID) profile from the
// Messenger User Profile API. Facebook users have NO username / follower_count
// (those are Instagram-only) and no single `name` field — the display name is
// composed from first_name + last_name.
type MessengerUserProfile struct {
	ID         string `json:"id"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	ProfilePic string `json:"profile_pic"`
}

// FullName joins first + last into a display name (either may be empty).
func (m MessengerUserProfile) FullName() string {
	return strings.TrimSpace(strings.TrimSpace(m.FirstName) + " " + strings.TrimSpace(m.LastName))
}

// GetMessengerUser fetches a Facebook page-scoped user's (PSID) profile via the
// Messenger User Profile API. Only first_name/last_name/profile_pic are valid
// here — requesting Instagram fields (username, follower_count) returns
// "(#100) Tried accessing nonexisting field". Requires the page token to hold
// pages_messaging and the user to have an open thread with the page.
//
//	GET graph.facebook.com/{version}/{psid}?fields=first_name,last_name,profile_pic
func GetMessengerUser(psid string, accessToken string) (*MessengerUserProfile, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=first_name,last_name,profile_pic&access_token=%s", BaseURL, ApiVersion, psid, accessToken)

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	data := &MessengerUserProfile{}
	if err := json.Unmarshal(body, data); err != nil {
		return nil, err
	}
	return data, nil
}

func GetUser(igsid string, accessToken string) (*UserProfile, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=name,username,profile_pic,follower_count,is_user_follow_business,is_business_follow_user&access_token=%s", BaseURL, ApiVersion, igsid, accessToken)

	// Send GET request
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error making GET request:", err)
		return nil, err
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error reading response body:", err)
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("Error: Unexpected status code - " + resp.Status + "\n" + string(body))
	}

	data := &UserProfile{}
	err = json.Unmarshal(body, data)
	if err != nil {
		return nil, err
	}

	return data, err
}
