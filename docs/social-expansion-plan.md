# Social Expansion — Phased Implementation Plan

> Covers all 11 Notion tickets: LinkedIn (Messaging/Media/Insights), Twitter/X
> (Posting/Messaging/Media/Insights), YouTube (Posting/Insights), Reddit (all).
> Spans **backend-sls** (Go) and **trendly-brands** (Expo). Built to mirror the
> existing Facebook/Instagram implementation. Created 2026-06-29.
>
> Dashboard/OAuth setup is **assumed done** — see
> `docs/social-expansion-dashboard-setup.md`. Code ships behind those approvals.

## Guiding rules (no shortcuts)
- All Firestore access via `internal/models/trendlymodels/*` models (standing rule).
- Keep `firestore.rules` + **both** index files in sync with any new collection/query.
- Per-platform logic dispatched by `switch platform` in the existing spine files —
  reuse the FB/IG normalized models (`InboxConversation`, `AccountAnalytics`,
  `Content`), don't fork new ones.
- Every capability degrades gracefully where the platform API can't deliver
  (LinkedIn DMs, Reddit analytics, Twitter reach) — no dead UI, clear states.
- Build gates: `go build ./...` (backend) and `tsc`/expo typecheck (frontend) must
  pass at the end of each wave.

---

## WAVE 0 — Foundation & scaffolding (backend)

- **P0.1** Add `PlatformReddit = "reddit"` to `trendlymodels/social_v2.go`.
- **P0.2** Extend `NormalizePlatform`/`NormalizePlatforms` in `content_format.go`
  to accept `reddit`; add reddit to `FormatPlatformSupport` (post, text, +link/image).
- **P0.3** `social_connect/scopes.go`: add `TwitterWriteScopes`/`TwitterDMScopes`
  additions, `YouTubeUploadScopes`, `LinkedInOrgScopes`, `RedditScopes` constants.
- **P0.4** `serverless.trendly.yml`: add `REDDIT_CLIENT_ID`, `REDDIT_CLIENT_SECRET`,
  `REDDIT_USER_AGENT` to `provider.environment` (global). **Also** wire them in
  `.github/workflows/deploy-trendly.yaml` (`vars.REDDIT_CLIENT_ID`,
  `secrets.REDDIT_CLIENT_SECRET`, `vars.REDDIT_USER_AGENT`) and add the matching
  GitHub Actions Variables/Secrets — see the dashboard setup doc §5.
- **P0.5** Expand existing connect scopes in-place: Twitter init adds
  `tweet.write media.write dm.read dm.write`; YouTube init adds `youtube.upload`
  + `youtube.force-ssl`; LinkedIn init optionally requests org scopes.

## WAVE 1 — Reddit connect (only platform with no connect yet)

- **P1.1** `pkg/reddit/var.go` — endpoints, scopes, env (ClientID/Secret/UserAgent).
- **P1.2** `pkg/reddit/token.go` — `ExchangeCode`, `RefreshAccessToken`, `ExpiresAt`
  (HTTP Basic auth, confidential client, `duration=permanent`).
- **P1.3** `pkg/reddit/profile.go` — `GetMe` (`/api/v1/me`: name, karma, icon).
- **P1.4** `social_connect/reddit.go` — `RedditInit` + `RedditCallback` (mirror twitter.go).
- **P1.5** Routes in `functions/unauth_apis/main.go` (`/connect/reddit`, `/callback`).
- **P1.6** `social_connect/scopes.go` — `RedditScopes`.

## WAVE 2 — Posting (Twitter, YouTube, Reddit)

### Twitter posting
- **P2.1** `pkg/twitter/media.go` — v2 chunked media upload (INIT/APPEND/FINALIZE),
  status poll for video; download source bytes from URL.
- **P2.2** `pkg/twitter/publish.go` — `CreateTweet(accessToken, text, mediaIDs)` +
  `PublishTweet(accessToken, text, imageURLs, videoURL)` wrapper (≤280 chars,
  ≤4 images, single video).
- **P2.3** `publishing/publish.go` — `publishToTwitter` + `case "twitter"`.

