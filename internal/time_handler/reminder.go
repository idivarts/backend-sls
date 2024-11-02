package timehandler

import (
	"github.com/idivarts/backend-sls/internal/models"
)

const REMINDER_SECONDS = 60 * 60 * 6

func CalculateRemiderDelay(conv *models.Conversation) int {
	calcTime := int(REMINDER_SECONDS)

	campaign := &models.Campaign{}
	err := campaign.Get(conv.OrganizationID, conv.CampaignID)
	if err != nil {
		// Do Nothing
	} else {
		calcTime = (campaign.ReminderTiming.Min)
	}

	calcTime = calcTime + calcTime*conv.ReminderCount

	return calcTime
}
