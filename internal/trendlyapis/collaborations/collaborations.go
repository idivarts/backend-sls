package trendlyCollabs

import (
	"context"
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
	"github.com/idivarts/backend-sls/pkg/myemail"
	"github.com/idivarts/backend-sls/pkg/myopenai"
	"github.com/idivarts/backend-sls/pkg/mytime"
	"github.com/idivarts/backend-sls/pkg/myutil"
	"github.com/idivarts/backend-sls/pkg/streamchat"
	"github.com/idivarts/backend-sls/templates"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
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

func TestEvaluateCollab(collab *trendlymodels.Collaboration) (bool, *trendlymodels.DiscoverPreferences) {
	return evaluateCollab(collab, nil)
}
func evaluateCollab(collab *trendlymodels.Collaboration, brand *trendlymodels.Brand) (bool, *trendlymodels.DiscoverPreferences) {
	// {
	// 	"id": "pmpt_690a4bed81408190affad862efc917dd00fc63fdff223ab2",
	// 	"version": "1",
	// 	"variables": {
	// 	  "collaboration_name": "example collaboration_name",
	// 	  "collaboration_description": "example collaboration_description"
	// 	}
	//   }

	budget := "Barter"
	if collab.Budget != nil && *collab.Budget.Max != 0 {
		budget = toString(collab.Budget)
	}

	brandDetails := "Trendly"
	if brand != nil {
		brandDetails = toString(*brand)
	}

	// Build prompt variables with "NA" fallbacks when fields are empty/missing.
	vars := map[string]responses.ResponsePromptVariableUnionParam{
		"collaboration_name":        {OfString: openai.String(toString(collab.Name))},
		"collaboration_description": {OfString: openai.String(toString(collab.Description))},
		"budget":                    {OfString: openai.String(budget)},
		"location":                  {OfString: openai.String(toString(collab.Location))},
		"questions":                 {OfString: openai.String(toString(collab.QuestionsToInfluencers))},
		"links":                     {OfString: openai.String(toString(collab.ExternalLinks))},
		"brand_details":             {OfString: openai.String(brandDetails)},
	}

	response, err := myopenai.Client.Responses.New(context.Background(), responses.ResponseNewParams{
		Prompt: responses.ResponsePromptParam{
			ID:        "pmpt_690a4bed81408190affad862efc917dd00fc63fdff223ab2",
			Variables: vars,
		},
	})
	if err != nil {
		log.Println("Error evaluating collab:", err.Error())
		return false, nil
	}
	jsonStr := response.JSON.Output.Raw()
	mMap := []map[string]interface{}{}
	err = json.Unmarshal([]byte(jsonStr), &mMap)
	if err != nil {
		log.Println("Error parsing evaluation response:", err.Error())
		return false, nil
	}

	responseStr := mMap[0]["content"].([]interface{})[0].(map[string]interface{})["text"].(string)
	rMap := map[string]interface{}{}
	err = json.Unmarshal([]byte(responseStr), &rMap)
	if err != nil {
		log.Println("Error parsing evaluation content:", err.Error())
		return false, nil
	}

	valid := rMap["validCollaboration"].(bool)
	log.Println("Evaluation Response:", valid)
	if valid {
		filtersMap := rMap["filters"].(map[string]interface{})
		filters := &trendlymodels.DiscoverPreferences{}
		b, err := json.Marshal(filtersMap)
		if err != nil {
			log.Println("Error marshalling filters:", err.Error())
			return false, nil
		}
		err = json.Unmarshal(b, filters)
		if err != nil {
			log.Println("Error unmarshalling filters:", err.Error())
			return false, nil
		}
		return true, filters
	}
	return false, nil
}
func PostCollaboration(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType == "user" {
		requestToStart(c)
		return
	}
	collabId := c.Param(("collabId"))
	// updating := (c.Query("update") != "")

	collab := &trendlymodels.Collaboration{}
	err := collab.Get(collabId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Cant fetch Collab"})
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "collaboration-payload-not-approved", "message": "The Collaboration posted was not put live as it did not meet our guidelines. Please review the collaboration details and make necessary changes before reposting.",
			"collabId": collabId, "discoverFilters": filters, "updatedStatus": collab.Status}) //, "updating": updating
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Collaboration Started", "collabId": collabId, "discoverFilters": filters, "updatedStatus": collab.Status}) //, "updating": updating
}

