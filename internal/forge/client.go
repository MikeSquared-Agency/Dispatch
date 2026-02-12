package forge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Persona struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Capabilities []string `json:"capabilities"`
}

type Client interface {
	ListPersonas(ctx context.Context) ([]Persona, error)
	GetAgentsByCapability(ctx context.Context, scope string) ([]Persona, error)
}

type HTTPClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewHTTPClient(baseURL string) *HTTPClient {
	return &HTTPClient{
		baseURL:    baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

type personaResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Sections []struct {
		ID      string `json:"id"`
		Content string `json:"content"`
	} `json:"sections"`
}

func (c *HTTPClient) ListPersonas(ctx context.Context) ([]Persona, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/personas", nil)
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("promptforge: %d %s", resp.StatusCode, string(body))
	}

	var raw []personaResponse
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}

	var personas []Persona
	for _, r := range raw {
		p := Persona{ID: r.ID, Name: r.Name}
		for _, s := range r.Sections {
			if s.ID == "capabilities" {
				p.Capabilities = ParseCapabilities(s.Content)
			}
		}
		personas = append(personas, p)
	}
	return personas, nil
}

func (c *HTTPClient) GetAgentsByCapability(ctx context.Context, scope string) ([]Persona, error) {
	all, err := c.ListPersonas(ctx)
	if err != nil {
		return nil, err
	}
	var matched []Persona
	for _, p := range all {
		for _, cap := range p.Capabilities {
			if strings.EqualFold(cap, scope) {
				matched = append(matched, p)
				break
			}
		}
	}
	return matched, nil
}

func ParseCapabilities(content string) []string {
	parts := strings.Split(content, ",")
	var caps []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			caps = append(caps, strings.ToLower(p))
		}
	}
	return caps
}
