package youtube

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
)

// ChannelStatistics is the parsed snapshot of a channel's lifetime statistics.
//
// NOTE: the requested name for this type was `ChannelStats`, but that identifier
// is already taken in profile.go by the raw-string Data-API DTO (and is in use by
// internal/trendlyapis/social_connect/youtube.go). Since existing files must not
// be edited, this parsed (int64) variant is named `ChannelStatistics` to avoid a
// redeclaration. GetChannelStats returns *ChannelStatistics. See deviation note.
type ChannelStatistics struct {
	Subscribers int64
	Views       int64
	Videos      int64
}

// AnalyticsPoint is a single day's worth of channel analytics.
type AnalyticsPoint struct {
	Date                    string
	Views                   int64
	Likes                   int64
	Comments                int64
	Shares                  int64
	EstimatedMinutesWatched int64
	SubscribersGained       int64
}

// DemoBucket groups demographic entries under one dimension (e.g. "age", "gender", "country").
type DemoBucket struct {
	Dimension string
	Entries   []DemoKV
}

// DemoKV is a single demographic label/value pair.
type DemoKV struct {
	Label string
	Value int64
}

// TopVideo is a high-performing video with hydrated snippet metadata.
type TopVideo struct {
	ID           string
	Title        string
	ThumbnailURL string
	Views        int64
	Likes        int64
	Comments     int64
}

// reportResponse mirrors the YouTube Analytics API reports response shape.
type reportResponse struct {
	ColumnHeaders []struct {
		Name string `json:"name"`
	} `json:"columnHeaders"`
	Rows [][]interface{} `json:"rows"`
}

// ytGet performs an authenticated GET and returns the raw body, erroring on non-200.
func ytGet(accessToken, fullURL string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("youtube: failed to build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("youtube: request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("youtube: endpoint returned %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}

// cellInt64 coerces an analytics row cell (often a float64 from JSON) to int64.
func cellInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return int64(f)
		}
	}
	return 0
}

// cellString coerces an analytics row cell to string.
func cellString(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// cellRoundInt64 rounds a (possibly float) analytics cell to the nearest int64.
func cellRoundInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(math.Round(n))
	case string:
		if f, err := strconv.ParseFloat(n, 64); err == nil {
			return int64(math.Round(f))
		}
	}
	return cellInt64(v)
}

// GetChannelStats fetches a channel's lifetime statistics via the Data API v3.
func GetChannelStats(accessToken, channelID string) (*ChannelStatistics, error) {
	fullURL := fmt.Sprintf("%s/channels?part=statistics&id=%s", APIURL, channelID)
	body, err := ytGet(accessToken, fullURL)
	if err != nil {
		return nil, err
	}

	var list channelListResponse
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, fmt.Errorf("youtube: failed to parse channel stats response: %w", err)
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("youtube: no channel found for id %s", channelID)
	}

	s := list.Items[0].Stats
	parse := func(v string) int64 {
		n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		return n
	}
	return &ChannelStatistics{
		Subscribers: parse(s.SubscriberCount),
		Views:       parse(s.ViewCount),
		Videos:      parse(s.VideoCount),
	}, nil
}

// GetChannelAnalytics fetches per-day channel analytics between startDate and
// endDate (both YYYY-MM-DD). accessToken must have yt-analytics.readonly scope.
func GetChannelAnalytics(accessToken, channelID, startDate, endDate string) ([]AnalyticsPoint, error) {
	fullURL := fmt.Sprintf(
		"%s/reports?ids=channel==%s&startDate=%s&endDate=%s&metrics=views,likes,comments,shares,estimatedMinutesWatched,subscribersGained&dimensions=day&sort=day",
		AnalyticsURL, channelID, startDate, endDate,
	)
	body, err := ytGet(accessToken, fullURL)
	if err != nil {
		return nil, err
	}

	var rep reportResponse
	if err := json.Unmarshal(body, &rep); err != nil {
		return nil, fmt.Errorf("youtube: failed to parse channel analytics response: %w", err)
	}

	// Build a name→index map from the column headers.
	idx := make(map[string]int, len(rep.ColumnHeaders))
	for i, h := range rep.ColumnHeaders {
		idx[h.Name] = i
	}
	get := func(row []interface{}, name string) interface{} {
		if i, ok := idx[name]; ok && i < len(row) {
			return row[i]
		}
		return nil
	}

	points := make([]AnalyticsPoint, 0, len(rep.Rows))
	for _, row := range rep.Rows {
		points = append(points, AnalyticsPoint{
			Date:                    cellString(get(row, "day")),
			Views:                   cellInt64(get(row, "views")),
			Likes:                   cellInt64(get(row, "likes")),
			Comments:                cellInt64(get(row, "comments")),
			Shares:                  cellInt64(get(row, "shares")),
			EstimatedMinutesWatched: cellInt64(get(row, "estimatedMinutesWatched")),
			SubscribersGained:       cellInt64(get(row, "subscribersGained")),
		})
	}
	return points, nil
}

