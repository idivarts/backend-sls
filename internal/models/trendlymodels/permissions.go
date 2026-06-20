package trendlymodels

// Brand access is modelled as Features held by Teams. A Team grants, per Feature,
// a set of Privileges (the "toggle set"). A brand member belongs to exactly one
// team and automatically inherits that team's feature privileges — there are no
// member-level roles or overrides. See the "Repurpose Teams inside brands"
// roadmap ticket for the full feature × privilege matrix.

// Feature is a product area a team can be granted access to.
type Feature string

const (
	FeatureStrategy            Feature = "strategy"
	FeatureContentCalendar     Feature = "content_calendar"
	FeatureContent             Feature = "content"
	FeatureSocialAccounts      Feature = "social_accounts"
	FeatureInfluencerMarketing Feature = "influencer_marketing"
	FeatureGrowth              Feature = "growth"
	// FeatureBrandAdmin governs brand administration — members, teams, billing
	// and brand deletion. The default team always holds it so a brand can never
	// be locked out of its own settings.
	FeatureBrandAdmin Feature = "brand_admin"
)

// Privilege is a single grant within a feature. Values are only unique within a
// feature (e.g. both Strategy and Social Accounts have an "admin" privilege);
// always resolve a privilege together with its feature.
type Privilege string

const (
	// Strategy
	PrivStrategyAdmin    Privilege = "admin"    // create + edit
	PrivStrategyEditor   Privilege = "editor"   // edit when invited to a strategy
	PrivStrategyApprover Privilege = "approver" // comment / approve in Review
	PrivStrategyViewer   Privilege = "viewer"   // view + comment

	// Content Calendar
	PrivCalendarEditor  Privilege = "editor"  // move dates / edit on the calendar
	PrivCalendarView    Privilege = "view"    // view + comment
	PrivCalendarPublish Privilege = "publish" // publish / schedule a post

	// Content
	PrivContentCreateEdit Privilege = "create_edit" // create + edit
	PrivContentEditor     Privilege = "editor"      // edit only
	PrivContentView       Privilege = "view"        // view + comment

	// Social Accounts
	PrivSocialAdmin     Privilege = "admin"     // add / remove accounts
	PrivSocialInbox     Privilege = "inbox"     // access the connected-account inbox
	PrivSocialAnalytics Privilege = "analytics" // access account analytics
	PrivSocialView      Privilege = "view"      // view connected accounts

	// Influencer Marketing
	PrivInfluencerAdmin    Privilege = "admin"    // full control incl. funding contracts
	PrivInfluencerManage   Privilege = "manage"   // run collabs / contracts / discovery
	PrivInfluencerApprover Privilege = "approver" // approve only

	// Growth
	PrivGrowthAllAccess Privilege = "all_access" // unlock the growth pages

	// Brand Admin
	PrivAdminMembers     Privilege = "members"      // invite / edit / remove members
	PrivAdminTeams       Privilege = "teams"        // create / edit / delete teams
	PrivAdminBilling     Privilege = "billing"      // view & manage billing
	PrivAdminDeleteBrand Privilege = "delete_brand" // delete the brand
)

// validFeaturePrivileges is the canonical set of privileges grantable per
// feature. It is the single source of truth for validation, the access-control
// UI, and seeding the default team.
var validFeaturePrivileges = map[Feature][]Privilege{
	FeatureStrategy:            {PrivStrategyAdmin, PrivStrategyEditor, PrivStrategyApprover, PrivStrategyViewer},
	FeatureContentCalendar:     {PrivCalendarEditor, PrivCalendarView, PrivCalendarPublish},
	FeatureContent:             {PrivContentCreateEdit, PrivContentEditor, PrivContentView},
	FeatureSocialAccounts:      {PrivSocialAdmin, PrivSocialInbox, PrivSocialAnalytics, PrivSocialView},
	FeatureInfluencerMarketing: {PrivInfluencerAdmin, PrivInfluencerManage, PrivInfluencerApprover},
	FeatureGrowth:              {PrivGrowthAllAccess},
	FeatureBrandAdmin:          {PrivAdminMembers, PrivAdminTeams, PrivAdminBilling, PrivAdminDeleteBrand},
}

// IsValid reports whether f is a known feature.
func (f Feature) IsValid() bool {
	_, ok := validFeaturePrivileges[f]
	return ok
}

// IsValidPrivilege reports whether priv is grantable under feature.
func IsValidPrivilege(feature Feature, priv Privilege) bool {
	for _, p := range validFeaturePrivileges[feature] {
		if p == priv {
			return true
		}
	}
	return false
}

// FilterValidPrivileges returns the subset of privs that are valid for feature,
// de-duplicated and as plain strings (Firestore shape).
func FilterValidPrivileges(feature Feature, privs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, p := range privs {
		if IsValidPrivilege(feature, Privilege(p)) && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

// AllFeaturePrivilegesMap returns every feature mapped to all of its privileges
// (as the Firestore map[string][]string shape). Used to seed the default Admin
// team, which always holds full access.
func AllFeaturePrivilegesMap() map[string][]string {
	return FeaturePrivilegesMapExcept()
}

// FeaturePrivilegesMapExcept returns every feature (except the excluded ones)
// mapped to all of its privileges, in the Firestore map[string][]string shape.
// Used to seed the default Editor team, which holds full access to everything
// except brand administration.
func FeaturePrivilegesMapExcept(exclude ...Feature) map[string][]string {
	skip := map[Feature]bool{}
	for _, f := range exclude {
		skip[f] = true
	}
	out := map[string][]string{}
	for feature, privs := range validFeaturePrivileges {
		if skip[feature] {
			continue
		}
		list := make([]string, len(privs))
		for i, p := range privs {
			list[i] = string(p)
		}
		out[string(feature)] = list
	}
	return out
}

// viewPrivileges maps each feature to its read-only ("view") privilege, where one
// exists. Features whose privilege set has no pure view/observer grant
// (Influencer Marketing, Growth, Brand Admin) are intentionally absent, so a
// view-only team receives no access to them.
var viewPrivileges = map[Feature]Privilege{
	FeatureStrategy:        PrivStrategyViewer,
	FeatureContentCalendar: PrivCalendarView,
	FeatureContent:         PrivContentView,
	FeatureSocialAccounts:  PrivSocialView,
}

// ViewOnlyFeaturePrivilegesMap returns each viewable feature mapped to only its
// view privilege, in the Firestore map[string][]string shape. Used to seed the
// default Viewer team.
func ViewOnlyFeaturePrivilegesMap() map[string][]string {
	out := map[string][]string{}
	for feature, priv := range viewPrivileges {
		out[string(feature)] = []string{string(priv)}
	}
	return out
}
