package mm

var CONTENT_NICHE = []string{
	"Fashion / Beauty",
	"Lifestyle Vlogs",
	"Food",
	"Travel",
	"Fun / Meme",
	"Health",
	"Tech",
	"Others",
}

var OLD_CONTENT_NICHE_MAP = map[string]string{
	// On Onboard
	"Fun":              "Fun / Meme",
	"Lifestyle Vlogs":  "Lifestyle Vlogs",
	"Roast":            "Fun / Meme",
	"Meme":             "Fun / Meme",
	"Fashion / Beauty": "Fashion / Beauty",
	"Tech":             "Tech",
	"Others":           "Others",

	// On Edit Profile
	"Fashion":   "Fashion / Beauty",
	"Lifestyle": "Lifestyle Vlogs",
	"Food":      "Food",
	"Travel":    "Travel",
	"Health":    "Health",

	// Influencer Category on Brand's app
	"Beauty":           "Fashion / Beauty",
	"Food Vlogs":       "Food",
	"Technology":       "Tech",
	"Fitness & Health": "Health",
	"Entertainment":    "Fun / Meme",
}

func GetContentReverseMapping() map[string][]string {
	mSetKey := make(map[string]map[string]bool)
	for k, v := range OLD_CONTENT_NICHE_MAP {
		if mSetKey[k] == nil {
			mSetKey[k] = make(map[string]bool)
		}
		mSetKey[k][v] = true
	}

	result := make(map[string][]string)
	for k, vMap := range mSetKey {
		for v := range vMap {
			result[k] = append(result[k], v)
		}
	}
	return result
}

var CONTENT_NICHE_REVERSE_MAP = GetContentReverseMapping()