func CreateCollaborationWithPrompt(c *gin.Context) {
	var body struct {
		Prompt string `json:"prompt"`
		Model  string `json:"model"`
	}
	if err := c.BindJSON(&body); err != nil || body.Prompt == "" {
		c.AbortWithStatusJSON(400, gin.H{"error": "missing prompt"})
		return
	}
	model := body.Model
	if model == "" {
		model = "gpt-4o" // pick any streaming capable chat model
	}

	// Prepare SSE headers
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.AbortWithStatusJSON(500, gin.H{"error": "streaming not supported"})
		return
	}

	// Start the OpenAI stream
	ctx := context.Background()
	stream := myopenai.Client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(body.Prompt),
		},
		Model: openai.ChatModel(model),
	})
	defer stream.Close()

	// Optional helper to assemble partial deltas
	acc := openai.ChatCompletionAccumulator{}

	for stream.Next() {
		chunk := stream.Current()
		acc.AddChunk(chunk)

		// Send the raw delta content as SSE
		if len(chunk.Choices) > 0 {
			delta := chunk.Choices[0].Delta.Content
			if delta != "" {
				// Standard SSE line
				c.Writer.Write([]byte("data: " + delta + "\n\n"))
				flusher.Flush()
			}
		}

		// You can also detect end of a message or a tool call:
		// if content, ok := acc.JustFinishedContent(); ok { ... }
		// if tool, ok := acc.JustFinishedToolCall(); ok { ... }
	}

	if err := stream.Err(); err != nil {
		// send a final SSE error event then end
		c.Writer.Write([]byte("event: error\ndata: " + err.Error() + "\n\n"))
		flusher.Flush()
		return
	}

	// SSE end marker is optional. Many clients just stop on socket close.
	c.Writer.Write([]byte("data: [DONE]\n\n"))
	flusher.Flush()
}

// Starting a collab | Request to start
func StartContract(c *gin.Context) {
	userType := middlewares.GetUserType(c)
	if userType == "user" {
		requestToStart(c)
		return
	}
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
	if brand.Credits.Contract <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "insufficient-credits", "message": "Insufficient Credit to Start Contract"})
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
		Title:       fmt.Sprintf("The contract is started : %s", collab.Name),
		Description: "You can now find the details on the contract's menu",
		IsRead:      false,
		Data: &trendlymodels.NotificationData{
			CollaborationID: &contract.CollaborationID,
			UserID:          &contract.UserID,
			GroupID:         &contractId,
		},
		TimeStamp: time.Now().UnixMilli(),
		Type:      "contract-started",
	}
	_, emails, err := notif.Insert(trendlymodels.BRAND_COLLECTION, collab.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	_, _, err = notif.Insert(trendlymodels.USER_COLLECTION, contract.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send Email notification

	// 	<!--
	//   Dynamic Variables:
	// {{.RecipientName}}     => Name of the recipient (Brand or Influencer)
	// {{.CollabTitle}}       => Title of the collaboration
	// {{.StartDate}}         => Date when the collaboration was started
	// {{.ContractLink}}      => Link to view the created contract
	// -->

	if user.Email != nil {
		data := map[string]interface{}{
			"RecipientName": user.Name,
			"CollabTitle":   collab.Name,
			"StartDate":     mytime.FormatPrettyIST(time.Now()),
			"ContractLink":  fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_CREATORS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmail(*user.Email, templates.CollaborationStarted, templates.SubjectCollaborationStarted, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	if len(emails) > 0 {
		data := map[string]interface{}{
			"RecipientName": brand.Name,
			"CollabTitle":   collab.Name,
			"StartDate":     mytime.FormatPrettyIST(time.Now()),
			"ContractLink":  fmt.Sprintf("%s/contract-details/%s", constants.TRENDLY_BRANDS_FE, contractId),
		}
		err = myemail.SendCustomHTMLEmailToMultipleRecipients(emails, templates.CollaborationStarted, templates.SubjectCollaborationStarted, data)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}

	// Send Stream Notification
	err = streamchat.SendSystemMessage(contract.StreamChannelID, "Congratulations!! The contract has been started!\nYou can find the contract details on the contract menu")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Stream Error"})
		return
	}

	brand.Credits.Contract -= 1
	_, err = brand.Insert(contract.BrandID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Error updating brand Credits"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Successfully Notified for starting contract"})
}

func requestToStart(c *gin.Context) {
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
