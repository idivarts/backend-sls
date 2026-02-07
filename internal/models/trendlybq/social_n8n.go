package trendlybq

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
	SocialsN8NFullTableName = "`trendly-9ab99.matches.socials-n8n`"
)

type SocialsScrapePending struct {
	ID    string `db:"id" bigquery:"id" json:"id" firestore:"id"`
	State int    `db:"state" bigquery:"state" json:"state" firestore:"state"`

	Username string `db:"username" bigquery:"username" json:"username" firestore:"username"`

	// Existing fields from V1 worth preserving
	Gender       string   `db:"gender" bigquery:"gender" json:"gender" firestore:"gender"`
	Niches       []string `db:"niches" bigquery:"niches" json:"niches" firestore:"niches"`
	Location     string   `db:"location" bigquery:"location" json:"location" firestore:"location"`
	QualityScore int      `db:"quality_score" bigquery:"quality_score" json:"quality_score" firestore:"quality_score"`

	CreationTime   int64 `db:"creation_time" bigquery:"creation_time" json:"creation_time" firestore:"creation_time"`
	LastUpdateTime int64 `db:"last_update_time" bigquery:"last_update_time" json:"last_update_time" firestore:"last_update_time"`
}

type SocialsBreif struct {
	ID    string `db:"id" bigquery:"id" json:"id" firestore:"id"`
	State int    `db:"state" bigquery:"state" json:"state" firestore:"state"`

	Name     string `db:"name" bigquery:"name" json:"name" firestore:"name"`
	Username string `db:"username" bigquery:"username" json:"username" firestore:"username"`

	ProfilePic     string  `db:"profile_pic" bigquery:"profile_pic" json:"profile_pic" firestore:"profile_pic"`
	FollowerCount  int64   `db:"follower_count" bigquery:"follower_count" json:"follower_count" firestore:"follower_count"`
	ViewsCount     int64   `db:"views_count" bigquery:"views_count" json:"views_count" firestore:"views_count"`                     //views
	EnagamentCount int64   `db:"engagement_count" bigquery:"engagement_count" json:"engagement_count" firestore:"engagement_count"` //engagement
	EngagementRate float32 `db:"engagement_rate" bigquery:"engagement_rate" json:"engagement_rate" firestore:"engagement_rate"`

	SocialType string `db:"social_type" bigquery:"social_type" json:"social_type" firestore:"social_type"`

	Location string `db:"location" bigquery:"location" json:"location" firestore:"location"`

	Bio string `db:"bio" bigquery:"bio" json:"bio" firestore:"bio"`

	ProfileVerified bool `db:"profile_verified" bigquery:"profile_verified" json:"profile_verified" firestore:"profile_verified"`

	CreationTime   int64 `db:"creation_time" bigquery:"creation_time" json:"creation_time" firestore:"creation_time"`
	LastUpdateTime int64 `db:"last_update_time" bigquery:"last_update_time" json:"last_update_time" firestore:"last_update_time"`
}

type SocialLink struct {
	Title    string `db:"title" bigquery:"title" json:"title" firestore:"title"`
	URL      string `db:"url" bigquery:"url" json:"url" firestore:"url"`
	LinkType string `db:"link_type" bigquery:"link_type" json:"link_type" firestore:"link_type"`
}

type ChildPost struct {
	ID             string             `db:"id" bigquery:"id" json:"id" firestore:"id"`
	Type           string             `db:"type" bigquery:"type" json:"type" firestore:"type"`
	ShortCode      string             `db:"short_code" bigquery:"short_code" json:"short_code" firestore:"short_code"`
	Caption        string             `db:"caption" bigquery:"caption" json:"caption" firestore:"caption"`
	URL            string             `db:"url" bigquery:"url" json:"url" firestore:"url"`
	DisplayURL     string             `db:"display_url" bigquery:"display_url" json:"display_url" firestore:"display_url"`
	VideoURL       string             `db:"video_url" bigquery:"video_url" json:"video_url" firestore:"video_url"`
	LikesCount     bigquery.NullInt64 `db:"likes_count" bigquery:"likes_count" json:"likes_count" firestore:"likes_count"`
	CommentsCount  bigquery.NullInt64 `db:"comments_count" bigquery:"comments_count" json:"comments_count" firestore:"comments_count"`
	VideoViewCount bigquery.NullInt64 `db:"video_view_count" bigquery:"video_view_count" json:"video_view_count" firestore:"video_view_count"`
	VideoPlayCount bigquery.NullInt64 `db:"video_play_count" bigquery:"video_play_count" json:"video_play_count" firestore:"video_play_count"`
	VideoDuration  float64            `db:"video_duration" bigquery:"video_duration" json:"video_duration" firestore:"video_duration"`
	Timestamp      string             `db:"timestamp" bigquery:"timestamp" json:"timestamp" firestore:"timestamp"`
	LocationName   string             `db:"location_name" bigquery:"location_name" json:"location_name" firestore:"location_name"`
	LocationID     string             `db:"location_id" bigquery:"location_id" json:"location_id" firestore:"location_id"`
	IsPinned       bool               `db:"is_pinned" bigquery:"is_pinned" json:"is_pinned" firestore:"is_pinned"`
}

