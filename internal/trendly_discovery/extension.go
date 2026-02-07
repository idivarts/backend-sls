package trendlydiscovery

import (
	"context"
	"net/http"
	"sort"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlybq"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	sqshandler "github.com/idivarts/backend-sls/pkg/sqs_handler"
)

func medianInt64(xs []int64) float32 {
	if len(xs) == 0 {
		return 0
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	n := len(xs)
	if n%2 == 1 {
		return float32(xs[n/2])
	}
	a := xs[n/2-1]
	b := xs[n/2]
	return float32(a+b) / 2
}

func medianFloat32(xs []float32) float32 {
	if len(xs) == 0 {
		return 0
	}
	sort.Slice(xs, func(i, j int) bool { return xs[i] < xs[j] })
	n := len(xs)
	if n%2 == 1 {
		return xs[n/2]
	}
	a := xs[n/2-1]
	b := xs[n/2]
	return (a + b) / 2
}

// ScrapedProfile represents the payload coming from your scraper.
type ScrapedProfile struct {
	Username string `json:"username" binding:"required"`
	Manual   Manual `json:"manual"`
}

type Manual struct {
	Gender          string   `json:"gender"`
	Niches          []string `json:"niches"`
	Location        string   `json:"location"`
	AestheticsScore int      `json:"aestheticsScore" binding:"gte=0,lte=100"`
}

func AddProfile(c *gin.Context) {
	var req ScrapedProfile
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid Input", "error": err.Error()})
		return
	}

	adderUserId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not authenticated", "error": "UserId not found"})
		return
	}

	checkData := trendlybq.SocialsN8N{}
	err := checkData.GetInstagram(req.Username)
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{"message": "Profile already exists", "id": checkData.ID})
		return
	}

	data := &trendlybq.SocialsScrapePending{
		SocialType: "instagram",
		Username:   req.Username,

		Gender:       req.Manual.Gender,
		Niches:       req.Manual.Niches,
		Location:     req.Manual.Location,
		QualityScore: req.Manual.AestheticsScore,

		CreationTime:   time.Now().UnixMicro(), // TODO: set actual creation time
		LastUpdateTime: time.Now().UnixMicro(),
		AddedBy:        adderUserId,
	}

	err = data.InsertToFirestore()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Data Insert Error", "error": err.Error()})
		return
	}

	allDocs, err := firestoredb.Client.Collection("scrapped-socials-n8n").Where("state", "==", 0).Where("added_by", "==", adderUserId).Documents(context.Background()).GetAll()
	dLen := 0
	if err == nil {
		dLen = len(allDocs)
	}

	sqshandler.SendToMessageQueue(data.ID, 0)

	c.JSON(http.StatusAccepted, gin.H{"message": "Profile received", "id": data.ID, "count": dLen})
}

func LoadAllProfiles(c *gin.Context) {
	//Example URL to test this - https://api.apify.com/v2/datasets/1vX9FW3yaOzkrFeBT/items

}

// func calculateFunctionLater(){
// 	eRates := []float32{}
// 	viewsList := []int64{}
// 	likesList := []int64{}
// 	commentsList := []int64{}

// 	totalLikes := int64(0)
// 	totalViews := int64(0)
// 	totalComments := int64(0)

// 	for index, reel := range req.Reels.Items {
// 		parts := strings.Split(reel.URL, "/")
// 		id := "reelindex" + strconv.Itoa(index)
// 		if len(parts) >= 2 {
// 			id = parts[len(parts)-2]
// 		}
// 		data.LatestReels = append(data.LatestReels, trendlybq.SinglePost{
// 			ID:             id,
// 			DisplayURL:     reel.Thumbnail,
// 			URL:            reel.URL,
// 			Caption:        "",
// 			IsPinned:       reel.Pinned,
// 			VideoViewCount: bigquery.NullInt64{Int64: 0, Valid: reel.Views.Value != nil && *reel.Views.Value > 0},
// 			LikesCount:     bigquery.NullInt64{Int64: 0, Valid: reel.Overlays.Likes.Value != nil && *reel.Overlays.Likes.Value > 0},
// 			CommentsCount:  bigquery.NullInt64{Int64: 0, Valid: reel.Overlays.Comments.Value != nil && *reel.Overlays.Comments.Value > 0},
// 		})

// 		var views, likes, comments int64

// 		if reel.Views.Value != nil {
// 			views = *reel.Views.Value
// 			if views > 0 {
// 				data.LatestReels[len(data.LatestReels)-1].VideoViewCount.Int64 = views
// 				viewsList = append(viewsList, views)
// 			}
// 			if !reel.Pinned {
// 				data.ViewsCount += views
// 			}
// 		}
// 		if reel.Overlays.Likes.Value != nil {
// 			likes = *reel.Overlays.Likes.Value
// 			if likes > 0 {
// 				data.LatestReels[len(data.LatestReels)-1].LikesCount.Int64 = likes
// 				likesList = append(likesList, likes)
// 			}
// 			if !reel.Pinned {
// 				data.EngagementCount += likes
// 			}
// 		}
// 		if reel.Overlays.Comments.Value != nil {
// 			comments = *reel.Overlays.Comments.Value
// 			if comments > 0 {
// 				data.LatestReels[len(data.LatestReels)-1].CommentsCount.Int64 = comments
// 				commentsList = append(commentsList, comments)
// 			}
// 			if !reel.Pinned {
// 				data.EngagementCount += comments
// 			}
// 		}

// 		// Per-reel engagement rate for median calculation (treat missing likes/comments as 0)
// 		if views > 0 {
// 			eRates = append(eRates, float32(likes+comments)*100/float32(views))
// 		}

// 		totalLikes += likes
// 		totalComments += comments
// 		totalViews += views
// 	}

// 	// Use median for the three "averages"
// 	data.AverageViews = medianInt64(viewsList)
// 	data.AverageLikes = medianInt64(likesList)
// 	data.AverageComments = medianInt64(commentsList)

// 	// Engagement rate as median of per-reel rates
// 	data.EngagementRate = medianFloat32(eRates)
// }

func CheckUsername(c *gin.Context) {
	username := c.Query("username")
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Username is required"})
		return
	}

	user := trendlybq.SocialsN8N{}
	err := user.GetInstagramFromFirestore(username)

	c.JSON(http.StatusAccepted, gin.H{"username": username, "exists": err == nil, "lastUpdate": user.LastUpdateTime})
}
