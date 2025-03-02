package messenger

import (
	"encoding/json"
	"log"
	"testing"
)

func TestConversationFetch(t *testing.T) {
	pToken := "EAAGDG5jzw5QBOyTEBNU9EMBmICh4WuTppVDZC4A3pqQDyaeIxXg0KhU0nbeSkO1Bb4El3U2sPIwcpLdp17MtNg7HTOgBip7lMVMhnktYy7M0P00yWvUe29a0C1ozMrqJGdUo3ZAWEENy0qoKDweZBALZBZAEo2i0QWQTOBlRcHWyZAMoCKGXTIhLvA4vMZBBIbdEg0FHL4cM0H7i1lvMnO0MVLi"
	data := FetchAllConversations(nil, pToken)
	log.Println(len(data))
	log.Println(data)
}

func TestFetchMessages(t *testing.T) {
	// msg := &IGMessagehandler{}
	convId := "aWdfZAG06MTpJR01lc3NhZA2VUaHJlYWQ6MTc4NDE0NjY2MTgxNTEyOTQ6MzQwMjgyMzY2ODQxNzEwMzAxMjQ0Mjc2MDE4NTg4NTUwMzA3NTgy"
	pageAccessToken := "EAAGDG5jzw5QBO75McvccVCMJTbrhPZBkqhX80HwtjRSLriQ6UGecV8345ZCE6ZA4VeMY7ZBr3LucHlffJbxRAV27eNDG5rwjViYFqjFpNJwYZB7dvOG4UVQl7U3W9LmBXBlxuYHettVX2PxbT2ORdZBZBMNXgNCvD7HheqH0IbffskA2ImwYa923jouVFITwhB3aSYoD57nFXaRsZAG8Jacn8fRU"
	msgs := FetchAllMessages(convId, nil, pageAccessToken)
	b, err := json.Marshal(&msgs)

	if err != nil {
		t.Fail()
	}

	log.Println(string(b))
	log.Println("Total Messages", len(msgs))
}

func TestFetchPosts(t *testing.T) {
	pageAccessToken := "EAAID6icQOs4BO7wIl6hDNTBdRWmMhHgnoeF4AgZA5D96CIOBl7WlTeFslrMZC4OtZA44cgeRd4jxJXarkARDwZCjHZArvv1pgC8QA9EBXnARFbrPk1wulK8zaJM4FfMZAnAnwBhPhr4PRbMdEMMWeGQvuLvHKZBjUQGpV54HX5awZCpW2YupSfrfljbgrMFiq0bN"
	posts, err := GetPosts("", pageAccessToken, IFBPostsParams{Count: 10})
	if err != nil {
		t.Error(err)
		return
	}
	log.Println("Posts:", len(posts))
}
