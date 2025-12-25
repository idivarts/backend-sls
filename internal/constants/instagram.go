package constants

import (
	"fmt"

	"github.com/idivarts/backend-sls/pkg/myutil"
)

func getStage() string {
	if myutil.IsDevEnvironment() {
		return "/dev"
	}
	return ""
}

var INSTAGRAM_REDIRECT = fmt.Sprintf("%s%s%s", TRENDLY_BE, getStage(), "/instagram/auth")

type IInstaAuth struct {
	Code         string `json:"code"`
	RedirectType string `json:"redirect_type"`
}
