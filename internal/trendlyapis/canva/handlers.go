package canva

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	canvapkg "github.com/idivarts/backend-sls/pkg/canva"
	"github.com/idivarts/backend-sls/pkg/mediastore"
)

// canvaEntitled reports whether the brand's org plan includes the Canva bridge.
// Fail-open when the brand has no org yet (rollout-safe).
func canvaEntitled(brandID string) bool {
	orgID, ok := trendlymodels.OrgIDForBrand(brandID)
	if !ok {
		return true
	}
	var org trendlymodels.Organization
	if err := org.Get(orgID); err != nil || org.Entitlements == nil {
		return true
	}
	return org.Entitlements.CanvaBridge
}

// ensureToken returns a valid access token for the brand-member, refreshing and
// persisting the rotated pair if the current one is near expiry.
func ensureToken(managerID string) (string, error) {
	conn, err := trendlymodels.GetCanvaConnection(managerID)
	if err != nil {
		return "", err
	}
	if conn == nil || !conn.Connected {
		return "", fmt.Errorf("canva not connected")
	}
	// Refresh if within 5 minutes of expiry.
	if conn.ExpiresAt-time.Now().UnixMilli() < 5*60*1000 {
		tok, err := canvapkg.RefreshToken(conn.RefreshToken)
		if err != nil {
			return "", fmt.Errorf("canva refresh failed: %w", err)
		}
		if err := trendlymodels.UpdateCanvaTokens(managerID, tok.AccessToken, tok.RefreshToken, tok.ExpiresAtMs); err != nil {
			return "", err
		}
		return tok.AccessToken, nil
	}
	return conn.AccessToken, nil
}

// ConnectStart begins the OAuth flow: gate the entitlement, generate PKCE +
// state, persist the attempt, and return the Canva authorize URL for the app to
// open in a browser / in-app tab.
func ConnectStart(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	brandID := c.Query("brandId")
	if !canvapkg.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "canva integration not configured"})
		return
	}
	if !canvaEntitled(brandID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "plan_locked", "feature": "canva_bridge"})
		return
	}
	pkce, err := canvapkg.NewPKCE()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	state, _ := canvapkg.RandomState()
	if err := trendlymodels.CreateCanvaOAuthState(&trendlymodels.CanvaOAuthState{
		State: state, ManagerID: managerID, BrandID: brandID, Verifier: pkce.Verifier,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"authUrl": canvapkg.AuthorizeURL(pkce.Challenge, state)})
}

// Callback is the PUBLIC OAuth redirect target. It exchanges the code for tokens
// (using the stored PKCE verifier), persists the connection, and redirects the
// browser back into the app.
func Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")
	fe := constants.TRENDLY_BRANDS_FE + "/connected-accounts"
	if code == "" || state == "" {
		c.Redirect(http.StatusFound, fe+"?canva=error")
		return
	}
	st, err := trendlymodels.ConsumeCanvaOAuthState(state)
	if err != nil || st == nil {
		c.Redirect(http.StatusFound, fe+"?canva=error")
		return
	}
	tok, err := canvapkg.ExchangeCode(code, st.Verifier)
	if err != nil {
		c.Redirect(http.StatusFound, fe+"?canva=error")
		return
	}
	_ = trendlymodels.UpsertCanvaConnection(&trendlymodels.CanvaConnection{
		ManagerID:    st.ManagerID,
		BrandID:      st.BrandID,
		AccessToken:  tok.AccessToken,
		RefreshToken: tok.RefreshToken,
		Scope:        tok.Scope,
		ExpiresAt:    tok.ExpiresAtMs,
		Connected:    true,
	})
	c.Redirect(http.StatusFound, fe+"?canva=connected")
}

// Status reports whether the current brand-member has an active Canva connection.
func Status(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	conn, err := trendlymodels.GetCanvaConnection(managerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connected": conn != nil && conn.Connected})
}

// Disconnect removes the brand-member's Canva connection.
func Disconnect(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	if err := trendlymodels.DeleteCanvaConnection(managerID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"connected": false})
}

// ListDesigns lists the user's Canva designs (library import picker).
func ListDesigns(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	token, err := ensureToken(managerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	designs, err := canvapkg.ListDesigns(token, c.Query("query"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"designs": designs})
}

// CreateDesignFromContent seeds a Canva design with the content's current render
// and returns its edit_url for deep editing.
func CreateDesignFromContent(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")
	if !canvaEntitled(brandID) {
		c.JSON(http.StatusPaymentRequired, gin.H{"error": "upgrade_required", "reason": "plan_locked", "feature": "canva_bridge"})
		return
	}
	token, err := ensureToken(managerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "content not found"})
		return
	}
	w, h := sizeForFormat(string(content.ContentFormat))
	assetID := ""
	if seed := renderURLFor(content); seed != "" {
		jobID, err := canvapkg.UploadURLAsset(token, seed, content.Title)
		if err == nil {
			assetID = pollAsset(token, jobID)
		}
	}
	design, err := canvapkg.CreateCustomDesign(token, w, h, content.Title, assetID)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	_ = trendlymodels.UpdateContentFields(brandID, contentID, map[string]interface{}{
		"canvaDesignId": design.ID,
	})
	// The app opens edit_url?correlation_state=<contentId> so the return carries
	// which content to attach back to.
	c.JSON(http.StatusOK, gin.H{
		"designId": design.ID,
		"editUrl":  design.URLs.EditURL,
	})
}

type exportReq struct {
	Format string `json:"format"` // png|jpg|mp4|gif|pdf
}

