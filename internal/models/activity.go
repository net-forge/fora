package models

type ActivityEvent struct {
	ID       string  `json:"id"`
	Type     string  `json:"type"`
	Author   string  `json:"author"`
	Title    *string `json:"title,omitempty"`
	ThreadID string  `json:"thread_id"`
	ParentID *string `json:"parent_id,omitempty"`
	Created  string  `json:"created"`
}
