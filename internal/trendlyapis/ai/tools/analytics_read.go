package tools

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	analytics "github.com/idivarts/backend-sls/internal/trendlyapis/analytics"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// rangeDays resolves a "7d"|"28d"|"90d" range arg (default 28d) to a day count.
func rangeDays(args map[string]any) int {
	return analytics.ParseRange(argStr(args, "range")).Days()
}

func rangeProp() map[string]any {
	return openrouter.EnumProp("Analytics window (default 28d).", []string{"7d", "28d", "90d"})
}

// gatherSnapshots collects daily analytics snapshots for one account (when
// socialId is given) or all of the brand's accounts, going back `days`.
func gatherSnapshots(brandID, socialID string, days int) ([]trendlymodels.AnalyticsSnapshot, error) {
	since := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
	if socialID != "" {
		return trendlymodels.ListAnalyticsSnapshots(brandID, socialID, since)
	}
	accs, err := trendlymodels.ListBrandSocialAccounts(brandID)
	if err != nil {
		return nil, err
	}
	var all []trendlymodels.AnalyticsSnapshot
	for _, a := range accs {
		s, err := trendlymodels.ListAnalyticsSnapshots(brandID, a.ID, since)
		if err == nil {
			all = append(all, s...)
		}
	}
	return all, nil
}

type dayAgg struct {
	followers, reach, impressions, engagement, views int64
}

// aggregateByDate sums snapshots per calendar date (across accounts) and returns
// the sorted date keys plus the per-date aggregate.
func aggregateByDate(snaps []trendlymodels.AnalyticsSnapshot) ([]string, map[string]*dayAgg) {
	agg := map[string]*dayAgg{}
	for _, s := range snaps {
		a := agg[s.Date]
		if a == nil {
			a = &dayAgg{}
			agg[s.Date] = a
		}
		a.followers += s.Followers
		a.reach += s.Reach
		a.impressions += s.Impressions
		a.engagement += s.Engagement
		a.views += s.Views
	}
	dates := make([]string, 0, len(agg))
	for d := range agg {
		dates = append(dates, d)
	}
	sort.Strings(dates)
	return dates, agg
}

func accountOverview() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_account_overview",
			"Brand-wide analytics summary for a window: total followers, reach, impressions, engagement and views, plus a per-account breakdown.",
			openrouter.ObjectSchema(map[string]any{"range": rangeProp()}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			ov, err := loadOverview(brandID, argStr(args, "range"))
			if err != nil {
				return map[string]any{"error": err.Error()}, nil
			}
			accts := make([]map[string]any, 0, len(ov.Accounts))
			for _, a := range ov.Accounts {
				metrics := map[string]int64{}
				for k, m := range a.Metrics {
					metrics[k] = m.Total
				}
				accts = append(accts, map[string]any{
					"platform":      a.Platform,
					"username":      a.Username,
					"followerCount": a.FollowerCount,
					"metrics":       metrics,
					"supported":     a.Supported,
				})
			}
			return map[string]any{"range": ov.Range, "totals": ov.Totals, "accounts": accts, "generatedAt": ov.GeneratedAt}, nil
		},
	}
}

func topMediaTool(name, desc string, asc bool) Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			name, desc,
			openrouter.ObjectSchema(map[string]any{
				"range":  rangeProp(),
				"metric": openrouter.EnumProp("Ranking metric (default engagement).", []string{"engagement", "likes", "comments"}),
				"limit":  openrouter.NumberProp("Max posts to return (default 5)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			ov, err := loadOverview(brandID, argStr(args, "range"))
			if err != nil {
				return map[string]any{"error": err.Error()}, nil
			}
			metric := argStr(args, "metric")
			rows := sortMediaByMetric(allTopMedia(ov), metric, asc)
			limit := argInt(args, "limit", 5)
			out := make([]map[string]any, 0, limit)
			for _, m := range rows {
				if len(out) >= limit {
					break
				}
				out = append(out, map[string]any{
					"id": m.ID, "platform": m.Platform, "username": m.Username,
					"caption": m.Caption, "mediaType": m.MediaType, "permalink": m.Permalink,
					"timestamp": m.Timestamp, "likes": m.Likes, "comments": m.Comments, "engagement": m.Engagement,
				})
			}
			return map[string]any{"posts": out, "rankedBy": metric, "note": "Ranked within the brand's tracked top media for the window."}, nil
		},
	}
}

