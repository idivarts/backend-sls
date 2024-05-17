package businessapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type PageUnit struct {
	Id          string
	Name        string
	UserName    string
	IsInstagram bool
}
type PagesGetResponse struct {
	Start      int
	Count      int
	MyPages    []PageUnit
	OtherPages []PageUnit
}

func GetPages(c *gin.Context) {
	pagesResp := PagesGetResponse{}
	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "pages": pagesResp})
}
