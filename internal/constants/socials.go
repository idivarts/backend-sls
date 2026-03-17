package constants

// AllowedNiches is the canonical list of influencer niches.
// To add a new niche, simply append to this slice — the JSON schema and prompt
// are derived from it automatically, so no other changes are needed.
var AllowedNiches = []string{
	// Fashion & Appearance
	"Fashion & Style",
	"Beauty & Makeup",
	"Skincare",

	// Health & Body
	"Fitness & Gym",
	"Yoga & Wellness",
	"Health & Nutrition",
	"Mental Health",

	// Food
	"Food & Cooking",

	// Lifestyle & Home
	"Travel",
	"Lifestyle",
	"Parenting & Family",
	"Home & Interior",
	"DIY & Crafts",
	"Gardening & Plants",

	// Creative & Arts
	"Photography & Videography",
	"Art & Illustration",
	"Music",
	"Dance",

	// Entertainment
	"Comedy & Skits",
	"Memes & Humor",
	"Gaming",
	"Anime & Cosplay",
	"Pop Culture & Entertainment",

	// Knowledge & Education
	"Tech & Gadgets",
	"Science & Education",
	"Books & Reading",
	"Finance & Investing",
	"Business & Entrepreneurship",
	"Motivation & Self-Help",

	// Sports & Outdoors
	"Sports",
	"Outdoor & Adventure",

	// Niche Interest
	"Pets & Animals",
	"Automotive",
	"Astrology & Spirituality",
	"Real Estate",

	// Identity & Community
	"Body Positivity",
	"LGBTQ+",
	"Social Causes & Activism",
	"Sustainability & Environment",

	// Life Events & Luxury
	"Wedding & Events",
	"Luxury & High-End",

	// Other Content Types
	"Relationships & Dating",
	"Quotes & Affirmations",
	"Kids & Toys",
	"NSFW & Adult",
	"Others",
}
var Genders = []string{
	"male",
	"female",
	"couple",
	"animal",
	"lgbtq",
	"gender-neutral",
}
