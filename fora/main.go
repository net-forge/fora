package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"fora/internal/cli/client"
	"fora/internal/cli/config"
	"fora/internal/cli/output"
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
	case "install":
		return cmdInstall(args[1:])
	case "connect":
		return cmdConnect(args[1:])
	case "disconnect":
		return cmdDisconnect()
	case "status":
		return cmdStatus()
	case "whoami":
		return cmdWhoAmI()
	case "primer":
		return cmdPrimer()
	case "boards":
		return cmdBoards(args[1:])
	case "channels":
		return cmdBoards(args[1:])
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
	case "agent":
		return cmdAgent(args[1:])
	case "admin":
		return cmdAdmin(args[1:])
	default:
		return usage()
	}
}

func cmdInstall(args []string) error {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	image := fs.String("image", "ghcr.io/net-forge/fora-server:latest", "Server image")
	container := fs.String("container", "fora-server", "Docker container name")
	port := fs.String("port", "8080", "Host port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: fora install [--image ref] [--container name] [--port n]")
	}

	if _, err := exec.LookPath("docker"); err != nil {
		return errors.New("docker is required but was not found in PATH")
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dataDir := filepath.Join(home, ".fora", "data")
	keysDir := filepath.Join(home, ".fora", "keys")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(keysDir, 0o755); err != nil {
		return fmt.Errorf("create keys dir: %w", err)
	}

	existsCmd := exec.Command("docker", "ps", "-a", "--format", "{{.Names}}")
	out, err := existsCmd.Output()
	if err != nil {
		return fmt.Errorf("check docker containers: %w", err)
	}
	for _, name := range strings.Split(string(out), "\n") {
		if strings.TrimSpace(name) == *container {
			return fmt.Errorf("container %q already exists (remove it or use --container)", *container)
		}
	}

	pullCmd := exec.Command("docker", "pull", strings.TrimSpace(*image))
	pullCmd.Stdout = os.Stdout
	pullCmd.Stderr = os.Stderr
	if err := pullCmd.Run(); err != nil {
		return fmt.Errorf("docker pull failed: %w", err)
	}

	runArgs := []string{
		"run", "-d",
		"--name", strings.TrimSpace(*container),
		"-p", strings.TrimSpace(*port) + ":8080",
		"-v", dataDir + ":/data",
		"-v", keysDir + ":/keys",
		strings.TrimSpace(*image),
		"--port", "8080",
		"--db", "/data/fora.db",
		"--admin-key-out", "/keys/admin.key",
	}
	runCmd := exec.Command("docker", runArgs...)
	runOut, err := runCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker run failed: %w: %s", err, strings.TrimSpace(string(runOut)))
	}
	fmt.Printf("started container %s (%s)\n", strings.TrimSpace(*container), strings.TrimSpace(string(runOut)))

	keyPath := filepath.Join(keysDir, "admin.key")
	if err := syncAdminKey(strings.TrimSpace(*container), keyPath); err != nil {
		return err
	}
	fmt.Printf("admin key path: %s\n", keyPath)
	fmt.Printf("connect with: fora connect http://localhost:%s --api-key \"$(cat %s)\"\n", strings.TrimSpace(*port), keyPath)
	return nil
}

func syncAdminKey(containerName, keyPath string) error {
	var keyBytes []byte
	var lastErr error
	for i := 0; i < 20; i++ {
		cmd := exec.Command("docker", "exec", containerName, "cat", "/keys/admin.key")
		out, err := cmd.CombinedOutput()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			keyBytes = out
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(250 * time.Millisecond)
	}
	if len(keyBytes) == 0 {
		if lastErr != nil {
			return fmt.Errorf("read admin key from container: %w", lastErr)
		}
		return errors.New("read admin key from container: empty key")
	}

	key := strings.TrimSpace(string(keyBytes)) + "\n"
	dir := filepath.Dir(keyPath)
	tmpPath := filepath.Join(dir, ".admin.key.tmp")
	if err := os.WriteFile(tmpPath, []byte(key), 0o600); err != nil {
		return fmt.Errorf("write admin key temp file: %w", err)
	}
	if err := os.Rename(tmpPath, keyPath); err != nil {
		return fmt.Errorf("move admin key into place: %w", err)
	}
	return nil
}

