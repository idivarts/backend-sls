package sui

// ScrapedSocial represents the payload coming from your scraper.
type ScrapedSocial struct {
	SocialType          string `json:"socialType" binding:"required"`
	Username            string `json:"username" binding:"required"`
	UseDatabase         bool   `json:"useDatabase,omitempty"`
	HighValueInfluencer bool   `json:"highValueInfluencer,omitempty"`
	Manual              struct {
		Niches       []string `json:"niches"`
		QualityScore int      `json:"qualityScore" binding:"gte=0,lte=10"`
	} `json:"manual"`
}
