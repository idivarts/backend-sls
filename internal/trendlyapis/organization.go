package trendlyapis

import (
	"context"
	"net/http"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	firestoredb "github.com/idivarts/backend-sls/pkg/firebase/firestore"
	"github.com/idivarts/backend-sls/pkg/myutil"
)

// ── Organization CRUD + brand lifecycle (delete / org-delete / transfer) ──────
//
// Decisions (see the Organization ticket §4b):
//   - Delete brand     → soft-delete; blocked while the brand has active
//                        contracts; removed from its org's brandIds.
//   - Delete org       → soft-delete; blocked while it still owns active brands
//                        or holds a paid (non-free) subscription.
//   - Transfer brand   → only into an org the caller OWNS, and only if that org
//                        is under its maxBrands cap; transactional re-parent.

type ICreateOrganization struct {
	Name  string  `json:"name" binding:"required"`
	Image *string `json:"image"`
}

// CreateOrganization creates a new org owned by the caller and seeds the owner
// membership. New orgs start on the forever-free plan (cap = 1 brand).
func CreateOrganization(c *gin.Context) {
	var req ICreateOrganization
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userId, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}

	planKey := "free"
	org := trendlymodels.Organization{
		Name:         req.Name,
		Image:        req.Image,
		OwnerID:      userId,
		BrandIds:     []string{},
		PlanKey:      myutil.StrPtr(planKey),
		MaxBrands:    trendlymodels.ResolveMaxBrands(planKey),
		Billing:      &trendlymodels.OrgBilling{PlanKey: myutil.StrPtr(planKey), BillingStatus: myutil.StrPtr("active")},
		CreationTime: time.Now().UnixMilli(),
	}

	orgId, err := org.Insert()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to create organization"})
		return
	}

	owner := trendlymodels.OrganizationMember{ManagerID: userId, Role: trendlymodels.OrgRoleOwner, Status: 1}
	if _, err := owner.Set(orgId); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to seed owner membership"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Organization created", "organization": trendlymodels.OrganizationWithID{ID: orgId, Organization: org}})
}

// ListMyOrganizations returns every org the caller belongs to (across all orgs),
// so the app can render an Organizations hub + grouped brand switcher.
func ListMyOrganizations(c *gin.Context) {
	userId, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}

	orgs, err := trendlymodels.GetMyOrganizations(userId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to list organizations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"organizations": orgs})
}

// GetOrganization returns one org plus its (non-deleted) brands.
func GetOrganization(c *gin.Context) {
	userId, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}
	orgId := c.Param("id")

	if _, found := getOrgRole(orgId, userId); !found {
		c.JSON(http.StatusForbidden, gin.H{"message": "You are not a member of this organization"})
		return
	}

	org := trendlymodels.Organization{}
	if err := org.Get(orgId); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error(), "message": "Organization not found"})
		return
	}
	if org.DeletedAt != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "Organization not found"})
		return
	}

	brands := []gin.H{}
	for _, brandId := range org.BrandIds {
		b := trendlymodels.Brand{}
		if err := b.Get(brandId); err != nil {
			continue
		}
		if b.DeletedAt != nil {
			continue
		}
		brands = append(brands, gin.H{"id": brandId, "name": b.Name, "image": b.Image})
	}

	c.JSON(http.StatusOK, gin.H{
		"organization": trendlymodels.OrganizationWithID{ID: orgId, Organization: org},
		"brands":       brands,
		"maxBrands":    org.MaxBrands,
		"brandCount":   len(org.BrandIds),
	})
}

type IAddBrand struct {
	Name  string  `json:"name" binding:"required"`
	Image *string `json:"image"`
}

