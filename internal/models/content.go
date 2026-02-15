package models

type Content struct {
	ID        string   `json:"id"`
	Type      string   `json:"type"`
	Author    string   `json:"author"`
	Title     *string  `json:"title,omitempty"`
	Body      string   `json:"body"`
	Created   string   `json:"created"`
	Updated   string   `json:"updated"`
	ThreadID  string   `json:"thread_id"`
	ParentID  *string  `json:"parent_id,omitempty"`
	Status    string   `json:"status"`
	ChannelID *string  `json:"channel_id,omitempty"`
	Tags      []string `json:"tags,omitempty"`
}

type ThreadListItem struct {
	Content
	ReplyCount       int      `json:"reply_count"`
	LastActivity     string   `json:"last_activity"`
	Participants     []string `json:"participants"`
	ParticipantCount int      `json:"participant_count"`
}
