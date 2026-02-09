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

type Links struct {
	Title    string `db:"title" bigquery:"title" json:"title" firestore:"title"`
	URL      string `db:"url" bigquery:"url" json:"url" firestore:"url"`
	LinkType string `db:"link_type" bigquery:"link_type" json:"link_type" firestore:"link_type"`
}

type MusicInfo struct {
	ArtistName        string `db:"artist_name" bigquery:"artist_name" json:"artist_name" firestore:"artist_name"`
	SongName          string `db:"song_name" bigquery:"song_name" json:"song_name" firestore:"song_name"`
	UsesOriginalAudio bool   `db:"uses_original_audio" bigquery:"uses_original_audio" json:"uses_original_audio" firestore:"uses_original_audio"`
	AudioID           string `db:"audio_id" bigquery:"audio_id" json:"audio_id" firestore:"audio_id"`
}

type User struct {
	FullName      string `db:"full_name" bigquery:"full_name" json:"full_name" firestore:"full_name"`
	ID            string `db:"id" bigquery:"id" json:"id" firestore:"id"`
	IsPrivate     bool   `db:"is_private" bigquery:"is_private" json:"is_private" firestore:"is_private"`
	IsVerified    bool   `db:"is_verified" bigquery:"is_verified" json:"is_verified" firestore:"is_verified"`
	ProfilePicURL string `db:"profile_pic_url" bigquery:"profile_pic_url" json:"profile_pic_url" firestore:"profile_pic_url"`
	Username      string `db:"username" bigquery:"username" json:"username" firestore:"username"`
}

type SocialsN8N struct {
	ID string `db:"id" bigquery:"id" json:"id" firestore:"id"`

	Username     string `db:"username" bigquery:"username" json:"username" firestore:"username"`
	Name         string `db:"name" bigquery:"name" json:"name" firestore:"name"`
	Bio          string `db:"bio" bigquery:"bio" json:"bio" firestore:"bio"`
	ProfilePic   string `db:"profile_pic" bigquery:"profile_pic" json:"profile_pic" firestore:"profile_pic"`
	ProfilePicHD string `db:"profile_pic_hd" bigquery:"profile_pic_hd" json:"profile_pic_hd" firestore:"profile_pic_hd"`
	Category     string `db:"category" bigquery:"category" json:"category" firestore:"category"`

	SocialType      string `db:"social_type" bigquery:"social_type" json:"social_type" firestore:"social_type"`
	ProfileVerified bool   `db:"profile_verified" bigquery:"profile_verified" json:"profile_verified" firestore:"profile_verified"`

	FollowerCount  int64 `db:"follower_count" bigquery:"follower_count" json:"follower_count" firestore:"follower_count"`
	FollowingCount int64 `db:"following_count" bigquery:"following_count" json:"following_count" firestore:"following_count"`
	ContentCount   int64 `db:"content_count" bigquery:"content_count" json:"content_count" firestore:"content_count"`

	// Analytics/Metrics (preserved from V1)
	ViewsCount      int64   `db:"views_count" bigquery:"views_count" json:"views_count" firestore:"views_count"`
	EngagementCount int64   `db:"engagement_count" bigquery:"engagement_count" json:"engagement_count" firestore:"engagement_count"`
	EngagementRate  float32 `db:"engagement_rate" bigquery:"engagement_rate" json:"engagement_rate" firestore:"engagement_rate"`
	AverageViews    float32 `db:"average_views" bigquery:"average_views" json:"average_views" firestore:"average_views"`
	AverageLikes    float32 `db:"average_likes" bigquery:"average_likes" json:"average_likes" firestore:"average_likes"`
	AverageComments float32 `db:"average_comments" bigquery:"average_comments" json:"average_comments" firestore:"average_comments"`

	// Scraper specific fields
	Links []Links `db:"links" bigquery:"links" json:"links" firestore:"links"`

	// Existing fields from V1 worth preserving - We used to input it in past. But now thinking of calculating it using AI

	// Not needed - We will deduce the full name, username, and bio (pronouns in bio)
	Gender string `db:"gender" bigquery:"gender" json:"gender" firestore:"gender"`
	// Not needed - We will deduce this from bio and posts' location
	Location string `db:"location" bigquery:"location" json:"location" firestore:"location"`

	// Optional - We will deduce this from posts, hashtags and bio
	Niches []string `db:"niches" bigquery:"niches" json:"niches" firestore:"niches"`
	// Optional - Need not be integer - Average (60) - Good (75) - Very Good (90) - Excellent (100)
	QualityScore int `db:"quality_score" bigquery:"quality_score" json:"quality_score" firestore:"quality_score"`

	// Metadata
	AddedBy        string `db:"added_by" bigquery:"added_by" json:"added_by" firestore:"added_by"`
	CreationTime   int64  `db:"creation_time" bigquery:"creation_time" json:"creation_time" firestore:"creation_time"`
	LastUpdateTime int64  `db:"last_update_time" bigquery:"last_update_time" json:"last_update_time" firestore:"last_update_time"`

	// Enhanced Profile fields
	ExternalId string `db:"external_id" bigquery:"external_id" json:"external_id" firestore:"external_id"`
}

