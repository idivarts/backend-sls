package ai

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
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

func ListConversations(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	brandID := c.Query("brandId")
	if brandID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId is required"})
		return
	}
	if !verifyBrandAccess(brandID, managerID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}
	module := c.Query("module")
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := atoi(v); err == nil && n > 0 && n <= 200 {
			limit = n
		}
	}
	out, err := openrouter.ListConversations(c.Request.Context(), brandID, managerID, module, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if out == nil {
		out = []trendlymodels.AIConversation{}
	}
	c.JSON(http.StatusOK, gin.H{"conversations": out})
}

func GetConversation(c *gin.Context) {
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
	msgs, err := openrouter.LoadHistory(c.Request.Context(), conv.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if msgs == nil {
		msgs = []trendlymodels.AIMessage{}
	}
	c.JSON(http.StatusOK, gin.H{
		"conversation": conv,
		"messages":     msgs,
	})
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

func atoi(s string) (int, error) {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errAtoi
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}

var errAtoi = &parseError{"not a number"}

type parseError struct{ msg string }

func (e *parseError) Error() string { return e.msg }
