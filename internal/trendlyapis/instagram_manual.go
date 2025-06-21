package trendlyapis

import (
	"net/http"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

type IInstagramManualReq struct {
	Handle         string `json:"handle"`
	ProfileImage   string `json:"profileImage"`
	DashboardImage string `json:"dashboardImage"`

	SocialID            *string `json:"socialId"`
	FollowerRange       string  `json:"followerRange"`
	MonthlyViews        string  `json:"monthlyViews"`
	MonthlyInteractions string  `json:"monthlyInteractions"`
}

func ConnectInstagramManual(ctx *gin.Context) {
	var req IInstagramManualReq
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	userId, b := middlewares.GetUserId(ctx)
	if !b {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}

	// Generate a random social ID
	socialId := uuid.NewString()
	if req.SocialID != nil {
		socialId = *req.SocialID
	}

	user := trendlymodels.User{}
	err := user.Get(userId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Add the socials for that user
	social := trendlymodels.Socials{
		ID:          socialId,
		Name:        req.Handle,
		Image:       "",
		IsInstagram: true,
		ConnectedID: nil,
		UserID:      userId,
		OwnerName:   user.Name,
		InstaProfile: &messenger.InstagramProfile{
			InstagramBriefProfile: messenger.InstagramBriefProfile{
				Name:      req.Handle,
				Username:  req.Handle,
				Biography: "",
				ID:        socialId,
			},
			ProfilePictureURL: "",
			FollowersCount:    0,
			FollowsCount:      0,
			MediaCount:        0,
			Website:           "",
			ApproxMetrics: messenger.InstaApproxMetrics{
				Views:        req.MonthlyViews,
				Interactions: req.MonthlyInteractions,
				Followers:    req.FollowerRange,
			},
		},
		FBProfile: nil,
		SocialScreenShots: []string{
			req.ProfileImage,
			req.DashboardImage,
		},
	}
	_, err = social.Insert(userId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	// Save the access token in the firestore database
	socialPrivate := trendlymodels.SocialsPrivate{
		AccessToken: aws.String(""),
		GraphType:   trendlymodels.InstagramManual,
	}
	_, err = socialPrivate.Set(userId, socialId)
	if err != nil {
		ctx.JSON(400, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Successfully social added", "social": social})

}
