// Command seed provisions (or refreshes) the dev fixture the virality checks
// run against, and mints an ID token for it.
//
// Why this exists: Phase 0 of the Virality Engine changes what the model
// WRITES, and nothing about that is visible to a unit test or a green CI run.
// The only honest verification is to generate against the deployed dev stack
// and read the output. Doing that through a browser needs a password and a
// human; doing it this way needs neither, so it can run unattended and
// repeatably, and every run leaves a diffable transcript.
//
// The fixture is deliberately fixed-id and idempotent: re-running reuses the
// same manager, org, brand and content rather than littering dev with new
// ones. Everything it creates is named so it is obviously a test fixture.
//
// Usage, from the backend-sls root (service-account.json must resolve):
//
//	FIREBASE_WEB_API_KEY=... go run ./scripts/viralitycheck/seed
//
// Writes the fixture (including a short-lived ID token) to a file OUTSIDE the
// repository — it is a credential, and it has no business in a working tree.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/firebase/fauth"
)

// Fixed ids so the fixture is stable across runs and unmistakably ours.
const (
	fixtureManagerUID = "viralitycheck-manager"
	fixtureEmail      = "viralitycheck@idiv.in"
	fixtureBrandID    = "viralitycheck-brand"
	fixtureOrgName    = "Virality Check (automated test fixture)"
	fixtureBrandName  = "Northwind Coffee (test fixture)"
	fixturePlan       = "pro"
)

// Fixture is what the driver reads. The token is short-lived (one hour) and
// the file lands outside the repo, but treat it as a credential regardless.
type Fixture struct {
	IDToken    string   `json:"idToken"`
	UID        string   `json:"uid"`
	BrandID    string   `json:"brandId"`
	OrgID      string   `json:"orgId"`
	ContentIDs []string `json:"contentIds"`
	WSHost     string   `json:"wsHost"`
	MintedAt   string   `json:"mintedAt"`
}

