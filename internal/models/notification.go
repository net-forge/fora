package models

type Notification struct {
	ID        string `json:"id"`
	Recipient string `json:"recipient"`
	Type      string `json:"type"`
	FromAgent string `json:"from_agent"`
	ThreadID  string `json:"thread_id,omitempty"`
	ContentID string `json:"content_id,omitempty"`
	Preview   string `json:"preview,omitempty"`
	Created   string `json:"created"`
	Read      bool   `json:"read"`
}
