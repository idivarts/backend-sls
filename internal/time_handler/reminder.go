package timehandler

import (
	"github.com/TrendsHub/th-backend/internal/models"
)

const REMINDER_SECONDS = 60 * 60 * 6

func CalculateRemiderDelay(conv *models.Conversation) int {

	calcTime := int(REMINDER_SECONDS)
	calcTime = calcTime + calcTime*conv.ReminderCount
	// if calcTime > 30*60 {
	// 	calcTime = rand.Intn(30*60) + (15 * 60)
	// } else if calcTime > 10*60 {
	// 	calcTime = rand.Intn(10*60) + (5 * 60)
	// } else {
	// 	calcTime = rand.Intn(60) + (45)
	// }

	// TODO: Remove this one the testing phase is crossed
	// calcTime = int(15)

	return calcTime
}
