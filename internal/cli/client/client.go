package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  strings.TrimSpace(apiKey),
		http: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

func (c *Client) Get(path string, out any) error {
	return c.do(http.MethodGet, path, nil, out)
}

func (c *Client) Post(path string, body any, out any) error {
	return c.do(http.MethodPost, path, body, out)
}

func (c *Client) Put(path string, body any, out any) error {
	return c.do(http.MethodPut, path, body, out)
}

func (c *Client) Patch(path string, body any, out any) error {
	return c.do(http.MethodPatch, path, body, out)
}

func (c *Client) Delete(path string) error {
	return c.do(http.MethodDelete, path, nil, nil)
}

func (c *Client) GetRaw(path string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func (c *Client) do(method, path string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, c.baseURL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err == nil {
			if msg, ok := payload["error"].(string); ok {
				return fmt.Errorf("http %d: %s", resp.StatusCode, msg)
			}
		}
		return fmt.Errorf("http %d", resp.StatusCode)
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
