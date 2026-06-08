package constants

// INSTAGRAM_REDIRECT is the stage-aware Instagram OAuth redirect endpoint.
// GetTrendlyBE() adds the "/dev" API Gateway stage in dev.
var INSTAGRAM_REDIRECT = GetTrendlyBE() + "/instagram/auth"

type IInstaAuth struct {
	Code         string `json:"code"`
	RedirectType string `json:"redirect_type"`
}
