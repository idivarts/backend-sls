package messenger

type FacebookLoginRequest struct {
	Name        string `json:"name"`
	ID          string `json:"id" binding:"required"`
	AccessToken string `json:"accessToken" binding:"required"`

	Accounts struct {
		Data []struct {
			AccessToken              string `json:"access_token"`
			ID                       string `json:"id"`
			InstagramBusinessAccount struct {
				ID string `json:"id"`
			} `json:"instagram_business_account"`
			Name string `json:"name"`
		} `json:"data"`
		Paging struct {
			Cursors struct {
				Before string `json:"before"`
				After  string `json:"after"`
			} `json:"cursors"`
		} `json:"paging"`
	} `json:"accounts"`
}

// func main() {
// 	r := gin.Default()
// 	r.POST("/data", func(c *gin.Context) {
// 		var data Data
// 		if err := c.ShouldBindJSON(&data); err != nil {
// 			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
// 			return
// 		}

// 		c.JSON(http.StatusOK, gin.H{"data": data})
// 	})

// 	r.Run(":8080")
// }
