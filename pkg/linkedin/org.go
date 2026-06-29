package linkedin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// Organization is a LinkedIn Company Page the connected member administers.
type Organization struct {
	URN        string `json:"urn"`        // urn:li:organization:{id}
	ID         string `json:"id"`         // numeric id
	Name       string `json:"name"`       // localized name (best-effort)
	VanityName string `json:"vanityName"` // human-readable slug (best-effort)
	LogoURL    string `json:"logoUrl"`    // best-effort
}

// GetAdministeredOrg returns the first Company Page the authenticated member is
// an APPROVED ADMINISTRATOR of, via the organizationAcls API. Requires the
// rw_organization_admin (or r_organization_admin) scope + CMA approval. Returns
// (nil, nil) when the member administers no page; an error only on transport/API
// failure. Name/logo are filled best-effort and may be empty.
func GetAdministeredOrg(accessToken string) (*Organization, error) {
	urn, err := firstAdministeredOrgURN(accessToken)
	if err != nil {
		return nil, err
	}
	if urn == "" {
		return nil, nil
	}
	org := &Organization{URN: urn, ID: orgIDFromURN(urn)}
	// Best-effort enrich with name + vanity + logo; ignore failures.
	if name, vanity, logo, derr := organizationDetails(accessToken, org.ID); derr == nil {
		org.Name = name
		org.VanityName = vanity
		org.LogoURL = logo
	}
	return org, nil
}

// ListAdministeredOrgs returns ALL Company/Showcase Pages the authenticated
// member is an APPROVED ADMINISTRATOR of, enriched with name/vanity/logo
// (best-effort). Powers the page picker. Requires rw_organization_admin + CMA.
func ListAdministeredOrgs(accessToken string) ([]Organization, error) {
	urns, err := administeredOrgURNs(accessToken)
	if err != nil {
		return nil, err
	}
	out := make([]Organization, 0, len(urns))
	for _, urn := range urns {
		org := Organization{URN: urn, ID: orgIDFromURN(urn)}
		if name, vanity, logo, derr := organizationDetails(accessToken, org.ID); derr == nil {
			org.Name = name
			org.VanityName = vanity
			org.LogoURL = logo
		}
		if org.Name == "" {
			org.Name = "LinkedIn Page " + org.ID
		}
		out = append(out, org)
	}
	return out, nil
}

// administeredOrgURNs returns every APPROVED ADMINISTRATOR organization URN.
func administeredOrgURNs(accessToken string) ([]string, error) {
	u := RestBaseURL + "/organizationAcls?q=roleAssignee&role=ADMINISTRATOR&state=APPROVED"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("linkedin: build organizationAcls request: %w", err)
	}
	restHeaders(req, accessToken)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("linkedin: organizationAcls request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("linkedin: organizationAcls returned %d: %s", resp.StatusCode, string(body))
	}
	var parsed struct {
		Elements []struct {
			Organization string `json:"organization"`
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("linkedin: parse organizationAcls: %w", err)
	}
	urns := make([]string, 0, len(parsed.Elements))
	for _, e := range parsed.Elements {
		if e.Organization != "" {
			urns = append(urns, e.Organization)
		}
	}
	return urns, nil
}

// firstAdministeredOrgURN queries organizationAcls for the member's APPROVED
// ADMINISTRATOR roles and returns the first organization URN.
func firstAdministeredOrgURN(accessToken string) (string, error) {
	u := RestBaseURL + "/organizationAcls?q=roleAssignee&role=ADMINISTRATOR&state=APPROVED"
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", fmt.Errorf("linkedin: build organizationAcls request: %w", err)
	}
	restHeaders(req, accessToken)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("linkedin: organizationAcls request failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("linkedin: organizationAcls returned %d: %s", resp.StatusCode, string(body))
	}

	var parsed struct {
		Elements []struct {
			Organization string `json:"organization"` // urn:li:organization:{id}
		} `json:"elements"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("linkedin: parse organizationAcls: %w", err)
	}
	if len(parsed.Elements) == 0 {
		return "", nil
	}
	return parsed.Elements[0].Organization, nil
}

// organizationDetails fetches a page's localized name + vanity + logo (best-effort).
func organizationDetails(accessToken, orgID string) (name, vanityName, logoURL string, err error) {
	if orgID == "" {
		return "", "", "", fmt.Errorf("linkedin: empty org id")
	}
	u := RestBaseURL + "/organizations/" + orgID
	req, rerr := http.NewRequest(http.MethodGet, u, nil)
	if rerr != nil {
		return "", "", "", rerr
	}
	restHeaders(req, accessToken)
	resp, derr := http.DefaultClient.Do(req)
	if derr != nil {
		return "", "", "", derr
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", "", fmt.Errorf("linkedin: organizations/%s returned %d", orgID, resp.StatusCode)
	}
	var parsed struct {
		LocalizedName string `json:"localizedName"`
		VanityName    string `json:"vanityName"`
		Name          struct {
			Localized map[string]string `json:"localized"`
		} `json:"name"`
	}
	if jerr := json.Unmarshal(body, &parsed); jerr != nil {
		return "", "", "", jerr
	}
	name = parsed.LocalizedName
	if name == "" {
		for _, v := range parsed.Name.Localized {
			name = v
			break
		}
	}
	return name, parsed.VanityName, "", nil
}

// orgIDFromURN extracts the numeric id from urn:li:organization:{id}.
func orgIDFromURN(urn string) string {
	parts := strings.Split(urn, ":")
	id := parts[len(parts)-1]
	if _, err := strconv.Atoi(id); err == nil {
		return id
	}
	return id
}