func topPerformingPosts() Registered {
	return topMediaTool("get_top_performing_posts",
		"The brand's best-performing posts in a window, ranked by a metric — use to learn what content works.", false)
}

func underperformers() Registered {
	return topMediaTool("get_underperformers",
		"The lowest-performing posts among the brand's tracked media in a window — use to learn what to stop doing.", true)
}

func followerGrowth() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_follower_growth",
			"Daily follower time-series over a window (summed across accounts, or one account) with net growth.",
			openrouter.ObjectSchema(map[string]any{
				"range":    rangeProp(),
				"socialId": openrouter.StringProp("Optional connected-account ID to scope to a single platform."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			snaps, err := gatherSnapshots(brandID, argStr(args, "socialId"), rangeDays(args))
			if err != nil {
				return nil, err
			}
			dates, agg := aggregateByDate(snaps)
			series := make([]map[string]any, 0, len(dates))
			for _, d := range dates {
				series = append(series, map[string]any{"date": d, "followers": agg[d].followers})
			}
			net := int64(0)
			if len(dates) >= 2 {
				net = agg[dates[len(dates)-1]].followers - agg[dates[0]].followers
			}
			return map[string]any{"series": series, "netGrowth": net, "points": len(series)}, nil
		},
	}
}

func engagementTrends() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_engagement_trends",
			"Daily engagement/reach/views time-series over a window (summed across accounts, or one account).",
			openrouter.ObjectSchema(map[string]any{
				"range":    rangeProp(),
				"socialId": openrouter.StringProp("Optional connected-account ID to scope to a single platform."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			snaps, err := gatherSnapshots(brandID, argStr(args, "socialId"), rangeDays(args))
			if err != nil {
				return nil, err
			}
			dates, agg := aggregateByDate(snaps)
			series := make([]map[string]any, 0, len(dates))
			var totalEng int64
			for _, d := range dates {
				series = append(series, map[string]any{"date": d, "engagement": agg[d].engagement, "reach": agg[d].reach, "views": agg[d].views})
				totalEng += agg[d].engagement
			}
			return map[string]any{"series": series, "totalEngagement": totalEng, "points": len(series)}, nil
		},
	}
}

func formatPerformance() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_format_performance",
			"Compare content formats (reel/carousel/image/video) by average engagement across the brand's tracked top media.",
			openrouter.ObjectSchema(map[string]any{"range": rangeProp()}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			ov, err := loadOverview(brandID, argStr(args, "range"))
			if err != nil {
				return map[string]any{"error": err.Error()}, nil
			}
			type agg struct{ posts, total int64 }
			byType := map[string]*agg{}
			for _, m := range allTopMedia(ov) {
				t := strings.ToUpper(m.MediaType)
				if t == "" {
					t = "UNKNOWN"
				}
				a := byType[t]
				if a == nil {
					a = &agg{}
					byType[t] = a
				}
				a.posts++
				a.total += m.Engagement
			}
			out := make([]map[string]any, 0, len(byType))
			for t, a := range byType {
				avg := int64(0)
				if a.posts > 0 {
					avg = a.total / a.posts
				}
				out = append(out, map[string]any{"format": t, "posts": a.posts, "avgEngagement": avg, "totalEngagement": a.total})
			}
			sort.Slice(out, func(i, j int) bool { return out[i]["avgEngagement"].(int64) > out[j]["avgEngagement"].(int64) })
			return map[string]any{"formats": out, "note": "Based on the brand's tracked top media for the window."}, nil
		},
	}
}

