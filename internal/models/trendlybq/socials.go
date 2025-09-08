package trendlybq

import (
	"context"
	"log"

	"cloud.google.com/go/bigquery"
	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/pkg/myquery"
)

const (
	SocialsFullTableName = "`trendly-9ab99.matches.socials`"
)

type Socials struct {
	ID         string `db:"id" bigquery:"id"`
	SocialType string `db:"social_type" bigquery:"social_type"`

	Gender   string   `db:"gender" bigquery:"gender"`
	Niches   []string `db:"niches" bigquery:"niches"`
	Location string   `db:"location" bigquery:"location"`

	FollowerCount   int64 `db:"follower_count" bigquery:"follower_count"`
	FollowingCount  int64 `db:"following_count" bigquery:"following_count"`
	ContentCount    int64 `db:"content_count" bigquery:"content_count"`        //posts
	ViewsCount      int64 `db:"views_count" bigquery:"views_count"`            //views
	EnagamentsCount int64 `db:"engagement_count" bigquery:"engagements_count"` //engagement

	ReelScrappedCount int `db:"reel_scrapped_count" bigquery:"reel_scrapped_count"` //scrapped reels

	AverageViews    float32 `db:"average_views" bigquery:"average_views"`
	AverageLikes    float32 `db:"average_likes" bigquery:"average_likes"`
	AverageComments float32 `db:"average_comments" bigquery:"average_comments"`
	QualityScore    int     `db:"quality_score" bigquery:"quality_score"`
	EngagementRate  float32 `db:"engagement_rate" bigquery:"engagement_rate"`

	Username   string `db:"username" bigquery:"username"`
	Name       string `db:"name" bigquery:"name"`
	Bio        string `db:"bio" bigquery:"bio"`
	Category   string `db:"category" bigquery:"category"`
	ProfilePic string `db:"profile_pic" bigquery:"profile_pic"`

	ProfileVerified bool `db:"profile_verified" bigquery:"profile_verified"`
	HasContacts     bool `db:"has_contacts" bigquery:"has_contacts"`

	Reels []Reel `db:"reels" bigquery:"reels"`
	Links []Link `db:"links" bigquery:"links"`

	HasFollowButton  bool `db:"has_follow_button" bigquery:"has_follow_button"`
	HasMessageButton bool `db:"has_message_button" bigquery:"has_message_button"`

	AddedBy string `db:"added_by" bigquery:"added_by"`

	CreationTime   int64 `db:"creation_time" bigquery:"creation_time"`
	LastUpdateTime int64 `db:"last_update_time" bigquery:"last_update_time"`
}

type Link struct {
	URL  string `db:"url" bigquery:"url"`
	Text string `db:"text" bigquery:"text"`
}
type Reel struct {
	ID            string             `db:"id" bigquery:"id"`
	ThumbnailURL  string             `db:"thumbnail_url" bigquery:"thumbnail_url"`
	URL           string             `db:"url" bigquery:"url"`
	Caption       string             `db:"caption" bigquery:"caption"`
	Pinned        bool               `db:"pinned" bigquery:"pinned"`
	ViewsCount    bigquery.NullInt64 `db:"views_count" bigquery:"views_count"`
	LikesCount    bigquery.NullInt64 `db:"likes_count" bigquery:"likes_count"`
	CommentsCount bigquery.NullInt64 `db:"comments_count" bigquery:"comments_count"`
}

func (data *Socials) GetID() string {
	ID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(data.SocialType+data.Username))
	return ID.String()
}

func (data *Socials) Insert() error {
	data.ID = data.GetID()
	inserter := myquery.Client.Dataset("matches").Table(`socials`).Inserter()
	if err := inserter.Put(context.Background(), []*Socials{
		data,
	}); err != nil {
		return err
	}
	return nil
}

func (data *Socials) GetInstagram(username string) error {
	data.Username = username
	data.SocialType = "instagram"
	id := data.GetID()

	// query := myquery.Client.Dataset("matches").Table(`socials`).Read(context.Background())
	q := myquery.Client.Query(`
	SELECT *
	FROM ` + SocialsFullTableName + `
	WHERE id = '` + id + `'
	LIMIT 1
`)

	it, err := q.Read(context.Background())
	if err != nil {
		log.Fatal(err)
	}

	for {
		err := it.Next(data)
		if err != nil {
			return err
		}
	}
	return nil
}
