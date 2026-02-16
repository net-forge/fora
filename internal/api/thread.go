package api

import "fora/internal/models"

type treeNode struct {
	val     models.ThreadNode
	replies []*treeNode
}

func buildThreadTree(items []models.Content) (models.ThreadNode, bool) {
	if len(items) == 0 {
		return models.ThreadNode{}, false
	}

	nodes := make(map[string]*treeNode, len(items))
	for _, c := range items {
		n := &treeNode{
			val: models.ThreadNode{
				ID:        c.ID,
				Type:      c.Type,
				Author:    c.Author,
				Title:     c.Title,
				Body:      c.Body,
				Created:   c.Created,
				Updated:   c.Updated,
				ThreadID:  c.ThreadID,
				ParentID:  c.ParentID,
				Status:    c.Status,
				ChannelID: c.ChannelID,
				Tags:      c.Tags,
				Replies:   []models.ThreadNode{},
			},
			replies: []*treeNode{},
		}
		nodes[c.ID] = n
	}

	var root *treeNode
	for _, c := range items {
		n := nodes[c.ID]
		if c.ParentID == nil {
			root = n
			continue
		}
		parent, ok := nodes[*c.ParentID]
		if !ok {
			continue
		}
		parent.replies = append(parent.replies, n)
	}

	if root == nil {
		return models.ThreadNode{}, false
	}
	return flattenThread(root), true
}

func flattenThread(n *treeNode) models.ThreadNode {
	out := n.val
	out.Replies = make([]models.ThreadNode, 0, len(n.replies))
	for _, c := range n.replies {
		out.Replies = append(out.Replies, flattenThread(c))
	}
	return out
}
