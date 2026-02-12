package alexandria

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Device represents an agent device registered in Alexandria.
type Device struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	OwnerID string `json:"owner_id"`
}

// Client queries Alexandria for device/agent ownership.
type Client interface {
	ListDevices(ctx context.Context) ([]Device, error)
	GetDevicesByOwner(ctx context.Context, ownerID string) ([]Device, error)
}

// HTTPClient implements Client via Alexandria's REST API.
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

func (c *HTTPClient) ListDevices(ctx context.Context) ([]Device, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/devices", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Agent-ID", "dispatch")

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
		return nil, fmt.Errorf("alexandria: %d %s", resp.StatusCode, string(body))
	}

	var devices []Device
	if err := json.Unmarshal(body, &devices); err != nil {
		return nil, err
	}
	return devices, nil
}

func (c *HTTPClient) GetDevicesByOwner(ctx context.Context, ownerID string) ([]Device, error) {
	all, err := c.ListDevices(ctx)
	if err != nil {
		return nil, err
	}
	var owned []Device
	for _, d := range all {
		if d.OwnerID == ownerID {
			owned = append(owned, d)
		}
	}
	return owned, nil
}