// AddBrandToOrganization creates a brand inside the org, enforcing the plan's
// maxBrands cap inside a transaction so concurrent adds can't exceed it.
func AddBrandToOrganization(c *gin.Context) {
	var req IAddBrand
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userId, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}
	orgId := c.Param("id")

	if role, found := getOrgRole(orgId, userId); !found || (role != trendlymodels.OrgRoleOwner && role != trendlymodels.OrgRoleAdmin) {
		c.JSON(http.StatusForbidden, gin.H{"message": "Only an org owner/admin can add brands"})
		return
	}

	ctx := context.Background()
	orgRef := firestoredb.Client.Collection("organizations").Doc(orgId)
	brandRef := firestoredb.Client.Collection("brands").NewDoc()
	now := time.Now().UnixMilli()

	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		orgSnap, err := tx.Get(orgRef)
		if err != nil {
			return err
		}
		var org trendlymodels.Organization
		if err := orgSnap.DataTo(&org); err != nil {
			return err
		}
		if org.DeletedAt != nil {
			return errBrandLimit // treat a deleted org as unusable
		}

		limit := org.MaxBrands
		if limit <= 0 {
			limit = trendlymodels.ResolveMaxBrands(myutil.DerefString(org.PlanKey))
		}
		if len(org.BrandIds) >= limit {
			return errBrandLimit
		}

		brandData := map[string]interface{}{
			"name":               req.Name,
			"image":              req.Image,
			"organizationId":     orgId,
			"onboardingComplete": false,
			"creationTime":       now,
		}
		// Billing lives on the org only — nothing to stamp on the brand.
		if err := tx.Set(brandRef, brandData, firestore.MergeAll); err != nil {
			return err
		}
		return tx.Update(orgRef, []firestore.Update{{Path: "brandIds", Value: firestore.ArrayUnion(brandRef.ID)}})
	})

	if err == errBrandLimit {
		c.JSON(http.StatusConflict, gin.H{"error": "brand-limit-reached", "message": "This organization has reached its plan's brand limit. Upgrade to add more brands."})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to add brand"})
		return
	}

	// Every brand gets a default team that owns its socials, and the creator is
	// added as an active member. The brand switcher / brand-context list keys off
	// brands/{brandId}/members/{managerId}, so WITHOUT this membership the new
	// brand would never appear in the creator's switcher. Done post-transaction.
	defTeam, err := trendlymodels.EnsureDefaultTeam(brandRef.ID, userId, now)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Brand created but default team failed"})
		return
	}
	member := trendlymodels.BrandMember{ManagerID: userId, Status: 1, TeamID: defTeam}
	if _, err := member.Set(brandRef.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Brand created but membership failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Brand added to organization", "brandId": brandRef.ID})
}

// DeleteOrganization soft-deletes an org. Blocked while it still owns active
// brands (move/delete them first) or holds a paid subscription (downgrade
// first). Owner-only.
func DeleteOrganization(c *gin.Context) {
	userId, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}
	orgId := c.Param("id")

	if role, found := getOrgRole(orgId, userId); !found || role != trendlymodels.OrgRoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"message": "Only the org owner can delete the organization"})
		return
	}

	org := trendlymodels.Organization{}
	if err := org.Get(orgId); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error(), "message": "Organization not found"})
		return
	}
	if org.DeletedAt != nil {
		c.JSON(http.StatusOK, gin.H{"message": "Organization already deleted"})
		return
	}

	// Guard 1 — no active brands.
	for _, brandId := range org.BrandIds {
		b := trendlymodels.Brand{}
		if err := b.Get(brandId); err != nil {
			continue
		}
		if b.DeletedAt == nil {
			c.JSON(http.StatusConflict, gin.H{"error": "org-has-brands", "message": "Move or delete all brands before deleting this organization."})
			return
		}
	}

	// Guard 2 — no active paid subscription.
	if org.Billing != nil && org.Billing.PlanKey != nil {
		plan := *org.Billing.PlanKey
		if plan != "" && plan != "free" {
			c.JSON(http.StatusConflict, gin.H{"error": "org-has-active-subscription", "message": "Cancel/downgrade the subscription before deleting this organization."})
			return
		}
	}

	now := time.Now().UnixMilli()
	if _, err := firestoredb.Client.Collection("organizations").Doc(orgId).
		Update(context.Background(), []firestore.Update{{Path: "deletedAt", Value: now}}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to delete organization"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Organization deleted"})
}

// DeleteBrand soft-deletes a brand. Blocked while the brand has active
// contracts. Removes the brand from its org's brandIds so it stops counting
// against the cap. Allowed for a brand member or the org owner/admin.
func DeleteBrand(c *gin.Context) {
	userId, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}
	brandId := c.Param("brandId")

	brand := trendlymodels.Brand{}
	if err := brand.Get(brandId); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error(), "message": "Brand not found"})
		return
	}
	if brand.DeletedAt != nil {
		c.JSON(http.StatusOK, gin.H{"message": "Brand already deleted"})
		return
	}

	if !canManageBrand(brand, brandId, userId) {
		c.JSON(http.StatusForbidden, gin.H{"message": "You don't have permission to delete this brand"})
		return
	}

	active, err := trendlymodels.HasActiveContracts(brandId)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to check active contracts"})
		return
	}
	if active {
		c.JSON(http.StatusConflict, gin.H{"error": "brand-has-active-contracts", "message": "This brand has active contracts. Settle or cancel them before deleting."})
		return
	}

	now := time.Now().UnixMilli()
	if _, err := firestoredb.Client.Collection("brands").Doc(brandId).
		Update(context.Background(), []firestore.Update{{Path: "deletedAt", Value: now}}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to delete brand"})
		return
	}

	if brand.OrganizationID != nil && *brand.OrganizationID != "" {
		_, _ = firestoredb.Client.Collection("organizations").Doc(*brand.OrganizationID).
			Update(context.Background(), []firestore.Update{{Path: "brandIds", Value: firestore.ArrayRemove(brandId)}})
	}

	c.JSON(http.StatusOK, gin.H{"message": "Brand deleted"})
}

