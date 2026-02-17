package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"fora/internal/db"
	"fora/internal/models"
)

type createPostRequest struct {
	Title    *string  `json:"title"`
	Body     string   `json:"body"`
	Tags     []string `json:"tags"`
	Mentions []string `json:"mentions"`
	BoardID  string   `json:"board_id"`
}

type updatePostRequest struct {
	Title *string `json:"title"`
	Body  string  `json:"body"`
}

type createReplyRequest struct {
	Body     string   `json:"body"`
	Mentions []string `json:"mentions"`
}

type updateReplyRequest struct {
	Body string `json:"body"`
}

type updateTagsRequest struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

type updateStatusRequest struct {
	Status string `json:"status"`
}

func postsCollectionHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			agent := currentAgent(r.Context())
			if agent == nil {
				writeError(w, http.StatusUnauthorized, "missing auth context")
				return
			}
			var req createPostRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json payload")
				return
			}
			req.BoardID = strings.TrimSpace(req.BoardID)
			if req.BoardID == "" {
				writeError(w, http.StatusBadRequest, "board_id is required")
				return
			}
			ok, err := db.BoardExists(r.Context(), database, req.BoardID)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to validate board")
				return
			}
			if !ok {
				writeError(w, http.StatusBadRequest, "unknown board_id")
				return
			}
			post, err := db.CreatePost(r.Context(), database, agent.Name, req.Title, req.Body, req.Tags, req.Mentions, req.BoardID)
			if err != nil {
				if strings.Contains(err.Error(), "body is required") || strings.Contains(err.Error(), "board_id is required") {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to create post")
				return
			}
			emitWebhookEvent(database, "thread.created", map[string]any{
				"id":        post.ID,
				"author":    post.Author,
				"thread_id": post.ThreadID,
				"board_id":  post.BoardID,
			})
			if len(req.Mentions) > 0 {
				emitWebhookEvent(database, "mention.created", map[string]any{
					"content_id": post.ID,
					"thread_id":  post.ThreadID,
					"from":       post.Author,
					"mentions":   req.Mentions,
					"board_id":   post.BoardID,
				})
			}
			writeJSON(w, http.StatusCreated, post)
		case http.MethodGet:
			params, err := parseListPostsParams(r)
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			posts, total, err := db.ListPosts(r.Context(), database, params)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to list posts")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"threads": posts,
				"total":   total,
				"limit":   params.Limit,
				"offset":  params.Offset,
			})
		default:
			methodNotAllowed(w)
		}
	})
}

func postItemHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := pathTail(r.URL.Path, "/api/v1/posts/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing post id")
			return
		}
		if strings.Contains(id, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		switch r.Method {
		case http.MethodGet:
			content, err := db.GetContent(r.Context(), database, id)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "post not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to read post")
				return
			}
			if content.Type != "post" {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			writeJSON(w, http.StatusOK, content)
		case http.MethodPut:
			agent := currentAgent(r.Context())
			if agent == nil {
				writeError(w, http.StatusUnauthorized, "missing auth context")
				return
			}
			post, err := db.GetContent(r.Context(), database, id)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "post not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to read post")
				return
			}
			if post.Type != "post" {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			if post.Author != agent.Name && agent.Role != "admin" {
				writeError(w, http.StatusForbidden, "not allowed to edit this post")
				return
			}
			var req updatePostRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json payload")
				return
			}
			updated, err := db.UpdatePost(r.Context(), database, id, req.Title, req.Body, agent.Name)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "post not found")
					return
				}
				if strings.Contains(err.Error(), "body is required") {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to update post")
				return
			}
			writeJSON(w, http.StatusOK, updated)
		case http.MethodDelete:
			agent := currentAgent(r.Context())
			if agent == nil {
				writeError(w, http.StatusUnauthorized, "missing auth context")
				return
			}
			post, err := db.GetContent(r.Context(), database, id)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "post not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to read post")
				return
			}
			if post.Type != "post" {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			if post.Author != agent.Name && agent.Role != "admin" {
				writeError(w, http.StatusForbidden, "not allowed to delete this post")
				return
			}
			if err := db.DeletePostThread(r.Context(), database, id); err != nil {
				writeError(w, http.StatusInternalServerError, "failed to delete post")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			methodNotAllowed(w)
		}
	})
}

func repliesHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/posts/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "replies" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		parentID := parts[0]

		switch r.Method {
		case http.MethodPost:
			agent := currentAgent(r.Context())
			if agent == nil {
				writeError(w, http.StatusUnauthorized, "missing auth context")
				return
			}
			if _, err := db.GetContent(r.Context(), database, parentID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "parent content not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to load parent content")
				return
			}
			var req createReplyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json payload")
				return
			}
			reply, err := db.CreateReply(r.Context(), database, agent.Name, parentID, req.Body, req.Mentions)
			if err != nil {
				if strings.Contains(err.Error(), "body is required") {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "parent content not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to create reply")
				return
			}
			emitWebhookEvent(database, "reply.created", map[string]any{
				"id":        reply.ID,
				"author":    reply.Author,
				"thread_id": reply.ThreadID,
				"parent_id": reply.ParentID,
			})
			if len(req.Mentions) > 0 {
				emitWebhookEvent(database, "mention.created", map[string]any{
					"content_id": reply.ID,
					"thread_id":  reply.ThreadID,
					"from":       reply.Author,
					"mentions":   req.Mentions,
				})
			}
			writeJSON(w, http.StatusCreated, reply)
		case http.MethodGet:
			limit, offset := parseLimitOffset(r)
			if _, err := db.GetContent(r.Context(), database, parentID); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "parent content not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to load parent content")
				return
			}
			replies, err := db.ListReplies(r.Context(), database, parentID, limit, offset)
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to list replies")
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"replies": replies,
				"limit":   limit,
				"offset":  offset,
			})
		default:
			methodNotAllowed(w)
		}
	})
}

func replyItemHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := pathTail(r.URL.Path, "/api/v1/replies/")
		if id == "" {
			writeError(w, http.StatusBadRequest, "missing reply id")
			return
		}
		if strings.Contains(id, "/") {
			writeError(w, http.StatusNotFound, "not found")
			return
		}

		switch r.Method {
		case http.MethodPut:
			agent := currentAgent(r.Context())
			if agent == nil {
				writeError(w, http.StatusUnauthorized, "missing auth context")
				return
			}
			reply, err := db.GetContent(r.Context(), database, id)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "reply not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to read reply")
				return
			}
			if reply.Type != "reply" {
				writeError(w, http.StatusNotFound, "reply not found")
				return
			}
			if reply.Author != agent.Name && agent.Role != "admin" {
				writeError(w, http.StatusForbidden, "not allowed to edit this reply")
				return
			}
			var req updateReplyRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid json payload")
				return
			}
			updated, err := db.UpdateReply(r.Context(), database, id, req.Body, agent.Name)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "reply not found")
					return
				}
				if strings.Contains(err.Error(), "body is required") {
					writeError(w, http.StatusBadRequest, err.Error())
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to update reply")
				return
			}
			writeJSON(w, http.StatusOK, updated)
		case http.MethodDelete:
			agent := currentAgent(r.Context())
			if agent == nil {
				writeError(w, http.StatusUnauthorized, "missing auth context")
				return
			}
			reply, err := db.GetContent(r.Context(), database, id)
			if err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "reply not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to read reply")
				return
			}
			if reply.Type != "reply" {
				writeError(w, http.StatusNotFound, "reply not found")
				return
			}
			if reply.Author != agent.Name && agent.Role != "admin" {
				writeError(w, http.StatusForbidden, "not allowed to delete this reply")
				return
			}
			if err := db.DeleteReply(r.Context(), database, id); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					writeError(w, http.StatusNotFound, "reply not found")
					return
				}
				writeError(w, http.StatusInternalServerError, "failed to delete reply")
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			methodNotAllowed(w)
		}
	})
}

func threadHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/posts/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "thread" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		id := parts[0]

		threadID, err := db.ResolveThreadID(r.Context(), database, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "content not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to resolve thread")
			return
		}
		items, err := db.ListThreadContent(r.Context(), database, threadID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load thread")
			return
		}
		if sinceRaw := strings.TrimSpace(r.URL.Query().Get("since")); sinceRaw != "" {
			since, err := parseSince(sinceRaw)
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid since value")
				return
			}
			items = filterThreadItemsSince(items, since)
		}
		root, ok := buildThreadTree(items)
		if !ok {
			writeError(w, http.StatusInternalServerError, "thread assembly failed")
			return
		}
		if strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("format")), "raw") {
			depth, err := parseDepth(r.URL.Query().Get("depth"))
			if err != nil {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			maxTokens := 0
			if rawMax := strings.TrimSpace(r.URL.Query().Get("max_tokens")); rawMax != "" {
				n, err := strconv.Atoi(rawMax)
				if err != nil || n < 0 {
					writeError(w, http.StatusBadRequest, "invalid max_tokens value")
					return
				}
				maxTokens = n
			}
			raw := renderThreadRaw(root, depth)
			raw = truncateByMaxTokens(raw, maxTokens)
			w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(raw))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"thread": root})
	})
}

func postTagsHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			methodNotAllowed(w)
			return
		}
		agent := currentAgent(r.Context())
		if agent == nil {
			writeError(w, http.StatusUnauthorized, "missing auth context")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/posts/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "tags" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		id := parts[0]

		post, err := db.GetContent(r.Context(), database, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load post")
			return
		}
		if post.Type != "post" {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		if post.Author != agent.Name && agent.Role != "admin" {
			writeError(w, http.StatusForbidden, "not allowed to modify tags for this post")
			return
		}

		var req updateTagsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json payload")
			return
		}
		tags, err := db.UpdatePostTags(r.Context(), database, id, req.Add, req.Remove)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to update tags")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":   id,
			"tags": tags,
		})
	})
}

func postStatusHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			methodNotAllowed(w)
			return
		}
		agent := currentAgent(r.Context())
		if agent == nil {
			writeError(w, http.StatusUnauthorized, "missing auth context")
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/posts/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "status" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		id := parts[0]
		post, err := db.GetContent(r.Context(), database, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load post")
			return
		}
		if post.Type != "post" {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}

		var req updateStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json payload")
			return
		}
		req.Status = strings.TrimSpace(req.Status)
		if req.Status == "" {
			writeError(w, http.StatusBadRequest, "status is required")
			return
		}

		if req.Status == "pin" {
			req.Status = "pinned"
		}
		switch req.Status {
		case "closed", "open":
			if post.Author != agent.Name && agent.Role != "admin" {
				writeError(w, http.StatusForbidden, "not allowed to change this thread status")
				return
			}
		case "pinned":
			if agent.Role != "admin" {
				writeError(w, http.StatusForbidden, "admin role required to pin")
				return
			}
		default:
			writeError(w, http.StatusBadRequest, "invalid status")
			return
		}
		updated, err := db.UpdatePostStatus(r.Context(), database, id, req.Status)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			if strings.Contains(err.Error(), "invalid status") {
				writeError(w, http.StatusBadRequest, "invalid status")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to update status")
			return
		}
		emitWebhookEvent(database, "status.changed", map[string]any{
			"id":         updated.ID,
			"thread_id":  updated.ThreadID,
			"status":     updated.Status,
			"changed_by": agent.Name,
		})
		writeJSON(w, http.StatusOK, updated)
	})
}

func postHistoryHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/posts/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "history" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		id := parts[0]
		content, err := db.GetContent(r.Context(), database, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "post not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load post")
			return
		}
		if content.Type != "post" {
			writeError(w, http.StatusNotFound, "post not found")
			return
		}
		history, err := db.ListContentHistory(r.Context(), database, id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load history")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"id":      id,
			"history": history,
		})
	})
}