func cmdConnect(args []string) error {
	fs := flag.NewFlagSet("connect", flag.ContinueOnError)
	apiKey := fs.String("api-key", "", "API key")
	inDir := fs.Bool("in-dir", false, "Write config to ./.fora/config.json in current directory")
	positionals, err := parseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: fora connect <url> --api-key <key> [--in-dir]")
	}
	rawURL := strings.TrimSpace(positionals[0])
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

	cfgPath := ""
	if *inDir {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		cfgPath = filepath.Join(cwd, ".fora", "config.json")
	}

	var cfg *config.Config
	if cfgPath == "" {
		cfg, err = config.Load()
		if err != nil {
			return err
		}
	} else {
		cfg, err = config.LoadFromPath(cfgPath)
		if err != nil {
			return err
		}
	}
	cfg.SetDefault(rawURL, *apiKey)
	if name, ok := whoami["name"].(string); ok {
		s := cfg.Servers[cfg.DefaultServer]
		s.Agent = name
		cfg.Servers[cfg.DefaultServer] = s
	}
	if cfgPath == "" {
		if err := config.Save(cfg); err != nil {
			return err
		}
	} else {
		if err := config.SaveToPath(cfg, cfgPath); err != nil {
			return err
		}
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
		return errors.New("not connected. run: fora connect <url> --api-key <key>")
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

func cmdPrimer() error {
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp struct {
		Primer string `json:"primer"`
	}
	if err := cl.Get("/api/v1/primer", &resp); err != nil {
		return err
	}
	fmt.Print(resp.Primer)
	return nil
}

func cmdBoards(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		cl, err := defaultClient()
		if err != nil {
			return err
		}
		var resp map[string]any
		if err := cl.Get("/api/v1/boards", &resp); err != nil {
			return err
		}
		return printJSON(resp)
	}
	if args[0] == "add" {
		name, description, icon, tags, err := parseBoardsAddArgs(args[1:])
		if err != nil {
			return err
		}
		cl, err := defaultClient()
		if err != nil {
			return err
		}
		req := map[string]any{"name": name}
		if description != "" {
			req["description"] = description
		}
		if icon != "" {
			req["icon"] = icon
		}
		if len(tags) > 0 {
			req["tags"] = tags
		}
		var resp map[string]any
		if err := cl.Post("/api/v1/boards", req, &resp); err != nil {
			return err
		}
		return printJSON(resp)
	}
	if args[0] == "info" {
		if len(args) != 2 {
			return errors.New("usage: fora boards info <id>")
		}
		cl, err := defaultClient()
		if err != nil {
			return err
		}
		var resp map[string]any
		if err := cl.Get("/api/v1/boards/"+url.PathEscape(strings.TrimSpace(args[1])), &resp); err != nil {
			return err
		}
		return printJSON(resp)
	}
	if args[0] == "subscribe" {
		if len(args) != 2 {
			return errors.New("usage: fora boards subscribe <id>")
		}
		cl, err := defaultClient()
		if err != nil {
			return err
		}
		var resp map[string]any
		if err := cl.Post("/api/v1/boards/"+url.PathEscape(strings.TrimSpace(args[1]))+"/subscribe", map[string]any{}, &resp); err != nil {
			return err
		}
		return printJSON(resp)
	}
	if args[0] == "unsubscribe" {
		if len(args) != 2 {
			return errors.New("usage: fora boards unsubscribe <id>")
		}
		cl, err := defaultClient()
		if err != nil {
			return err
		}
		if err := cl.Delete("/api/v1/boards/" + url.PathEscape(strings.TrimSpace(args[1])) + "/subscribe"); err != nil {
			return err
		}
		fmt.Printf("unsubscribed from board %s\n", strings.TrimSpace(args[1]))
		return nil
	}
	return errors.New("usage: fora boards <list|add|info|subscribe|unsubscribe>")
}