// GetAgeGenderDemographics returns two buckets — "age" and "gender" — each with
// viewerPercentage entries (rounded to int64).
func GetAgeGenderDemographics(accessToken, channelID, startDate, endDate string) ([]DemoBucket, error) {
	fetch := func(dimension string) (DemoBucket, error) {
		fullURL := fmt.Sprintf(
			"%s/reports?ids=channel==%s&startDate=%s&endDate=%s&metrics=viewerPercentage&dimensions=%s",
			AnalyticsURL, channelID, startDate, endDate, dimension,
		)
		body, err := ytGet(accessToken, fullURL)
		if err != nil {
			return DemoBucket{}, err
		}
		var rep reportResponse
		if err := json.Unmarshal(body, &rep); err != nil {
			return DemoBucket{}, fmt.Errorf("youtube: failed to parse %s demographics response: %w", dimension, err)
		}
		entries := make([]DemoKV, 0, len(rep.Rows))
		for _, row := range rep.Rows {
			if len(row) < 2 {
				continue
			}
			entries = append(entries, DemoKV{
				Label: cellString(row[0]),
				Value: cellRoundInt64(row[1]),
			})
		}
		return DemoBucket{Entries: entries}, nil
	}

	ageBucket, err := fetch("ageGroup")
	if err != nil {
		return nil, err
	}
	ageBucket.Dimension = "age"

	genderBucket, err := fetch("gender")
	if err != nil {
		return nil, err
	}
	genderBucket.Dimension = "gender"

	return []DemoBucket{ageBucket, genderBucket}, nil
}

// GetCountryViews returns the top-10 countries by views as a single DemoBucket.
func GetCountryViews(accessToken, channelID, startDate, endDate string) (*DemoBucket, error) {
	fullURL := fmt.Sprintf(
		"%s/reports?ids=channel==%s&startDate=%s&endDate=%s&metrics=views&dimensions=country&sort=-views&maxResults=10",
		AnalyticsURL, channelID, startDate, endDate,
	)
	body, err := ytGet(accessToken, fullURL)
	if err != nil {
		return nil, err
	}

	var rep reportResponse
	if err := json.Unmarshal(body, &rep); err != nil {
		return nil, fmt.Errorf("youtube: failed to parse country views response: %w", err)
	}

	bucket := &DemoBucket{Dimension: "country"}
	for _, row := range rep.Rows {
		if len(row) < 2 {
			continue
		}
		bucket.Entries = append(bucket.Entries, DemoKV{
			Label: cellString(row[0]),
			Value: cellInt64(row[1]),
		})
	}
	return bucket, nil
}

// videoListResponse mirrors the Data API videos.list snippet response.
type videoListResponse struct {
	Items []struct {
		ID      string `json:"id"`
		Snippet struct {
			Title      string `json:"title"`
			Thumbnails struct {
				Medium ThumbnailItem `json:"medium"`
			} `json:"thumbnails"`
		} `json:"snippet"`
	} `json:"items"`
}

// GetTopVideos returns the top videos (by views) within the date range, hydrated
// with title + medium thumbnail from the Data API.
func GetTopVideos(accessToken, channelID, startDate, endDate string, limit int) ([]TopVideo, error) {
	fullURL := fmt.Sprintf(
		"%s/reports?ids=channel==%s&startDate=%s&endDate=%s&metrics=views,likes,comments&dimensions=video&sort=-views&maxResults=%d",
		AnalyticsURL, channelID, startDate, endDate, limit,
	)
	body, err := ytGet(accessToken, fullURL)
	if err != nil {
		return nil, err
	}

	var rep reportResponse
	if err := json.Unmarshal(body, &rep); err != nil {
		return nil, fmt.Errorf("youtube: failed to parse top videos response: %w", err)
	}

	idx := make(map[string]int, len(rep.ColumnHeaders))
	for i, h := range rep.ColumnHeaders {
		idx[h.Name] = i
	}
	get := func(row []interface{}, name string) interface{} {
		if i, ok := idx[name]; ok && i < len(row) {
			return row[i]
		}
		return nil
	}

	videos := make([]TopVideo, 0, len(rep.Rows))
	order := make(map[string]int, len(rep.Rows))
	ids := make([]string, 0, len(rep.Rows))
	for _, row := range rep.Rows {
		id := cellString(get(row, "video"))
		if id == "" {
			continue
		}
		order[id] = len(videos)
		ids = append(ids, id)
		videos = append(videos, TopVideo{
			ID:       id,
			Views:    cellInt64(get(row, "views")),
			Likes:    cellInt64(get(row, "likes")),
			Comments: cellInt64(get(row, "comments")),
		})
	}

	if len(ids) == 0 {
		return videos, nil
	}

	// Hydrate title + thumbnail from the Data API.
	metaURL := fmt.Sprintf("%s/videos?part=snippet&id=%s", APIURL, strings.Join(ids, ","))
	metaBody, err := ytGet(accessToken, metaURL)
	if err != nil {
		// Best-effort hydration: return the analytics rows without metadata.
		return videos, nil
	}
	var meta videoListResponse
	if err := json.Unmarshal(metaBody, &meta); err != nil {
		return videos, nil
	}
	for _, item := range meta.Items {
		if i, ok := order[item.ID]; ok {
			videos[i].Title = item.Snippet.Title
			videos[i].ThumbnailURL = item.Snippet.Thumbnails.Medium.URL
		}
	}
	return videos, nil
}
