package tools

import (
	"context"

	"github.com/idivarts/backend-sls/internal/models/trendlymodels"
	"github.com/idivarts/backend-sls/pkg/openrouter"
)

func summarizeConversation(c trendlymodels.InboxConversation) map[string]any {
	return map[string]any{
		"id":             c.ID,
		"kind":           c.Kind,
		"channel":        c.Channel,
		"participant":    c.Participant.Name,
		"handle":         c.Participant.Handle,
		"preview":        c.Preview,
		"unread":         c.Unread,
		"lastActivityAt": c.LastActivityAt,
	}
}

func inboxConversations() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_inbox_conversations",
			"List the brand's social inbox conversations (DMs and comment threads), optionally filtered to unread / a kind / a channel.",
			openrouter.ObjectSchema(map[string]any{
				"unreadOnly": openrouter.StringProp("Pass true to return only unread conversations."),
				"kind":       openrouter.EnumProp("Filter by kind.", []string{"dm", "comment"}),
				"channel":    openrouter.EnumProp("Filter by channel.", []string{"instagram", "facebook"}),
				"limit":      openrouter.NumberProp("Max conversations (default 30)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			q := trendlymodels.InboxQuery{
				UnreadOnly: argBool(args, "unreadOnly"),
				Kind:       argStr(args, "kind"),
				Channel:    argStr(args, "channel"),
			}
			convs, err := trendlymodels.ListInboxConversationsFiltered(brandID, q)
			if err != nil {
				return nil, err
			}
			limit := argInt(args, "limit", 30)
			out := make([]map[string]any, 0, limit)
			for _, c := range convs {
				if len(out) >= limit {
					break
				}
				out = append(out, summarizeConversation(c))
			}
			return map[string]any{"count": len(out), "conversations": out}, nil
		},
	}
}

func inboxThread() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_inbox_thread",
			"Fetch the full message thread of one inbox conversation by ID.",
			openrouter.ObjectSchema(map[string]any{
				"conversationId": openrouter.StringProp("The inbox conversation ID."),
			}, []string{"conversationId"}),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			id := argStr(args, "conversationId")
			if id == "" {
				return map[string]any{"error": "conversationId is required"}, nil
			}
			c, err := trendlymodels.GetInboxConversation(brandID, id)
			if err != nil {
				return nil, err
			}
			msgs := make([]map[string]any, 0, len(c.Messages))
			for _, m := range c.Messages {
				msgs = append(msgs, map[string]any{"author": m.Author, "text": m.Text, "sentAt": m.SentAt})
			}
			res := map[string]any{
				"id": c.ID, "kind": c.Kind, "channel": c.Channel,
				"participant": c.Participant.Name, "handle": c.Participant.Handle, "messages": msgs,
			}
			if c.Comment != nil {
				res["comment"] = map[string]any{"text": c.Comment.Text, "authoredAt": c.Comment.AuthoredAt}
			}
			if c.Post != nil {
				res["post"] = map[string]any{"postId": c.Post.PostID, "caption": c.Post.Caption}
			}
			return res, nil
		},
	}
}

func postComments() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_post_comments",
			"List comments the brand has received, optionally scoped to one post (by its platform postId). Reads the ingested inbox comment threads.",
			openrouter.ObjectSchema(map[string]any{
				"postId": openrouter.StringProp("Optional platform post/media ID to filter comments to a single post."),
				"limit":  openrouter.NumberProp("Max comments (default 40)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			convs, err := trendlymodels.ListInboxConversationsFiltered(brandID, trendlymodels.InboxQuery{Kind: trendlymodels.InboxKindComment})
			if err != nil {
				return nil, err
			}
			postID := argStr(args, "postId")
			limit := argInt(args, "limit", 40)
			out := make([]map[string]any, 0, limit)
			for _, c := range convs {
				if len(out) >= limit {
					break
				}
				if postID != "" && (c.Post == nil || c.Post.PostID != postID) {
					continue
				}
				row := map[string]any{
					"conversationId": c.ID,
					"author":         c.Participant.Name,
					"handle":         c.Participant.Handle,
					"unread":         c.Unread,
				}
				if c.Comment != nil {
					row["text"] = c.Comment.Text
					row["authoredAt"] = c.Comment.AuthoredAt
					row["replied"] = len(c.Comment.Replies) > 0
				}
				if c.Post != nil {
					row["postId"] = c.Post.PostID
				}
				out = append(out, row)
			}
			return map[string]any{"count": len(out), "comments": out}, nil
		},
	}
}

func unansweredQuestions() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_unanswered_questions",
			"Surface inbox items that appear to need a reply from the brand: DMs whose latest message is from the contact, and comments with no brand reply yet.",
			openrouter.ObjectSchema(map[string]any{
				"limit": openrouter.NumberProp("Max items (default 30)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			convs, err := trendlymodels.ListInboxConversationsFiltered(brandID, trendlymodels.InboxQuery{})
			if err != nil {
				return nil, err
			}
			limit := argInt(args, "limit", 30)
			out := make([]map[string]any, 0, limit)
			for _, c := range convs {
				if len(out) >= limit {
					break
				}
				needs := false
				text := c.Preview
				switch c.Kind {
				case trendlymodels.InboxKindComment:
					if c.Comment != nil && len(c.Comment.Replies) == 0 && !c.Comment.Hidden {
						needs = true
						text = c.Comment.Text
					}
				default: // dm
					if n := len(c.Messages); n > 0 && c.Messages[n-1].Author == trendlymodels.InboxAuthorContact {
						needs = true
						text = c.Messages[n-1].Text
					}
				}
				if !needs {
					continue
				}
				out = append(out, map[string]any{
					"conversationId": c.ID, "kind": c.Kind, "channel": c.Channel,
					"from": c.Participant.Name, "handle": c.Participant.Handle, "text": text,
				})
			}
			return map[string]any{"count": len(out), "items": out}, nil
		},
	}
}

func communitySentiment() Registered {
	return Registered{
		Tool: openrouter.NewFunctionTool(
			"get_community_sentiment",
			"Return recent inbox message/comment texts plus counts so you can gauge overall community sentiment. This tool provides the raw material; you form the sentiment read.",
			openrouter.ObjectSchema(map[string]any{
				"limit": openrouter.NumberProp("Max recent texts to sample (default 30)."),
			}, nil),
		),
		Handler: func(ctx context.Context, brandID string, args map[string]any) (any, error) {
			convs, err := trendlymodels.ListInboxConversationsFiltered(brandID, trendlymodels.InboxQuery{})
			if err != nil {
				return nil, err
			}
			limit := argInt(args, "limit", 30)
			var dmCount, commentCount, unread int
			texts := make([]map[string]any, 0, limit)
			for _, c := range convs {
				if c.Kind == trendlymodels.InboxKindComment {
					commentCount++
				} else {
					dmCount++
				}
				if c.Unread {
					unread++
				}
				if len(texts) >= limit {
					continue
				}
				t := c.Preview
				if c.Kind == trendlymodels.InboxKindComment && c.Comment != nil {
					t = c.Comment.Text
				}
				if t != "" {
					texts = append(texts, map[string]any{"channel": c.Channel, "kind": c.Kind, "text": t})
				}
			}
			return map[string]any{
				"dmCount": dmCount, "commentCount": commentCount, "unreadCount": unread,
				"recentTexts": texts,
			}, nil
		},
	}
}