// // Content Data
// LatestPosts      []Post       `db:"latest_posts" bigquery:"latest_posts" json:"latest_posts" firestore:"latest_posts"`
// LatestReels      []SinglePost `db:"latest_reels" bigquery:"latest_reels" json:"latest_reels" firestore:"latest_reels"`
// LatestIgtvVideos []SinglePost `db:"latest_igtv_videos" bigquery:"latest_igtv_videos" json:"latest_igtv_videos" firestore:"latest_igtv_videos"`

func (data *SocialsN8N) GetID() string {
	ID := uuid.NewSHA1(uuid.NameSpaceURL, []byte(data.SocialType+data.Username))
	return ID.String()
}

func (data *SocialsN8N) Insert() error {
	data.ID = data.GetID()
	inserter := myquery.Client.Dataset("matches").Table(`socials-n8n`).Inserter()
	if err := inserter.Put(context.Background(), []*SocialsN8N{
		data,
	}); err != nil {
		return err
	}
	return nil
}

func (_ SocialsN8N) InsertMultiple(socials []SocialsN8N) error {
	inserter := myquery.Client.Dataset("matches").Table(`socials-n8n`).Inserter()
	if err := inserter.Put(context.Background(), socials); err != nil {
		return err
	}
	return nil
}

func (data *SocialsN8N) InsertToFirestore(isImagesOnS3 bool) error {
	if data.ID == "" {
		data.ID = data.GetID()
	}
	_, err := firestoredb.Client.Collection("scrapped-socials-n8n").Doc(data.ID).Set(context.Background(), data)
	return err
}

func (_ SocialsN8N) GetPaginated(offset, limit int) ([]SocialsN8N, error) {
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

	results := []SocialsN8N{}
	for {
		data := &SocialsN8N{}
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

func (_ SocialsN8N) GetPaginatedFromFirestore(offset, limit int) ([]SocialsN8N, error) {
	temp := firestoredb.Client.Collection("scrapped-socials-n8n").Offset(offset)
	if limit > 0 {
		temp = temp.Limit(limit)
	}
	it, err := temp.Documents(context.Background()).GetAll()
	if err != nil {
		return nil, err
	}
	results := []SocialsN8N{}
	for _, v := range it {
		social := &SocialsN8N{}
		err = v.DataTo(social)
		if err != nil {
			continue
		}
		results = append(results, *social)
	}
	return results, nil
}

func (s *SocialsN8N) GetByIdFromFirestore(id string) error {
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

func (data *SocialsN8N) Get(id string) error {
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

func (_ SocialsN8N) GetMultiple(ids []string) ([]SocialsN8N, error) {
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

	results := []SocialsN8N{}
	for {
		row := &SocialsN8N{}
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

func (data *SocialsN8N) GetInstagram(username string) error {
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

func (data *SocialsN8N) GetInstagramFromFirestore(username string) error {
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
