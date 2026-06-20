package constants

import "github.com/idivarts/backend-sls/pkg/myutil"

const (
	TRENDLY_BE          = "https://be.trendly.now"
	TRENDLY_CREATORS_FE = "https://creators.trendly.now"
	TRENDLY_BRANDS_FE   = "https://brands.trendly.now"
	TRENDLY_CONNECT     = "https://connect.trendly.now"

	TRENDLY_DEV_CREATORS_FE = "https://dev.creators.trendly.now"
	TRENDLY_DEV_BRANDS_FE   = "https://dev.brands.trendly.now"
	TRENDLY_DEV_CONNECT     = "https://dev.connect.trendly.now"
)

func GetCreatorsFronted() string {
	if myutil.IsDevEnvironment() {
		return TRENDLY_DEV_CREATORS_FE
	}
	return TRENDLY_CREATORS_FE
}

func GetBrandsFronted() string {
	if myutil.IsDevEnvironment() {
		return TRENDLY_DEV_BRANDS_FE
	}
	return TRENDLY_BRANDS_FE
}

func GetConnectFronted() string {
	if myutil.IsDevEnvironment() {
		return TRENDLY_DEV_CONNECT
	}
	return TRENDLY_CONNECT
}

func GetTrendlyBE() string {
	if myutil.IsDevEnvironment() {
		return TRENDLY_BE + "/dev"
	}
	return TRENDLY_BE
}
