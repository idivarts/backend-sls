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
	SocialsFullTableName = "`trendly-9ab99.matches.socials`"
)

type SocialsBreif struct {
	ID       string `db:"id" bigquery:"id" json:"id" firestore:"id"`
	Name     string `db:"name" bigquery:"name" json:"name" firestore:"name"`
	Username string `db:"username" bigquery:"username" json:"username" firestore:"username"`

	ProfilePic      string  `db:"profile_pic" bigquery:"profile_pic" json:"profile_pic" firestore:"profile_pic"`
	FollowerCount   int64   `db:"follower_count" bigquery:"follower_count" json:"follower_count" firestore:"follower_count"`
	ViewsCount      int64   `db:"views_count" bigquery:"views_count" json:"views_count" firestore:"views_count"`                      //views
	EnagamentsCount int64   `db:"engagement_count" bigquery:"engagements_count" json:"engagement_count" firestore:"engagement_count"` //engagement
	EngagementRate  float32 `db:"engagement_rate" bigquery:"engagement_rate" json:"engagement_rate" firestore:"engagement_rate"`

	SocialType string `db:"social_type" bigquery:"social_type" json:"social_type" firestore:"social_type"`

	Location string `db:"location" bigquery:"location" json:"location" firestore:"location"`

	Bio string `db:"bio" bigquery:"bio" json:"bio" firestore:"bio"`

	ProfileVerified bool `db:"profile_verified" bigquery:"profile_verified" json:"profile_verified" firestore:"profile_verified"`
}

type Socials struct {
	ID         string `db:"id" bigquery:"id" json:"id" firestore:"id"`
	SocialType string `db:"social_type" bigquery:"social_type" json:"social_type" firestore:"social_type"`

	Gender   string   `db:"gender" bigquery:"gender" json:"gender" firestore:"gender"`
	Niches   []string `db:"niches" bigquery:"niches" json:"niches" firestore:"niches"`
	Location string   `db:"location" bigquery:"location" json:"location" firestore:"location"`

	FollowerCount   int64 `db:"follower_count" bigquery:"follower_count" json:"follower_count" firestore:"follower_count"`
	FollowingCount  int64 `db:"following_count" bigquery:"following_count" json:"following_count" firestore:"following_count"`
	ContentCount    int64 `db:"content_count" bigquery:"content_count" json:"content_count" firestore:"content_count"`              //posts
	ViewsCount      int64 `db:"views_count" bigquery:"views_count" json:"views_count" firestore:"views_count"`                      //views
	EnagamentsCount int64 `db:"engagement_count" bigquery:"engagements_count" json:"engagement_count" firestore:"engagement_count"` //engagement

	ReelScrappedCount int `db:"reel_scrapped_count" bigquery:"reel_scrapped_count" json:"reel_scrapped_count" firestore:"reel_scrapped_count"` //scrapped reels

	AverageViews    float32 `db:"average_views" bigquery:"average_views" json:"average_views" firestore:"average_views"`
	AverageLikes    float32 `db:"average_likes" bigquery:"average_likes" json:"average_likes" firestore:"average_likes"`
	AverageComments float32 `db:"average_comments" bigquery:"average_comments" json:"average_comments" firestore:"average_comments"`
	QualityScore    int     `db:"quality_score" bigquery:"quality_score" json:"quality_score" firestore:"quality_score"`
	EngagementRate  float32 `db:"engagement_rate" bigquery:"engagement_rate" json:"engagement_rate" firestore:"engagement_rate"`

	Username   string `db:"username" bigquery:"username" json:"username" firestore:"username"`
	Name       string `db:"name" bigquery:"name" json:"name" firestore:"name"`
	Bio        string `db:"bio" bigquery:"bio" json:"bio" firestore:"bio"`
	Category   string `db:"category" bigquery:"category" json:"category" firestore:"category"`
	ProfilePic string `db:"profile_pic" bigquery:"profile_pic" json:"profile_pic" firestore:"profile_pic"`

	ProfileVerified bool `db:"profile_verified" bigquery:"profile_verified" json:"profile_verified" firestore:"profile_verified"`
	HasContacts     bool `db:"has_contacts" bigquery:"has_contacts" json:"has_contacts" firestore:"has_contacts"`

	Reels []Reel `db:"reels" bigquery:"reels" json:"reels" firestore:"reels"`
	Links []Link `db:"links" bigquery:"links" json:"links" firestore:"links"`

	HasFollowButton  bool `db:"has_follow_button" bigquery:"has_follow_button" json:"has_follow_button" firestore:"has_follow_button"`
	HasMessageButton bool `db:"has_message_button" bigquery:"has_message_button" json:"has_message_button" firestore:"has_message_button"`

	AddedBy string `db:"added_by" bigquery:"added_by" json:"added_by" firestore:"added_by"`

	CreationTime   int64 `db:"creation_time" bigquery:"creation_time" json:"creation_time" firestore:"creation_time"`
	LastUpdateTime int64 `db:"last_update_time" bigquery:"last_update_time" json:"last_update_time" firestore:"last_update_time"`
}

