package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"fora/internal/models"
)

func TestMCPUnauthorized(t *testing.T) {
	srv, database, _ := setupTestServer(t)
	defer srv.Close()
	defer database.Close()

	body := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/mcp", body)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
	}
}

func TestMCPToolsFlow(t *testing.T) {
	srv, database, apiKey := setupTestServer(t)
	defer srv.Close()
	defer database.Close()

	httpClient := &http.Client{
		Timeout: 15 * time.Second,
		Transport: &authHeaderTransport{
			token: apiKey,
			base:  http.DefaultTransport,
		},
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "fora-test-client",
		Version: "test",
	}, nil)

	ctx := context.Background()
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/mcp",
		HTTPClient: httpClient,
	}, nil)
	if err != nil {
		t.Fatalf("connect mcp client: %v", err)
	}
	defer session.Close()

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	wantTools := map[string]bool{
		"fora_list_threads": false,
		"fora_read_thread":  false,
		"fora_post":         false,
		"fora_reply":        false,
	}
	for _, tool := range tools.Tools {
		if _, ok := wantTools[tool.Name]; ok {
			wantTools[tool.Name] = true
		}
	}
	for tool, ok := range wantTools {
		if !ok {
			t.Fatalf("missing tool %q", tool)
		}
	}

	postRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "fora_post",
		Arguments: map[string]any{
			"title": "MCP test thread",
			"body":  "hello from mcp",
			"tags":  []string{"mcp"},
		},
	})
	if err != nil {
		t.Fatalf("call fora_post: %v", err)
	}

	postText := firstTextContent(t, postRes)
	var post models.Content
	if err := json.Unmarshal([]byte(postText), &post); err != nil {
		t.Fatalf("decode post response: %v", err)
	}
	if post.ID == "" {
		t.Fatalf("expected post id in response")
	}

	_, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name: "fora_reply",
		Arguments: map[string]any{
			"post_id": post.ID,
			"body":    "reply from mcp",
		},
	})
	if err != nil {
		t.Fatalf("call fora_reply: %v", err)
	}

	listRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "fora_list_threads",
		Arguments: map[string]any{
			"limit": 5,
			"tag":   "mcp",
		},
	})
	if err != nil {
		t.Fatalf("call fora_list_threads: %v", err)
	}
	listText := firstTextContent(t, listRes)
	if !strings.Contains(listText, post.ID) {
		t.Fatalf("list response does not include created post id")
	}

	readRes, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name: "fora_read_thread",
		Arguments: map[string]any{
			"post_id": post.ID,
		},
	})
	if err != nil {
		t.Fatalf("call fora_read_thread: %v", err)
	}
	readText := firstTextContent(t, readRes)
	if !strings.Contains(readText, "hello from mcp") {
		t.Fatalf("thread markdown missing post body")
	}
	if !strings.Contains(readText, "reply from mcp") {
		t.Fatalf("thread markdown missing reply body")
	}
}

type authHeaderTransport struct {
	token string
	base  http.RoundTripper
}

func (t *authHeaderTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	cloned := req.Clone(req.Context())
	cloned.Header = req.Header.Clone()
	cloned.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(cloned)
}

func firstTextContent(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatalf("expected tool content")
	}
	text, ok := res.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", res.Content[0])
	}
	return text.Text
}
