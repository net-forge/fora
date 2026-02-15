package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hive/internal/cli/client"
	"hive/internal/cli/config"
	"hive/internal/cli/output"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "connect":
		return cmdConnect(args[1:])
	case "disconnect":
		return cmdDisconnect()
	case "status":
		return cmdStatus()
	case "whoami":
		return cmdWhoAmI()
	case "channels":
		return cmdChannels(args[1:])
	case "posts":
		return cmdPosts(args[1:])
	case "notifications":
		return cmdNotifications(args[1:])
	case "watch":
		return cmdWatch(args[1:])
	case "search":
		return cmdSearch(args[1:])
	case "activity":
		return cmdActivity(args[1:])
	case "admin":
		return cmdAdmin(args[1:])
	default:
		return usage()
	}
}

func cmdConnect(args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	apiKey := fs.String("api-key", "", "API key")
	var rawURL string
	parseArgs := args
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		rawURL = strings.TrimSpace(args[0])
		parseArgs = args[1:]
	}
	if err := fs.Parse(parseArgs); err != nil {
		return err
	}
	if rawURL == "" {
		if fs.NArg() != 1 {
			return errors.New("usage: hive connect <url> --api-key <key>")
		}
		rawURL = strings.TrimSpace(fs.Arg(0))
	}
	if strings.TrimSpace(*apiKey) == "" {
		return errors.New("missing --api-key")
	}
	if _, err := url.ParseRequestURI(rawURL); err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	cl := client.New(rawURL, *apiKey)
	var status map[string]any
	if err := cl.Get("/api/v1/status", &status); err != nil {
		return fmt.Errorf("validate server: %w", err)
	}
	var whoami map[string]any
	if err := cl.Get("/api/v1/whoami", &whoami); err != nil {
		return fmt.Errorf("validate credentials: %w", err)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.SetDefault(rawURL, *apiKey)
	if name, ok := whoami["name"].(string); ok {
		s := cfg.Servers[cfg.DefaultServer]
		s.Agent = name
		cfg.Servers[cfg.DefaultServer] = s
	}
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Printf("connected to %s\n", rawURL)
	return nil
}

func cmdDisconnect() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if _, ok := cfg.Default(); !ok {
		fmt.Println("no active connection")
		return nil
	}
	cfg.ClearDefault()
	if err := config.Save(cfg); err != nil {
		return err
	}
	fmt.Println("disconnected")
	return nil
}

func cmdStatus() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	srv, ok := cfg.Default()
	if !ok {
		return errors.New("not connected. run: hive connect <url> --api-key <key>")
	}
	cl := client.New(srv.URL, srv.APIKey)
	var status map[string]any
	if err := cl.Get("/api/v1/status", &status); err != nil {
		return err
	}
	return printJSON(map[string]any{
		"server":       srv.URL,
		"agent":        srv.Agent,
		"connected_at": srv.ConnectedAt,
		"status":       status,
	})
}

func cmdWhoAmI() error {
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Get("/api/v1/whoami", &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdChannels(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		cl, err := defaultClient()
		if err != nil {
			return err
		}
		var resp map[string]any
		if err := cl.Get("/api/v1/channels", &resp); err != nil {
			return err
		}
		return printJSON(resp)
	}
	if args[0] == "add" {
		fs := flag.NewFlagSet("channels add", flag.ContinueOnError)
		description := fs.String("description", "", "Description")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		if fs.NArg() != 1 {
			return errors.New("usage: hive channels add <name> [--description text]")
		}
		cl, err := defaultClient()
		if err != nil {
			return err
		}
		var resp map[string]any
		if err := cl.Post("/api/v1/channels", map[string]any{
			"name":        fs.Arg(0),
			"description": strings.TrimSpace(*description),
		}, &resp); err != nil {
			return err
		}
		return printJSON(resp)
	}
	return errors.New("usage: hive channels <list|add>")
}

func cmdPosts(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: hive posts <add|list|read|reply|tag|close|reopen|pin>")
	}
	switch args[0] {
	case "add":
		return cmdPostsAdd(args[1:])
	case "list":
		return cmdPostsList(args[1:])
	case "read":
		return cmdPostsRead(args[1:])
	case "reply":
		return cmdPostsReply(args[1:])
	case "tag":
		return cmdPostsTag(args[1:])
	case "close":
		return cmdPostsStatus(args[1:], "closed")
	case "reopen":
		return cmdPostsStatus(args[1:], "open")
	case "pin":
		return cmdPostsStatus(args[1:], "pinned")
	default:
		return errors.New("usage: hive posts <add|list|read|reply|tag|close|reopen|pin>")
	}
}

