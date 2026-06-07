package trendlyCollabs

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	ai_collaboration "github.com/idivarts/backend-sls/internal/openai/collaboration"
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/mytime"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
)

// nzString returns "NA" if s is empty after trimming; otherwise returns s.
func nzString(s string) string {
	if strings.TrimSpace(s) == "" {
		return "NA"
	}
	return s
}

// toString converts any value to a compact JSON or string.
// For zero/empty values it returns "NA".
func toString(v interface{}) string {
	if v == nil {
		return "NA"
	}
	switch t := v.(type) {
	case string:
		return nzString(t)
	case *string:
		if t == nil {
			return "NA"
		}
		return nzString(*t)
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		// Render numbers/bools; treat explicit zero as "NA" for numeric types.
		switch n := v.(type) {
		case int:
			if n == 0 {
				return "NA"
			}
		case int64:
			if n == 0 {
				return "NA"
			}
		case float64:
			if n == 0 {
				return "NA"
			}
		case float32:
			if n == 0 {
				return "NA"
			}
		case uint, uint8, uint16, uint32, uint64:
			// leave as-is; zero can be meaningful but align with "NA" fallback
			// convert zero to "NA" for consistency
			if fmt.Sprintf("%v", n) == "0" {
				return "NA"
			}
		}
		return fmt.Sprintf("%v", v)
	default:
		b, err := json.Marshal(v)
		if err != nil || len(b) == 0 || string(b) == "null" || string(b) == "[]" || string(b) == "{}" {
			return "NA"
		}
		return string(b)
	}
}

func formatBrandDetails(brand *trendlymodels.Brand) string {
	if brand == nil {
		return ""
	}

	// Keep only context that can improve generation quality.
	brandContext := map[string]interface{}{
		"name": brand.Name,
	}
	if brand.Profile != nil {
		if brand.Profile.About != nil && strings.TrimSpace(*brand.Profile.About) != "" {
			brandContext["about"] = *brand.Profile.About
		}
		if len(brand.Profile.Industries) > 0 {
			brandContext["industries"] = brand.Profile.Industries
		}
		if brand.Profile.Website != nil && strings.TrimSpace(*brand.Profile.Website) != "" {
			brandContext["website"] = *brand.Profile.Website
		}
	}
	if brand.DiscoverPreferences != nil {
		brandContext["preferences"] = brand.DiscoverPreferences
	}
	return toString(brandContext)
}

func TestEvaluateCollab(collab *trendlymodels.Collaboration) (bool, *trendlymodels.DiscoverPreferences) {
	return evaluateCollab(collab, nil)
}

