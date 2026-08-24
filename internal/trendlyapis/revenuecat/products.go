// Package revenuecat handles native In-App Purchase (Apple App Store / Google
// Play) billing via RevenueCat. RevenueCat validates the store receipts and
// posts server-to-server webhook events here; this package maps those events
// onto the SAME org billing engine the Razorpay webhooks use
// (trendlymodels.ApplyPlanToOrg for subscriptions, AddTopup for token packs).
//
// See the IAP ticket + docs/revenuecat-iap-setup.md.
package revenuecat

// The RevenueCat App User ID we log in with is the organizationId, so every
// purchase attaches to the org (billing is org-level, not per-manager).

// productPlanKey maps a store subscription product identifier to our internal
// plan key (drives PlanLimitsMap → entitlements + token allotment). Entitlements
// are identical across providers — only the store price differs — so these keys
// are the same "pro"/"team" the web/Razorpay path uses.
//
// ⚠️ The keys MUST match the product identifiers configured in App Store Connect,
// Google Play Console, and RevenueCat. Add both the iOS and Android identifiers
// here if they differ per store. Update this map (not the billing engine) when
// adding a new plan or renaming a store product.
var productPlanKey = map[string]string{
	"trendly_pro_monthly":  "pro",
	"trendly_team_monthly": "team",
	// Android product ids often carry a base-plan suffix; map any aliases here.
	"trendly_pro_monthly:monthly":  "pro",
	"trendly_team_monthly:monthly": "team",
}

// topupProductTokens maps a store consumable (token top-up pack) product
// identifier to the number of wallet tokens it grants. The token grant is
// provider-independent (a pack is worth the same tokens however it was bought);
// only the store price differs. See the Credit ticket: $10 → 1M tokens.
var topupProductTokens = map[string]int64{
	"trendly_topup_1m": 1_000_000,
}

// PlanKeyForProduct resolves a subscription product id to a plan key. Falls back
// to matching on the RevenueCat entitlement ids when the product id is unknown
// (RC entitlements are configured as "pro"/"team"). Returns "" when nothing
// matches, so the caller can skip applying an unknown plan.
func PlanKeyForProduct(productID string, entitlementIDs []string) string {
	if k, ok := productPlanKey[productID]; ok {
		return k
	}
	// Fallback: pick the highest tier present in the entitlement ids.
	best := ""
	for _, e := range entitlementIDs {
		switch e {
		case "team":
			return "team"
		case "pro":
			best = "pro"
		}
	}
	return best
}

// TopupTokensForProduct resolves a consumable product id to its token grant
// (0 when the product id is not a known top-up pack).
func TopupTokensForProduct(productID string) int64 {
	return topupProductTokens[productID]
}

// storeLabel normalizes RevenueCat's store string ("APP_STORE" / "PLAY_STORE" /
// "MAC_APP_STORE" / …) to our short "apple" | "google" label used on
// Billing.Store. Anything else passes through lowercased.
func storeLabel(store string) string {
	switch store {
	case "APP_STORE", "MAC_APP_STORE":
		return "apple"
	case "PLAY_STORE":
		return "google"
	default:
		return ""
	}
}
