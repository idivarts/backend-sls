package paymentwebhooks

import (
	"errors"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/payments/webhook"
)

// billingTarget is the org (preferred) + brand that a billing webhook applies
// to. Billing has moved Brand -> Organization, so the Organization is the
// source of truth; during the transition window we also mirror the billing onto
// the brand so existing brand-level billing reads keep working until the
// frontend reads org billing.
type billingTarget struct {
	orgID   string
	org     *trendlymodels.Organization
	brandID string
	brand   *trendlymodels.Brand
}

// resolveBillingTarget resolves the target from webhook notes that may carry
// organizationId and/or brandId. It prefers organizationId, then falls back to
// the brand's organizationId (for legacy subscriptions that only carry brandId).
// At least one of org/brand must resolve.
func resolveBillingTarget(notes webhook.SubscriptionNotes) (*billingTarget, error) {
	t := &billingTarget{orgID: notes.OrganizationID, brandID: notes.BrandID}

	if t.brandID != "" {
		b := &trendlymodels.Brand{}
		if err := b.Get(t.brandID); err == nil {
			t.brand = b
			if t.orgID == "" && b.OrganizationID != nil {
				t.orgID = *b.OrganizationID
			}
		}
	}

	if t.orgID != "" {
		o := &trendlymodels.Organization{}
		if err := o.Get(t.orgID); err == nil {
			t.org = o
		}
	}

	if t.org == nil && t.brand == nil {
		return nil, errors.New("no-billing-target")
	}
	return t, nil
}

// currentBilling returns the billing object to mutate — the org's when an org is
// resolved (the new source of truth), otherwise the brand's (legacy path).
func (t *billingTarget) currentBilling() *trendlymodels.BrandBilling {
	if t.org != nil {
		if t.org.Billing == nil {
			t.org.Billing = &trendlymodels.BrandBilling{}
		}
		return t.org.Billing
	}
	if t.brand.Billing == nil {
		t.brand.Billing = &trendlymodels.BrandBilling{}
	}
	return t.brand.Billing
}

// save persists the (already-mutated) billing to the org (source of truth) and
// mirrors it onto the brand during the transition window.
func (t *billingTarget) save(billing *trendlymodels.BrandBilling) error {
	if t.org != nil {
		if err := t.org.SetBilling(t.orgID, billing); err != nil {
			return err
		}
	}
	if t.brand != nil {
		t.brand.Billing = billing
		if _, err := t.brand.Insert(t.brandID); err != nil {
			return err
		}
	}
	return nil
}
