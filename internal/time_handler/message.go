package timehandler

import (
	"math/rand"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/TrendsHub/th-backend/pkg/openai"
)

func CalculateMessageDelay(conv *models.Conversation) (*int, error) {

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

	calcTime := int(difference)
	if calcTime > 30*60 {
		calcTime = rand.Intn(30*60) + (15 * 60)
	} else if calcTime > 10*60 {
		calcTime = rand.Intn(10*60) + (5 * 60)
	} else {
		calcTime = rand.Intn(60) + (45)
	}
	// calcTime = rand.Intn(calcTime)
	// calcTime = int(math.Min(900, float64(calcTime)))

	return &calcTime, nil
}
