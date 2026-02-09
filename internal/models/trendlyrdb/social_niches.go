package trendlyrdb

const (
	SocialNichesTableName = "social_niches"
)

type SocialNiches struct {
	SocialID string `db:"social_id" json:"social_id"`
	Niche    string `db:"niche" json:"niche"`
}
