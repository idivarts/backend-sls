package trendlymodels

// BrandRole is the preset role a brand member holds. Roles are split across the
// app's two pillars — influencer/collab and content/strategy — while Owner and
// Admin span both. See the "A better privilege control for brand members"
// roadmap ticket for the full role × capability matrix.
type BrandRole string

const (
	RoleOwner           BrandRole = "owner"
	RoleAdmin           BrandRole = "admin"
	RoleCampaignManager BrandRole = "campaign_manager"
	RoleContentManager  BrandRole = "content_manager"
	RoleContentCreator  BrandRole = "content_creator"
	RoleViewer          BrandRole = "viewer"
)

// IsValid reports whether r is a known role.
func (r BrandRole) IsValid() bool {
	switch r {
	case RoleOwner, RoleAdmin, RoleCampaignManager, RoleContentManager, RoleContentCreator, RoleViewer:
		return true
	}
	return false
}

// Capability is a single permission a member may hold within a brand. Viewing
// (read access) is implicit for every role and has no capability constant.
type Capability string

const (
	// Influencer & Collab pillar
	CapManageCollaborations Capability = "manage_collaborations"
	CapManageContracts      Capability = "manage_contracts"
	CapDiscoveryMessaging   Capability = "discovery_messaging"
	CapFundContracts        Capability = "fund_contracts"

	// Content & Strategy pillar
	CapManageContentStrategy Capability = "manage_content_strategy"
	CapManageContent         Capability = "manage_content"
	CapPublishContent        Capability = "publish_content"
	CapDeleteContent         Capability = "delete_content"

	// Admin & settings
	CapManageMembers  Capability = "manage_members"
	CapManageTeams    Capability = "manage_teams"
	CapConnectSocials Capability = "connect_socials"
	CapManageBilling  Capability = "manage_billing"
	CapDeleteBrand    Capability = "delete_brand"
)

// roleCapabilities is the default capability set granted by each role. Owner is
// granted everything implicitly (handled in BrandMember.HasCapability) and is
// therefore not listed here.
var roleCapabilities = map[BrandRole]map[Capability]bool{
	RoleAdmin: {
		CapManageCollaborations:  true,
		CapManageContracts:       true,
		CapDiscoveryMessaging:    true,
		CapFundContracts:         true,
		CapManageContentStrategy: true,
		CapManageContent:         true,
		CapPublishContent:        true,
		CapDeleteContent:         true,
		CapManageMembers:         true,
		CapManageTeams:           true,
		CapConnectSocials:        true,
	},
	RoleCampaignManager: {
		CapManageCollaborations: true,
		CapManageContracts:      true,
		CapDiscoveryMessaging:   true,
	},
	RoleContentManager: {
		CapManageContentStrategy: true,
		CapManageContent:         true,
		CapPublishContent:        true,
		CapDeleteContent:         true,
	},
	RoleContentCreator: {
		CapManageContent: true,
	},
	RoleViewer: {},
}

// OverridableCapabilities are the sensitive capabilities a brand can grant or
// revoke per-member independently of their base role (the "toggles").
var OverridableCapabilities = []Capability{
	CapFundContracts,
	CapPublishContent,
	CapManageMembers,
	CapConnectSocials,
	CapManageBilling,
}

// IsOverridable reports whether cap may be set as a per-member override toggle.
func (c Capability) IsOverridable() bool {
	for _, oc := range OverridableCapabilities {
		if oc == c {
			return true
		}
	}
	return false
}
