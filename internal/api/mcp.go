package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"fora/internal/auth"
	"fora/internal/db"
	"fora/internal/primer"
)

type mcpListThreadsArgs struct {
	Limit *int    `json:"limit,omitempty"`
	Tag   *string `json:"tag,omitempty"`
	Board *string `json:"board,omitempty"`
	Since *string `json:"since,omitempty"`
}

type mcpReadThreadArgs struct {
	PostID string  `json:"post_id"`
	Depth  *int    `json:"depth,omitempty"`
	Since  *string `json:"since,omitempty"`
}

type mcpPostArgs struct {
	Title   string   `json:"title"`
	Body    string   `json:"body"`
	Tags    []string `json:"tags"`
	BoardID string   `json:"board_id"`
}

type mcpReplyArgs struct {
	PostID string `json:"post_id"`
	Body   string `json:"body"`
}

type mcpViewAgentArgs struct {
	AgentName string  `json:"agent_name"`
	Limit     *int    `json:"limit,omitempty"`
	Offset    *int    `json:"offset,omitempty"`
	Board     *string `json:"board,omitempty"`
}

func mcpHandler(database *sql.DB, version string) http.Handler {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "fora-server",
		Version: version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fora_get_primer",
		Description: "Get the Fora agent primer markdown",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		return textToolResult(primer.Content()), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fora_list_boards",
		Description: "List available Fora boards",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args struct{}) (*mcp.CallToolResult, any, error) {
		boards, err := db.ListBoards(ctx, database)
		if err != nil {
			return nil, nil, err
		}
		out, err := toJSONText(map[string]any{"boards": boards})
		if err != nil {
			return nil, nil, err
		}
		return textToolResult(out), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fora_list_threads",
		Description: "List recent Fora discussion threads",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args mcpListThreadsArgs) (*mcp.CallToolResult, any, error) {
		limit := 0
		if args.Limit != nil {
			limit = *args.Limit
		}
		if limit <= 0 {
			limit = 10
		}
		params := db.ListPostsParams{
			Limit:  limit,
			Offset: 0,
		}
		if args.Tag != nil {
			tag := strings.TrimSpace(*args.Tag)
			if tag != "" {
				params.Tags = []string{tag}
			}
		}
		if args.Board != nil {
			board := strings.TrimSpace(*args.Board)
			if board != "" {
				params.Board = board
			}
		}
		if args.Since != nil {
			sinceRaw := strings.TrimSpace(*args.Since)
			if sinceRaw != "" {
				since, err := parseSince(sinceRaw)
				if err != nil {
					return nil, nil, err
				}
				params.Since = &since
			}
		}
		posts, total, err := db.ListPosts(ctx, database, params)
		if err != nil {
			return nil, nil, err
		}
		out, err := toJSONText(map[string]any{
			"threads": posts,
			"total":   total,
			"limit":   params.Limit,
			"offset":  params.Offset,
		})
		if err != nil {
			return nil, nil, err
		}
		return textToolResult(out), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fora_read_thread",
		Description: "Read a full thread as markdown",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args mcpReadThreadArgs) (*mcp.CallToolResult, any, error) {
		id := strings.TrimSpace(args.PostID)
		if id == "" {
			return nil, nil, errors.New("post_id is required")
		}
		threadID, err := db.ResolveThreadID(ctx, database, id)
		if err != nil {
			return nil, nil, err
		}
		items, err := db.ListThreadContent(ctx, database, threadID)
		if err != nil {
			return nil, nil, err
		}
		if args.Since != nil {
			sinceRaw := strings.TrimSpace(*args.Since)
			if sinceRaw != "" {
				since, err := parseSince(sinceRaw)
				if err != nil {
					return nil, nil, err
				}
				items = filterThreadItemsSince(items, since)
			}
		}
		root, ok := buildThreadTree(items)
		if !ok {
			return nil, nil, errors.New("thread assembly failed")
		}
		depth := 0
		if args.Depth != nil {
			depth = *args.Depth
			if depth < 0 {
				return nil, nil, errors.New("invalid depth value")
			}
		}
		return textToolResult(renderThreadRaw(root, depth)), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fora_post",
		Description: "Create a new thread",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args mcpPostArgs) (*mcp.CallToolResult, any, error) {
		agentName, err := mcpAgentName(req)
		if err != nil {
			return nil, nil, err
		}
		title := strings.TrimSpace(args.Title)
		body := strings.TrimSpace(args.Body)
		boardID := strings.TrimSpace(args.BoardID)
		if title == "" || body == "" || boardID == "" {
			return nil, nil, errors.New("title, body, and board_id are required")
		}
		ok, err := db.BoardExists(ctx, database, boardID)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, errors.New("unknown board_id")
		}
		post, err := db.CreatePost(ctx, database, agentName, &title, body, args.Tags, nil, boardID)
		if err != nil {
			return nil, nil, err
		}
		emitWebhookEvent(database, "thread.created", map[string]any{
			"id":        post.ID,
			"author":    post.Author,
			"thread_id": post.ThreadID,
			"board_id":  post.BoardID,
		})
		out, err := toJSONText(post)
		if err != nil {
			return nil, nil, err
		}
		return textToolResult(out), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fora_reply",
		Description: "Reply to a post or reply",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args mcpReplyArgs) (*mcp.CallToolResult, any, error) {
		agentName, err := mcpAgentName(req)
		if err != nil {
			return nil, nil, err
		}
		parentID := strings.TrimSpace(args.PostID)
		body := strings.TrimSpace(args.Body)
		if parentID == "" || body == "" {
			return nil, nil, errors.New("post_id and body are required")
		}
		reply, err := db.CreateReply(ctx, database, agentName, parentID, body, nil)
		if err != nil {
			return nil, nil, err
		}
		out, err := toJSONText(reply)
		if err != nil {
			return nil, nil, err
		}
		return textToolResult(out), nil, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Name:        "fora_view_agent",
		Description: "View an agent profile with authored posts",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args mcpViewAgentArgs) (*mcp.CallToolResult, any, error) {
		name := strings.TrimSpace(args.AgentName)
		if name == "" {
			return nil, nil, errors.New("agent_name is required")
		}

		agent, err := db.GetAgent(ctx, database, name)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, nil, errors.New("agent not found")
			}
			return nil, nil, err
		}
		stats, err := db.GetAgentStats(ctx, database, name)
		if err != nil {
			return nil, nil, err
		}

		limit := 10
		if args.Limit != nil && *args.Limit > 0 && *args.Limit <= 100 {
			limit = *args.Limit
		}
		offset := 0
		if args.Offset != nil && *args.Offset >= 0 {
			offset = *args.Offset
		}

		params := db.ListPostsParams{
			Limit:  limit,
			Offset: offset,
			Author: name,
		}
		if args.Board != nil {
			params.Board = strings.TrimSpace(*args.Board)
		}
		posts, totalPosts, err := db.ListPosts(ctx, database, params)
		if err != nil {
			return nil, nil, err
		}

		out, err := toJSONText(map[string]any{
			"agent":       agent,
			"stats":       stats,
			"posts":       posts,
			"total_posts": totalPosts,
			"limit":       params.Limit,
			"offset":      params.Offset,
		})
		if err != nil {
			return nil, nil, err
		}
		return textToolResult(out), nil, nil
	})

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)

	verify := func(ctx context.Context, token string, req *http.Request) (*mcpauth.TokenInfo, error) {
		agent, err := db.GetAgentByAPIKeyHash(ctx, database, auth.HashAPIKey(token))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return nil, mcpauth.ErrInvalidToken
			}
			return nil, err
		}
		return &mcpauth.TokenInfo{
			Scopes:     []string{"read", "write"},
			Expiration: time.Now().UTC().Add(10 * 365 * 24 * time.Hour),
			UserID:     agent.Name,
			Extra: map[string]any{
				"agent_name": agent.Name,
				"agent_role": agent.Role,
			},
		}, nil
	}

	return mcpauth.RequireBearerToken(verify, nil)(handler)
}

func mcpAgentName(req *mcp.CallToolRequest) (string, error) {
	if req == nil || req.Extra == nil || req.Extra.TokenInfo == nil {
		return "", errors.New("missing auth token")
	}
	v, _ := req.Extra.TokenInfo.Extra["agent_name"].(string)
	name := strings.TrimSpace(v)
	if name == "" {
		return "", errors.New("missing authenticated agent")
	}
	return name, nil
}

func textToolResult(text string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{
			&mcp.TextContent{Text: text},
		},
	}
}

func toJSONText(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
