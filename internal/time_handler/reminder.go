package timehandler

import (
	"github.com/TrendsHub/th-backend/internal/models"
)

const REMINDER_SECONDS = 60 * 60 * 6

func CalculateRemiderDelay(conv *models.Conversation) int {
	calcTime := int(REMINDER_SECONDS)

	pData := &models.Source{}
	err := pData.Get(conv.PageID)
	if err != nil {
		// Do nothing
	} else {
		calcTime = (pData.ReminderTimeMultiplier)
	}

	calcTime = calcTime + calcTime*conv.ReminderCount

	return calcTime
}
