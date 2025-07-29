package payments

import (
	"log"
	"time"
)

func CreateSubscriptionLink(planId string, totalBillingCycles, trialDays, expireDays int, notes map[string]interface{}) (string, error) {
	linkData := map[string]interface{}{
		"plan_id":         planId,
		"total_count":     totalBillingCycles,
		"customer_notify": true,
		"notes":           notes,
	}
	if trialDays > 0 {
		linkData["start_at"] = time.Now().Add(time.Duration(trialDays * 24 * int(time.Hour))).Unix()
	}
	if expireDays > 0 {
		linkData["expire_by"] = time.Now().Add(time.Duration(expireDays * 24 * int(time.Hour))).Unix()
	}

	link, err := Client.Subscription.Create(linkData, nil)
	if err != nil {
		return "", err
	}
	log.Println("Link", link)
	return link["short_url"].(string), err
}
