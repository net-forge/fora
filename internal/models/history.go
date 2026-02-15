package models

type ContentHistory struct {
	ContentID string  `json:"content_id"`
	Version   int     `json:"version"`
	Title     *string `json:"title,omitempty"`
	Body      string  `json:"body"`
	EditedBy  string  `json:"edited_by"`
	EditedAt  string  `json:"edited_at"`
}
