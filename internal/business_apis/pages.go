package businessapis

import (
	"net/http"

	"github.com/TrendsHub/th-backend/internal/models"
	"github.com/gin-gonic/gin"
)

type PageUnit struct {
	Id                 string `json:"id"`
	Name               string `json:"name"`
	UserName           string `json:"userName"`
	OwnerName          string `json:"ownerName"`
	IsInstagram        bool   `json:"isInstagram"`
	IsWebhookConnected bool   `json:"isWebHookConnected"`
}

type PagesGetResponse struct {
	Start      int        `json:"start"`
	Count      int        `json:"count"`
	MyPages    []PageUnit `json:"myPages"`
	OtherPages []PageUnit `json:"otherPages"`
}

type GetPageRequest struct {
	UserID string `form:"userId" binding:"required"`
}

func GetPages(c *gin.Context) {
	var req GetPageRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	pagesResp := PagesGetResponse{
		Start:      0,
		Count:      0,
		MyPages:    []PageUnit{},
		OtherPages: []PageUnit{},
	}

	// models.Page{}
	pages, err := models.FetchAllPages()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	for _, v := range pages {
		if v.Status != 1 {
			continue
		}
		if v.UserID == req.UserID {
			pagesResp.MyPages = append(pagesResp.MyPages, PageUnit{
				Id:                 v.PageID,
				Name:               v.Name,
				UserName:           v.UserName,
				OwnerName:          v.OwnerName,
				IsInstagram:        v.IsInstagram,
				IsWebhookConnected: v.IsWebhookConnected,
			})
		} else {
			pagesResp.OtherPages = append(pagesResp.OtherPages, PageUnit{
				Id:                 v.PageID,
				Name:               v.Name,
				UserName:           v.UserName,
				OwnerName:          v.OwnerName,
				IsInstagram:        v.IsInstagram,
				IsWebhookConnected: v.IsWebhookConnected,
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "pages": pagesResp})
}
