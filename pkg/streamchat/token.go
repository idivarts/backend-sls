package streamchat

import "time"

func CreateToken(userID string) {
	Client.CreateToken(userID, time.Now().Add(time.Hour*1))
}
