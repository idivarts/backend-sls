package trendlyapis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"
	"net/http"
	"strings"
	"time"
	"unicode"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/hubspot"
	"github.com/idivarts/backend-sls/pkg/streamchat"
)

func ChatAuth(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not found"})
		return
	}

	isManager := false
	if middlewares.GetUserType(c) == "manager" {
		isManager = true
	}

	userObject := middlewares.GetUserObject(c)

	name, _ := userObject["name"].(string)
	profileImage, _ := userObject["profileImage"].(string)
	// Upsert user to the stream chat
	_, err := streamchat.CreateOrUpdateUser(streamchat.User{
		ID:        userId,
		Name:      name,
		Image:     profileImage,
		IsManager: isManager,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating/updating user in chat", "error": err.Error()})
		return
	}
	if !isManager {
		user := trendlymodels.User{}
		err = user.Get(userId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Error in getting user", "error": err.Error()})
			return
		}
		if user.CreationTime == nil {
			fUser, err := fauth.Client.GetUser(context.Background(), userId)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Error in getting user", "error": err.Error()})
				return
			}
			user.CreationTime = aws.Int64(fUser.UserMetadata.CreationTimestamp)
		}
		user.LastUseTime = aws.Int64(time.Now().UnixMilli())
		if user.PrimarySocial != nil && *user.PrimarySocial != "" {
			// Get the user's primary social media account
			primarySocial := userObject["primarySocial"].(string)
			social := trendlymodels.Socials{}
			err := social.Get(userId, primarySocial)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"message": "Error in getting primary social media account", "error": err.Error()})
				return
			}

			if social.FBProfile != nil {
				user.Backend = &trendlymodels.BackendData{
					Followers:  &social.FBProfile.FollowersCount,
					Reach:      aws.Int(0),
					Engagement: aws.Int(0),
					Rating:     aws.Int(5),
				}
			}
			if social.InstaProfile != nil {
				user.Backend = &trendlymodels.BackendData{
					Followers:  &social.InstaProfile.FollowersCount,
					Reach:      aws.Int(0),
					Engagement: aws.Int(0),
					Rating:     aws.Int(5),
				}
			}

		} else if userObject["backend"] == nil {
			user.Backend = &trendlymodels.BackendData{
				Followers:  aws.Int(0),
				Reach:      aws.Int(0),
				Engagement: aws.Int(0),
				Rating:     aws.Int(5),
			}
		}
		_, err = user.Insert(userId)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "Error in updating user", "error": err.Error()})
			return
		}
	}

	updateHubSpot(isManager, userObject)

	c.JSON(http.StatusOK, gin.H{"message": "Chat Authentication successful"})
}

func ChatConnect(c *gin.Context) {
	userId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not found"})
		return
	}

	userObject := middlewares.GetUserObject(c)
	isManager := false
	if middlewares.GetUserType(c) == "manager" {
		isManager = true
	}

	updateHubSpot(isManager, userObject)

	if userObject["isChatConnected"] != true {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Chat not connected"})
		return
	}

	name, _ := userObject["name"].(string)
	profileImage, _ := userObject["profileImage"].(string)
	// Upsert user to the stream chat
	_, err := streamchat.CreateOrUpdateUser(streamchat.User{
		ID:        userId,
		Name:      name,
		Image:     profileImage,
		IsManager: isManager,
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating/updating user in chat", "error": err.Error()})
		return
	}

	token, err := streamchat.CreateToken(userId)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating token", "error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Chat Connected", "token": token})
}

func updateHubSpot(isManager bool, userObject map[string]interface{}) error {
	jsonBody, err := json.Marshal(userObject)
	if err != nil {
		return err
	}
	if isManager {
		user := trendlymodels.User{}
		err = json.Unmarshal(jsonBody, &user)
		if err != nil {
			return err
		}

		if user.Email != nil && *user.Email != "" {
			phone := ""
			pCent := 0
			if user.PhoneNumber != nil {
				phone = *user.PhoneNumber
			}
			if user.Profile != nil {
				pCent = *user.Profile.CompletionPercentage
			}
			contacts := []hubspot.ContactDetails{{
				Email:             *user.Email,
				Name:              user.Name,
				Phone:             phone,
				IsManager:         false,
				ProfileCompletion: pCent,
				LastActivityTime:  aws.Int64(time.Now().UnixMilli()),
				CreationTime:      user.CreationTime,
			}}
			err := hubspot.CreateOrUpdateContacts(contacts)
			if err != nil {
				return err
			}
		}
	} else {
		manager := trendlymodels.Manager{}
		err = json.Unmarshal(jsonBody, &manager)
		if err != nil {
			return err
		}

		contacts := []hubspot.ContactDetails{{
			Email:            manager.Email,
			Name:             manager.Name,
			Phone:            manager.PhoneNumber,
			IsManager:        true,
			CompanyName:      "", // Currenly its difficult to fetch the company name
			LastActivityTime: aws.Int64(time.Now().UnixMilli()),
		}}

		err := hubspot.CreateOrUpdateContacts(contacts)
		if err != nil {
			return err
		}
	}
	return nil
}