type Post struct {
	ChildPost
	ChildPosts []ChildPost `db:"child_posts" bigquery:"child_posts" json:"child_posts" firestore:"child_posts"`
}

type SocialsN8N struct {
	ID    string `db:"id" bigquery:"id" json:"id" firestore:"id"`
	State int    `db:"state" bigquery:"state" json:"state" firestore:"state"`

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

	// Existing fields from V1 worth preserving
	Gender       string   `db:"gender" bigquery:"gender" json:"gender" firestore:"gender"`
	Niches       []string `db:"niches" bigquery:"niches" json:"niches" firestore:"niches"`
	Location     string   `db:"location" bigquery:"location" json:"location" firestore:"location"`
	QualityScore int      `db:"quality_score" bigquery:"quality_score" json:"quality_score" firestore:"quality_score"`

	// Scraper specific fields
	ExternalURL string       `db:"external_url" bigquery:"external_url" json:"external_url" firestore:"external_url"`
	Links       []SocialLink `db:"links" bigquery:"links" json:"links" firestore:"links"`
	LatestPosts []Post       `db:"latest_posts" bigquery:"latest_posts" json:"latest_posts" firestore:"latest_posts"`
	LatestReels []Post       `db:"latest_reels" bigquery:"latest_reels" json:"latest_reels" firestore:"latest_reels"`

	// Metadata
	AddedBy        string `db:"added_by" bigquery:"added_by" json:"added_by" firestore:"added_by"`
	CreationTime   int64  `db:"creation_time" bigquery:"creation_time" json:"creation_time" firestore:"creation_time"`
	LastUpdateTime int64  `db:"last_update_time" bigquery:"last_update_time" json:"last_update_time" firestore:"last_update_time"`

	// Optional/Future use
	IsBusinessAccount  bool `db:"is_business_account" bigquery:"is_business_account" json:"is_business_account" firestore:"is_business_account"`
	HighlightReelCount int  `db:"highlight_reel_count" bigquery:"highlight_reel_count" json:"highlight_reel_count" firestore:"highlight_reel_count"`
	HasContacts        bool `db:"has_contacts" bigquery:"has_contacts" json:"has_contacts" firestore:"has_contacts"`
}

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

func (data *SocialsN8N) InsertToFirestore() error {
	if data.ID == "" {
		data.ID = data.GetID()
	}
	_, err := firestoredb.Client.Collection("scrapped-socials-n8n").Doc(data.ID).Set(context.Background(), data)
	return err
}

func (data *SocialsN8N) ConvertToSocialBreif() *SocialsBreif {
	return &SocialsBreif{
		ID:              data.ID,
		Name:            data.Name,
		Username:        data.Username,
		ProfilePic:      data.ProfilePic,
		FollowerCount:   data.FollowerCount,
		ViewsCount:      data.ViewsCount,
		EnagamentCount:  data.EngagementCount,
		EngagementRate:  data.EngagementRate,
		SocialType:      data.SocialType,
		Location:        data.Location,
		Bio:             data.Bio,
		ProfileVerified: data.ProfileVerified,
		CreationTime:    data.CreationTime,
		LastUpdateTime:  data.LastUpdateTime,
	}
}

func (data *SocialsN8N) UpdateMinified() error {
	x := data.ConvertToSocialBreif()

	_, err := firestoredb.Client.Collection("scrapped-socials-n8n").Doc(data.ID).Set(context.Background(), x)
	return err
}

func (data *SocialsN8N) UpdateAllImages() error {
	if data.ID == "" {
		data.ID = data.GetID()
	}

	q := myquery.Client.Query(`
		UPDATE ` + SocialsN8NFullTableName + `
		SET
		  profile_pic = @profile_pic,
		  latest_posts = @latest_posts,
		  latest_reels = @latest_reels,
		  last_update_time = @last_update_time
		WHERE id = @id
	`)

	q.Parameters = []bigquery.QueryParameter{
		{Name: "id", Value: data.ID},
		{Name: "profile_pic", Value: data.ProfilePic},
		{Name: "latest_posts", Value: data.LatestPosts},
		{Name: "latest_reels", Value: data.LatestReels},
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

func (_ SocialsN8N) GetMultipleBreifs(ids []string) ([]SocialsBreif, error) {
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

	results := []SocialsBreif{}
	for {
		row := &SocialsBreif{}
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
