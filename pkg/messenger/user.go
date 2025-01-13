package messenger

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
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

func GetUser(igsid string, pageAccessToken string) (*UserProfile, error) {
	url := fmt.Sprintf("%s/%s/%s?fields=name,username,profile_pic,follower_count,is_user_follow_business,is_business_follow_user&access_token=%s", BaseURL, ApiVersion, igsid, pageAccessToken)

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
