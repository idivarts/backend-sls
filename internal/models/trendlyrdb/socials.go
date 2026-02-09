package trendlyrdb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"

	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/pkg/rdb"
	"github.com/lib/pq"
)

const (
	SocialsTableName = "socials"
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

	// Optional - We will deduce this from posts, hashtags and bio
	Niches []string `db:"niches" json:"niches"`
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

	// Marshal Links to JSONB
	linksJSON, err := json.Marshal(data.Links)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO socials (
			id, username, name, bio, profile_pic, profile_pic_hd, category,
			social_type, profile_verified, follower_count, following_count, content_count,
			views_count, engagement_count, engagement_rate, average_views, average_likes, average_comments,
			links, gender, location, niches, quality_score,
			added_by, creation_time, last_update_time, external_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23, $24, $25, $26, $27
		)
		ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username,
			name = EXCLUDED.name,
			bio = EXCLUDED.bio,
			profile_pic = EXCLUDED.profile_pic,
			profile_pic_hd = EXCLUDED.profile_pic_hd,
			category = EXCLUDED.category,
			social_type = EXCLUDED.social_type,
			profile_verified = EXCLUDED.profile_verified,
			follower_count = EXCLUDED.follower_count,
			following_count = EXCLUDED.following_count,
			content_count = EXCLUDED.content_count,
			views_count = EXCLUDED.views_count,
			engagement_count = EXCLUDED.engagement_count,
			engagement_rate = EXCLUDED.engagement_rate,
			average_views = EXCLUDED.average_views,
			average_likes = EXCLUDED.average_likes,
			average_comments = EXCLUDED.average_comments,
			links = EXCLUDED.links,
			gender = EXCLUDED.gender,
			location = EXCLUDED.location,
			niches = EXCLUDED.niches,
			quality_score = EXCLUDED.quality_score,
			added_by = EXCLUDED.added_by,
			last_update_time = EXCLUDED.last_update_time,
			external_id = EXCLUDED.external_id
	`

	_, err = rdb.DB.Exec(query,
		data.ID, data.Username, data.Name, data.Bio, data.ProfilePic, data.ProfilePicHD, data.Category,
		data.SocialType, data.ProfileVerified, data.FollowerCount, data.FollowingCount, data.ContentCount,
		data.ViewsCount, data.EngagementCount, data.EngagementRate, data.AverageViews, data.AverageLikes, data.AverageComments,
		linksJSON, data.Gender, data.Location, pq.Array(data.Niches), data.QualityScore,
		data.AddedBy, data.CreationTime, data.LastUpdateTime, data.ExternalId,
	)

	return err
}

func (_ Socials) InsertMultiple(socials []Socials) error {
	// Use a transaction for bulk insert
	tx, err := rdb.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT INTO socials (
			id, username, name, bio, profile_pic, profile_pic_hd, category,
			social_type, profile_verified, follower_count, following_count, content_count,
			views_count, engagement_count, engagement_rate, average_views, average_likes, average_comments,
			links, gender, location, niches, quality_score,
			added_by, creation_time, last_update_time, external_id
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18,
			$19, $20, $21, $22, $23, $24, $25, $26, $27
		)
		ON CONFLICT (id) DO UPDATE SET
			username = EXCLUDED.username,
			last_update_time = EXCLUDED.last_update_time
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, social := range socials {
		social.ID = social.GetID()
		linksJSON, err := json.Marshal(social.Links)
		if err != nil {
			return err
		}

		_, err = stmt.Exec(
			social.ID, social.Username, social.Name, social.Bio, social.ProfilePic, social.ProfilePicHD, social.Category,
			social.SocialType, social.ProfileVerified, social.FollowerCount, social.FollowingCount, social.ContentCount,
			social.ViewsCount, social.EngagementCount, social.EngagementRate, social.AverageViews, social.AverageLikes, social.AverageComments,
			linksJSON, social.Gender, social.Location, pq.Array(social.Niches), social.QualityScore,
			social.AddedBy, social.CreationTime, social.LastUpdateTime, social.ExternalId,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (_ Socials) GetPaginated(offset, limit int) ([]Socials, error) {
	query := `
		SELECT id, username, name, bio, profile_pic, profile_pic_hd, category,
			social_type, profile_verified, follower_count, following_count, content_count,
			views_count, engagement_count, engagement_rate, average_views, average_likes, average_comments,
			links, gender, location, niches, quality_score,
			added_by, creation_time, last_update_time, external_id
		FROM socials
		ORDER BY last_update_time DESC
		LIMIT $1 OFFSET $2
	`

	rows, err := rdb.DB.Query(query, limit, offset)
	if err != nil {
		log.Println("Error:", err.Error())
		return nil, err
	}
	defer rows.Close()

	results := []Socials{}
	for rows.Next() {
		var data Socials
		var linksJSON []byte

		err = rows.Scan(
			&data.ID, &data.Username, &data.Name, &data.Bio, &data.ProfilePic, &data.ProfilePicHD, &data.Category,
			&data.SocialType, &data.ProfileVerified, &data.FollowerCount, &data.FollowingCount, &data.ContentCount,
			&data.ViewsCount, &data.EngagementCount, &data.EngagementRate, &data.AverageViews, &data.AverageLikes, &data.AverageComments,
			&linksJSON, &data.Gender, &data.Location, pq.Array(&data.Niches), &data.QualityScore,
			&data.AddedBy, &data.CreationTime, &data.LastUpdateTime, &data.ExternalId,
		)
		if err != nil {
			log.Println("Error scanning row:", err.Error())
			continue
		}

		// Unmarshal JSONB links
		if len(linksJSON) > 0 {
			if err := json.Unmarshal(linksJSON, &data.Links); err != nil {
				log.Println("Error unmarshaling links:", err.Error())
			}
		}

		results = append(results, data)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func (data *Socials) Get(id string) error {
	query := `
		SELECT id, username, name, bio, profile_pic, profile_pic_hd, category,
			social_type, profile_verified, follower_count, following_count, content_count,
			views_count, engagement_count, engagement_rate, average_views, average_likes, average_comments,
			links, gender, location, niches, quality_score,
			added_by, creation_time, last_update_time, external_id
		FROM socials
		WHERE id = $1
		LIMIT 1
	`

	var linksJSON []byte
	err := rdb.DB.QueryRow(query, id).Scan(
		&data.ID, &data.Username, &data.Name, &data.Bio, &data.ProfilePic, &data.ProfilePicHD, &data.Category,
		&data.SocialType, &data.ProfileVerified, &data.FollowerCount, &data.FollowingCount, &data.ContentCount,
		&data.ViewsCount, &data.EngagementCount, &data.EngagementRate, &data.AverageViews, &data.AverageLikes, &data.AverageComments,
		&linksJSON, &data.Gender, &data.Location, pq.Array(&data.Niches), &data.QualityScore,
		&data.AddedBy, &data.CreationTime, &data.LastUpdateTime, &data.ExternalId,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return errors.New("social not found")
		}
		return err
	}

	// Unmarshal JSONB links
	if len(linksJSON) > 0 {
		if err := json.Unmarshal(linksJSON, &data.Links); err != nil {
			return err
		}
	}

	return nil
}

func (_ Socials) GetMultiple(ids []string) ([]Socials, error) {
	query := `
		SELECT id, username, name, bio, profile_pic, profile_pic_hd, category,
			social_type, profile_verified, follower_count, following_count, content_count,
			views_count, engagement_count, engagement_rate, average_views, average_likes, average_comments,
			links, gender, location, niches, quality_score,
			added_by, creation_time, last_update_time, external_id
		FROM socials
		WHERE id = ANY($1)
	`

	rows, err := rdb.DB.Query(query, pq.Array(ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []Socials{}
	for rows.Next() {
		var data Socials
		var linksJSON []byte

		err = rows.Scan(
			&data.ID, &data.Username, &data.Name, &data.Bio, &data.ProfilePic, &data.ProfilePicHD, &data.Category,
			&data.SocialType, &data.ProfileVerified, &data.FollowerCount, &data.FollowingCount, &data.ContentCount,
			&data.ViewsCount, &data.EngagementCount, &data.EngagementRate, &data.AverageViews, &data.AverageLikes, &data.AverageComments,
			&linksJSON, &data.Gender, &data.Location, pq.Array(&data.Niches), &data.QualityScore,
			&data.AddedBy, &data.CreationTime, &data.LastUpdateTime, &data.ExternalId,
		)
		if err != nil {
			log.Println("Error scanning row:", err.Error())
			continue
		}

		// Unmarshal JSONB links
		if len(linksJSON) > 0 {
			if err := json.Unmarshal(linksJSON, &data.Links); err != nil {
				log.Println("Error unmarshaling links:", err.Error())
			}
		}

		results = append(results, data)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return results, nil
}

func (data *Socials) GetInstagram(username string) error {
	data.Username = username
	data.SocialType = "instagram"
	id := data.GetID()

	return data.Get(id)
}
