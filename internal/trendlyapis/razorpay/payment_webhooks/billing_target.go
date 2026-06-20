package paymentwebhooks

import (
	"errors"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
)

// billingTarget is the organization that a billing webhook applies to. Billing
// lives solely on the Organization (Brand no longer carries billing).
type billingTarget struct {
	orgID string
	org   *trendlymodels.Organization
}

// resolveBillingTarget resolves the target org from webhook notes that may
// carry organizationId and/or brandId. It prefers organizationId; if only
// brandId is present (legacy subscriptions), it loads the brand to discover the
// owning org.
func resolveBillingTarget(notes webhook.SubscriptionNotes) (*billingTarget, error) {
	orgID := notes.OrganizationID

	if orgID == "" && notes.BrandID != "" {
		b := &trendlymodels.Brand{}
		if err := b.Get(notes.BrandID); err == nil && b.OrganizationID != nil {
			orgID = *b.OrganizationID
		}
	}

	if orgID == "" {
		return nil, errors.New("no-billing-target")
	}

	o := &trendlymodels.Organization{}
	if err := o.Get(orgID); err != nil {
		return nil, err
	}
	return &billingTarget{orgID: orgID, org: o}, nil
}

// currentBilling returns the org billing object to mutate, initialising it if
// the org has no billing shell yet.
func (t *billingTarget) currentBilling() *trendlymodels.BrandBilling {
	if t.org.Billing == nil {
		t.org.Billing = &trendlymodels.BrandBilling{}
	}
	return t.org.Billing
}

// save persists the (already-mutated) billing to the org.
func (t *billingTarget) save(billing *trendlymodels.BrandBilling) error {
	return t.org.SetBilling(t.orgID, billing)
}
