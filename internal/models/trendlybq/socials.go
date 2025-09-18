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
	ID         string `db:"id" bigquery:"id" json:"id"`
	SocialType string `db:"social_type" bigquery:"social_type" json:"social_type"`

	Gender   string   `db:"gender" bigquery:"gender" json:"gender"`
	Niches   []string `db:"niches" bigquery:"niches" json:"niches"`
	Location string   `db:"location" bigquery:"location" json:"location"`

	FollowerCount   int64 `db:"follower_count" bigquery:"follower_count" json:"follower_count"`
	FollowingCount  int64 `db:"following_count" bigquery:"following_count" json:"following_count"`
	ContentCount    int64 `db:"content_count" bigquery:"content_count" json:"content_count"`           //posts
	ViewsCount      int64 `db:"views_count" bigquery:"views_count" json:"views_count"`                 //views
	EnagamentsCount int64 `db:"engagement_count" bigquery:"engagements_count" json:"engagement_count"` //engagement

	ReelScrappedCount int `db:"reel_scrapped_count" bigquery:"reel_scrapped_count" json:"reel_scrapped_count"` //scrapped reels

	AverageViews    float32 `db:"average_views" bigquery:"average_views" json:"average_views"`
	AverageLikes    float32 `db:"average_likes" bigquery:"average_likes" json:"average_likes"`
	AverageComments float32 `db:"average_comments" bigquery:"average_comments" json:"average_comments"`
	QualityScore    int     `db:"quality_score" bigquery:"quality_score" json:"quality_score"`
	EngagementRate  float32 `db:"engagement_rate" bigquery:"engagement_rate" json:"engagement_rate"`

	Username   string `db:"username" bigquery:"username" json:"username"`
	Name       string `db:"name" bigquery:"name" json:"name"`
	Bio        string `db:"bio" bigquery:"bio" json:"bio"`
	Category   string `db:"category" bigquery:"category" json:"category"`
	ProfilePic string `db:"profile_pic" bigquery:"profile_pic" json:"profile_pic"`

	ProfileVerified bool `db:"profile_verified" bigquery:"profile_verified" json:"profile_verified"`
	HasContacts     bool `db:"has_contacts" bigquery:"has_contacts" json:"has_contacts"`

	Reels []Reel `db:"reels" bigquery:"reels" json:"reels"`
	Links []Link `db:"links" bigquery:"links" json:"links"`

	HasFollowButton  bool `db:"has_follow_button" bigquery:"has_follow_button" json:"has_follow_button"`
	HasMessageButton bool `db:"has_message_button" bigquery:"has_message_button" json:"has_message_button"`

	AddedBy string `db:"added_by" bigquery:"added_by" json:"added_by"`

	CreationTime   int64 `db:"creation_time" bigquery:"creation_time" json:"creation_time"`
	LastUpdateTime int64 `db:"last_update_time" bigquery:"last_update_time" json:"last_update_time"`
}

type Link struct {
	URL  string `db:"url" bigquery:"url" json:"url"`
	Text string `db:"text" bigquery:"text" json:"text"`
}
type Reel struct {
	ID            string             `db:"id" bigquery:"id" json:"id"`
	ThumbnailURL  string             `db:"thumbnail_url" bigquery:"thumbnail_url" json:"thumbnail_url"`
	URL           string             `db:"url" bigquery:"url" json:"url"`
	Caption       string             `db:"caption" bigquery:"caption" json:"caption"`
	Pinned        bool               `db:"pinned" bigquery:"pinned" json:"pinned"`
	ViewsCount    bigquery.NullInt64 `db:"views_count" bigquery:"views_count" json:"views_count"`
	LikesCount    bigquery.NullInt64 `db:"likes_count" bigquery:"likes_count" json:"likes_count"`
	CommentsCount bigquery.NullInt64 `db:"comments_count" bigquery:"comments_count" json:"comments_count"`
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

func (data *Socials) UpdateAllImages() error {
	// Perform a parameterized UPDATE instead of delete+insert.
	// This focuses on fields that change most frequently: profile_pic and reels (including thumbnail_url in each reel).
	// We also bump last_update_time if the caller has set it.
	if data.ID == "" {
		// Derive ID if it hasn't been set explicitly.
		data.ID = data.GetID()
	}

	q := myquery.Client.Query(`
		UPDATE ` + SocialsFullTableName + `
		SET
		  profile_pic = @profile_pic,
		  reels = @reels,
		  last_update_time = @last_update_time
		WHERE id = @id
	`)

	q.Parameters = []bigquery.QueryParameter{
		{Name: "id", Value: data.ID},
		{Name: "profile_pic", Value: data.ProfilePic},
		// Reels is a repeated RECORD. Passing the Go slice of `Reel` structs will be mapped to an ARRAY<STRUCT> parameter.
		{Name: "reels", Value: data.Reels},
		{Name: "last_update_time", Value: data.LastUpdateTime},
	}

	job, err := q.Run(context.Background())
	if err != nil {
		return err
	}
	status, err := job.Wait(context.Background())
	if err != nil {
		return err
	}
	if status.Err() != nil {
		return status.Err()
	}
	return nil
}

func (data *Socials) Get(id string) error {
	q := myquery.Client.Query(`
    SELECT *
    FROM ` + SocialsFullTableName + `
    WHERE id = @id
    LIMIT 1
`)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "id", Value: id},
	}

	it, err := q.Read(context.Background())
	if err != nil {
		return err
	}

	err = it.Next(data)
	if err != nil {
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

	err = it.Next(data)
	if err != nil {
		return err
	}

	return nil
}
