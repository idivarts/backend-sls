package businessapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type PageUnit struct {
	Id          string `json:"id"`
	Name        string `json:"name"`
	UserName    string `json:"userName"`
	OwnerName   string `json:"ownerName"`
	IsInstagram bool   `json:"isInstagram"`
}

type PagesGetResponse struct {
	Start      int        `json:"start"`
	Count      int        `json:"count"`
	MyPages    []PageUnit `json:"myPages"`
	OtherPages []PageUnit `json:"otherPages"`
}

func GetPages(c *gin.Context) {
	pagesResp := PagesGetResponse{}

	// models.Page{}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "pages": pagesResp})
}
