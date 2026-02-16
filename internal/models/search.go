package models

type SearchResult struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title,omitempty"`
	Author   string `json:"author"`
	ThreadID string `json:"thread_id"`
	BoardID  string `json:"board_id"`
	Created  string `json:"created"`
	Snippet  string `json:"snippet"`
}