// TransferBrand moves a brand into a destination org the caller OWNS, enforcing
// the destination's maxBrands cap. The move is transactional: re-parent the
// brand, remove it from the source org's brandIds, add it to the destination's.
func TransferBrand(c *gin.Context) {
	userId, ok := middlewares.GetUserId(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "User not found"})
		return
	}
	destOrgId := c.Param("id")
	brandId := c.Param("brandId")

	// Destination must be owned by the caller (requirement: "an org you own").
	if role, found := getOrgRole(destOrgId, userId); !found || role != trendlymodels.OrgRoleOwner {
		c.JSON(http.StatusForbidden, gin.H{"message": "You can only move a brand into an organization you own"})
		return
	}

	brand := trendlymodels.Brand{}
	if err := brand.Get(brandId); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error(), "message": "Brand not found"})
		return
	}
	if brand.DeletedAt != nil {
		c.JSON(http.StatusConflict, gin.H{"message": "Cannot transfer a deleted brand"})
		return
	}
	if !canManageBrand(brand, brandId, userId) {
		c.JSON(http.StatusForbidden, gin.H{"message": "You don't have permission to move this brand"})
		return
	}

	srcOrgId := ""
	if brand.OrganizationID != nil {
		srcOrgId = *brand.OrganizationID
	}
	if srcOrgId == destOrgId {
		c.JSON(http.StatusOK, gin.H{"message": "Brand already in this organization"})
		return
	}

	ctx := context.Background()
	destRef := firestoredb.Client.Collection("organizations").Doc(destOrgId)
	brandRef := firestoredb.Client.Collection("brands").Doc(brandId)

	err := firestoredb.Client.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		destSnap, err := tx.Get(destRef)
		if err != nil {
			return err
		}
		var dest trendlymodels.Organization
		if err := destSnap.DataTo(&dest); err != nil {
			return err
		}
		if dest.DeletedAt != nil {
			return errBrandLimit
		}

		limit := dest.MaxBrands
		if limit <= 0 {
			limit = trendlymodels.ResolveMaxBrands(myutil.DerefString(dest.PlanKey))
		}
		if len(dest.BrandIds) >= limit {
			return errBrandLimit
		}

		if err := tx.Update(brandRef, []firestore.Update{{Path: "organizationId", Value: destOrgId}}); err != nil {
			return err
		}
		if srcOrgId != "" {
			srcRef := firestoredb.Client.Collection("organizations").Doc(srcOrgId)
			if err := tx.Update(srcRef, []firestore.Update{{Path: "brandIds", Value: firestore.ArrayRemove(brandId)}}); err != nil {
				return err
			}
		}
		return tx.Update(destRef, []firestore.Update{{Path: "brandIds", Value: firestore.ArrayUnion(brandId)}})
	})

	if err == errBrandLimit {
		c.JSON(http.StatusConflict, gin.H{"error": "brand-limit-reached", "message": "The destination organization has reached its plan's brand limit."})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "message": "Failed to transfer brand"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Brand transferred", "brandId": brandId, "organizationId": destOrgId})
}

// ── helpers ───────────────────────────────────────────────────────────────────

// errBrandLimit is a sentinel returned from transactions when the destination
// org is at (or over) its maxBrands cap, mapped to a 409 by callers.
var errBrandLimit = &capError{}

type capError struct{}

func (e *capError) Error() string { return "brand-limit-reached" }

// getOrgRole returns the caller's role in the org and whether they are a member.
func getOrgRole(orgId, managerId string) (trendlymodels.OrgRole, bool) {
	m := trendlymodels.OrganizationMember{}
	if err := m.Get(orgId, managerId); err != nil {
		return "", false
	}
	return m.Role, true
}

// canManageBrand allows the action if the caller is a member of the brand, or an
// owner/admin of the brand's organization.
func canManageBrand(brand trendlymodels.Brand, brandId, userId string) bool {
	bm := trendlymodels.BrandMember{}
	if err := bm.Get(brandId, userId); err == nil {
		return true
	}
	if brand.OrganizationID != nil && *brand.OrganizationID != "" {
		if role, found := getOrgRole(*brand.OrganizationID, userId); found {
			return role == trendlymodels.OrgRoleOwner || role == trendlymodels.OrgRoleAdmin
		}
	}
	return false
}
