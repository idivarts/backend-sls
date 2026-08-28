// Package admin serves Trendly's own internal ops screens (the Brand CRM), as
// opposed to the brand-scoped APIs the product itself calls. Every handler here
// reads across ALL brands, so every handler must gate on Manager.IsAdmin.
package admin

import (
	"context"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/idivarts/backend-sls/internal/middlewares"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

// usageFanout bounds how many brands are aggregated concurrently on the list
// endpoint, and how many member manager lookups run concurrently on the detail
// endpoint. These are small, independent Firestore reads (COUNT aggregations
// and single-doc Gets) bottlenecked entirely on network round-trip time, not
// CPU or Firestore throughput — a real single-brand detail call was observed
// taking 3-9s over ~10 SEQUENTIAL round trips (see GetBrandUsage), so a high
// number here buys real wall-clock time back. API Gateway hard-caps every
// request at 29s with no override, so the list endpoint fanning out across
// every brand has to fit inside that regardless of brand count.
const usageFanout = 32

// BrandOwner identifies the human accountable for a brand — the org owner where
// the brand belongs to an organization, otherwise the brand's creator.
type BrandOwner struct {
	ManagerID string `json:"managerId,omitempty"`
	Name      string `json:"name,omitempty"`
	Email     string `json:"email,omitempty"`
	// LastSeenPlatform is the owner's most recent client ("ios" | "android" |
	// "web-desktop" | "web-mobile"), empty until they make a request.
	LastSeenPlatform string `json:"lastSeenPlatform,omitempty"`
	LastSeenAt       int64  `json:"lastSeenAt,omitempty"`
	// Source records how the owner was resolved: "organization" | "brand_creator".
	Source string `json:"source,omitempty"`
}

// TokenUsage reports the org's AI token wallet.
//
// The wallet is org-level, so when an org holds several brands these numbers
// describe all of them together — SharedAcrossBrands tells the UI when it must
// say so rather than implying the spend belongs to this brand alone. There is
// no consumption ledger, so Consumed is only what has been drawn from the
// CURRENT monthly allotment; it resets on the 1st and excludes top-ups.
type TokenUsage struct {
	OrganizationID     string `json:"organizationId,omitempty"`
	PlanKey            string `json:"planKey,omitempty"`
	MonthlyAllotment   int64  `json:"monthlyAllotment"`
	Balance            int64  `json:"balance"`
	Consumed           int64  `json:"consumed"`
	TopupBalance       int64  `json:"topupBalance"`
	PeriodResetAt      int64  `json:"periodResetAt,omitempty"`
	SharedAcrossBrands int    `json:"sharedAcrossBrands"`
}

// BrandUsageSummary is the per-brand card payload for the CRM board.
type BrandUsageSummary struct {
	BrandID         string      `json:"brandId"`
	AIConversations int         `json:"aiConversations"`
	ContentTotal    int         `json:"contentTotal"`
	Owner           *BrandOwner `json:"owner,omitempty"`
	Tokens          *TokenUsage `json:"tokens,omitempty"`
}

// BrandUsageDetail is the bottom-sheet payload for a single brand.
type BrandUsageDetail struct {
	BrandUsageSummary
	StrategiesTotal int                         `json:"strategiesTotal"`
	Content         *trendlymodels.ContentUsage `json:"content,omitempty"`
	Members         []BrandMemberUsage          `json:"members"`
}

// BrandMemberUsage is one row of the modal's member list, carrying the device
// each member was last seen on.
type BrandMemberUsage struct {
	ManagerID        string `json:"managerId"`
	Name             string `json:"name,omitempty"`
	Email            string `json:"email,omitempty"`
	ProfileImage     string `json:"profileImage,omitempty"`
	LastSeenPlatform string `json:"lastSeenPlatform,omitempty"`
	LastSeenAt       int64  `json:"lastSeenAt,omitempty"`
}

// requireAdmin enforces that the caller is a Trendly admin. These endpoints
// expose every customer's usage, and the Firestore rules cannot gate them
// (isAdmin() in firestore.rules is a stub that returns true), so this check is
// the only thing standing in front of the data.
func requireAdmin(c *gin.Context) bool {
	manager := middlewares.GetManagerModel(c)
	if !manager.IsAdmin {
		c.JSON(http.StatusForbidden, gin.H{"message": "Only Trendly admins can view brand usage"})
		return false
	}
	return true
}

// orgIndex resolves brand -> organization and caches owner lookups, so a list
// spanning many brands in the same org reads each org and owner only once.
type orgIndex struct {
	byBrand map[string]*trendlymodels.OrganizationWithID
	owners  map[string]*trendlymodels.Manager
}

func buildOrgIndex(orgs []trendlymodels.OrganizationWithID) *orgIndex {
	idx := &orgIndex{
		byBrand: map[string]*trendlymodels.OrganizationWithID{},
		owners:  map[string]*trendlymodels.Manager{},
	}
	for i := range orgs {
		org := &orgs[i]
		if org.DeletedAt != nil {
			continue
		}
		for _, brandID := range org.BrandIds {
			idx.byBrand[brandID] = org
		}
	}
	return idx
}

// manager returns a cached manager document, loading it on first use. Not safe
// for concurrent use — call it before fanning out.
func (idx *orgIndex) manager(managerID string) *trendlymodels.Manager {
	if managerID == "" {
		return nil
	}
	if cached, ok := idx.owners[managerID]; ok {
		return cached
	}
	var m trendlymodels.Manager
	if err := m.Get(managerID); err != nil {
		idx.owners[managerID] = nil
		return nil
	}
	idx.owners[managerID] = &m
	return &m
}

func (idx *orgIndex) tokensFor(brandID string) *TokenUsage {
	org, ok := idx.byBrand[brandID]
	if !ok {
		return nil
	}
	usage := &TokenUsage{
		OrganizationID:     org.ID,
		SharedAcrossBrands: len(org.BrandIds),
	}
	if org.PlanKey != nil {
		usage.PlanKey = *org.PlanKey
	}
	if w := org.TokenWallet; w != nil {
		usage.MonthlyAllotment = w.MonthlyAllotment
		usage.Balance = w.Balance
		usage.TopupBalance = w.TopupBalance
		usage.PeriodResetAt = w.PeriodResetAt
		if consumed := w.MonthlyAllotment - w.Balance; consumed > 0 {
			usage.Consumed = consumed
		}
	}
	return usage
}

const (
	ownerSourceOrganization = "organization"
	ownerSourceBrandCreator = "brand_creator"
)

// resolveOwnerID finds which manager owns a brand, returning the id and how it
// was determined. Reads only the immutable org index, so it is safe to call
// concurrently — unlike owner(), which populates the shared manager cache.
//
// Falls back to a Firestore lookup only for brands with no organization (those
// predating the Organization rollout).
func (idx *orgIndex) resolveOwnerID(ctx context.Context, brandID string) (managerID, source string) {
	if org, ok := idx.byBrand[brandID]; ok && org.OwnerID != "" {
		return org.OwnerID, ownerSourceOrganization
	}
	if creator := trendlymodels.GetDefaultTeamCreator(ctx, brandID); creator != "" {
		return creator, ownerSourceBrandCreator
	}
	return "", ""
}

// owner builds the owner payload for an already-resolved manager id. Not safe
// for concurrent use — it populates the shared manager cache.
func (idx *orgIndex) owner(managerID, source string) *BrandOwner {
	manager := idx.manager(managerID)
	if manager == nil {
		return nil
	}
	return &BrandOwner{
		ManagerID:        managerID,
		Name:             manager.Name,
		Email:            manager.Email,
		LastSeenPlatform: manager.LastSeenPlatform,
		LastSeenAt:       manager.LastSeenAt,
		Source:           source,
	}
}

// ListBrandUsage returns card-level usage for every brand, keyed by brand id so
// the CRM board can merge it into the brand list it already reads from
// Firestore. Admin-only.
//
// GET /api/v2/admin/brands/usage
func ListBrandUsage(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	ctx := c.Request.Context()

	refs, err := trendlymodels.ListAllBrandRefs(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	orgs, err := trendlymodels.ListAllOrganizations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	idx := buildOrgIndex(orgs)

	// Every per-brand Firestore read happens here, in parallel. Manager lookups
	// are deliberately left out: they share a cache, and many brands resolve to
	// the same owner, so they are cheaper resolved once afterwards.
	type aggregate struct {
		conversations int
		content       int
		ownerID       string
		ownerSource   string
	}
	aggregates := make(map[string]*aggregate, len(refs))

	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, usageFanout)
	for _, ref := range refs {
		wg.Add(1)
		go func(ref trendlymodels.BrandRef) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			agg := &aggregate{}
			// The 3 lookups below are independent Firestore round trips (two
			// COUNT aggregations, one that only reads for brands with no
			// organization). Running them concurrently means this brand's cost
			// is the SLOWEST of the three, not their sum.
			var inner sync.WaitGroup
			inner.Add(3)
			go func() {
				defer inner.Done()
				// A failed count must not blank out the whole board — leave
				// that metric at zero and keep the rest of the row.
				if conversations, err := openrouter.CountConversationsByBrand(ctx, ref.ID); err == nil {
					agg.conversations = conversations
				}
			}()
			go func() {
				defer inner.Done()
				if content, err := trendlymodels.CountContent(ctx, ref.ID); err == nil {
					agg.content = content
				}
			}()
			go func() {
				defer inner.Done()
				agg.ownerID, agg.ownerSource = idx.resolveOwnerID(ctx, ref.ID)
			}()
			inner.Wait()

			mu.Lock()
			aggregates[ref.ID] = agg
			mu.Unlock()
		}(ref)
	}
	wg.Wait()

	summaries := make(map[string]*BrandUsageSummary, len(refs))
	for brandID, agg := range aggregates {
		summaries[brandID] = &BrandUsageSummary{
			BrandID:         brandID,
			AIConversations: agg.conversations,
			ContentTotal:    agg.content,
			Owner:           idx.owner(agg.ownerID, agg.ownerSource),
			Tokens:          idx.tokensFor(brandID),
		}
	}

	c.JSON(http.StatusOK, gin.H{"brands": summaries})
}

