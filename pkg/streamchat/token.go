package streamchat

import "time"

func CreateToken(userID string) (string, error) {
	token, err := Client.CreateToken(userID, time.Now().Add(time.Hour*1))
	return token, err
}
