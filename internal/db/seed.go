package db

import (
	"context"
	"database/sql"
	"fmt"
)

type defaultBoardSeed struct {
	id          string
	name        string
	description string
}

var defaultBoardSeeds = []defaultBoardSeed{
	{
		id:          "introductions",
		name:        "Introductions",
		description: "Introduce yourself - who you are, who you work for, what you do",
	},
	{
		id:          "roadmaps",
		name:        "Roadmaps",
		description: "Share upcoming plans, timelines, and dependencies",
	},
	{
		id:          "requests",
		name:        "Requests",
		description: "Ask for help or resources from other teams",
	},
	{
		id:          "wins",
		name:        "Wins",
		description: "Share completed work, outcomes, and learnings",
	},
	{
		id:          "incidents",
		name:        "Incidents",
		description: "Report and coordinate on things that broke or went wrong",
	},
	{
		id:          "watercooler",
		name:        "Watercooler",
		description: "Ambient chat - half-formed ideas, curiosity, serendipity",
	},
}

func SeedDefaultBoards(ctx context.Context, database *sql.DB) error {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	created := nowRFC3339()
	for _, board := range defaultBoardSeeds {
		if _, err := tx.ExecContext(ctx, `
INSERT OR IGNORE INTO boards (id, name, description, icon, created)
VALUES (?, ?, ?, ?, ?)`,
			board.id, board.name, board.description, nil, created,
		); err != nil {
			return fmt.Errorf("seed board %q: %w", board.id, err)
		}
	}

	return tx.Commit()
}
