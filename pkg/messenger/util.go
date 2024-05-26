package messenger

func FetchAllMessages(convId string, after *string, pageAccessToken string) []Message {
	if after != nil && *after == "" {
		return []Message{}
	}
	messages := []Message{}
	aStr := ""
	if after != nil {
		aStr = *after
	}
	data, err := GetMessagesWithPagination(convId, aStr, 20, pageAccessToken)
	if err != nil {
		return []Message{}
	}
	messages = append(messages, data.Data...)
	messages = append(messages, FetchAllMessages(convId, &data.Paging.Cursors.After, pageAccessToken)...)
	return messages
}

func FetchAllConversations(after *string, pageAccessToken string) []ConversationMessagesData {
	if after != nil && *after == "" {
		return []ConversationMessagesData{}
	}
	aStr := ""
	if after != nil {
		aStr = *after
	}
	data, err := GetConversationsPaginated(aStr, 10, pageAccessToken)
	if err != nil {
		return []ConversationMessagesData{}
	}
	messages := []ConversationMessagesData{}
	messages = append(messages, data.Data...)
	messages = append(messages, FetchAllConversations(&data.Paging.Cursors.After, pageAccessToken)...)

	return messages
}
