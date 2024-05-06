package messenger

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
)

type UserProfile struct {
	Name                 string `json:"name"`
	Username             string `json:"username"`
	ProfilePic           string `json:"profile_pic"`
	FollowerCount        int    `json:"follower_count"`
	IsUserFollowBusiness bool   `json:"is_user_follow_business"`
	IsBusinessFollowUser bool   `json:"is_business_follow_user"`
}

func (user UserProfile) GenerateUserDescription() string {
	description := "Here are the details abot the user(ignore if anythig is blank) -\n"
	description += "Name: " + user.Name + "\n"
	description += "Username: " + user.Username + "\n"
	description += "Profile Picture: " + user.ProfilePic + "\n"
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

func GetUser(igsid string) (*UserProfile, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=name,username,profile_pic,follower_count,is_user_follow_business,is_business_follow_user&access_token=%s", baseURL, apiVersion, igsid, pageAccessToken)

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

	data := &UserProfile{}
	err = json.Unmarshal(body, data)
	if err != nil {
		return nil, err
	}

	return data, err
}