// CreateExport starts an export of the content's linked Canva design.
func CreateExport(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")
	var req exportReq
	_ = c.ShouldBindJSON(&req)
	token, err := ensureToken(managerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	content, err := trendlymodels.GetContent(brandID, contentID)
	if err != nil || content.CanvaDesignID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content has no linked Canva design"})
		return
	}
	format := req.Format
	if format == "" {
		format = defaultFormat(string(content.ContentFormat))
	}
	jobID, err := canvapkg.CreateExport(token, content.CanvaDesignID, format)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobId": jobID})
}

// GetExport polls an export job.
func GetExport(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	token, err := ensureToken(managerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	job, err := canvapkg.GetExport(token, c.Param("jobId"))
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": job.Status, "urls": job.URLs})
}

type returnReq struct {
	JWT    string `json:"jwt" binding:"required"`
	Format string `json:"format"`
}

// HandleReturn verifies the return-navigation JWT, exports the edited design,
// copies the result into our storage, and attaches it to the content.
func HandleReturn(c *gin.Context) {
	managerID, _ := middlewares.GetUserId(c)
	brandID := c.Param("brandId")
	contentID := c.Param("contentId")
	var req returnReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	claims, err := canvapkg.VerifyReturnJWT(req.JWT)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid return token", "detail": err.Error()})
		return
	}
	token, err := ensureToken(managerID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	content, _ := trendlymodels.GetContent(brandID, contentID)
	designID := claims.DesignID
	if designID == "" && content != nil {
		designID = content.CanvaDesignID
	}
	if designID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no design to export"})
		return
	}
	format := req.Format
	if format == "" && content != nil {
		format = defaultFormat(string(content.ContentFormat))
	}
	jobID, err := canvapkg.CreateExport(token, designID, format)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": err.Error()})
		return
	}
	url := pollExport(token, jobID)
	if url == "" {
		// Still processing — hand the job back for the app to poll.
		c.JSON(http.StatusAccepted, gin.H{"jobId": jobID, "status": "in_progress"})
		return
	}
	storedURL, err := downloadAndStore(c.Request.Context(), url, brandID, format)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "could not store export"})
		return
	}
	attach := attachmentFor(format, storedURL)
	_ = trendlymodels.UpdateContentFields(brandID, contentID, map[string]interface{}{
		"attachments":      []interface{}{attach},
		"exportedAssetRef": storedURL,
		"source":           "canva",
	})
	c.JSON(http.StatusOK, gin.H{"url": storedURL, "attachment": attach})
}

// ---- helpers ----

func renderURLFor(content *trendlymodels.Content) string {
	if content.DesignRef != nil && content.DesignRef.RenderURL != "" {
		return content.DesignRef.RenderURL
	}
	for _, a := range content.Attachments {
		if a.ImageURL != "" {
			return a.ImageURL
		}
	}
	return ""
}

func sizeForFormat(format string) (int, int) {
	switch format {
	case "reel", "story":
		return 1080, 1920
	case "video":
		return 1920, 1080
	case "post", "carousel":
		return 1080, 1350
	default:
		return 1080, 1080
	}
}

func defaultFormat(contentFormat string) string {
	switch contentFormat {
	case "reel", "video", "story":
		return "mp4"
	default:
		return "png"
	}
}

func attachmentFor(format, url string) map[string]interface{} {
	if format == "mp4" || format == "gif" {
		return map[string]interface{}{"type": "video", "playUrl": url}
	}
	return map[string]interface{}{"type": "image", "imageUrl": url}
}

func pollAsset(token, jobID string) string {
	for i := 0; i < 10; i++ {
		job, err := canvapkg.GetURLAssetJob(token, jobID)
		if err != nil {
			return ""
		}
		if job.Status == "success" {
			return job.AssetID
		}
		if job.Status == "failed" {
			return ""
		}
		time.Sleep(time.Second)
	}
	return ""
}

func pollExport(token, jobID string) string {
	for i := 0; i < 18; i++ {
		job, err := canvapkg.GetExport(token, jobID)
		if err != nil {
			return ""
		}
		if job.Status == "success" && len(job.URLs) > 0 {
			return job.URLs[0]
		}
		if job.Status == "failed" {
			return ""
		}
		time.Sleep(time.Second)
	}
	return ""
}

// downloadAndStore copies a (24h-valid) Canva export URL into our bucket,
// files it in the brand's media library, and returns the permanent CDN URL.
//
// ⚠️ The hosting is not optional: a Canva export URL dies within the day, and a
// scene pointing at a dead URL is a scene that cannot be re-rendered. The
// library row is not optional either — a design a brand exported is theirs to
// reuse, and before this it was stored with nothing pointing at it.
func downloadAndStore(ctx context.Context, srcURL, brandID, format string) (string, error) {
	kind := trendlymodels.AssetKindImage
	ctype := "image/png"
	switch format {
	case "mp4":
		ctype, kind = "video/mp4", trendlymodels.AssetKindVideo
	case "jpg":
		ctype = "image/jpeg"
	}
	stored, err := mediastore.HostFromURL(ctx, mediastore.Spec{
		BrandID: brandID,
		// The attachment bucket under `uploads/`, which is where Canva
		// exports have always landed. Kept rather than migrated: the URLs are
		// already in designs and published posts.
		Target:      mediastore.TargetAttachment,
		Prefix:      "uploads",
		Filename:    "canva." + format,
		ContentType: ctype,
		Kind:        kind,
		Source:      trendlymodels.AssetSourceGenerated,
		Origin: &trendlymodels.AssetOrigin{
			Kind:     trendlymodels.AssetOriginGenerated,
			Provider: "canva",
		},
	}, srcURL)
	if err != nil {
		return "", err
	}
	return stored.URL, nil
}
