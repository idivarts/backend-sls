// Package sharing turns a public share token into a Branch deep link.
//
// The share token itself is minted by the brand app (hooks/use-share-link.ts),
// which owns the shareLinks/{token} doc and the `enabled` toggle. Only the Branch
// link is made here, because it needs the Branch key + secret, which must never
// reach the client.
package sharing

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/branch"
)

// sharePermissions maps a share type to the privilege a member needs to hand out
// a link to it. Minting a link is a disclosure action, so it is gated on the same
// feature the resource lives behind rather than on bare brand membership.
var sharePermissions = map[string]struct {
	feature trendlymodels.Feature
	priv    trendlymodels.Privilege
}{
	trendlymodels.ShareTypeStrategy:      {trendlymodels.FeatureStrategy, trendlymodels.PrivStrategyViewer},
	trendlymodels.ShareTypeContent:       {trendlymodels.FeatureContent, trendlymodels.PrivContentView},
	trendlymodels.ShareTypeCalendarMonth: {trendlymodels.FeatureContentCalendar, trendlymodels.PrivCalendarView},
}

// CreateShareDeepLink returns the Branch deep link for an already-public share
// token, minting it on first call and reusing the cached one afterwards.
//
// Idempotent by design: a Branch link is permanent, and re-minting on every share
// -sheet open would both fragment the link's click analytics and leave older
// copied URLs pointing at a different link record.
//
//	POST /api/v2/brands/:brandId/share-links/:token/deep-link
func CreateShareDeepLink(c *gin.Context) {
	brandID := c.Param("brandId")
	token := c.Param("token")
	if token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Missing share token"})
		return
	}

	ctx := context.Background()

	link, err := trendlymodels.GetShareLink(ctx, token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Share link not found"})
		return
	}
	// The token is unguessable, but it is still a client-supplied id: without this
	// a member of brand A could mint a link for brand B's token by passing their
	// own brandId (which is the one the privilege check below trusts).
	if link.BrandID != brandID {
		c.JSON(http.StatusForbidden, gin.H{"message": "Share link does not belong to this brand"})
		return
	}
	if !link.Enabled {
		c.JSON(http.StatusConflict, gin.H{"message": "Enable the public link first"})
		return
	}

	perm, known := sharePermissions[link.Type]
	if !known {
		c.JSON(http.StatusBadRequest, gin.H{"message": "Unsupported share type"})
		return
	}
	if _, ok := middlewares.RequireFeaturePrivilege(c, brandID, perm.feature, perm.priv); !ok {
		return
	}

	webURL := fmt.Sprintf("%s/share/%s", constants.GetBrandsFronted(), token)

	// `deepLink` says whether `url` is a Branch link or the plain web fallback, so
	// the client never has to compare URLs to find out. It is always present.
	respond := func(url string, isDeepLink, created bool, extra gin.H) {
		body := gin.H{"url": url, "webUrl": webURL, "deepLink": isDeepLink, "created": created}
		for k, v := range extra {
			body[k] = v
		}
		c.JSON(http.StatusOK, body)
	}

	// No Branch credentials on this stage: the web URL still works everywhere, so
	// degrade instead of failing the share. Checked before the preview is built so
	// an unconfigured stage does no resource reads at all. A link minted while the
	// stage WAS configured stays valid, so it is still preferred over the web URL.
	if !branch.Brand.Configured() {
		if link.DeepLink != "" {
			respond(link.DeepLink, true, false, nil)
		} else {
			respond(webURL, false, false, nil)
		}
		return
	}

	// Building the card costs one or two resource reads (plus the month query for a
	// calendar share). Worth it on a share-sheet open: it is both what the link is
	// minted with and how a since-renamed resource is detected below.
	preview := buildPreview(ctx, link)
	linkReq := shareLinkRequest(token, webURL, link, preview)

	// Already minted — hand back the same URL. Copies of it are already out in the
	// world, so the URL must not change; only the card behind it is refreshed, and
	// only when the resource has actually been renamed or has gained an image.
	if link.DeepLink != "" {
		if link.DeepLinkPreview != preview.fingerprint() && branch.Brand.Secret != "" {
			if err := branch.Brand.UpdateLink(ctx, link.DeepLink, linkReq); err != nil {
				// Leave the fingerprint alone so the next open retries; a stale
				// card is not worth failing the share over.
				log.Printf("sharing: refresh deep link %s: %v", token, err)
			} else if err := trendlymodels.SetShareLinkPreview(ctx, token, preview.fingerprint()); err != nil {
				log.Printf("sharing: record deep link preview %s: %v", token, err)
			}
		}
		respond(link.DeepLink, true, false, nil)
		return
	}

	deepLink, err := branch.Brand.CreateLink(ctx, linkReq)
	if err != nil {
		// A Branch outage must not break sharing — the web URL is the fallback.
		respond(webURL, false, false, gin.H{"error": err.Error()})
		return
	}

	if err := trendlymodels.SetShareLinkDeepLink(ctx, token, deepLink, preview.fingerprint(), time.Now().UnixMilli()); err != nil {
		// Cache write failed: the link itself is valid, so return it. The next
		// call just mints another one.
		respond(deepLink, true, true, gin.H{"cached": false})
		return
	}

	respond(deepLink, true, true, nil)
}

// shareLinkRequest is the Branch link for a share token — one definition, used
// both to mint and to re-point, so the two can never drift apart.
func shareLinkRequest(token, webURL string, link *trendlymodels.ShareLink, preview sharePreview) branch.LinkRequest {
	return branch.LinkRequest{
		Channel:  "app-share",
		Feature:  "public-share",
		Stage:    link.Type,
		Campaign: "share-" + link.Type,
		Tags:     []string{link.Type},
		Type:     branch.TypeDefault,
		Data: branch.LinkData{
			OGTitle:       preview.title,
			OGDescription: preview.description,
			OGImageURL:    preview.imageURL,
			// Native opens land on the in-app public viewer, which is the same
			// screen the web URL renders.
			DeeplinkPath: "/share/" + token,
			DesktopURL:   webURL,
			FallbackURL:  webURL,
			CanonicalURL: webURL,
			Extra: map[string]string{
				"shareToken": token,
				"shareType":  link.Type,
				"brandId":    link.BrandID,
				"resourceId": link.ResourceID,
				"month":      link.Month,
			},
		},
	}
}