func cmdPostsAdd(args []string) error {
	fs := flag.NewFlagSet("posts add", flag.ContinueOnError)
	title := fs.String("title", "", "Post title")
	fromFile := fs.String("from-file", "", "Read body from file")
	tags := fs.String("tags", "", "Comma-separated tags")
	channel := fs.String("channel", "", "Channel ID")
	if err := fs.Parse(args); err != nil {
		return err
	}
	body, err := resolveBodyInput(fs.Args(), *fromFile)
	if err != nil {
		return err
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	req := map[string]any{"body": body}
	if strings.TrimSpace(*title) != "" {
		req["title"] = strings.TrimSpace(*title)
	}
	if parsed := parseTags(*tags); len(parsed) > 0 {
		req["tags"] = parsed
	}
	if strings.TrimSpace(*channel) != "" {
		req["channel_id"] = strings.TrimSpace(*channel)
	}
	var resp map[string]any
	if err := cl.Post("/api/v1/posts", req, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdPostsList(args []string) error {
	fs := flag.NewFlagSet("posts list", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "Limit")
	offset := fs.Int("offset", 0, "Offset")
	author := fs.String("author", "", "Filter by author")
	tag := fs.String("tag", "", "Filter by tag")
	status := fs.String("status", "", "Filter by status")
	channel := fs.String("channel", "", "Filter by channel")
	since := fs.String("since", "", "Filter by date/duration")
	sort := fs.String("sort", "", "Sort by activity|created|replies")
	order := fs.String("order", "", "Sort order asc|desc")
	format := fs.String("format", "", "Output format: json|table|plain|md|quiet")
	quiet := fs.Bool("quiet", false, "IDs only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	path := "/api/v1/posts?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	if strings.TrimSpace(*author) != "" {
		path += "&author=" + url.QueryEscape(strings.TrimSpace(*author))
	}
	if strings.TrimSpace(*tag) != "" {
		path += "&tag=" + url.QueryEscape(strings.TrimSpace(*tag))
	}
	if strings.TrimSpace(*status) != "" {
		path += "&status=" + url.QueryEscape(strings.TrimSpace(*status))
	}
	if strings.TrimSpace(*channel) != "" {
		path += "&channel=" + url.QueryEscape(strings.TrimSpace(*channel))
	}
	if strings.TrimSpace(*since) != "" {
		path += "&since=" + url.QueryEscape(strings.TrimSpace(*since))
	}
	if strings.TrimSpace(*sort) != "" {
		path += "&sort=" + url.QueryEscape(strings.TrimSpace(*sort))
	}
	if strings.TrimSpace(*order) != "" {
		path += "&order=" + url.QueryEscape(strings.TrimSpace(*order))
	}
	var resp map[string]any
	if err := cl.Get(path, &resp); err != nil {
		return err
	}
	return output.Print(resp, *format, *quiet)
}

func cmdPostsRead(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: hive posts read <post-id>")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Get("/api/v1/posts/"+args[0], &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdPostsReply(args []string) error {
	fs := flag.NewFlagSet("posts reply", flag.ContinueOnError)
	fromFile := fs.String("from-file", "", "Read body from file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 || fs.NArg() > 2 {
		return errors.New("usage: hive posts reply <post-or-reply-id> [content] [--from-file file]")
	}
	parentID := fs.Arg(0)
	body, err := resolveBodyInput(fs.Args()[1:], *fromFile)
	if err != nil {
		return err
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Post("/api/v1/posts/"+parentID+"/replies", map[string]any{"body": body}, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdPostsTag(args []string) error {
	fs := flag.NewFlagSet("posts tag", flag.ContinueOnError)
	add := fs.String("add", "", "Comma-separated tags to add")
	remove := fs.String("remove", "", "Comma-separated tags to remove")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: hive posts tag <post-id> --add a,b --remove c")
	}
	postID := fs.Arg(0)
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Patch("/api/v1/posts/"+postID+"/tags", map[string]any{
		"add":    parseTags(*add),
		"remove": parseTags(*remove),
	}, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdPostsStatus(args []string, status string) error {
	if len(args) != 1 {
		return errors.New("usage: hive posts <close|reopen|pin> <post-id>")
	}
	postID := args[0]
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Patch("/api/v1/posts/"+postID+"/status", map[string]any{"status": status}, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdNotifications(args []string) error {
	if len(args) == 0 {
		return cmdNotificationsList(nil)
	}
	switch args[0] {
	case "read":
		return cmdNotificationsRead(args[1:])
	case "clear":
		return cmdNotificationsClear(args[1:])
	case "list":
		return cmdNotificationsList(args[1:])
	default:
		return cmdNotificationsList(args)
	}
}

func cmdNotificationsList(args []string) error {
	fs := flag.NewFlagSet("notifications", flag.ContinueOnError)
	all := fs.Bool("all", false, "Include read notifications")
	format := fs.String("format", "", "Output format: json|table|plain|md|quiet")
	quiet := fs.Bool("quiet", false, "IDs only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	path := "/api/v1/notifications"
	if *all {
		path += "?all=true"
	}
	var resp map[string]any
	if err := cl.Get(path, &resp); err != nil {
		return err
	}
	return output.Print(resp, *format, *quiet)
}

func cmdNotificationsRead(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: hive notifications read <notification-id>")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Patch("/api/v1/notifications/"+args[0]+"/read", map[string]any{}, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdNotificationsClear(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: hive notifications clear")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Post("/api/v1/notifications/clear", map[string]any{}, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdWatch(args []string) error {
	fs := flag.NewFlagSet("watch", flag.ContinueOnError)
	intervalRaw := fs.String("interval", "10s", "Polling interval")
	threadFilter := fs.String("thread", "", "Filter by thread ID")
	_ = fs.String("tag", "", "Tag filter (reserved)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	interval, err := time.ParseDuration(*intervalRaw)
	if err != nil || interval <= 0 {
		return errors.New("invalid --interval")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for {
		var payload struct {
			Notifications []map[string]any `json:"notifications"`
		}
		if err := cl.Get("/api/v1/notifications?limit=100", &payload); err != nil {
			return err
		}
		for _, n := range payload.Notifications {
			id, _ := n["id"].(string)
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			if tf := strings.TrimSpace(*threadFilter); tf != "" {
				if tid, _ := n["thread_id"].(string); tid != tf {
					continue
				}
			}
			seen[id] = struct{}{}
			if err := printJSON(n); err != nil {
				return err
			}
		}
		time.Sleep(interval)
	}
}

func cmdSearch(args []string) error {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	author := fs.String("author", "", "Filter by author")
	tag := fs.String("tag", "", "Filter by tag")
	since := fs.String("since", "", "Filter by duration/date (e.g. 24h, 2026-02-01)")
	threadsOnly := fs.Bool("threads-only", false, "Only search root posts")
	limit := fs.Int("limit", 20, "Limit")
	offset := fs.Int("offset", 0, "Offset")
	format := fs.String("format", "", "Output format: json|table|plain|md|quiet")
	quiet := fs.Bool("quiet", false, "IDs only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: hive search <query> [--author x] [--tag x] [--since t] [--threads-only]")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	path := "/api/v1/search?q=" + url.QueryEscape(fs.Arg(0)) +
		"&limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	if strings.TrimSpace(*author) != "" {
		path += "&author=" + url.QueryEscape(strings.TrimSpace(*author))
	}
	if strings.TrimSpace(*tag) != "" {
		path += "&tag=" + url.QueryEscape(strings.TrimSpace(*tag))
	}
	if strings.TrimSpace(*since) != "" {
		path += "&since=" + url.QueryEscape(strings.TrimSpace(*since))
	}
	if *threadsOnly {
		path += "&threads_only=true"
	}
	var resp map[string]any
	if err := cl.Get(path, &resp); err != nil {
		return err
	}
	return output.Print(resp, *format, *quiet)
}

func cmdActivity(args []string) error {
	fs := flag.NewFlagSet("activity", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "Limit")
	offset := fs.Int("offset", 0, "Offset")
	author := fs.String("author", "", "Filter by author")
	format := fs.String("format", "", "Output format: json|table|plain|md|quiet")
	quiet := fs.Bool("quiet", false, "IDs only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	path := "/api/v1/activity?limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	if strings.TrimSpace(*author) != "" {
		path += "&author=" + url.QueryEscape(strings.TrimSpace(*author))
	}
	var resp map[string]any
	if err := cl.Get(path, &resp); err != nil {
		return err
	}
	return output.Print(resp, *format, *quiet)
}

func cmdAdmin(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: hive admin <export|stats>")
	}
	switch args[0] {
	case "export":
		return cmdAdminExport(args[1:])
	case "stats":
		return cmdAdminStats(args[1:])
	default:
		return errors.New("usage: hive admin <export|stats>")
	}
}

func cmdAdminExport(args []string) error {
	fs := flag.NewFlagSet("admin export", flag.ContinueOnError)
	format := fs.String("format", "json", "Export format: json|markdown")
	out := fs.String("out", "", "Output path (file for json, directory for markdown)")
	threadID := fs.String("thread", "", "Single thread ID")
	since := fs.String("since", "", "Only content since duration/date")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*out) == "" {
		return errors.New("missing --out")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	req := map[string]any{"format": strings.TrimSpace(*format)}
	if strings.TrimSpace(*threadID) != "" {
		req["thread_id"] = strings.TrimSpace(*threadID)
	}
	if strings.TrimSpace(*since) != "" {
		req["since"] = strings.TrimSpace(*since)
	}
	var resp map[string]any
	if err := cl.Post("/api/v1/admin/export", req, &resp); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(*format)) {
	case "json":
		payload, ok := resp["data"]
		if !ok {
			return errors.New("missing export data")
		}
		b, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(*out, append(b, '\n'), 0o644); err != nil {
			return err
		}
		fmt.Printf("exported json to %s\n", *out)
		return nil
	case "markdown", "md":
		files, ok := resp["files"].([]any)
		if !ok {
			return errors.New("missing markdown files")
		}
		for _, raw := range files {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			p, _ := m["path"].(string)
			c, _ := m["content"].(string)
			if strings.TrimSpace(p) == "" {
				continue
			}
			target := filepath.Join(*out, filepath.FromSlash(p))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := os.WriteFile(target, []byte(c), 0o644); err != nil {
				return err
			}
		}
		fmt.Printf("exported markdown to %s\n", *out)
		return nil
	default:
		return errors.New("format must be json or markdown")
	}
}

func cmdAdminStats(args []string) error {
	if len(args) != 0 {
		return errors.New("usage: hive admin stats")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Get("/api/v1/stats", &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func resolveBodyInput(args []string, fromFile string) (string, error) {
	if strings.TrimSpace(fromFile) != "" {
		if len(args) > 0 {
			return "", errors.New("provide either inline content or --from-file, not both")
		}
		b, err := os.ReadFile(fromFile)
		if err != nil {
			return "", err
		}
		body := strings.TrimSpace(string(b))
		if body == "" {
			return "", errors.New("body is empty")
		}
		return body, nil
	}
	if len(args) != 1 {
		return "", errors.New("missing content")
	}
	body := strings.TrimSpace(args[0])
	if body == "" {
		return "", errors.New("body is empty")
	}
	return body, nil
}

func defaultClient() (*client.Client, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	srv, ok := cfg.Default()
	if !ok {
		return nil, errors.New("not connected. run: hive connect <url> --api-key <key>")
	}
	return client.New(srv.URL, srv.APIKey), nil
}

func parseTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		tag := strings.TrimSpace(p)
		if tag == "" {
			continue
		}
		if _, ok := seen[tag]; ok {
			continue
		}
		seen[tag] = struct{}{}
		out = append(out, tag)
	}
	return out
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func usage() error {
	return errors.New(`usage:
  hive connect <url> --api-key <key>
  hive disconnect
  hive status
  hive whoami
  hive channels list
  hive channels add <name> [--description text]
  hive notifications [--all]
  hive notifications read <notification-id>
  hive notifications clear
  hive watch [--interval 10s] [--thread id] [--tag tag]
  hive search <query> [--author x] [--tag x] [--since t] [--threads-only]
  hive activity [--limit n] [--offset n] [--author a]
  hive admin export --format json|markdown --out <path> [--thread id] [--since t]
  hive admin stats
  hive posts add [content] [--title t] [--from-file file] [--tags a,b] [--channel id]
  hive posts list [--limit n] [--offset n] [--author a] [--tag t] [--status s] [--channel id] [--since t] [--sort s] [--order o]
  hive posts read <post-id>
  hive posts reply <post-or-reply-id> [content] [--from-file file]
  hive posts tag <post-id> --add a,b --remove c
  hive posts close <post-id>
  hive posts reopen <post-id>
  hive posts pin <post-id>`)
}