func bestPostingTimes() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_best_posting_times",
			"Suggest the best posting weekday/hour slots, derived from when the brand's top-performing media were published.",
			openrouter.ObjectSchema(map[string]any{
				"range": rangeProp(),
				"limit": openrouter.NumberProp("Max slots to return (default 5)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			ov, err := loadOverview(brandID, argStr(args, "range"))
			if err != nil {
				return map[string]any{"error": err.Error()}, nil
			}
			type agg struct{ posts, total int64 }
			buckets := map[string]*agg{}
			for _, m := range allTopMedia(ov) {
				if m.Timestamp <= 0 {
					continue
				}
				t := time.Unix(m.Timestamp, 0).UTC()
				key := t.Weekday().String() + " " + t.Format("15:00")
				a := buckets[key]
				if a == nil {
					a = &agg{}
					buckets[key] = a
				}
				a.posts++
				a.total += m.Engagement
			}
			out := make([]map[string]any, 0, len(buckets))
			for k, a := range buckets {
				avg := int64(0)
				if a.posts > 0 {
					avg = a.total / a.posts
				}
				out = append(out, map[string]any{"slot": k, "posts": a.posts, "avgEngagement": avg})
			}
			sort.Slice(out, func(i, j int) bool { return out[i]["avgEngagement"].(int64) > out[j]["avgEngagement"].(int64) })
			limit := argInt(args, "limit", 5)
			if len(out) > limit {
				out = out[:limit]
			}
			return map[string]any{"slots": out, "timezone": "UTC", "note": "Derived from tracked top-media publish times; more history improves accuracy."}, nil
		},
	}
}

func hashtagPerformance() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_hashtag_topic_performance",
			"Rank the hashtags the brand uses by usage and (where a post is among tracked top media) by average engagement.",
			openrouter.ObjectSchema(dateRangeProps(map[string]any{
				"limit": openrouter.NumberProp("Max hashtags to return (default 15)."),
			}), nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			// Engagement lookup: published media id -> engagement (from top media).
			mediaEng := map[string]int64{}
			if ov, err := loadOverview(brandID, "28d"); err == nil {
				for _, m := range allTopMedia(ov) {
					mediaEng[m.ID] = m.Engagement
				}
			}
			start, end := rangeFromArgs(args)
			items, err := trendlymodels.ListContentInRange(ctx, brandID, start, end, true)
			if err != nil {
				return nil, err
			}
			type agg struct{ uses, matched, total int64 }
			byTag := map[string]*agg{}
			for _, ct := range items {
				tags := splitHashtags(ct.Hashtags)
				if len(tags) == 0 {
					continue
				}
				var eng int64
				matched := false
				for _, mid := range ct.PublishedIds {
					if e, ok := mediaEng[mid]; ok {
						eng += e
						matched = true
					}
				}
				for _, tag := range tags {
					a := byTag[tag]
					if a == nil {
						a = &agg{}
						byTag[tag] = a
					}
					a.uses++
					if matched {
						a.matched++
						a.total += eng
					}
				}
			}
			out := make([]map[string]any, 0, len(byTag))
			for tag, a := range byTag {
				avg := int64(0)
				if a.matched > 0 {
					avg = a.total / a.matched
				}
				out = append(out, map[string]any{"hashtag": tag, "uses": a.uses, "matchedPosts": a.matched, "avgEngagement": avg})
			}
			sort.Slice(out, func(i, j int) bool { return out[i]["uses"].(int64) > out[j]["uses"].(int64) })
			limit := argInt(args, "limit", 15)
			if len(out) > limit {
				out = out[:limit]
			}
			return map[string]any{"hashtags": out, "note": "avgEngagement is only available for posts among tracked top media."}, nil
		},
	}
}

// splitHashtags normalizes a free-form hashtag string into lowercase tags.
func splitHashtags(s string) []string {
	if s == "" {
		return nil
	}
	fields := strings.FieldsFunc(s, func(r rune) bool {
		return r == ' ' || r == ',' || r == '\n' || r == '\t' || r == '\r'
	})
	seen := map[string]bool{}
	var out []string
	for _, f := range fields {
		t := strings.ToLower(strings.TrimSpace(f))
		if len(t) <= 1 {
			continue
		}
		if !strings.HasPrefix(t, "#") {
			t = "#" + t
		}
		if !seen[t] {
			seen[t] = true
			out = append(out, t)
		}
	}
	return out
}

