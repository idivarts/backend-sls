package trendlyunauth

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"google.golang.org/api/iterator"
)

// shareLinkDoc mirrors the top-level shareLinks/{token} document written by the
// brand app. Only the fields the public endpoint needs are modelled.
type shareLinkDoc struct {
	Type       string `firestore:"type"`
	BrandID    string `firestore:"brandId"`
	ResourceID string `firestore:"resourceId"`
	Month      string `firestore:"month"`
	Enabled    bool   `firestore:"enabled"`
}

// pubContentDoc is the subset of a content document exposed publicly.
type pubContentDoc struct {
	Title            string                            `firestore:"title"`
	ContentFormat    string                            `firestore:"contentFormat"`
	Platform         string                            `firestore:"platform"`
	Status           string                            `firestore:"status"`
	PostingTimeStamp int64                             `firestore:"postingTimeStamp"`
	IsArchived       bool                              `firestore:"isArchived"`
	Attachments      []trendlymodels.ContentAttachment `firestore:"attachments"`
}

type publicCalendarItem struct {
	ID               string `json:"id"`
	Title            string `json:"title"`
	ContentFormat    string `json:"contentFormat,omitempty"`
	Platform         string `json:"platform,omitempty"`
	Status           string `json:"status,omitempty"`
	PostingTimeStamp int64  `json:"postingTimeStamp,omitempty"`
	ImageURL         string `json:"imageUrl,omitempty"`
}

type publicBrandInfo struct {
	Name  string `json:"name,omitempty"`
	Image string `json:"image,omitempty"`
}

// PublicShareResolve serves an unauthenticated, view-only payload for a public
// share link. Strategy and single-content shares are read client-side directly
// from Firestore (gated by rules); this endpoint exists for the Calendar month
// share, whose data is a query across many content documents that the client
// rules can't cleanly gate.
//
//	GET /public/shares/:token
func PublicShareResolve(c *gin.Context) {
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing token"})
		return
	}

	ctx := context.Background()

	// 1. Resolve + validate the share link.
	linkSnap, err := firestoredb.Client.Collection("shareLinks").Doc(token).Get(ctx)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return
	}
	var link shareLinkDoc
	if err := linkSnap.DataTo(&link); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "link not found"})
		return
	}
	if !link.Enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "link disabled"})
		return
	}

	// 2. Only calendar-month shares are served here.
	if link.Type != "calendarMonth" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported share type"})
		return
	}
	if link.BrandID == "" || link.Month == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "invalid share"})
		return
	}

	// 3. Derive the month window [start, end) in epoch ms.
	monthStart, err := time.Parse("2006-01", link.Month)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid month"})
		return
	}
	start := monthStart.UnixMilli()
	end := monthStart.AddDate(0, 1, 0).UnixMilli()

	// 4. Query the month's (non-archived) content items.
	iter := firestoredb.Client.
		Collection(fmt.Sprintf("brands/%s/contents", link.BrandID)).
		Where("isArchived", "==", false).
		Where("postingTimeStamp", ">=", start).
		Where("postingTimeStamp", "<", end).
		OrderBy("postingTimeStamp", firestore.Asc).
		Documents(ctx)
	defer iter.Stop()

	items := []publicCalendarItem{}
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load calendar"})
			return
		}
		var ct pubContentDoc
		if err := doc.DataTo(&ct); err != nil {
			continue
		}
		imageURL := ""
		for _, a := range ct.Attachments {
			if a.ImageURL != "" {
				imageURL = a.ImageURL
				break
			}
		}
		items = append(items, publicCalendarItem{
			ID:               doc.Ref.ID,
			Title:            ct.Title,
			ContentFormat:    ct.ContentFormat,
			Platform:         ct.Platform,
			Status:           ct.Status,
			PostingTimeStamp: ct.PostingTimeStamp,
			ImageURL:         imageURL,
		})
	}

	// 5. Attach lightweight brand display info (best-effort).
	brandInfo := publicBrandInfo{}
	brand := trendlymodels.Brand{}
	if err := brand.Get(link.BrandID); err == nil {
		brandInfo.Name = brand.Name
		if brand.Image != nil {
			brandInfo.Image = *brand.Image
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"brand": brandInfo,
		"month": link.Month,
		"items": items,
	})
}