### YouTube posting
- **P2.4** `pkg/youtube/publish.go` — resumable upload (`videos.insert`), metadata
  (title/description/tags/privacyStatus/publishAt), `SetThumbnail`; stream bytes
  from URL. Shorts = vertical + `#Shorts`.
- **P2.5** `publishing/publish.go` — `publishToYouTube` + `case "youtube"`
  (require a video attachment; title from `ct.Title`).
- **P2.6** Decide & implement scheduling: native `publishAt` (private→auto-public).

### Reddit posting
- **P2.7** `pkg/reddit/publish.go` — `Submit` (self/link/image), image upload-lease
  flow; parse Reddit's in-200-OK error bodies.
- **P2.8** `publishing/publish.go` — `publishToReddit` + `case "reddit"`
  (needs subreddit + title from content destination/fields).
- **P2.9** Content model: add optional `RedditSubreddit`, `RedditFlairID`,
  `YouTubeTitle`/`YouTubePrivacy` fields (or a generic `PlatformOptions` map) to
  `trendlymodels/content.go`; thread through publish + frontend.

## WAVE 3 — Inbox spine refactor (enables non-Meta DMs & comments)

- **P3.1** Add platform-neutral DM upsert to `inbox/service.go`:
  `upsertNeutralDMConversation(brandID, s, contact, messages, replyWindow)` that
  doesn't depend on `facebook.ConversationMessagesData`.
- **P3.2** Make `isInboxChannel` channel-aware per capability: split into
  `isDMChannel(p)` and `isCommentChannel(p)` so DMs and comments can enable
  different platform sets.
- **P3.3** Make reply-window logic channel-aware (24h only for Meta).
- **P3.4** `inbox/media.go`: generalize media listing/comment dispatch to switch
  on platform (currently IG/FB only).

## WAVE 4 — Messaging Inbox (Twitter, Reddit, LinkedIn)

### Twitter DMs
- **P4.1** `pkg/twitter/messaging.go` — `GetDMEvents`, `GetUserByID`, `SendDM`.
- **P4.2** `inbox/service.go` — twitter branch in `SyncFromMeta`/`syncAccountDMs`
  → neutral upsert; twitter branch in `Reply` (DM).
- **P4.3** Poll cadence: add a low-frequency twitter DM sync via `social_sqs`
  (respect 15/15min cap); enable on cold-load like Meta.

### Reddit PMs (read-only)
- **P4.4** `pkg/reddit/messaging.go` — `GetInbox` (PMs); `ComposeMessage`
  (best-effort, gated).
- **P4.5** `inbox/service.go` — reddit branch (read-only); do NOT enable reply if
  compose unavailable.

### LinkedIn messaging (graceful unsupported)
- **P4.6** Ensure LinkedIn is **excluded** from `isDMChannel`; add regression note.
  Frontend shows no LinkedIn DM channel. (No backend sync path — documented.)

## WAVE 5 — Media Inbox / comments (LinkedIn, Twitter, Reddit)

### LinkedIn comments
- **P5.1** `pkg/linkedin/comments.go` — `ListOrgPosts`, `GetComments`,
  `CreateCommentReply`, `DeleteComment` (CMA Social Actions API, org URN).
- **P5.2** `inbox/media.go` — linkedin branch (list posts + comments + reply/delete;
  hide conditionally hidden).

### Twitter replies/mentions
- **P5.3** `pkg/twitter/replies.go` — `GetUserTweets`, `GetReplies`
  (search conversation_id), `GetMentions`, `ReplyToTweet`.
- **P5.4** `inbox/media.go` — twitter branch (tweets as media; replies as comments;
  delete-own only; no hide).

### Reddit comments
- **P5.5** `pkg/reddit/comments.go` — `GetUserSubmissions`, `GetComments`,
  `ReplyToComment`, `EditComment`, `DeleteComment`.
- **P5.6** `inbox/media.go` — reddit branch.

## WAVE 6 — Insights (LinkedIn, Twitter, YouTube, Reddit)

- **P6.1** `analytics` dispatcher: replace the IG/FB if-chain in
  `getAccountAnalytics` (handlers.go) with a `switch platform` that calls
  `fetchLinkedIn/fetchTwitter/fetchYouTube/fetchReddit`.
