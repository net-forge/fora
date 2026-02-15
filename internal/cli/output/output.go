package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

func DefaultFormat() string {
	if isatty.IsTerminal(os.Stdout.Fd()) {
		return "table"
	}
	return "json"
}

func Print(payload map[string]any, format string, quiet bool) error {
	if quiet {
		format = "quiet"
	}
	format = strings.TrimSpace(strings.ToLower(format))
	if format == "" {
		format = DefaultFormat()
	}

	switch format {
	case "json":
		return printJSON(payload)
	case "table":
		return printTable(payload)
	case "plain":
		return printPlain(payload)
	case "md":
		return printMarkdown(payload)
	case "quiet":
		return printQuiet(payload)
	default:
		return errors.New("invalid --format value")
	}
}

func printJSON(v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func printTable(payload map[string]any) error {
	switch {
	case hasKey(payload, "agents"):
		fmt.Println("NAME\tROLE\tLAST_ACTIVE\tCREATED")
		for _, row := range toObjectSlice(payload["agents"]) {
			fmt.Printf("%s\t%s\t%s\t%s\n",
				str(row["name"]), str(row["role"]), str(row["last_active"]), str(row["created"]))
		}
	case hasKey(payload, "threads"):
		fmt.Println("ID\tAUTHOR\tTITLE\tSTATUS\tCREATED")
		for _, row := range toObjectSlice(payload["threads"]) {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
				str(row["id"]), str(row["author"]), str(row["title"]), str(row["status"]), str(row["created"]))
		}
	case hasKey(payload, "activity"):
		fmt.Println("ID\tTYPE\tAUTHOR\tTHREAD\tCREATED")
		for _, row := range toObjectSlice(payload["activity"]) {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
				str(row["id"]), str(row["type"]), str(row["author"]), str(row["thread_id"]), str(row["created"]))
		}
	case hasKey(payload, "results"):
		fmt.Println("ID\tTYPE\tAUTHOR\tCREATED\tSNIPPET")
		for _, row := range toObjectSlice(payload["results"]) {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
				str(row["id"]), str(row["type"]), str(row["author"]), str(row["created"]), str(row["snippet"]))
		}
	case hasKey(payload, "notifications"):
		fmt.Println("ID\tTYPE\tFROM\tTHREAD\tCREATED")
		for _, row := range toObjectSlice(payload["notifications"]) {
			fmt.Printf("%s\t%s\t%s\t%s\t%s\n",
				str(row["id"]), str(row["type"]), str(row["from_agent"]), str(row["thread_id"]), str(row["created"]))
		}
	default:
		return printJSON(payload)
	}
	return nil
}

func printPlain(payload map[string]any) error {
	switch {
	case hasKey(payload, "agents"):
		for _, row := range toObjectSlice(payload["agents"]) {
			fmt.Printf("%s %s\n", str(row["name"]), str(row["role"]))
		}
	case hasKey(payload, "name") && hasKey(payload, "role"):
		fmt.Printf("%s %s\n", str(payload["name"]), str(payload["role"]))
	case hasKey(payload, "threads"):
		for _, row := range toObjectSlice(payload["threads"]) {
			fmt.Printf("%s %s %s\n", str(row["id"]), str(row["author"]), str(row["title"]))
		}
	case hasKey(payload, "activity"):
		for _, row := range toObjectSlice(payload["activity"]) {
			fmt.Printf("%s %s %s\n", str(row["id"]), str(row["type"]), str(row["author"]))
		}
	case hasKey(payload, "results"):
		for _, row := range toObjectSlice(payload["results"]) {
			fmt.Printf("%s %s %s\n", str(row["id"]), str(row["type"]), str(row["snippet"]))
		}
	case hasKey(payload, "notifications"):
		for _, row := range toObjectSlice(payload["notifications"]) {
			fmt.Printf("%s %s from=%s\n", str(row["id"]), str(row["type"]), str(row["from_agent"]))
		}
	default:
		return printJSON(payload)
	}
	return nil
}

func printMarkdown(payload map[string]any) error {
	switch {
	case hasKey(payload, "agents"):
		for _, row := range toObjectSlice(payload["agents"]) {
			fmt.Printf("- `%s` (%s)\n", str(row["name"]), str(row["role"]))
		}
	case hasKey(payload, "name") && hasKey(payload, "role"):
		fmt.Printf("- `%s` (%s)\n", str(payload["name"]), str(payload["role"]))
	case hasKey(payload, "threads"):
		for _, row := range toObjectSlice(payload["threads"]) {
			fmt.Printf("- `%s` **%s** by %s (%s)\n",
				str(row["id"]), str(row["title"]), str(row["author"]), str(row["status"]))
		}
	case hasKey(payload, "activity"):
		for _, row := range toObjectSlice(payload["activity"]) {
			fmt.Printf("- `%s` %s by %s in thread `%s`\n",
				str(row["id"]), str(row["type"]), str(row["author"]), str(row["thread_id"]))
		}
	case hasKey(payload, "results"):
		for _, row := range toObjectSlice(payload["results"]) {
			fmt.Printf("- `%s` %s: %s\n",
				str(row["id"]), str(row["type"]), str(row["snippet"]))
		}
	case hasKey(payload, "notifications"):
		for _, row := range toObjectSlice(payload["notifications"]) {
			fmt.Printf("- `%s` %s from %s\n",
				str(row["id"]), str(row["type"]), str(row["from_agent"]))
		}
	default:
		return printJSON(payload)
	}
	return nil
}

func printQuiet(payload map[string]any) error {
	switch {
	case hasKey(payload, "agents"):
		for _, row := range toObjectSlice(payload["agents"]) {
			fmt.Println(str(row["name"]))
		}
	case hasKey(payload, "name"):
		fmt.Println(str(payload["name"]))
	case hasKey(payload, "threads"):
		for _, row := range toObjectSlice(payload["threads"]) {
			fmt.Println(str(row["id"]))
		}
	case hasKey(payload, "activity"):
		for _, row := range toObjectSlice(payload["activity"]) {
			fmt.Println(str(row["id"]))
		}
	case hasKey(payload, "results"):
		for _, row := range toObjectSlice(payload["results"]) {
			fmt.Println(str(row["id"]))
		}
	case hasKey(payload, "notifications"):
		for _, row := range toObjectSlice(payload["notifications"]) {
			fmt.Println(str(row["id"]))
		}
	default:
		if id, ok := payload["id"]; ok {
			fmt.Println(str(id))
			return nil
		}
		return printJSON(payload)
	}
	return nil
}

func hasKey(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func toObjectSlice(v any) []map[string]any {
	in, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(in))
	for _, item := range in {
		if row, ok := item.(map[string]any); ok {
			out = append(out, row)
		}
	}
	return out
}

func str(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	default:
		return fmt.Sprintf("%v", t)
	}
}
