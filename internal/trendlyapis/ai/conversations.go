package ai

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

type createConversationReq struct {
	BrandID   string `json:"brandId" binding:"required"`
	Module    string `json:"module" binding:"required"`
	ContextID string `json:"contextId"`
	Model     string `json:"model"`
	Title     string `json:"title"`
}

func CreateConversation(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	var req createConversationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !verifyBrandAccess(req.BrandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	model := openrouter.ResolveModel(openrouter.TaskChat, req.Model)
	title := req.Title
	if title == "" {
		title = "New chat"
	}
	conv, err := openrouter.CreateConversation(c.Request.Context(), req.BrandID, managerID, req.Module, req.ContextID, model, title)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"conversation": conv})
}

func DeleteConversation(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	conversationID := c.Param("conversationId")
	conv, err := openrouter.GetConversation(c.Request.Context(), conversationID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if conv.UserID != managerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := openrouter.DeleteConversation(c.Request.Context(), conv.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true})
}

type renameConversationReq struct {
	Title string `json:"title" binding:"required"`
}

func RenameConversation(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	conversationID := c.Param("conversationId")
	var req renameConversationReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	conv, err := openrouter.GetConversation(c.Request.Context(), conversationID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	if conv.UserID != managerID {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	if err := openrouter.UpdateConversationTitle(c.Request.Context(), conv.ID, req.Title); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
