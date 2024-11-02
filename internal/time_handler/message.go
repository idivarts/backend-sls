package timehandler

import (
	"math/rand"

	"github.com/idivarts/backend-sls/internal/models"
	"github.com/idivarts/backend-sls/pkg/openai"
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

	campaign := &models.Campaign{}
	err := campaign.Get(conv.OrganizationID, conv.CampaignID)
	if err != nil {
		return nil, err
	}

	// pData := &models.Source{}
	// err = pData.Get(conv.SourceID)
	// if err != nil {
	// 	return nil, err
	// }

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

	rt1, rt2 := splitRange(campaign.ReplySpeed.Min, campaign.ReplySpeed.Max)
	calcTime := int(difference)
	if calcTime > 30*60 {
		calcTime = generateRandomNumberInSeconds(rt2, campaign.ReplySpeed.Max)
	} else if calcTime > 10*60 {
		calcTime = generateRandomNumberInSeconds(rt1, rt2)
	} else {
		calcTime = generateRandomNumberInSeconds(campaign.ReplySpeed.Min, rt1)
	}

	// // TODO: Remove this one the testing phase is crossed
	// calcTime = int(15)

	return &calcTime, nil
}