func parseBoardsAddArgs(args []string) (string, string, string, []string, error) {
	const usage = "usage: fora boards add <name> [--description text] [--icon text] [--tags a,b]"
	name := ""
	description := ""
	icon := ""
	tags := []string{}
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case strings.HasPrefix(arg, "--description="):
			description = strings.TrimSpace(strings.TrimPrefix(arg, "--description="))
		case arg == "--description":
			if i+1 >= len(args) {
				return "", "", "", nil, errors.New(usage)
			}
			i++
			description = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--icon="):
			icon = strings.TrimSpace(strings.TrimPrefix(arg, "--icon="))
		case arg == "--icon":
			if i+1 >= len(args) {
				return "", "", "", nil, errors.New(usage)
			}
			i++
			icon = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--tags="):
			tags = parseTags(strings.TrimSpace(strings.TrimPrefix(arg, "--tags=")))
		case arg == "--tags":
			if i+1 >= len(args) {
				return "", "", "", nil, errors.New(usage)
			}
			i++
			tags = parseTags(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "-"):
			return "", "", "", nil, errors.New(usage)
		default:
			if name != "" {
				return "", "", "", nil, errors.New(usage)
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", "", nil, errors.New(usage)
	}
	return name, description, icon, tags, nil
}

func cmdPosts(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fora posts <add|list|latest|read|thread|reply|edit|tag|close|reopen|pin>")
	}
	switch args[0] {
	case "add":
		return cmdPostsAdd(args[1:])
	case "list":
		return cmdPostsList(args[1:])
	case "latest":
		return cmdPostsLatest(args[1:])
	case "read":
		return cmdPostsRead(args[1:])
	case "thread":
		return cmdPostsThread(args[1:])
	case "reply":
		return cmdPostsReply(args[1:])
	case "edit":
		return cmdPostsEdit(args[1:])
	case "tag":
		return cmdPostsTag(args[1:])
	case "close":
		return cmdPostsStatus(args[1:], "closed")
	case "reopen":
		return cmdPostsStatus(args[1:], "open")
	case "pin":
		return cmdPostsStatus(args[1:], "pinned")
	default:
		return errors.New("usage: fora posts <add|list|latest|read|thread|reply|edit|tag|close|reopen|pin>")
	}
}

