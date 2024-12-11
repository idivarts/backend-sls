package trendlyapis

import (
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/pkg/messenger"
)

func FacebookLogin(c *gin.Context) {
	var person messenger.FacebookLoginRequest

	if err := c.ShouldBindJSON(&person); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// sources, err := models.GetSourcesByUserId(organizationID, person.ID)
	// if err != nil {
	// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 	return
	// }
	// for i := 0; i < len(sources); i++ {
	// 	sources[i].Status = 0
	// 	_, err = sources[i].Insert()
	// 	if err != nil {
	// 		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	// 		return
	// 	}
	// }

	for _, v := range person.Accounts.Data {
		lRes, err := messenger.GetLongLivedAccessToken(v.AccessToken)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		log.Println("Token", v.AccessToken, lRes.AccessToken)

		// var instagram *models.InstagramObject = nil
		if v.InstagramBusinessAccount.ID != "" {
			// inst, err := messenger.GetInstagram(v.InstagramBusinessAccount.ID, lRes.AccessToken)
			// if err != nil {
			// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			// 	return
			// }
			// instaPage := models.Source{
			// 	OrganizationID:     organizationID,
			// 	ID:                 inst.ID,
			// 	Name:               inst.Name,
			// 	UserID:             person.ID,
			// 	OwnerName:          person.Name,
			// 	IsWebhookConnected: false,
			// 	Status:             1,
			// 	UserName:           &inst.Username,
			// 	Bio:                &inst.Biography,
			// 	SourceType:         models.Instagram,
			// 	ConnectedID:        &v.ID,
			// }
			// _, err = instaPage.Insert()
			// if err != nil {
			// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			// 	return
			// }
			// instaPPage := models.SourcePrivate{
			// 	AccessToken: &lRes.AccessToken,
			// }
			// _, err = instaPPage.Set(organizationID, inst.ID)
			// if err != nil {
			// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			// 	return
			// }
			// log.Println("Instagram Saved Accesstoken", instaPPage)
		}

		// fb, err := messenger.GetFacebook(lRes.AccessToken)
		// if err != nil {
		// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		// 	return
		// }

		// fbPage := models.Source{
		// 	OrganizationID:     organizationID,
		// 	ID:                 v.ID,
		// 	Name:               v.Name,
		// 	UserID:             person.ID,
		// 	OwnerName:          person.Name,
		// 	IsWebhookConnected: false,
		// 	Status:             1,
		// 	UserName:           nil,
		// 	Bio:                nil,
		// 	SourceType:         models.Facebook,
		// 	ConnectedID:        &v.InstagramBusinessAccount.ID,
		// 	// AccessToken:        &lRes.AccessToken,
		// }
		// _, err = fbPage.Insert()
		// if err != nil {
		// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		// 	return
		// }
		// fbPPage := models.SourcePrivate{
		// 	AccessToken: &lRes.AccessToken,
		// }
		// _, err = fbPPage.Set(organizationID, v.ID)
		// if err != nil {
		// 	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		// 	return
		// }
		// log.Println("FB Saved Accesstoken", fbPPage)
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully parsed JSON", "user": person})
}
