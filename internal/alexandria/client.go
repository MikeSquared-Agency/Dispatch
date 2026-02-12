package alexandria

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Device struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	DeviceType string `json:"device_type"`
	OwnerID    string `json:"owner_id"`
	Identifier string `json:"identifier"`
}

type Client interface {
	ListDevices(ctx context.Context) ([]Device, error)
	GetDevicesByOwner(ctx context.Context, ownerID string) ([]Device, error)
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

type devicesResponse struct {
	Data []Device `json:"data"`
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

	var wrapper devicesResponse
	if err := json.Unmarshal(body, &wrapper); err != nil {
		return nil, err
	}
	return wrapper.Data, nil
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
