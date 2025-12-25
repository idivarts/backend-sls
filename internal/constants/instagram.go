package constants

import (
	"fmt"
	"os"
)

func getStage() string {
	if os.Getenv("STAGE") == "dev" {
		return "/dev"
	}
	return ""
}

var INSTAGRAM_REDIRECT = fmt.Sprintf("%s%s%s", TRENDLY_BE, getStage(), "/instagram/auth")

type IInstaAuth struct {
	Code         string `json:"code"`
	RedirectType string `json:"redirect_type"`
}
