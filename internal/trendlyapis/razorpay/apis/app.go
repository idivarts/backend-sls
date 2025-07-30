package razorpayapis

import (
	"os"
	"strconv"
)

var (
	GROWTH_PLAN_ID       = "plan_QzPMAkZjXYbJV8"
	BUSINESS_PLAN_ID     = "plan_QzPN1kyqCq0ePk"
	COLLAB_BOOST_AMOUNT  = 799
	COLLAB_HANDLING_LINK = "https://rzp.io/rzp/A5hU0EUn"
)

func init() {
	gPlan := os.Getenv("GROWTH_PLAN_ID")
	bPlan := os.Getenv("BUSINESS_PLAN_ID")
	boostAmount := os.Getenv("COLLAB_BOOST_AMOUNT")
	collabHandlingLink := os.Getenv("COLLAB_HANDLING_LINK")

	if gPlan != "" {
		GROWTH_PLAN_ID = gPlan
	}
	if bPlan != "" {
		BUSINESS_PLAN_ID = bPlan
	}
	if boostAmount != "" {
		if amount, err := strconv.Atoi(boostAmount); err == nil {
			COLLAB_BOOST_AMOUNT = amount
		}
	}
	if collabHandlingLink != "" {
		COLLAB_HANDLING_LINK = collabHandlingLink
	}
}