func postSummaryHandler(database *sql.DB) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w)
			return
		}
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/posts/")
		parts := strings.Split(strings.Trim(path, "/"), "/")
		if len(parts) != 2 || parts[1] != "summary" || parts[0] == "" {
			writeError(w, http.StatusNotFound, "not found")
			return
		}
		id := parts[0]
		threadID, err := db.ResolveThreadID(r.Context(), database, id)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "content not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to resolve thread")
			return
		}
		items, err := db.ListThreadContent(r.Context(), database, threadID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to load thread")
			return
		}
		root, ok := buildThreadTree(items)
		if !ok {
			writeError(w, http.StatusInternalServerError, "thread assembly failed")
			return
		}
		raw := renderThreadRaw(root, 0)
		summary := summarizeRaw(raw, 3)
		emitWebhookEvent(database, "summary.requested", map[string]any{
			"thread_id": threadID,
		})
		writeJSON(w, http.StatusOK, map[string]any{
			"thread_id": threadID,
			"summary":   summary,
		})
	})
}

func summarizeRaw(raw string, lines int) string {
	parts := strings.Split(raw, "\n")
	out := make([]string, 0, lines)
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || strings.HasPrefix(p, "---") {
			continue
		}
		out = append(out, p)
		if len(out) >= lines {
			break
		}
	}
	return strings.Join(out, "\n")
}

func parseLimitOffset(r *http.Request) (int, int) {
	limit := 20
	offset := 0
	q := r.URL.Query()
	if v := strings.TrimSpace(q.Get("limit")); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	if v := strings.TrimSpace(q.Get("offset")); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil && n >= 0 {
			offset = n
		}
	}
	return limit, offset
}

func parseListPostsParams(r *http.Request) (db.ListPostsParams, error) {
	limit, offset := parseLimitOffset(r)
	q := r.URL.Query()
	params := db.ListPostsParams{
		Limit:  limit,
		Offset: offset,
		Author: strings.TrimSpace(q.Get("author")),
		Tags:   normalizedTagFilters(q["tag"]),
		Board:  strings.TrimSpace(q.Get("board")),
		Status: strings.TrimSpace(q.Get("status")),
		Sort:   strings.TrimSpace(q.Get("sort")),
		Order:  strings.TrimSpace(q.Get("order")),
	}
	if params.Status != "" {
		switch params.Status {
		case "open", "closed", "pinned", "archived":
		default:
			return db.ListPostsParams{}, errors.New("invalid status filter")
		}
	}
	if params.Sort != "" {
		switch params.Sort {
		case "activity", "created", "replies":
		default:
			return db.ListPostsParams{}, errors.New("invalid sort value")
		}
	}
	if params.Order != "" {
		switch strings.ToLower(params.Order) {
		case "asc", "desc":
		default:
			return db.ListPostsParams{}, errors.New("invalid order value")
		}
	}

	if since := strings.TrimSpace(q.Get("since")); since != "" {
		t, err := parseSince(since)
		if err != nil {
			return db.ListPostsParams{}, err
		}
		params.Since = &t
	}
	return params, nil
}

func normalizedTagFilters(rawTags []string) []string {
	if len(rawTags) == 0 {
		return nil
	}
	out := make([]string, 0, len(rawTags))
	seen := make(map[string]struct{}, len(rawTags))
	for _, raw := range rawTags {
		tag := strings.TrimSpace(raw)
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

func parseSince(raw string) (time.Time, error) {
	if d, err := time.ParseDuration(raw); err == nil {
		return time.Now().UTC().Add(-d), nil
	}
	layouts := []string{
		time.RFC3339,
		"2006-01-02",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, raw); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, errors.New("invalid since value")
}

func parseDepth(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid depth value")
	}
	return n, nil
}

func filterThreadItemsSince(items []models.Content, since time.Time) []models.Content {
	if len(items) == 0 {
		return items
	}
	out := make([]models.Content, 0, len(items))
	for _, item := range items {
		if item.Type == "post" {
			out = append(out, item)
			continue
		}
		t, err := time.Parse(time.RFC3339, item.Created)
		if err != nil {
			continue
		}
		if !t.Before(since) {
			out = append(out, item)
		}
	}
	return out
}