func comparePeriods() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"compare_periods",
			"Compare the current period against the immediately preceding one (engagement, reach, views summed; followers end-to-end) to show momentum.",
			openrouter.ObjectSchema(map[string]any{
				"days":     openrouter.NumberProp("Length of each period in days (default 28)."),
				"socialId": openrouter.StringProp("Optional connected-account ID to scope to a single platform."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			days := argInt(args, "days", 28)
			snaps, err := gatherSnapshots(brandID, argStr(args, "socialId"), days*2)
			if err != nil {
				return nil, err
			}
			dates, agg := aggregateByDate(snaps)
			boundary := time.Now().UTC().AddDate(0, 0, -days).Format("2006-01-02")
			sum := func(current bool) (eng, reach, views, followers int64) {
				var lastDate string
				for _, d := range dates {
					inCurrent := d >= boundary
					if inCurrent != current {
						continue
					}
					eng += agg[d].engagement
					reach += agg[d].reach
					views += agg[d].views
					if d > lastDate {
						lastDate = d
						followers = agg[d].followers
					}
				}
				return
			}
			cEng, cReach, cViews, cFollowers := sum(true)
			pEng, pReach, pViews, pFollowers := sum(false)
			return map[string]any{
				"periodDays": days,
				"current":    map[string]any{"engagement": cEng, "reach": cReach, "views": cViews, "endingFollowers": cFollowers},
				"previous":   map[string]any{"engagement": pEng, "reach": pReach, "views": pViews, "endingFollowers": pFollowers},
				"delta": map[string]any{
					"engagement": cEng - pEng, "reach": cReach - pReach, "views": cViews - pViews, "followers": cFollowers - pFollowers,
				},
			}, nil
		},
	}
}

func audienceDemographics() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_audience_demographics",
			"Audience demographics (age, gender, country, city) per connected account, where the platform supports it (Instagram/Facebook).",
			openrouter.ObjectSchema(map[string]any{"range": rangeProp()}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			ov, err := loadOverview(brandID, argStr(args, "range"))
			if err != nil {
				return map[string]any{"error": err.Error()}, nil
			}
			out := make([]map[string]any, 0, len(ov.Accounts))
			for _, a := range ov.Accounts {
				if len(a.Demographics) == 0 {
					continue
				}
				out = append(out, map[string]any{"platform": a.Platform, "username": a.Username, "demographics": a.Demographics})
			}
			return map[string]any{"accounts": out, "note": "Only Instagram/Facebook expose demographics in phase 1."}, nil
		},
	}
}

func postAnalytics() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_post_analytics",
			"Live per-post metrics (reach, likes, comments, saves, shares, views, engagement) for a published post. Pass contentId to resolve every published platform automatically, or socialId+mediaId for one.",
			openrouter.ObjectSchema(map[string]any{
				"contentId": openrouter.StringProp("Content document ID (resolves published media across its platforms)."),
				"socialId":  openrouter.StringProp("Connected-account ID (used with mediaId for a single post)."),
				"mediaId":   openrouter.StringProp("Published media ID on the platform."),
				"channel":   openrouter.EnumProp("Platform for the mediaId path (default instagram).", []string{"instagram", "facebook"}),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			// Explicit single-post path.
			if mid := argStr(args, "mediaId"); mid != "" {
				pa, err := analytics.PostAnalyticsFor(brandID, argStr(args, "socialId"), mid, argStr(args, "channel"))
				if err != nil {
					return map[string]any{"error": err.Error()}, nil
				}
				return pa, nil
			}
			// Content-resolved path.
			cid := argStr(args, "contentId")
			if cid == "" {
				return map[string]any{"error": "provide contentId, or socialId+mediaId"}, nil
			}
			ct, err := trendlymodels.GetContent(brandID, cid)
			if err != nil {
				return nil, err
			}
			if len(ct.PublishedIds) == 0 {
				return map[string]any{"error": "content has no published posts yet", "status": ct.Status}, nil
			}
			platformSocial := map[string]string{}
			for _, d := range ct.Destinations {
				platformSocial[d.Platform] = d.SocialAccountID
			}
			var posts []any
			for platform, mediaID := range ct.PublishedIds {
				socialID := platformSocial[platform]
				pa, perr := analytics.PostAnalyticsFor(brandID, socialID, mediaID, platform)
				if perr != nil {
					posts = append(posts, map[string]any{"platform": platform, "error": perr.Error()})
					continue
				}
				posts = append(posts, pa)
			}
			return map[string]any{"contentId": cid, "posts": posts}, nil
		},
	}
}
