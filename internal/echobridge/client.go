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

	"github.com/google/uuid"
)

// keyNamespaceID is the UUIDv5 namespace for auto-poster's idempotency keys.
// It keeps the keys apart from other EchoBridge clients' keys. Never change
// it: existing keys would no longer match.
const keyNamespaceID = "32ae211b-6cb1-4ae7-9612-f3181caea009"

var keyNamespace = uuid.MustParse(keyNamespaceID)

// IdempotencyKey returns the fixed Idempotency-Key for posting one blog post
// (feed URL and GUID) to one account. Retries reuse it, so EchoBridge returns
// the stored result instead of posting twice.
func IdempotencyKey(feedURL, guid, accountID string) string {
	return uuid.NewSHA1(keyNamespace, []byte(feedURL+"\n"+guid+"\n"+accountID)).String()
}

type Client struct {
	BaseURL string // e.g. https://host/echobridge
	// Token, if set, is sent as "Authorization: Bearer <Token>".
	Token string
	HTTP  *http.Client
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
// sends one request per account with text sized for that platform. key is
// sent as the Idempotency-Key header; see IdempotencyKey.
func (c *Client) Post(ctx context.Context, key, accountID, content string) (*Result, error) {
	body, _ := json.Marshal(map[string]any{
		"content":     content,
		"account_ids": []string{accountID},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url("/api/posts"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", key)
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
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
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