type Link struct {
	URL  string `db:"url" bigquery:"url" json:"url" firestore:"url"`
	Text string `db:"text" bigquery:"text" json:"text" firestore:"text"`
}
type Reel struct {
	ID            string             `db:"id" bigquery:"id" json:"id" firestore:"id"`
	ThumbnailURL  string             `db:"thumbnail_url" bigquery:"thumbnail_url" json:"thumbnail_url" firestore:"thumbnail_url"`
	URL           string             `db:"url" bigquery:"url" json:"url" firestore:"url"`
	Caption       string             `db:"caption" bigquery:"caption" json:"caption" firestore:"caption"`
	Pinned        bool               `db:"pinned" bigquery:"pinned" json:"pinned" firestore:"pinned"`
	ViewsCount    bigquery.NullInt64 `db:"views_count" bigquery:"views_count" json:"views_count" firestore:"views_count"`
	LikesCount    bigquery.NullInt64 `db:"likes_count" bigquery:"likes_count" json:"likes_count" firestore:"likes_count"`
	CommentsCount bigquery.NullInt64 `db:"comments_count" bigquery:"comments_count" json:"comments_count" firestore:"comments_count"`
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

func (_ Socials) InsertMultiple(socials []Socials) error {
	inserter := myquery.Client.Dataset("matches").Table(`socials`).Inserter()
	if err := inserter.Put(context.Background(), socials); err != nil {
		return err
	}
	return nil
}

func (data *Socials) InsertToFirestore() error {
	if data.ID == "" {
		data.ID = data.GetID()
	}
	_, err := firestoredb.Client.Collection("scrapped-socials").Doc(data.ID).Set(context.Background(), data)
	return err
}

func (data *Socials) UpdateMinified() error {
	x := SocialsBreif{
		ID:              data.ID,
		Name:            data.Name,
		Username:        data.Username,
		ProfilePic:      data.ProfilePic,
		FollowerCount:   data.FollowerCount,
		ViewsCount:      data.ViewsCount,
		EnagamentsCount: data.EnagamentsCount,
		EngagementRate:  data.EngagementRate,
		SocialType:      data.SocialType,
		Location:        data.Location,
		Bio:             data.Bio,
		ProfileVerified: data.ProfileVerified,
	}

	_, err := firestoredb.Client.Collection("scrapped-socials").Doc(data.ID).Set(context.Background(), x)
	return err
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

func (_ Socials) GetPaginated(offset, limit int) ([]Socials, error) {
	q := myquery.Client.Query(`
    SELECT *
    FROM ` + SocialsFullTableName + `
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

	mySocials := []Socials{}
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
		mySocials = append(mySocials, *data)
	}
	return mySocials, nil
}

func (_ Socials) GetPaginatedFromFirestore(offset, limit int) ([]Socials, error) {
	temp := firestoredb.Client.Collection("scrapped-socials").Where("reel_scrapped_count", ">", 0).Offset(offset)
	if limit > 0 {
		temp = temp.Limit(limit)
	}
	it := temp.Documents(context.Background())

	socials := []Socials{}
	for {
		d, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			continue
		}
		social := &Socials{}
		err = d.DataTo(social)
		if err != nil {
			continue
		}
		socials = append(socials, *social)
	}

	return socials, nil
}
func (s *Socials) GetByIdFromFirestore(id string) error {
	res, err := firestoredb.Client.Collection("scrapped-socials").Doc(id).Get(context.Background())
	if err != nil {
		return err
	}
	err = res.DataTo(s)
	if err != nil {
		return err
	}
	return nil
	// return socials, nil
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

func (_ Socials) GetMultipleBreifs(ids []string) ([]SocialsBreif, error) {
	q := myquery.Client.Query(`
    SELECT *
    FROM ` + SocialsFullTableName + `
    WHERE id IN UNNEST(@ids)
    ORDER BY ARRAY_POSITION(@ids, id)
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
func (_ Socials) GetMultiple(ids []string) ([]Socials, error) {
	q := myquery.Client.Query(`
    SELECT *
    FROM ` + SocialsFullTableName + `
    WHERE id IN UNNEST(@ids)
    ORDER BY ARRAY_POSITION(@ids, id)
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

func (data *Socials) GetInstagramFromFirestore(username string) error {
	data.Username = username
	data.SocialType = "instagram"
	id := data.GetID()

	d, err := firestoredb.Client.Collection("scrapped-socials").Doc(id).Get(context.Background())
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