// GetBrandUsage returns the full usage breakdown for one brand, backing the CRM
// bottom sheet. Admin-only.
//
// GET /api/v2/admin/brands/:brandId/usage
func GetBrandUsage(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	ctx := c.Request.Context()

	brandID := c.Param("brandId")
	if brandID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "brandId is required"})
		return
	}

	var brand trendlymodels.Brand
	if err := brand.Get(brandID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Brand not found"})
		return
	}

	// Resolve just this brand's own organization by id rather than scanning
	// every organization document (trendlymodels.ListAllOrganizations) —
	// buildOrgIndex works the same either way, since it only ever looks up by
	// brand id, but this is a single Get instead of the whole collection.
	var orgs []trendlymodels.OrganizationWithID
	if brand.OrganizationID != nil && *brand.OrganizationID != "" {
		var org trendlymodels.Organization
		if err := org.Get(*brand.OrganizationID); err == nil {
			orgs = append(orgs, trendlymodels.OrganizationWithID{
				ID:           *brand.OrganizationID,
				Organization: org,
			})
		}
	}
	idx := buildOrgIndex(orgs)

	// The 4 lookups below are independent Firestore reads. A single-brand
	// request was observed taking 3-9s run sequentially (see usageFanout);
	// running them concurrently cuts that to roughly the slowest one.
	var (
		ownerID, ownerSource             string
		conversations                    int
		contentUsage                     *trendlymodels.ContentUsage
		strategies                       int
		members                          []BrandMemberUsage
		convErr, contentErr, strategyErr error
	)
	var wg sync.WaitGroup
	wg.Add(5)
	go func() {
		defer wg.Done()
		ownerID, ownerSource = idx.resolveOwnerID(ctx, brandID)
	}()
	go func() {
		defer wg.Done()
		conversations, convErr = openrouter.CountConversationsByBrand(ctx, brandID)
	}()
	go func() {
		defer wg.Done()
		contentUsage, contentErr = trendlymodels.GetContentUsage(ctx, brandID)
	}()
	go func() {
		defer wg.Done()
		strategies, strategyErr = trendlymodels.CountStrategies(ctx, brandID)
	}()
	go func() {
		defer wg.Done()
		members = brandMemberUsage(ctx, brandID)
	}()
	wg.Wait()

	if convErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": convErr.Error()})
		return
	}
	if contentErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": contentErr.Error()})
		return
	}
	if strategyErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": strategyErr.Error()})
		return
	}

	detail := BrandUsageDetail{
		BrandUsageSummary: BrandUsageSummary{
			BrandID:         brandID,
			AIConversations: conversations,
			ContentTotal:    contentUsage.Total,
			Owner:           idx.owner(ownerID, ownerSource),
			Tokens:          idx.tokensFor(brandID),
		},
		StrategiesTotal: strategies,
		Content:         contentUsage,
		Members:         members,
	}

	c.JSON(http.StatusOK, detail)
}

