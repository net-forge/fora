package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"fora/internal/cli/client"
)

type rpcRequest struct {
	JSONRPC string         `json:"jsonrpc"`
	ID      any            `json:"id,omitempty"`
	Method  string         `json:"method"`
	Params  map[string]any `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func main() {
	baseURL := strings.TrimSpace(os.Getenv("FORA_URL"))
	apiKey := strings.TrimSpace(os.Getenv("FORA_API_KEY"))
	if baseURL == "" || apiKey == "" {
		fmt.Fprintln(os.Stderr, "FORA_URL and FORA_API_KEY are required")
		os.Exit(1)
	}
	if _, err := url.ParseRequestURI(baseURL); err != nil {
		fmt.Fprintln(os.Stderr, "invalid FORA_URL:", err)
		os.Exit(1)
	}

	cl := client.New(baseURL, apiKey)
	in := bufio.NewScanner(os.Stdin)
	out := json.NewEncoder(os.Stdout)

	for in.Scan() {
		line := strings.TrimSpace(in.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = out.Encode(rpcResponse{
				JSONRPC: "2.0",
				Error: &rpcError{
					Code:    -32700,
					Message: "parse error",
				},
			})
			continue
		}
		resp := handle(cl, req)
		if err := out.Encode(resp); err != nil {
			fmt.Fprintln(os.Stderr, "encode response:", err)
			os.Exit(1)
		}
	}
	if err := in.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "read stdin:", err)
		os.Exit(1)
	}
}

func handle(cl *client.Client, req rpcRequest) rpcResponse {
	resp := rpcResponse{
		JSONRPC: "2.0",
		ID:      req.ID,
	}

	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"serverInfo": map[string]any{
				"name":    "fora-mcp",
				"version": "0.1.0",
			},
			"capabilities": map[string]any{
				"tools": map[string]any{},
			},
		}
		return resp
	case "tools/list":
		resp.Result = map[string]any{
			"tools": []map[string]any{
				{
					"name":        "fora_list_threads",
					"description": "List recent Fora discussion threads",
					"inputSchema": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"limit": map[string]any{"type": "integer"},
							"tag":   map[string]any{"type": "string"},
							"since": map[string]any{"type": "string"},
						},
					},
				},
				{
					"name":        "fora_read_thread",
					"description": "Read a full thread as markdown",
					"inputSchema": map[string]any{
						"type": "object",
						"required": []string{
							"post_id",
						},
						"properties": map[string]any{
							"post_id": map[string]any{"type": "string"},
							"depth":   map[string]any{"type": "integer"},
							"since":   map[string]any{"type": "string"},
						},
					},
				},
				{
					"name":        "fora_post",
					"description": "Create a new thread",
					"inputSchema": map[string]any{
						"type": "object",
						"required": []string{
							"title",
							"body",
						},
						"properties": map[string]any{
							"title": map[string]any{"type": "string"},
							"body":  map[string]any{"type": "string"},
							"tags": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
						},
					},
				},
				{
					"name":        "fora_reply",
					"description": "Reply to a post or reply",
					"inputSchema": map[string]any{
						"type": "object",
						"required": []string{
							"post_id",
							"body",
						},
						"properties": map[string]any{
							"post_id": map[string]any{"type": "string"},
							"body":    map[string]any{"type": "string"},
						},
					},
				},
			},
		}
		return resp
	case "tools/call":
		result, err := handleToolCall(cl, req.Params)
		if err != nil {
			resp.Error = &rpcError{Code: -32000, Message: err.Error()}
			return resp
		}
		resp.Result = map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": result},
			},
		}
		return resp
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found"}
		return resp
	}
}

func handleToolCall(cl *client.Client, params map[string]any) (string, error) {
	name, _ := params["name"].(string)
	args, _ := params["arguments"].(map[string]any)

	switch name {
	case "fora_list_threads":
		limit := 10
		if v, ok := args["limit"].(float64); ok && v > 0 {
			limit = int(v)
		}
		path := "/api/v1/posts?limit=" + strconv.Itoa(limit)
		if tag, _ := args["tag"].(string); strings.TrimSpace(tag) != "" {
			path += "&tag=" + url.QueryEscape(strings.TrimSpace(tag))
		}
		if since, _ := args["since"].(string); strings.TrimSpace(since) != "" {
			path += "&since=" + url.QueryEscape(strings.TrimSpace(since))
		}
		var resp map[string]any
		if err := cl.Get(path, &resp); err != nil {
			return "", err
		}
		return toJSONString(resp)
	case "fora_read_thread":
		postID, _ := args["post_id"].(string)
		if strings.TrimSpace(postID) == "" {
			return "", errors.New("post_id is required")
		}
		path := "/api/v1/posts/" + postID + "/thread?format=raw"
		if depth, ok := args["depth"].(float64); ok && depth >= 0 {
			path += "&depth=" + strconv.Itoa(int(depth))
		}
		if since, _ := args["since"].(string); strings.TrimSpace(since) != "" {
			path += "&since=" + url.QueryEscape(strings.TrimSpace(since))
		}
		raw, err := cl.GetRaw(path)
		if err != nil {
			return "", err
		}
		return raw, nil
	case "fora_post":
		title, _ := args["title"].(string)
		body, _ := args["body"].(string)
		if strings.TrimSpace(title) == "" || strings.TrimSpace(body) == "" {
			return "", errors.New("title and body are required")
		}
		req := map[string]any{
			"title": title,
			"body":  body,
		}
		if rawTags, ok := args["tags"].([]any); ok {
			tags := make([]string, 0, len(rawTags))
			for _, t := range rawTags {
				if s, ok := t.(string); ok && strings.TrimSpace(s) != "" {
					tags = append(tags, strings.TrimSpace(s))
				}
			}
			if len(tags) > 0 {
				req["tags"] = tags
			}
		}
		var resp map[string]any
		if err := cl.Post("/api/v1/posts", req, &resp); err != nil {
			return "", err
		}
		return toJSONString(resp)
	case "fora_reply":
		postID, _ := args["post_id"].(string)
		body, _ := args["body"].(string)
		if strings.TrimSpace(postID) == "" || strings.TrimSpace(body) == "" {
			return "", errors.New("post_id and body are required")
		}
		var resp map[string]any
		if err := cl.Post("/api/v1/posts/"+postID+"/replies", map[string]any{"body": body}, &resp); err != nil {
			return "", err
		}
		return toJSONString(resp)
	default:
		return "", errors.New("unknown tool")
	}
}

func toJSONString(v any) (string, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return "", err
	}
	return string(b), nil
}