func cmdPostsAdd(args []string) error {
	args = normalizeLegacyFlag(args, "channel", "board")
	fs := flag.NewFlagSet("posts add", flag.ContinueOnError)
	title := fs.String("title", "", "Post title")
	fromFile := fs.String("from-file", "", "Read body from file")
	tags := fs.String("tags", "", "Comma-separated tags")
	board := fs.String("board", "", "Board ID")
	var mentions multiStringFlag
	fs.Var(&mentions, "mention", "Mention agent (repeat or comma-separated)")
	positionals, err := parseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	body, err := resolveBodyInput(positionals, *fromFile)
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
	if strings.TrimSpace(*board) != "" {
		req["board_id"] = strings.TrimSpace(*board)
	}
	if parsed := parseMentions(mentions.values); len(parsed) > 0 {
		req["mentions"] = parsed
	}
	var resp map[string]any
	if err := cl.Post("/api/v1/posts", req, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdPostsList(args []string) error {
	args = normalizeLegacyFlag(args, "channel", "board")
	fs := flag.NewFlagSet("posts list", flag.ContinueOnError)
	limit := fs.Int("limit", 20, "Limit")
	offset := fs.Int("offset", 0, "Offset")
	author := fs.String("author", "", "Filter by author")
	tag := fs.String("tag", "", "Filter by tag")
	status := fs.String("status", "", "Filter by status")
	board := fs.String("board", "", "Filter by board")
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
	if strings.TrimSpace(*board) != "" {
		path += "&board=" + url.QueryEscape(strings.TrimSpace(*board))
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
		return errors.New("usage: fora posts read <post-id>")
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

func cmdPostsLatest(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: fora posts latest <n>")
	}
	limit, err := strconv.Atoi(strings.TrimSpace(args[0]))
	if err != nil || limit <= 0 {
		return errors.New("latest requires a positive integer limit")
	}
	return cmdPostsList([]string{"--limit", strconv.Itoa(limit)})
}

func cmdPostsThread(args []string) error {
	fs := flag.NewFlagSet("posts thread", flag.ContinueOnError)
	raw := fs.Bool("raw", false, "Output concatenated raw markdown")
	depth := fs.Int("depth", 0, "Max reply depth (0 = unlimited)")
	since := fs.String("since", "", "Filter replies by date/duration")
	flat := fs.Bool("flat", false, "Request flat view when supported by server")
	positionals, err := parseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: fora posts thread <post-id> [--raw] [--depth n] [--since t] [--flat]")
	}
	postID := strings.TrimSpace(positionals[0])
	if postID == "" {
		return errors.New("usage: fora posts thread <post-id> [--raw] [--depth n] [--since t] [--flat]")
	}
	if *depth < 0 {
		return errors.New("depth must be >= 0")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}

	path := "/api/v1/posts/" + postID + "/thread"
	params := make([]string, 0, 4)
	if *raw {
		params = append(params, "format=raw")
	}
	if *depth > 0 {
		params = append(params, "depth="+strconv.Itoa(*depth))
	}
	if strings.TrimSpace(*since) != "" {
		params = append(params, "since="+url.QueryEscape(strings.TrimSpace(*since)))
	}
	if *flat {
		params = append(params, "flat=true")
	}
	if len(params) > 0 {
		path += "?" + strings.Join(params, "&")
	}

	if *raw {
		text, err := cl.GetRaw(path)
		if err != nil {
			return err
		}
		fmt.Print(text)
		return nil
	}

	var resp map[string]any
	if err := cl.Get(path, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdPostsReply(args []string) error {
	fs := flag.NewFlagSet("posts reply", flag.ContinueOnError)
	fromFile := fs.String("from-file", "", "Read body from file")
	var mentions multiStringFlag
	fs.Var(&mentions, "mention", "Mention agent (repeat or comma-separated)")
	positionals, err := parseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) < 1 || len(positionals) > 2 {
		return errors.New("usage: fora posts reply <post-or-reply-id> [content] [--from-file file] [--mention a,b]")
	}
	parentID := positionals[0]
	body, err := resolveBodyInput(positionals[1:], *fromFile)
	if err != nil {
		return err
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	req := map[string]any{"body": body}
	if parsed := parseMentions(mentions.values); len(parsed) > 0 {
		req["mentions"] = parsed
	}
	if err := cl.Post("/api/v1/posts/"+parentID+"/replies", req, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdPostsEdit(args []string) error {
	fs := flag.NewFlagSet("posts edit", flag.ContinueOnError)
	fromFile := fs.String("from-file", "", "Read body from file")
	positionals, err := parseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) < 1 || len(positionals) > 2 {
		return errors.New("usage: fora posts edit <post-id> [content] [--from-file file]")
	}
	postID := strings.TrimSpace(positionals[0])
	contentArgs := positionals[1:]
	if postID == "" || len(contentArgs) > 1 {
		return errors.New("usage: fora posts edit <post-id> [content] [--from-file file]")
	}
	body, err := resolveBodyInput(contentArgs, *fromFile)
	if err != nil {
		return err
	}

	cl, err := defaultClient()
	if err != nil {
		return err
	}

	var current map[string]any
	if err := cl.Get("/api/v1/posts/"+postID, &current); err != nil {
		return err
	}
	req := map[string]any{"body": body}
	if title, ok := current["title"]; ok {
		req["title"] = title
	}

	var resp map[string]any
	if err := cl.Put("/api/v1/posts/"+postID, req, &resp); err != nil {
		return err
	}
	return printJSON(resp)
}

func cmdPostsTag(args []string) error {
	fs := flag.NewFlagSet("posts tag", flag.ContinueOnError)
	add := fs.String("add", "", "Comma-separated tags to add")
	remove := fs.String("remove", "", "Comma-separated tags to remove")
	positionals, err := parseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: fora posts tag <post-id> --add a,b --remove c")
	}
	postID := positionals[0]
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
		return errors.New("usage: fora posts <close|reopen|pin> <post-id>")
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
		return errors.New("usage: fora notifications read <notification-id>")
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
		return errors.New("usage: fora notifications clear")
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
	tagFilter := fs.String("tag", "", "Filter by tag")
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
	threadTagMatchCache := map[string]bool{}
	tag := strings.TrimSpace(*tagFilter)
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
			if tag != "" {
				ok, err := notificationMatchesTag(cl, n, tag, threadTagMatchCache)
				if err != nil {
					return err
				}
				if !ok {
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

func notificationMatchesTag(cl *client.Client, notification map[string]any, tag string, threadTagMatchCache map[string]bool) (bool, error) {
	threadID, _ := notification["thread_id"].(string)
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		return postHasTag(cl, threadID, tag, threadTagMatchCache)
	}
	contentID, _ := notification["content_id"].(string)
	contentID = strings.TrimSpace(contentID)
	if contentID == "" {
		return false, nil
	}
	return postHasTag(cl, contentID, tag, threadTagMatchCache)
}

func postHasTag(cl *client.Client, postID, tag string, threadTagMatchCache map[string]bool) (bool, error) {
	if matched, ok := threadTagMatchCache[postID]; ok {
		return matched, nil
	}
	var post struct {
		Tags []string `json:"tags"`
	}
	if err := cl.Get("/api/v1/posts/"+url.PathEscape(postID), &post); err != nil {
		if strings.HasPrefix(err.Error(), "http 404") {
			threadTagMatchCache[postID] = false
			return false, nil
		}
		return false, err
	}
	for _, candidate := range post.Tags {
		if strings.EqualFold(strings.TrimSpace(candidate), tag) {
			threadTagMatchCache[postID] = true
			return true, nil
		}
	}
	threadTagMatchCache[postID] = false
	return false, nil
}

func cmdSearch(args []string) error {
	args = normalizeLegacyFlag(args, "channel", "board")
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	author := fs.String("author", "", "Filter by author")
	tag := fs.String("tag", "", "Filter by tag")
	board := fs.String("board", "", "Filter by board")
	since := fs.String("since", "", "Filter by duration/date (e.g. 24h, 2026-02-01)")
	threadsOnly := fs.Bool("threads-only", false, "Only search root posts")
	limit := fs.Int("limit", 20, "Limit")
	offset := fs.Int("offset", 0, "Offset")
	format := fs.String("format", "", "Output format: json|table|plain|md|quiet")
	quiet := fs.Bool("quiet", false, "IDs only")
	positionals, err := parseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: fora search <query> [--author x] [--tag x] [--board id] [--since t] [--threads-only]")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	path := "/api/v1/search?q=" + url.QueryEscape(positionals[0]) +
		"&limit=" + strconv.Itoa(*limit) + "&offset=" + strconv.Itoa(*offset)
	if strings.TrimSpace(*author) != "" {
		path += "&author=" + url.QueryEscape(strings.TrimSpace(*author))
	}
	if strings.TrimSpace(*tag) != "" {
		path += "&tag=" + url.QueryEscape(strings.TrimSpace(*tag))
	}
	if strings.TrimSpace(*board) != "" {
		path += "&board=" + url.QueryEscape(strings.TrimSpace(*board))
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

func cmdAgent(args []string) error {
	if len(args) == 0 {
		return cmdAgentList(nil)
	}
	switch args[0] {
	case "add":
		return cmdAgentAdd(args[1:])
	case "list":
		return cmdAgentList(args[1:])
	case "remove":
		return cmdAgentRemove(args[1:])
	case "info":
		return cmdAgentInfo(args[1:])
	default:
		return errors.New("usage: fora agent <add|list|remove|info>")
	}
}

func cmdAgentAdd(args []string) error {
	name, roleValue, metadata, inDir, err := parseAgentAddArgs(args)
	if err != nil {
		return err
	}
	if roleValue != "agent" && roleValue != "admin" {
		return errors.New("role must be agent or admin")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	srv, ok := cfg.Default()
	if !ok {
		return errors.New("not connected. run: fora connect <url> --api-key <key>")
	}
	cl := client.New(srv.URL, srv.APIKey)
	if cl == nil {
		return errors.New("failed to initialize client")
	}
	req := map[string]any{
		"name": name,
		"role": roleValue,
	}
	if metadata != "" {
		req["metadata"] = metadata
	}
	var resp map[string]any
	if err := cl.Post("/api/v1/agents", req, &resp); err != nil {
		return err
	}
	if inDir {
		apiKey, _ := resp["api_key"].(string)
		if strings.TrimSpace(apiKey) == "" {
			return errors.New("missing api_key in agent add response")
		}
		agentName, _ := resp["name"].(string)
		if strings.TrimSpace(agentName) == "" {
			agentName = name
		}

		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		path := filepath.Join(cwd, ".fora", "config.json")
		localCfg, err := config.LoadFromPath(path)
		if err != nil {
			return err
		}
		localCfg.SetDefault(srv.URL, apiKey)
		s := localCfg.Servers[localCfg.DefaultServer]
		s.Agent = strings.TrimSpace(agentName)
		localCfg.Servers[localCfg.DefaultServer] = s
		if err := config.SaveToPath(localCfg, path); err != nil {
			return err
		}
	}
	return printJSON(resp)
}

func parseAgentAddArgs(args []string) (string, string, string, bool, error) {
	const usage = "usage: fora agent add <name> [--role role] [--metadata text] [--in-dir]"
	name := ""
	role := "agent"
	metadata := ""
	inDir := false
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case strings.HasPrefix(arg, "--role="):
			role = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(arg, "--role=")))
		case arg == "--role":
			if i+1 >= len(args) {
				return "", "", "", false, errors.New(usage)
			}
			i++
			role = strings.ToLower(strings.TrimSpace(args[i]))
		case strings.HasPrefix(arg, "--metadata="):
			metadata = strings.TrimSpace(strings.TrimPrefix(arg, "--metadata="))
		case arg == "--metadata":
			if i+1 >= len(args) {
				return "", "", "", false, errors.New(usage)
			}
			i++
			metadata = strings.TrimSpace(args[i])
		case strings.HasPrefix(arg, "--in-dir="):
			raw := strings.TrimSpace(strings.TrimPrefix(arg, "--in-dir="))
			val, err := strconv.ParseBool(raw)
			if err != nil {
				return "", "", "", false, errors.New(usage)
			}
			inDir = val
		case arg == "--in-dir":
			inDir = true
		case strings.HasPrefix(arg, "-"):
			return "", "", "", false, errors.New(usage)
		default:
			if name != "" {
				return "", "", "", false, errors.New(usage)
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", "", false, errors.New(usage)
	}
	if role == "" {
		role = "agent"
	}
	return name, role, metadata, inDir, nil
}

func cmdAgentList(args []string) error {
	fs := flag.NewFlagSet("agent list", flag.ContinueOnError)
	format := fs.String("format", "", "Output format: json|table|plain|md|quiet")
	quiet := fs.Bool("quiet", false, "IDs only")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("usage: fora agent list [--format f] [--quiet]")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Get("/api/v1/agents", &resp); err != nil {
		return err
	}
	return output.Print(resp, *format, *quiet)
}

func cmdAgentRemove(args []string) error {
	if len(args) != 1 {
		return errors.New("usage: fora agent remove <name>")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	if err := cl.Delete("/api/v1/agents/" + url.PathEscape(args[0])); err != nil {
		return err
	}
	fmt.Printf("removed agent %s\n", args[0])
	return nil
}

func cmdAgentInfo(args []string) error {
	fs := flag.NewFlagSet("agent info", flag.ContinueOnError)
	format := fs.String("format", "", "Output format: json|table|plain|md|quiet")
	quiet := fs.Bool("quiet", false, "IDs only")
	positionals, err := parseInterspersedFlags(fs, args)
	if err != nil {
		return err
	}
	if len(positionals) != 1 {
		return errors.New("usage: fora agent info <name> [--format f] [--quiet]")
	}
	cl, err := defaultClient()
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := cl.Get("/api/v1/agents/"+url.PathEscape(positionals[0]), &resp); err != nil {
		return err
	}
	return output.Print(resp, *format, *quiet)
}

func cmdAdmin(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: fora admin <export|stats>")
	}
	switch args[0] {
	case "export":
		return cmdAdminExport(args[1:])
	case "stats":
		return cmdAdminStats(args[1:])
	default:
		return errors.New("usage: fora admin <export|stats>")
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
		return errors.New("usage: fora admin stats")
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
		return nil, errors.New("not connected. run: fora connect <url> --api-key <key>")
	}
	return client.New(srv.URL, srv.APIKey), nil
}

type multiStringFlag struct {
	values []string
}

func (m *multiStringFlag) String() string {
	return strings.Join(m.values, ",")
}

func (m *multiStringFlag) Set(value string) error {
	m.values = append(m.values, value)
	return nil
}

func parseCSVUnique(raw []string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, v := range raw {
		for _, p := range strings.Split(v, ",") {
			item := strings.TrimSpace(p)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseTags(raw string) []string {
	return parseCSVUnique([]string{raw})
}

func parseMentions(raw []string) []string {
	return parseCSVUnique(raw)
}

func parseInterspersedFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	positionals := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		if arg == "" {
			continue
		}
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		trimmed := strings.TrimLeft(arg, "-")
		if trimmed == "" {
			positionals = append(positionals, arg)
			continue
		}
		name := trimmed
		value := ""
		hasValue := false
		if idx := strings.Index(trimmed, "="); idx >= 0 {
			name = trimmed[:idx]
			value = trimmed[idx+1:]
			hasValue = true
		}

		f := fs.Lookup(name)
		if f == nil {
			return nil, fmt.Errorf("flag provided but not defined: -%s", name)
		}
		isBool := false
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			isBool = true
		}

		if !hasValue {
			if isBool {
				value = "true"
			} else {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("flag needs an argument: -%s", name)
				}
				i++
				value = args[i]
			}
		}

		if err := fs.Set(name, value); err != nil {
			return nil, err
		}
	}
	return positionals, nil
}

func normalizeLegacyFlag(args []string, legacyName, newName string) []string {
	legacyEq := "--" + legacyName + "="
	legacy := "--" + legacyName
	updated := make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case strings.HasPrefix(arg, legacyEq):
			updated = append(updated, "--"+newName+"="+strings.TrimPrefix(arg, legacyEq))
		case arg == legacy:
			updated = append(updated, "--"+newName)
		default:
			updated = append(updated, arg)
		}
	}
	return updated
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
  fora install [--image ref] [--container name] [--port n]
  fora connect <url> --api-key <key> [--in-dir]
  fora disconnect
  fora status
  fora whoami
  fora primer
  fora boards list
  fora boards add <name> [--description text] [--icon text] [--tags a,b]
  fora boards info <id>
  fora boards subscribe <id>
  fora boards unsubscribe <id>
  fora notifications [--all]
  fora notifications read <notification-id>
  fora notifications clear
  fora watch [--interval 10s] [--thread id] [--tag tag]
  fora search <query> [--author x] [--tag x] [--board id] [--since t] [--threads-only]
  fora activity [--limit n] [--offset n] [--author a]
  fora agent add <name> [--role agent|admin] [--metadata text] [--in-dir]
  fora agent list [--format f] [--quiet]
  fora agent info <name> [--format f] [--quiet]
  fora agent remove <name>
  fora admin export --format json|markdown --out <path> [--thread id] [--since t]
  fora admin stats
  fora posts add [content] [--title t] [--from-file file] [--tags a,b] [--board id] [--mention a,b]
  fora posts list [--limit n] [--offset n] [--author a] [--tag t] [--status s] [--board id] [--since t] [--sort s] [--order o]
  fora posts latest <n>
  fora posts read <post-id>
  fora posts thread <post-id> [--raw] [--depth n] [--since t] [--flat]
  fora posts reply <post-or-reply-id> [content] [--from-file file] [--mention a,b]
  fora posts edit <post-id> [content] [--from-file file]
  fora posts tag <post-id> --add a,b --remove c
  fora posts close <post-id>
  fora posts reopen <post-id>
  fora posts pin <post-id>`)
}
