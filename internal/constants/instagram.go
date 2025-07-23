package constants

import (
	"fmt"
)

var INSTAGRAM_REDIRECT = fmt.Sprintf("%s%s", TRENDLY_BE, "/instagram/auth")

type IInstaAuth struct {
	Code         string `json:"code"`
	RedirectType string `json:"redirect_type"`
}
