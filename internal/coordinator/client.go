package coordinator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	token      string
	agentID    string
	publicURL  string
	version    string
	httpClient *http.Client
}

func New(baseURL, token, agentID, publicURL, version string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &Client{
		baseURL:    strings.TrimSuffix(strings.TrimSpace(baseURL), "/"),
		token:      strings.TrimSpace(token),
		agentID:    strings.TrimSpace(agentID),
		publicURL:  strings.TrimSpace(publicURL),
		version:    strings.TrimSpace(version),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *Client) Enabled() bool {
	return c.baseURL != "" && c.agentID != "" && c.publicURL != ""
}

func (c *Client) Register(ctx context.Context) error {
	return c.send(ctx, "/internal/v1/agents/register")
}

func (c *Client) Heartbeat(ctx context.Context) error {
	return c.send(ctx, "/internal/v1/agents/heartbeat")
}

func (c *Client) send(ctx context.Context, path string) error {
	if !c.Enabled() {
		return fmt.Errorf("coordinator client is not configured")
	}

	reqBody, err := json.Marshal(map[string]string{
		"agent_id":   c.agentID,
		"public_url": c.publicURL,
		"version":    c.version,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal coordinator request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, bytes.NewReader(reqBody))
	if err != nil {
		return fmt.Errorf("failed to create coordinator request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("coordinator request failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("coordinator returned status %d: %s", resp.StatusCode, string(body))
	}
	return nil
}
