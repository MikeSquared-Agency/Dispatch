package warren

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type AgentState struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ready, sleeping, busy, degraded, stopped
}

type Client interface {
	GetAgentState(ctx context.Context, agentID string) (*AgentState, error)
	WakeAgent(ctx context.Context, agentID string) error
	ListAgents(ctx context.Context) ([]AgentState, error)
}

type HTTPClient struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

func NewHTTPClient(baseURL, token string) *HTTPClient {
	return &HTTPClient{
		baseURL:    baseURL,
		token:      token,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *HTTPClient) doReq(ctx context.Context, method, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("warren %s %s: %d %s", method, path, resp.StatusCode, string(body))
	}
	return body, nil
}

func (c *HTTPClient) GetAgentState(ctx context.Context, agentID string) (*AgentState, error) {
	data, err := c.doReq(ctx, "GET", "/admin/agents/"+agentID)
	if err != nil {
		return nil, err
	}
	var state AgentState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return &state, nil
}

func (c *HTTPClient) WakeAgent(ctx context.Context, agentID string) error {
	_, err := c.doReq(ctx, "POST", "/admin/agents/"+agentID+"/wake")
	return err
}

func (c *HTTPClient) ListAgents(ctx context.Context) ([]AgentState, error) {
	data, err := c.doReq(ctx, "GET", "/admin/agents")
	if err != nil {
		return nil, err
	}
	var agents []AgentState
	if err := json.Unmarshal(data, &agents); err != nil {
		return nil, err
	}
	return agents, nil
}
