package trendlyrdb

import (
	"context"
	"errors"
	"log"

	"cloud.google.com/go/bigquery"
	"github.com/google/uuid"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myquery"
	"google.golang.org/api/iterator"
)

const (
	SocialsN8NFullTableName = "`socials`"
)

type Socials struct {
	ID string `db:"id" json:"id"`

	Username     string `db:"username" json:"username"`
	Name         string `db:"name" json:"name"`
	Bio          string `db:"bio" json:"bio"`
	ProfilePic   string `db:"profile_pic" json:"profile_pic"`
	ProfilePicHD string `db:"profile_pic_hd" json:"profile_pic_hd"`
	Category     string `db:"category" json:"category"`

	SocialType      string `db:"social_type" json:"social_type"`
	ProfileVerified bool   `db:"profile_verified" json:"profile_verified"`

	FollowerCount  int64 `db:"follower_count" json:"follower_count"`
	FollowingCount int64 `db:"following_count" json:"following_count"`
	ContentCount   int64 `db:"content_count" json:"content_count"`

	// Analytics/Metrics (preserved from V1)
	ViewsCount      int64   `db:"views_count" json:"views_count"`
	EngagementCount int64   `db:"engagement_count" json:"engagement_count"`
	EngagementRate  float32 `db:"engagement_rate" json:"engagement_rate"`
	AverageViews    float32 `db:"average_views" json:"average_views"`
	AverageLikes    float32 `db:"average_likes" json:"average_likes"`
	AverageComments float32 `db:"average_comments" json:"average_comments"`

	// Scraper specific fields
	Links []Links `db:"links" json:"links"`

	// Existing fields from V1 worth preserving - We used to input it in past. But now thinking of calculating it using AI

	// Not needed - We will deduce the full name, username, and bio (pronouns in bio)
	Gender string `db:"gender" json:"gender"`
	// Not needed - We will deduce this from bio and posts' location
	Location string `db:"location" json:"location"`

	// Optional - Need not be integer - Average (60) - Good (75) - Very Good (90) - Excellent (100)
	QualityScore int `db:"quality_score" json:"quality_score"`

	// Metadata
	AddedBy        string `db:"added_by" json:"added_by"`
	CreationTime   int64  `db:"creation_time" json:"creation_time"`
	LastUpdateTime int64  `db:"last_update_time" json:"last_update_time"`

	// Enhanced Profile fields
	ExternalId string `db:"external_id" json:"external_id"`
}

type Links struct {
	Title    string `db:"title" json:"title"`
	URL      string `db:"url" json:"url"`
	LinkType string `db:"link_type" json:"link_type"`
}

// // Content Data
// LatestPosts      []Post       `db:"latest_posts" bigquery:"latest_posts" json:"latest_posts" firestore:"latest_posts"`
// LatestReels      []SinglePost `db:"latest_reels" bigquery:"latest_reels" json:"latest_reels" firestore:"latest_reels"`
// LatestIgtvVideos []SinglePost `db:"latest_igtv_videos" bigquery:"latest_igtv_videos" json:"latest_igtv_videos" firestore:"latest_igtv_videos"`

func (data *Socials) GetID() string {
	ID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(data.SocialType+data.Username))
	return ID.String()
}

func (data *Socials) Insert() error {
	data.ID = data.GetID()
	inserter := myquery.Client.Dataset("matches").Table(`socials-n8n`).Inserter()
	if err := inserter.Put(context.Background(), []*Socials{
		data,
	}); err != nil {
		return err
	}
	return nil
}

func (_ Socials) InsertMultiple(socials []Socials) error {
	inserter := myquery.Client.Dataset("matches").Table(`socials-n8n`).Inserter()
	if err := inserter.Put(context.Background(), socials); err != nil {
		return err
	}
	return nil
}

func (data *Socials) InsertToFirestore(isImagesOnS3 bool) error {
	if data.ID == "" {
		data.ID = data.GetID()
	}
	_, err := firestoredb.Client.Collection("scrapped-socials-n8n").Doc(data.ID).Set(context.Background(), data)
	return err
}

func (_ Socials) GetPaginated(offset, limit int) ([]Socials, error) {
	q := myquery.Client.Query(`
    SELECT *
    FROM ` + SocialsN8NFullTableName + `
	QUALIFY
		ROW_NUMBER() OVER (
			PARTITION BY id
			ORDER BY last_update_time DESC
		) = 1
    LIMIT @limit
	OFFSET @offset
`)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "limit", Value: limit},
		{Name: "offset", Value: offset},
	}

	it, err := q.Read(context.Background())
	if err != nil {
		log.Println("Error ", err.Error())
		return nil, err
	}

	results := []Socials{}
	for {
		data := &Socials{}
		err = it.Next(data)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Println("Error ", err.Error())
			continue
		}
		results = append(results, *data)
	}
	return results, nil
}

func (_ Socials) GetPaginatedFromFirestore(offset, limit int) ([]Socials, error) {
	temp := firestoredb.Client.Collection("scrapped-socials-n8n").Offset(offset)
	if limit > 0 {
		temp = temp.Limit(limit)
	}
	it, err := temp.Documents(context.Background()).GetAll()
	if err != nil {
		return nil, err
	}
	results := []Socials{}
	for _, v := range it {
		social := &Socials{}
		err = v.DataTo(social)
		if err != nil {
			continue
		}
		results = append(results, *social)
	}
	return results, nil
}

func (s *Socials) GetByIdFromFirestore(id string) error {
	res, err := firestoredb.Client.Collection("scrapped-socials-n8n").Doc(id).Get(context.Background())
	if err != nil {
		return err
	}
	err = res.DataTo(s)
	if err != nil {
		return err
	}
	return nil
}

func (data *Socials) Get(id string) error {
	q := myquery.Client.Query(`
    SELECT *
    FROM ` + SocialsN8NFullTableName + `
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

func (_ Socials) GetMultiple(ids []string) ([]Socials, error) {
	q := myquery.Client.Query(`
    SELECT *
    FROM ` + SocialsN8NFullTableName + `
    WHERE id IN UNNEST(@ids)
	QUALIFY
		ROW_NUMBER() OVER (
			PARTITION BY id
			ORDER BY last_update_time DESC
		) = 1
`)
	q.Parameters = []bigquery.QueryParameter{
		{Name: "ids", Value: ids},
	}

	it, err := q.Read(context.Background())
	if err != nil {
		return nil, err
	}

	results := []Socials{}
	for {
		row := &Socials{}
		err = it.Next(row)
		if err == iterator.Done {
			break
		}
		if err != nil {
			log.Println("Error ", err.Error())
			continue
		}
		results = append(results, *row)
	}
	return results, nil
}

func (data *Socials) GetInstagram(username string) error {
	data.Username = username
	data.SocialType = "instagram"
	id := data.GetID()

	q := myquery.Client.Query(`
	SELECT *
	FROM ` + SocialsN8NFullTableName + `
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

func (data *Socials) GetInstagramFromFirestore(username string) error {
	data.Username = username
	data.SocialType = "instagram"
	id := data.GetID()

	d, err := firestoredb.Client.Collection("scrapped-socials-n8n").Doc(id).Get(context.Background())
	if err != nil {
		return err
	}

	if !d.Exists() {
		return errors.New("document-doesnt-exists")
	}

	err = d.DataTo(data)
	if err != nil {
		return err
	}

	return nil
}
func IsPendingScanExists() bool {
	snap, err := firestoredb.Client.Collection("scrapped-socials-n8n").Where("state", "==", 0).Limit(1).Documents(context.Background()).Next()
	if err != nil {
		return false
	}
	return snap != nil
}
