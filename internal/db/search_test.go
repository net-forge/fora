package db

import (
	"context"
	"database/sql"
	"os"
	"testing"
)

func TestCountSearchContentIgnoresPaginationAndRespectsFilters(t *testing.T) {
	ctx := context.Background()
	database, dbPath := openTestDB(t, "search-count.db")
	defer database.Close()
	defer os.Remove(dbPath)

	if err := CreateAgent(ctx, database, "alice", "admin", "hash-alice", nil); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if err := CreateAgent(ctx, database, "bob", "agent", "hash-bob", nil); err != nil {
		t.Fatalf("create bob: %v", err)
	}

	postA, err := CreatePost(ctx, database, "alice", strPtr("Auth A"), "authentication alpha", []string{"security"}, nil, nil)
	if err != nil {
		t.Fatalf("create post A: %v", err)
	}
	if _, err := CreateReply(ctx, database, "bob", postA.ID, "authentication beta", nil); err != nil {
		t.Fatalf("create reply: %v", err)
	}
	if _, err := CreatePost(ctx, database, "bob", strPtr("Auth B"), "authentication gamma", []string{"security"}, nil, nil); err != nil {
		t.Fatalf("create post B: %v", err)
	}
	if _, err := CreatePost(ctx, database, "alice", strPtr("Unrelated"), "deployment checklist", []string{"ops"}, nil, nil); err != nil {
		t.Fatalf("create unrelated post: %v", err)
	}

	assertCountMatchesFullResults(t, ctx, database, SearchParams{
		Query:  "authentication",
		Limit:  1,
		Offset: 1,
	})
	assertCountMatchesFullResults(t, ctx, database, SearchParams{
		Query:       "authentication",
		Author:      "bob",
		Limit:       1,
		Offset:      1,
		ThreadsOnly: false,
	})
	assertCountMatchesFullResults(t, ctx, database, SearchParams{
		Query:  "authentication",
		Tag:    "security",
		Limit:  1,
		Offset: 1,
	})
	assertCountMatchesFullResults(t, ctx, database, SearchParams{
		Query:       "authentication",
		ThreadsOnly: true,
		Limit:       1,
		Offset:      1,
	})
}

func assertCountMatchesFullResults(t *testing.T, ctx context.Context, database *sql.DB, params SearchParams) {
	t.Helper()
	pagedResults, err := SearchContent(ctx, database, params)
	if err != nil {
		t.Fatalf("search content with pagination: %v", err)
	}
	total, err := CountSearchContent(ctx, database, params)
	if err != nil {
		t.Fatalf("count search content: %v", err)
	}

	fullParams := params
	fullParams.Limit = 100
	fullParams.Offset = 0
	fullResults, err := SearchContent(ctx, database, fullParams)
	if err != nil {
		t.Fatalf("search content without pagination: %v", err)
	}
	if total != len(fullResults) {
		t.Fatalf("count mismatch for params %+v: total=%d full_results=%d", params, total, len(fullResults))
	}
	if total <= len(pagedResults) {
		t.Fatalf("expected total > paged results for params %+v: total=%d paged=%d", params, total, len(pagedResults))
	}
}
