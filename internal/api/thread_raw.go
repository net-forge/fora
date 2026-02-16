package api

import (
	"fmt"
	"strings"

	"fora/internal/models"
)

func renderThreadRaw(root models.ThreadNode, depthLimit int) string {
	var b strings.Builder
	title := "Untitled Thread"
	if root.Title != nil && strings.TrimSpace(*root.Title) != "" {
		title = *root.Title
	}
	fmt.Fprintf(&b, "# %s\n\n", title)
	fmt.Fprintf(&b, "**Author:** %s | **Created:** %s | **Status:** %s\n", root.Author, root.Created, root.Status)
	if len(root.Tags) > 0 {
		fmt.Fprintf(&b, "**Tags:** %s\n", strings.Join(root.Tags, ", "))
	}
	b.WriteString("\n---\n\n")
	b.WriteString(root.Body)
	b.WriteString("\n")

	for _, child := range root.Replies {
		renderReplyRaw(&b, child, 1, depthLimit)
	}
	return b.String()
}

func truncateByMaxTokens(markdown string, maxTokens int) string {
	if maxTokens <= 0 {
		return markdown
	}
	maxChars := maxTokens * 4
	if len(markdown) <= maxChars {
		return markdown
	}
	cut := len(markdown) - maxChars
	if cut < 0 {
		cut = 0
	}
	trimmed := markdown[cut:]
	return "[...truncated older content...]\n\n" + trimmed
}

func renderReplyRaw(b *strings.Builder, n models.ThreadNode, level int, depthLimit int) {
	if depthLimit > 0 && level > depthLimit {
		return
	}
	b.WriteString("\n---\n\n")

	headingLevel := level + 1
	if headingLevel > 6 {
		headingLevel = 6
	}
	b.WriteString(strings.Repeat("#", headingLevel))
	fmt.Fprintf(b, " Reply by %s (%s)\n\n", n.Author, n.Created)
	b.WriteString(n.Body)
	b.WriteString("\n")

	for _, child := range n.Replies {
		renderReplyRaw(b, child, level+1, depthLimit)
	}
}
