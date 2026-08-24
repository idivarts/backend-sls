package tools

import (
	"context"

	"github.com/idivarts/backend-sls/internal/constants"
	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func connectedAccounts() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_connected_accounts",
			"List the brand's connected social accounts with platform, handle, follower count and whether a usable access token is present (so you don't plan for a disconnected platform).",
			openrouter.ObjectSchema(map[string]any{}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			accs, err := trendlymodels.ListBrandSocialAccounts(brandID)
			if err != nil {
				return nil, err
			}
			out := make([]map[string]any, 0, len(accs))
			for _, a := range accs {
				// LinkedIn Page is gated (CMA app review pending) — hide it from the
				// AI's connected-accounts context so it never plans/generates for it.
				if a.Platform == trendlymodels.PlatformLinkedInPage && !constants.LinkedInPageEnabled {
					continue
				}
				tok, terr := trendlymodels.GetBrandSocialToken(brandID, a.ID)
				hasToken := terr == nil && tok != nil && tok.AccessToken != ""
				out = append(out, map[string]any{
					"id": a.ID, "platform": a.Platform, "username": a.Username,
					"displayName": a.DisplayName, "followerCount": a.FollowerCount,
					"mediaCount": a.MediaCount, "connectedAt": a.ConnectedAt, "hasToken": hasToken,
				})
			}
			return map[string]any{"count": len(out), "accounts": out}, nil
		},
	}
}

func accountProfile() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_account_profile",
			"Fetch profile details for one connected account: bio, follower/following/media counts, account type and profile URL.",
			openrouter.ObjectSchema(map[string]any{
				"socialId": openrouter.StringProp("The connected-account ID."),
			}, []string{"socialId"}),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			id := argStr(args, "socialId")
			if id == "" {
				return map[string]any{"error": "socialId is required"}, nil
			}
			a, err := trendlymodels.GetBrandSocialAccount(brandID, id)
			if err != nil {
				return nil, err
			}
			if a.Platform == trendlymodels.PlatformLinkedInPage && !constants.LinkedInPageEnabled {
				return map[string]any{"error": "this account's platform is not currently available"}, nil
			}
			return map[string]any{
				"id": a.ID, "platform": a.Platform, "username": a.Username, "displayName": a.DisplayName,
				"bio": a.Bio, "profileUrl": a.ProfileURL, "followerCount": a.FollowerCount,
				"followingCount": a.FollowingCount, "mediaCount": a.MediaCount, "accountType": a.AccountType,
			}, nil
		},
	}
}
