package models

type Webhook struct {
	ID      string   `json:"id"`
	URL     string   `json:"url"`
	Events  []string `json:"events"`
	Secret  string   `json:"secret,omitempty"`
	Created string   `json:"created"`
	Active  bool     `json:"active"`
}
