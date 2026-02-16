package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

var envPattern = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func Path() (string, error) {
	if cwd, err := os.Getwd(); err == nil {
		if localPath, ok, err := findLocalPath(cwd); err != nil {
			return "", err
		} else if ok {
			return localPath, nil
		}
	}
	return homePath()
}

func homePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".fora", "config.json"), nil
}

func findLocalPath(start string) (string, bool, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, ".fora", "config.json")
		info, err := os.Stat(candidate)
		if err == nil {
			if info.Mode().IsRegular() {
				return candidate, true, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", false, err
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false, nil
		}
		dir = parent
	}
}

func Load() (*Config, error) {
	p, err := Path()
	if err != nil {
		return nil, err
	}
	return LoadFromPath(p)
}

func LoadFromPath(p string) (*Config, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return defaultConfig(), nil
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
	if err := resolveConfigEnv(&c); err != nil {
		return nil, err
	}
	return &c, nil
}

func Save(c *Config) error {
	p, err := Path()
	if err != nil {
		return err
	}
	return SaveToPath(c, p)
}

func SaveToPath(c *Config, p string) error {
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

func resolveConfigEnv(c *Config) error {
	c.DefaultServer = resolveEnvPlaceholders(c.DefaultServer)
	for key, srv := range c.Servers {
		resolvedKey := resolveEnvPlaceholders(key)
		srv.URL = resolveEnvPlaceholders(srv.URL)
		srv.APIKey = resolveEnvPlaceholders(srv.APIKey)
		srv.Agent = resolveEnvPlaceholders(srv.Agent)
		srv.ConnectedAt = resolveEnvPlaceholders(srv.ConnectedAt)
		if resolvedKey != key {
			delete(c.Servers, key)
		}
		c.Servers[resolvedKey] = srv
	}
	for k, v := range c.Preferences {
		c.Preferences[k] = resolveEnvPlaceholders(v)
	}
	return nil
}

func resolveEnvPlaceholders(raw string) string {
	if !strings.Contains(raw, "${") {
		return raw
	}
	return envPattern.ReplaceAllStringFunc(raw, func(token string) string {
		matches := envPattern.FindStringSubmatch(token)
		if len(matches) != 2 {
			return token
		}
		if val, ok := os.LookupEnv(matches[1]); ok {
			return val
		}
		return ""
	})
}

func defaultConfig() *Config {
	return &Config{
		Version:       1,
		DefaultServer: "main",
		Servers:       map[string]Server{},
		Preferences: map[string]string{
			"default_format": "table",
			"watch_interval": "10s",
		},
	}
}
