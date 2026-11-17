// Package echobridge is a client for the EchoBridge cross-posting API.
package echobridge

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type Client struct {
	BaseURL string // e.g. https://host/echobridge
	HTTP    *http.Client
}

type Account struct {
	ID          string `json:"id"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
}

type Result struct {
	AccountID   string `json:"account_id"`
	Provider    string `json:"provider"`
	DisplayName string `json:"display_name"`
	Success     bool   `json:"success"`
	PostURL     string `json:"post_url,omitempty"`
	Error       string `json:"error,omitempty"`
}

func (c *Client) url(path string) string {
	return strings.TrimRight(c.BaseURL, "/") + path
}

func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url("/api/accounts"), nil)
	if err != nil {
		return nil, err
	}
	var out []Account
	return out, c.do(req, &out)
}

// Post publishes content to one account. EchoBridge accepts several accounts
// per request, but each platform has its own length limit, so the caller
// sends one request per account with text sized for that platform.
func (c *Client) Post(ctx context.Context, accountID, content string) (*Result, error) {
	body, _ := json.Marshal(map[string]any{
		"content":     content,
		"account_ids": []string{accountID},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/api/posts"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	var out struct {
		Results []Result `json:"results"`
	}
	if err := c.do(req, &out); err != nil {
		return nil, err
	}
	if len(out.Results) != 1 {
		return nil, fmt.Errorf("expected 1 result, got %d", len(out.Results))
	}
	return &out.Results[0], nil
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: status %d: %s", req.Method, req.URL.Path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.Unmarshal(body, out)
}
