package streamchat

import "time"

func CreateToken(userID string) (string, error) {
	token, err := Client.CreateToken(userID, time.Time{})
	return token, err
}
