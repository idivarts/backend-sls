package mwh_handler

import (
	"encoding/json"
	"log"
	"testing"
)

func TestFetchMessages(t *testing.T) {
	msg := &IGMessagehandler{}
	convId := "aWdfZAG06MTpJR01lc3NhZA2VUaHJlYWQ6MTc4NDE0NjY2MTgxNTEyOTQ6MzQwMjgyMzY2ODQxNzEwMzAxMjQ0Mjc2MDE4NTg4NTUwMzA3NTgy"
	pageAccessToken := "EAAGDG5jzw5QBO75McvccVCMJTbrhPZBkqhX80HwtjRSLriQ6UGecV8345ZCE6ZA4VeMY7ZBr3LucHlffJbxRAV27eNDG5rwjViYFqjFpNJwYZB7dvOG4UVQl7U3W9LmBXBlxuYHettVX2PxbT2ORdZBZBMNXgNCvD7HheqH0IbffskA2ImwYa923jouVFITwhB3aSYoD57nFXaRsZAG8Jacn8fRU"
	msgs := msg._fetchMessages(convId, nil, pageAccessToken)
	b, err := json.Marshal(&msgs)
	if err != nil {
		t.Fail()
	}
	log.Println(string(b))
	log.Println("Total Messages", len(msgs))
}