// GenerateKey converts a string to a valid key and appends a random 5-digit number,
// ensuring the total length does not exceed 150 characters.
func GenerateKey(namePtr *string) string {
	if namePtr == nil {
		return ""
	}

	// Replace spaces with dashes and convert to lowercase
	name := strings.ToLower(strings.ReplaceAll(*namePtr, " ", "-"))

	// Remove invalid characters (keep only lowercase letters and dashes)
	validKey := strings.Builder{}
	for _, char := range name {
		if unicode.IsLower(char) || char == '-' {
			validKey.WriteRune(char)
		}
	}

	// Generate a random 5-digit number
	randomNumber := rand.Intn(90000) + 10000 // Ensures a 5-digit number
	randomSuffix := fmt.Sprintf("-%d", randomNumber)

	// Ensure the key does not exceed 150 characters
	// Subtract the length of the random suffix to determine max length for validKey
	maxKeyLength := 150 - len(randomSuffix)
	key := validKey.String()

	if len(key) > maxKeyLength {
		key = key[:maxKeyLength] // Truncate to fit within the limit
	}

	// Append the random number to the key
	return key + randomSuffix
}

// ICreateChannel includes both json binding
type ICreateChannel struct {
	Name            *string `json:"name,omitempty"`
	UserID          string  `json:"userId" binding:"required"`
	CollaborationID string  `json:"collaborationId" binding:"required"`
}

func ChatChannel(c *gin.Context) {
	var req ICreateChannel
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Invalid request", "error": err.Error()})
		return
	}
	if middlewares.GetUserType(c) != "manager" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Only Managers can create new channels"})
		return
	}
	managerId, b := middlewares.GetUserId(c)
	if !b {
		c.JSON(http.StatusBadRequest, gin.H{"message": "User not found"})
		return
	}

	res, err := CreateChannel(managerId, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Channel Created", "channel": res.Channel, "contractId": res.Contract.StreamChannelID, "contract": res.Contract})
}

type ChannelReturn struct {
	Channel  stream_chat.Channel
	Contract trendlymodels.Contract
}

func CreateChannel(managerId string, req ICreateChannel) (*ChannelReturn, error) {
	if req.UserID == managerId {
		return nil, errors.New("user-matches-manager-id")
	}

	collabObj, err := firestoredb.Client.Collection("collaborations").Doc(req.CollaborationID).Get(context.Background())
	if err != nil {
		return nil, err
	}
	collabMap := collabObj.Data()

	_, err = firestoredb.Client.Collection("contracts").Where("collaborationId", "==", req.CollaborationID).Where("userId", "==", req.UserID).Documents(context.Background()).Next()
	if err == nil {
		return nil, err
	}

	userIDs := []string{managerId, req.UserID}

	// Before creating channel make sure all users has isChatConnected true
	for _, id := range userIDs {

		var uObj map[string]interface{}
		isManager := false
		user, err := firestoredb.Client.Collection("users").Doc(id).Get(context.Background())
		if err != nil {
			manager, err2 := firestoredb.Client.Collection("managers").Doc(id).Get(context.Background())
			if err2 != nil {
				// c.JSON(http.StatusBadRequest, gin.H{"message": "Error in getting user and/or manager", "error1": err.Error(), "error2": err2.Error()})
				return nil, err2
			}
			uObj = manager.Data()
			isManager = true
		} else {
			uObj = user.Data()
		}

		name, _ := uObj["name"].(string)
		profileImage, _ := uObj["profileImage"].(string)

		if uObj["isChatConnected"] != true {
			_, err := streamchat.CreateOrUpdateUser(streamchat.User{
				ID:        id,
				Name:      name,
				Image:     profileImage,
				IsManager: isManager,
			})
			if err != nil {
				// c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating/updating user in chat", "error": err.Error()})
				return nil, err
			}
			uObj["isChatConnected"] = true
			if isManager {
				_, err := firestoredb.Client.Collection("managers").Doc(id).Set(context.Background(), uObj)
				if err != nil {
					// c.JSON(http.StatusBadRequest, gin.H{"message": "Error in updating manager", "error": err.Error()})
					return nil, err
				}
			} else {
				_, err := firestoredb.Client.Collection("users").Doc(id).Set(context.Background(), uObj)
				if err != nil {
					// c.JSON(http.StatusBadRequest, gin.H{"message": "Error in updating user", "error": err.Error()})
					return nil, err
				}
			}
		}
	}

	contractId := GenerateKey(req.Name)
	res, err := streamchat.Client.CreateChannel(context.Background(), "messaging", contractId, managerId, &stream_chat.ChannelRequest{
		Members: userIDs,
		ExtraData: map[string]interface{}{
			"name":            req.Name,
			"collaborationId": req.CollaborationID,
			"contractId":      contractId,
			// "image": "https://getstream.io/random_svg/?id=chat-1&name=Chat",
		},
	})
	if err != nil {
		// c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating channel", "error": err.Error()})
		return nil, err
	}

	contract := trendlymodels.Contract{
		UserID:          req.UserID,
		ManagerID:       managerId,
		CollaborationID: req.CollaborationID,
		StreamChannelID: res.Channel.ID,
		BrandID:         collabMap["brandId"].(string),
		Status:          0,
	}
	if contractId != "" {
		_, err = firestoredb.Client.Collection("contracts").Doc(contractId).Set(context.Background(), contract)
		if err != nil {
			// c.JSON(http.StatusBadRequest, gin.H{"message": "Error in creating contract", "error": err.Error()})
			return nil, err
		}
	}

	return &ChannelReturn{
		Contract: contract,
		Channel:  *res.Channel,
	}, nil
}