- **P6.2** `pkg/linkedin/insights.go` + `analytics/linkedin.go` (`fetchLinkedIn`):
  follower/share/page stats → buckets + demographics; `Supported=true` org-only.
- **P6.3** `pkg/youtube/insights.go` + `analytics/youtube.go` (`fetchYouTube`):
  channel stats + analytics reports (views/watch-time/engagement) + demographics +
  top videos; `Supported=true`.
- **P6.4** `pkg/twitter/insights.go` + `analytics/twitter.go` (`fetchTwitter`):
  followers + aggregated tweet impressions/engagement + top tweets; reach/demos
  marked unavailable; `Supported=true`.
- **P6.5** `analytics/reddit.go` (`fetchReddit`): derived karma/score/comments from
  listings; clearly-labeled; reach/impressions/demographics marked unavailable.
- **P6.6** `analytics/snapshot.go`: extend platform filter to include linkedin,
  twitter, youtube (reddit derived snapshot optional).

## WAVE 7 — Frontend wiring (trendly-brands)

- **P7.1** `shared-libs/.../constants/platform.ts` — add `Reddit`.
- **P7.2** `contexts/brand-social-context.provider.tsx` — add `reddit` to union.
- **P7.3** `constants/Socials.ts` — reddit meta (icon `faReddit`, color, blurb);
  fix LinkedIn blurb (comments+insights, not messaging).
- **P7.4** `shared-uis/constants/Colors.ts` — ensure `socialReddit`.
- **P7.5** Inbox: `components/inbox/types.ts` split DM vs media channels; add
  twitter/reddit to DM, linkedin/twitter/reddit to media; update
  `data/use-inbox.api.ts` + `data/use-inbox-media.ts` whitelists; channel
  icon/label maps in `utils.ts`.
- **P7.6** Posting: `components/contents/detail/ScheduleBar.tsx` — add
  twitter/youtube/reddit to `PUBLISHABLE` + `platformDotColor`; per-platform compose
  fields (YouTube title/visibility, Reddit subreddit/title/flair); 280-char counter
  when Twitter selected; disable platforms for incompatible formats.
- **P7.7** Analytics: verify `platformMeta()` covers all; render `available=false`
  metrics + non-Meta demographics gracefully; "derived metrics" note for Reddit.
- **P7.8** Connected accounts: ensure new providers render; honest capability blurbs.

## WAVE 8 — Firestore rules / indexes / config

- **P8.1** Review `firestore.rules` — inbox/inboxMedia/analytics collections are
  platform-agnostic + server-only; confirm no rule change needed (document).
- **P8.2** Any new query (e.g. twitter DM poll cursor, reddit submissions) →
  add composite index to **both** `firestore.indexes.json` and
  `firestore.indexes.enterprise.json`. Single-field → `gcloud` migration script.
- **P8.3** New content fields (subreddit/title/options) need no index.

## WAVE 9 — Build, typecheck, self-verify

- **P9.1** `go build ./...` in backend-sls — fix all compile errors.
- **P9.2** `go vet ./...` on touched packages.
- **P9.3** Frontend typecheck (tsc / expo) — fix type errors.
- **P9.4** Update Notion tickets → status **Review / On Hold**; note assumptions
  (pending approvals) per ticket.
- **P9.5** Update Postman collection for new endpoints (optional).
- **P9.6** Update root `CLAUDE.md` §8 + knowledge-graph with new subsystems.

---

## Per-ticket → wave mapping
| Ticket | Waves |
|---|---|
| LinkedIn Messaging | 3, 4.6, 7.5 (graceful unsupported) |
| LinkedIn Media | 5.1–5.2, 7.5 |
| LinkedIn Insights | 6.1–6.2, 6.6, 7.7 |
| Twitter Posting | 0, 2.1–2.3, 7.6 |
| Twitter Messaging | 3, 4.1–4.3, 7.5 |
| Twitter Media | 3.4, 5.3–5.4, 7.5 |
| Twitter Insights | 6.1, 6.4, 6.6, 7.7 |
| YouTube Posting | 0, 2.4–2.6, 7.6 |
| YouTube Insights | 6.1, 6.3, 6.6, 7.7 |
| Reddit (all) | 0, 1, 2.7–2.9, 4.4–4.5, 5.5–5.6, 6.5, 7.* |
