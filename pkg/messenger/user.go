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
	Name                 string `json:"name" dynamodbav:"name"`
	Username             string `json:"username" dynamodbav:"username"`
	ProfilePic           string `json:"profile_pic" dynamodbav:"profile_pic"`
	FollowerCount        int    `json:"follower_count" dynamodbav:"follower_count"`
	IsUserFollowBusiness bool   `json:"is_user_follow_business" dynamodbav:"is_user_follow_business"`
	IsBusinessFollowUser bool   `json:"is_business_follow_user" dynamodbav:"is_business_follow_user"`
}

func (user UserProfile) GenerateUserDescription() string {
	description := "Here are the details about the user(ignore if anything is blank) -\n"
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
