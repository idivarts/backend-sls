package linkedin

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"time"
)

// StatEntry is a single demographic facet of follower statistics: the facet
// URN (Label) and the combined follower count (Value).
type StatEntry struct {
	Label string `json:"label"`
	Value int64  `json:"value"`
}

// FollowerStats is the lifetime follower demographic breakdown for an
// organization, plus the total follower count.
type FollowerStats struct {
	TotalFollowers int64       `json:"totalFollowers"`
	ByIndustry     []StatEntry `json:"byIndustry"`
	BySeniority    []StatEntry `json:"bySeniority"`
	ByFunction     []StatEntry `json:"byFunction"`
	ByCountry      []StatEntry `json:"byCountry"`
}

// ShareDay is a single day of organization share (content) statistics.
type ShareDay struct {
	Date        string `json:"date"` // YYYY-MM-DD
	Impressions int64  `json:"impressions"`
	Engagement  int64  `json:"engagement"`
}

// ShareStats is the aggregated share (content) statistics for an organization
// over a time range, with a per-day series.
type ShareStats struct {
	Impressions       int64      `json:"impressions"`
	UniqueImpressions int64      `json:"uniqueImpressions"`
	Clicks            int64      `json:"clicks"`
	Likes             int64      `json:"likes"`
	Comments          int64      `json:"comments"`
	Shares            int64      `json:"shares"`
	Engagement        int64      `json:"engagement"`
	Series            []ShareDay `json:"series"`
}

// GetFollowerStatistics returns the lifetime follower demographic breakdowns for
// an organization plus its total follower count. The demographic breakdowns
// come from organizationalEntityFollowerStatistics (no timeIntervals → lifetime
// counts), and TotalFollowers from networkSizes.
func GetFollowerStatistics(accessToken, orgURN string) (*FollowerStats, error) {
	u := fmt.Sprintf(
		"%s/organizationalEntityFollowerStatistics?q=organizationalEntity&organizationalEntity=%s",
		RestBaseURL, url.QueryEscape(orgURN),
	)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("linkedin: build follower-statistics request: %w", err)
	}
	restHeaders(req, accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: follower-statistics request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin: organizationalEntityFollowerStatistics returned %d: %s", resp.StatusCode, string(body))
	}

	// Each facet array entry has its facet URN under a facet-specific key plus a
	// followerCounts{organicFollowerCount,paidFollowerCount} object.
	type followerCounts struct {
		Organic int64 `json:"organicFollowerCount"`
		Paid    int64 `json:"paidFollowerCount"`
	}
	var parsed struct {
		Elements []struct {
			ByIndustry []struct {
				Industry       string         `json:"industry"`
				FollowerCounts followerCounts `json:"followerCounts"`
			} `json:"followerCountsByIndustry"`
			BySeniority []struct {
				Seniority      string         `json:"seniority"`
				FollowerCounts followerCounts `json:"followerCounts"`
			} `json:"followerCountsBySeniority"`
			ByFunction []struct {
				Function       string         `json:"function"`
				FollowerCounts followerCounts `json:"followerCounts"`
			} `json:"followerCountsByFunction"`
			ByCountry []struct {
				Geo            string         `json:"geo"`
				FollowerCounts followerCounts `json:"followerCounts"`
			} `json:"followerCountsByGeoCountry"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("linkedin: parse follower-statistics: %w", err)
	}

	stats := &FollowerStats{}
	for _, el := range parsed.Elements {
		for _, e := range el.ByIndustry {
			stats.ByIndustry = append(stats.ByIndustry, StatEntry{
				Label: e.Industry,
				Value: e.FollowerCounts.Organic + e.FollowerCounts.Paid,
			})
		}
		for _, e := range el.BySeniority {
			stats.BySeniority = append(stats.BySeniority, StatEntry{
				Label: e.Seniority,
				Value: e.FollowerCounts.Organic + e.FollowerCounts.Paid,
			})
		}
		for _, e := range el.ByFunction {
			stats.ByFunction = append(stats.ByFunction, StatEntry{
				Label: e.Function,
				Value: e.FollowerCounts.Organic + e.FollowerCounts.Paid,
			})
		}
		for _, e := range el.ByCountry {
			stats.ByCountry = append(stats.ByCountry, StatEntry{
				Label: e.Geo,
				Value: e.FollowerCounts.Organic + e.FollowerCounts.Paid,
			})
		}
	}

	// Total follower count via networkSizes (best-effort: don't fail the whole
	// call if only this part errors).
	if total, terr := networkSize(accessToken, orgURN); terr == nil {
		stats.TotalFollowers = total
	}

	return stats, nil
}

// networkSize returns the first-degree network size (follower count) for an
// organization via the networkSizes API.
func networkSize(accessToken, orgURN string) (int64, error) {
	u := fmt.Sprintf(
		"%s/networkSizes/%s?edgeType=COMPANY_FOLLOWED_BY_MEMBER",
		RestBaseURL, url.PathEscape(orgURN),
	)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return 0, fmt.Errorf("linkedin: build networkSizes request: %w", err)
	}
	restHeaders(req, accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, fmt.Errorf("linkedin: networkSizes request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("linkedin: networkSizes returned %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		FirstDegreeSize int64 `json:"firstDegreeSize"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return 0, fmt.Errorf("linkedin: parse networkSizes: %w", err)
	}
	return parsed.FirstDegreeSize, nil
}

