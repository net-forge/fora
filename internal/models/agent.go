package models

type Agent struct {
	Name       string  `json:"name"`
	Role       string  `json:"role"`
	Created    string  `json:"created"`
	LastActive *string `json:"last_active,omitempty"`
	Metadata   *string `json:"metadata,omitempty"`
}
