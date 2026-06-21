package trendlyapis

import (
	"context"
	"net/http"
	"time"

	stream_chat "github.com/GetStream/stream-chat-go/v5"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
	"github.com/idivarts/backend-sls/pkg/streamchat"
)

// DeleteManager permanently deletes the signed-in manager (brand) account. This
// is the brand-app equivalent of DeleteUser and exists to satisfy the App Store
// / Play Store self-service account-deletion requirement.
//
// It soft-deletes the manager doc, strips the manager from every brand and
// organization they belong to, removes their Firebase auth (revoking all
// sessions + freeing the email) and Stream user, and soft-deletes any (now
// empty) personal org they solely own.
//
// Block-&-instruct guards (mirroring DeleteOrganization): the call is refused
// while the manager still SOLELY owns an organization that holds an active
// brand or a paid subscription — they must transfer/delete those brands and
// cancel the subscription first. Active contracts are enforced transitively: a
// brand can't be deleted while it has active contracts, so an org still holding
// such a brand keeps this blocked.
//
// DELETE /api/v2/managers/delete
func DeleteManager(c *gin.Context) {
	managerId, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Manager not authenticated", "message": "ManagerId not found"})
		return
	}

	manager := trendlymodels.Manager{}
	if err := manager.Get(managerId); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "message": "Manager not found"})
		return
	}

	ownedOrgs, err := trendlymodels.ListOrganizationsOwnedBy(managerId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to check owned organizations"})
		return
	}

	// ── Block-&-instruct guards ───────────────────────────────────────────────
	for _, org := range ownedOrgs {
		// Guard 1 — org still holds an active (non-deleted) brand.
		for _, brandId := range org.BrandIds {
			b := trendlymodels.Brand{}
			if err := b.Get(brandId); err != nil {
				continue
			}
			if b.DeletedAt == nil {
				c.JSON(http.StatusConflict, gin.H{
					"error":   "owns-active-brands",
					"message": "You still own brands in \"" + org.Name + "\". Delete or transfer every brand before deleting your account.",
				})
				return
			}
		}
		// Guard 2 — org holds a paid (non-free) subscription.
		if org.Billing != nil && org.Billing.PlanKey != nil {
			plan := *org.Billing.PlanKey
			if plan != "" && plan != "free" {
				c.JSON(http.StatusConflict, gin.H{
					"error":   "owns-active-subscription",
					"message": "Cancel the subscription for \"" + org.Name + "\" before deleting your account.",
				})
				return
			}
		}
	}

	now := time.Now().UnixMilli()

	// ── Strip brand memberships + collect the orgs to detach from ─────────────
	brandIds, err := trendlymodels.GetMyBrandIds(managerId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to read brand memberships"})
		return
	}
	orgIds := map[string]struct{}{}
	for _, brandId := range brandIds {
		_ = trendlymodels.DeleteBrandMember(brandId, managerId)
		b := trendlymodels.Brand{}
		if err := b.Get(brandId); err == nil && b.OrganizationID != nil && *b.OrganizationID != "" {
			orgIds[*b.OrganizationID] = struct{}{}
		}
	}

	// Soft-delete the (now empty, free) orgs they solely own, and make sure their
	// own membership in them is dropped too.
	for _, org := range ownedOrgs {
		_ = trendlymodels.SoftDeleteOrganization(org.ID, now)
		orgIds[org.ID] = struct{}{}
	}
	for orgId := range orgIds {
		_ = trendlymodels.DeleteOrgMember(orgId, managerId)
	}

	// ── Soft-delete the manager doc ───────────────────────────────────────────
	if err := trendlymodels.SoftDeleteManager(managerId, now); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to delete manager record"})
		return
	}

	// ── Remove Firebase auth (revokes every session + frees the email) ────────
	if err := fauth.Client.DeleteUser(context.Background(), managerId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Error removing account authentication"})
		return
	}

	// ── Best-effort Stream cleanup (a manager who never chatted has no user) ──
	_, _ = streamchat.Client.DeleteUser(context.Background(), managerId, stream_chat.DeleteUserWithMarkMessagesDeleted())

	c.JSON(http.StatusOK, gin.H{"message": "Account deleted successfully"})
}
