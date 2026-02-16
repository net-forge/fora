package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	Version       int               `json:"version"`
	DefaultServer string            `json:"default_server"`
	Servers       map[string]Server `json:"servers"`
	Preferences   map[string]string `json:"preferences,omitempty"`
}

type Server struct {
	URL         string `json:"url"`
	APIKey      string `json:"api_key"`
	Agent       string `json:"agent,omitempty"`
	ConnectedAt string `json:"connected_at"`
}

func Path() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".fora", "config.json"), nil
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &Config{
				Version:       1,
				DefaultServer: "main",
				Servers:       map[string]Server{},
				Preferences: map[string]string{
					"default_format": "table",
					"watch_interval": "10s",
				},
			}, nil
		}
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if c.Servers == nil {
		c.Servers = map[string]Server{}
	}
	if c.DefaultServer == "" {
		c.DefaultServer = "main"
	}
	if c.Version == 0 {
		c.Version = 1
	}
	return &c, nil
}

func Save(c *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, append(b, '\n'), 0o600)
}

func (c *Config) SetDefault(url, apiKey string) {
	if c.Servers == nil {
		c.Servers = map[string]Server{}
	}
	c.Servers["main"] = Server{
		URL:         url,
		APIKey:      apiKey,
		ConnectedAt: time.Now().UTC().Format(time.RFC3339),
	}
	c.DefaultServer = "main"
}

func (c *Config) ClearDefault() {
	delete(c.Servers, c.DefaultServer)
}

func (c *Config) Default() (Server, bool) {
	s, ok := c.Servers[c.DefaultServer]
	return s, ok
}
