package messenger

type FacebookLoginRequest struct {
	Name                     string `json:"name"`
	ID                       string `json:"id" binding:"required"`
	UserID                   string `json:"userID"`
	ExpiresIn                int    `json:"expiresIn"`
	AccessToken              string `json:"accessToken" binding:"required"`
	SignedRequest            string `json:"signedRequest"`
	GraphDomain              string `json:"graphDomain"`
	DataAccessExpirationTime int    `json:"data_access_expiration_time"`

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
