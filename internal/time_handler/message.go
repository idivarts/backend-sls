package timehandler

import (
	"math/rand"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

func splitRange(min, max int) (int, int) {
	rangeSize := max - min

	// Calculate the size of each third
	thirdSize := rangeSize / 3

	// Calculate the split points
	firstSplit := min + thirdSize
	secondSplit := max - thirdSize

	return firstSplit, secondSplit
}
func generateRandomNumberInSeconds(min, max int) int {
	// rand.Seed(time.Now().UnixNano()) // Seed the random number generator

	return (rand.Intn(max-min+1) + min)
}

func CalculateMessageDelay(conv *models.Conversation) (*int, error) {

	pData := &models.Page{}
	err := pData.Get(conv.PageID)
	if err != nil {
		return nil, err
	}

	msgs, err := openai.GetMessages(conv.ThreadID, 5, "")
	if err != nil {
		return nil, err
	}

	lastUserMessageTime := int64(0)
	difference := int64(60)
	for _, v := range msgs.Data {
		if v.Role == "user" {
			lastUserMessageTime = v.CreatedAt
		} else if v.Role == "assistant" && lastUserMessageTime != 0 {
			difference = lastUserMessageTime - v.CreatedAt
			break
		}
	}

	rt1, rt2 := splitRange(pData.ReplyTimeMin, pData.ReplyTimeMax)
	calcTime := int(difference)
	if calcTime > 30*60 {
		calcTime = generateRandomNumberInSeconds(rt2, pData.ReplyTimeMax)
	} else if calcTime > 10*60 {
		calcTime = generateRandomNumberInSeconds(rt1, rt2)
	} else {
		calcTime = generateRandomNumberInSeconds(pData.ReplyTimeMin, rt1)
	}

	// // TODO: Remove this one the testing phase is crossed
	// calcTime = int(15)

	return &calcTime, nil
}