// brandMemberUsage lists the brand's members with the device each was last seen
// on. Members whose manager document is missing are skipped rather than
// surfaced as blank rows.
//
// Each member needs its own Manager.Get — there is no batch-get in this
// codebase's Firestore wrapper — so for a brand with several members this ran
// as several SEQUENTIAL round trips. Fetching by index into a pre-sized slice
// lets those run concurrently while still assembling the result in the
// brand's original member order.
func brandMemberUsage(ctx context.Context, brandID string) []BrandMemberUsage {
	members, err := trendlymodels.GetAllBrandMembers(brandID)
	if err != nil {
		return []BrandMemberUsage{}
	}

	rows := make([]*BrandMemberUsage, len(members))
	var wg sync.WaitGroup
	sem := make(chan struct{}, usageFanout)
	for i, member := range members {
		wg.Add(1)
		go func(i int, member trendlymodels.BrandMember) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			var manager trendlymodels.Manager
			if err := manager.Get(member.ManagerID); err != nil {
				return
			}
			rows[i] = &BrandMemberUsage{
				ManagerID:        member.ManagerID,
				Name:             manager.Name,
				Email:            manager.Email,
				ProfileImage:     manager.ProfileImage,
				LastSeenPlatform: manager.LastSeenPlatform,
				LastSeenAt:       manager.LastSeenAt,
			}
		}(i, member)
	}
	wg.Wait()

	out := make([]BrandMemberUsage, 0, len(members))
	for _, row := range rows {
		if row != nil {
			out = append(out, *row)
		}
	}
	return out
}