func evaluateCollab(collab *trendlymodels.Collaboration, brand *trendlymodels.Brand) (bool, *trendlymodels.DiscoverPreferences) {
	budget := "Barter"
	if collab.Budget != nil && collab.Budget.Max != nil && *collab.Budget.Max != 0 {
		budget = toString(collab.Budget)
	}

	brandDetails := formatBrandDetails(brand)

	valid, filters, err := ai_collaboration.EvaluateCollaboration(ai_collaboration.CollabEvaluationInput{
		CollaborationName:        toString(collab.Name),
		CollaborationDescription: toString(collab.Description),
		Budget:                   budget,
		Location:                 toString(collab.Location),
		Questions:                toString(collab.QuestionsToInfluencers),
		Links:                    toString(collab.ExternalLinks),
		BrandDetails:             brandDetails,
	})
	if err != nil {
		log.Println("Error evaluating collab:", err.Error())
		return false, nil
	}
	log.Println("Evaluation Response:", valid)
	if valid {
		return true, filters
	}
	return false, nil
}
func PostCollaboration(c *gin.Context) {
	collabId := c.Param(("collabId"))
	// updating := (c.Query("update") != "")

	collab := &trendlymodels.Collaboration{}
	err := collab.Get(collabId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant fetch Collab"})
		return
	}

	if _, ok := middlewares.RequireFeaturePrivilege(c, collab.BrandID, trendlymodels.FeatureInfluencerMarketing, trendlymodels.PrivInfluencerManage); !ok {
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant fetch Brand"})
		return
	}

	if brand.Credits.Collaboration <= 0 {
		collab.Status = "deleted"
	}

	valid, filters := evaluateCollab(collab, &brand)

	throwError := false

	if collab.Status == "active" && !valid {
		collab.Status = "draft"
		throwError = true
	}

	if valid {
		collab.Preferences = filters
	}

	if brand.PostedCollaborations == nil {
		brand.PostedCollaborations = []string{}
	}

	if collab.Status == "active" && !myutil.Includes(brand.PostedCollaborations, collabId) {
		brand.Credits.Collaboration -= 1
		brand.PostedCollaborations = append(brand.PostedCollaborations, collabId)
	}
	collab.IsLive = !myutil.IsDevEnvironment()

	_, err = collab.Insert(collabId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error Inserting Collab"})
		return
	}

	_, err = brand.Insert(collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error Saving Brand"})
		return
	}

	if throwError {
		// Send email to manager
		manager := trendlymodels.Manager{}
		err = manager.Get(collab.ManagerID)
		if err == nil {
			data := map[string]interface{}{
				"ManagerName": manager.Name,
				"CollabTitle": collab.Name,
				"DraftLink":   fmt.Sprintf("%s/collaboration-details/%s", constants.TRENDLY_BRANDS_FE, collabId),
			}
			_ = myemail.SendCustomHTMLEmail(manager.Email, templates.CollaborationTakedown, templates.SubjectCollaborationTakedown, data)
		}

		c.JSON(http.StatusBadRequest, gin.H{"error": "collaboration-payload-not-approved", "message": "The Collaboration posted was not put live as it did not meet our guidelines. Please review the collaboration details and make necessary changes before reposting.",
			"collabId": collabId, "discoverFilters": filters, "updatedStatus": collab.Status}) //, "updating": updating
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Collaboration Started", "collabId": collabId, "discoverFilters": filters, "updatedStatus": collab.Status}) //, "updating": updating
}

func CreateCollaborationWithPrompt(c *gin.Context) {
	var body struct {
		Prompt  string `json:"prompt" binding:"required"`
		BrandID string `json:"brandId"`
	}
	if err := c.BindJSON(&body); err != nil || body.Prompt == "" {
		c.AbortWithStatusJSON(400, gin.H{"error": "missing prompt"})
		return
	}

	if body.BrandID != "" {
		if _, ok := middlewares.RequireFeaturePrivilege(c, body.BrandID, trendlymodels.FeatureInfluencerMarketing, trendlymodels.PrivInfluencerManage); !ok {
			return
		}
	}

	// Temporary adjustments as Image search is taking a lot of time
	disableWebsiteSearch := true

	brandDetails := ""
	brand := &trendlymodels.Brand{}
	hasWebsite := false
	if body.BrandID != "" {
		err := brand.Get(body.BrandID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant fetch Brand"})
			return
		}
		hasWebsite = brand.Profile != nil && brand.Profile.Website != nil && *brand.Profile.Website != ""
		brandDetails = formatBrandDetails(brand)
	}

	if disableWebsiteSearch {
		hasWebsite = false
	}

	collaboratioDraft, err := ai_collaboration.CollaborationDraft{}.GetResults(body.Prompt, brandDetails, hasWebsite)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error generating collaboration"})
		return
	}

	if disableWebsiteSearch {
		if brand.Image != nil {
			collaboratioDraft.RelevantImages = []string{*brand.Image}
		}
		if brand.Profile != nil && brand.Profile.Website != nil {
			collaboratioDraft.ExternalLinks = []struct {
				Name string `json:"name"`
				Link string `json:"link"`
			}{
				{
					Name: "Website",
					Link: *brand.Profile.Website,
				},
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"error":         false,
		"collaboration": collaboratioDraft,
	})
}

func RequestToStartContract(c *gin.Context) {
	contractId := c.Param(("contractId"))

	contract := trendlymodels.Contract{}
	err := contract.Get(contractId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching contract"})
		return
	}

	collab := trendlymodels.Collaboration{}
	err = collab.Get(contract.CollaborationID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching collaboration"})
		return
	}

	brand := trendlymodels.Brand{}
	err = brand.Get(contract.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching Brand"})
		return
	}

	user := trendlymodels.User{}
	err = user.Get(contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error fetching user"})
		return
	}

	// Send Push Notification
	notif := &trendlymodels.Notification{
		Title:       fmt.Sprintf("Please start the contract : %s", collab.Name),
		Description: fmt.Sprintf("%s has asked to start the contract. Please review that.", user.Name),
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "contract-start-request",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Email notification

	// 	<!--
	//   Dynamic Variables:
	// {{.BrandMemberName}}    => Name of the brand team member receiving the email
	// {{.InfluencerName}}     => Name of the influencer who sent the poke
	// {{.CollabTitle}}        => Title of the collaboration
	// {{.PokeTime}}           => Timestamp when the poke was sent
	// {{.StartLink}}          => Link for the brand to start the collaboration
	// -->
	if len(emails) > 0 {
		data := map[string]interface{}{
			"BrandMemberName": brand.Name,
			"InfluencerName":  user.Name,
			"CollabTitle":     collab.Name,
			"PokeTime":        mytime.FormatPrettyIST(time.Now()),
			"StartLink":       fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationStartRequested, templates.SubjectStartCollabReminderToBrand, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Send Stream Notification
	err = streamchat.SendSystemMessage(contract.StreamChannelID, fmt.Sprintf("To %s\nPlease start the contract if everything is discussed. %s is waiting on you to get started with his work", brand.Name, user.Name))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Error"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Requested to start contract"})
}