func main() {
	out := flag.String("out", defaultOutPath(), "where to write the fixture JSON")
	wsHost := flag.String("ws", "cuowcrxmii.execute-api.us-east-1.amazonaws.com/dev", "dev websocket host")
	flag.Parse()

	apiKey := os.Getenv("FIREBASE_WEB_API_KEY")
	if apiKey == "" {
		log.Fatal("FIREBASE_WEB_API_KEY is required (the Firebase web apiKey — a public client key, found in trendly-brands/firebase-config.js)")
	}
	if dbID := os.Getenv("FIRESTORE_DATABASE_ID"); dbID != "" && dbID != "(default)" {
		log.Fatalf("refusing to run against FIRESTORE_DATABASE_ID=%q — this fixture is dev-only, and dev is the (default) database", dbID)
	}

	ctx := context.Background()

	uid, err := ensureAuthUser(ctx)
	if err != nil {
		log.Fatalf("auth user: %v", err)
	}
	log.Printf("auth user ready: %s", uid)

	if err := ensureManager(uid); err != nil {
		log.Fatalf("manager doc: %v", err)
	}

	orgID, err := ensureOrg(uid)
	if err != nil {
		log.Fatalf("organization: %v", err)
	}
	log.Printf("organization ready: %s (plan %s)", orgID, fixturePlan)

	if err := ensureBrand(orgID); err != nil {
		log.Fatalf("brand: %v", err)
	}
	if err := ensureMember(uid); err != nil {
		log.Fatalf("brand member: %v", err)
	}
	log.Printf("brand ready: %s", fixtureBrandID)

	contentIDs, err := ensureContents(ctx)
	if err != nil {
		log.Fatalf("contents: %v", err)
	}
	log.Printf("contents ready: %v", contentIDs)

	idToken, err := mintIDToken(ctx, uid, apiKey)
	if err != nil {
		log.Fatalf("id token: %v", err)
	}

	fx := Fixture{
		IDToken:    idToken,
		UID:        uid,
		BrandID:    fixtureBrandID,
		OrgID:      orgID,
		ContentIDs: contentIDs,
		WSHost:     *wsHost,
		MintedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	blob, err := json.MarshalIndent(fx, "", "  ")
	if err != nil {
		log.Fatalf("encode fixture: %v", err)
	}
	if err := os.WriteFile(*out, blob, 0o600); err != nil {
		log.Fatalf("write fixture: %v", err)
	}
	fmt.Println(*out)
}

func defaultOutPath() string {
	return filepath.Join(os.TempDir(), "trendly-viralitycheck-fixture.json")
}

// ensureAuthUser creates the fixture's Firebase user, or returns the existing
// one. The UID is fixed, so this is safe to call repeatedly.
func ensureAuthUser(ctx context.Context) (string, error) {
	u, err := fauth.Client.GetUser(ctx, fixtureManagerUID)
	if err == nil {
		return u.UID, nil
	}
	created, err := fauth.Client.CreateUser(ctx, (&auth.UserToCreate{}).
		UID(fixtureManagerUID).
		Email(fixtureEmail).
		DisplayName("Virality Check (test fixture)").
		EmailVerified(true))
	if err != nil {
		return "", err
	}
	return created.UID, nil
}

func ensureManager(uid string) error {
	var m trendlymodels.Manager
	if err := m.Get(uid); err == nil {
		return nil
	}
	m = trendlymodels.Manager{
		Name:  "Virality Check",
		Email: fixtureEmail,
	}
	_, err := m.Insert(uid)
	return err
}

// ensureOrg reuses the org already attached to the fixture brand when there is
// one, so repeated runs do not pile up organizations. ApplyPlanToOrg is what
// gives it entitlements and a funded token wallet — hand-rolling those fields
// would produce an org shaped unlike any real one, which is exactly the kind
// of fixture that makes a test pass and production fail.
func ensureOrg(ownerUID string) (string, error) {
	var existing trendlymodels.Brand
	if err := existing.Get(fixtureBrandID); err == nil &&
		existing.OrganizationID != nil && *existing.OrganizationID != "" {
		return *existing.OrganizationID, refreshPlan(*existing.OrganizationID)
	}

	org := trendlymodels.Organization{
		Name:         fixtureOrgName,
		OwnerID:      ownerUID,
		BrandIds:     []string{fixtureBrandID},
		CreationTime: time.Now().UnixMilli(),
	}
	orgID, err := org.Insert()
	if err != nil {
		return "", err
	}
	return orgID, refreshPlan(orgID)
}

// refreshPlan re-applies the plan on every run so a wallet drained by earlier
// testing does not silently turn the next run's result into "upgrade_required"
// — which would look like a product bug rather than an exhausted fixture.
//
// ApplyPlanToOrg alone is NOT enough, and the reason is worth writing down
// because it cost a confusing red run: it sets the top-level planKey,
// entitlements and wallet, but AI model routing reads org.Billing.PlanKey
// (modelpick.go brandPlan), which only the org-creation path writes. An org
// seeded without a billing block therefore resolves to the free plan and every
// AI task comes back "upgrade_required" with no reason attached. Real orgs are
// fine — provisionPersonalOrg sets both — so this is a fixture concern, not a
// product bug.
func refreshPlan(orgID string) error {
	if err := trendlymodels.ApplyPlanToOrg(orgID, fixturePlan, trendlymodels.NextMonthlyReset(time.Now())); err != nil {
		return err
	}
	plan, status := fixturePlan, "active"
	var org trendlymodels.Organization
	return org.SetBilling(orgID, &trendlymodels.OrgBilling{PlanKey: &plan, BillingStatus: &status})
}

func ensureBrand(orgID string) error {
	var b trendlymodels.Brand
	if err := b.Get(fixtureBrandID); err == nil {
		return nil
	}
	now := time.Now().UnixMilli()
	country := "IN"
	b = trendlymodels.Brand{
		Name:           fixtureBrandName,
		OrganizationID: &orgID,
		CreationTime:   &now,
		Country:        &country,
	}
	_, err := b.Insert(fixtureBrandID)
	return err
}

func ensureMember(uid string) error {
	var m trendlymodels.BrandMember
	if err := m.Get(fixtureBrandID, uid); err == nil {
		return nil
	}
	m = trendlymodels.BrandMember{ManagerID: uid, Status: 1}
	_, err := m.Set(fixtureBrandID)
	return err
}

// contentFixtures are the pieces the driver generates scripts for. One per
// platform family, because the whole point of Phase 0's L1 layer is that the
// advice differs by surface — a single Instagram fixture would not show
// whether targeting works at all.
var contentFixtures = []struct {
	id        string
	title     string
	platforms []string
	format    string
}{
	{"viralitycheck-reel", "Why our cold brew takes 18 hours", []string{"instagram"}, "reel"},
	{"viralitycheck-short", "The grinder mistake costing you flavour", []string{"youtube"}, "reel"},
	{"viralitycheck-linkedin", "What we learned opening a second location", []string{"linkedin"}, "video"},
	{"viralitycheck-twitter", "Our espresso pricing, broken down", []string{"twitter"}, "video"},
}

func ensureContents(ctx context.Context) ([]string, error) {
	ids := make([]string, 0, len(contentFixtures))
	for _, c := range contentFixtures {
		ids = append(ids, c.id)
		if _, err := trendlymodels.GetContent(fixtureBrandID, c.id); err == nil {
			continue
		}
		fields := map[string]interface{}{
			"title":         c.title,
			"platforms":     c.platforms,
			"contentFormat": c.format,
			"status":        "draft",
			"description":   "Automated fixture for the virality craft checks. Safe to delete.",
		}
		if err := trendlymodels.UpsertContentAt(ctx, fixtureBrandID, c.id, fields); err != nil {
			return nil, fmt.Errorf("content %s: %w", c.id, err)
		}
	}
	return ids, nil
}

// mintIDToken turns the admin SDK's custom token into the ID token the
// websocket's VerifyIDToken expects. No password is involved anywhere.
func mintIDToken(ctx context.Context, uid, apiKey string) (string, error) {
	customToken, err := fauth.Client.CustomToken(ctx, uid)
	if err != nil {
		return "", fmt.Errorf("custom token: %w", err)
	}

	body, _ := json.Marshal(map[string]any{"token": customToken, "returnSecureToken": true})
	url := "https://identitytoolkit.googleapis.com/v1/accounts:signInWithCustomToken?key=" + apiKey
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("identitytoolkit %d: %s", resp.StatusCode, raw)
	}
	var out struct {
		IDToken string `json:"idToken"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	if out.IDToken == "" {
		return "", fmt.Errorf("identitytoolkit returned no idToken: %s", raw)
	}
	return out.IDToken, nil
}
