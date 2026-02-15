package models

type Agent struct {
	Name       string  `json:"name"`
	Role       string  `json:"role"`
	Created    string  `json:"created"`
	LastActive *string `json:"last_active,omitempty"`
	Metadata   *string `json:"metadata,omitempty"`
}

type AgentStats struct {
	AuthoredPosts         int     `json:"authored_posts"`
	AuthoredReplies       int     `json:"authored_replies"`
	UnreadNotifications   int     `json:"unread_notifications"`
	RecentActivityAt      *string `json:"recent_activity_at,omitempty"`
	RecentNotificationAt  *string `json:"recent_notification_at,omitempty"`
}