// GetShareStatistics returns the aggregated share (content) statistics for an
// organization over [startMs, endMs] at DAY granularity, with a per-day series.
func GetShareStatistics(accessToken, orgURN string, startMs, endMs int64) (*ShareStats, error) {
	u := fmt.Sprintf(
		"%s/organizationalEntityShareStatistics?q=organizationalEntity&organizationalEntity=%s"+
			"&timeIntervals.timeRange.start=%d&timeIntervals.timeRange.end=%d&timeIntervals.timeGranularityType=DAY",
		RestBaseURL, url.QueryEscape(orgURN), startMs, endMs,
	)
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("linkedin: build share-statistics request: %w", err)
	}
	restHeaders(req, accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: share-statistics request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin: organizationalEntityShareStatistics returned %d: %s", resp.StatusCode, string(body))
	}

	type totalShareStatistics struct {
		ImpressionCount        int64   `json:"impressionCount"`
		UniqueImpressionsCount int64   `json:"uniqueImpressionsCount"`
		ClickCount             int64   `json:"clickCount"`
		LikeCount              int64   `json:"likeCount"`
		CommentCount           int64   `json:"commentCount"`
		ShareCount             int64   `json:"shareCount"`
		Engagement             float64 `json:"engagement"`
	}
	var parsed struct {
		Elements []struct {
			TotalShareStatistics totalShareStatistics `json:"totalShareStatistics"`
			TimeRange            struct {
				Start int64 `json:"start"`
				End   int64 `json:"end"`
			} `json:"timeRange"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("linkedin: parse share-statistics: %w", err)
	}

	stats := &ShareStats{}
	for _, el := range parsed.Elements {
		t := el.TotalShareStatistics
		stats.Impressions += t.ImpressionCount
		stats.UniqueImpressions += t.UniqueImpressionsCount
		stats.Clicks += t.ClickCount
		stats.Likes += t.LikeCount
		stats.Comments += t.CommentCount
		stats.Shares += t.ShareCount
		stats.Engagement += int64(math.Round(t.Engagement))

		// Per-day engagement here = likes + comments + shares for that day.
		dayEngagement := t.LikeCount + t.CommentCount + t.ShareCount
		date := ""
		if el.TimeRange.Start > 0 {
			date = time.UnixMilli(el.TimeRange.Start).UTC().Format("2006-01-02")
		}
		stats.Series = append(stats.Series, ShareDay{
			Date:        date,
			Impressions: t.ImpressionCount,
			Engagement:  dayEngagement,
		})
	}

	return stats, nil
}
