package businessapis

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type PageUnit struct {
	PageId   string
	PageName string
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
