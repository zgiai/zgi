package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	appconfig "github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/observability"
)

// MarketplaceClient handles communication with the Marketplace service
type MarketplaceClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

// NewMarketplaceClientFromEnv keeps the legacy name for compatibility, but now
// reads marketplace settings from the loaded application config.
func NewMarketplaceClientFromEnv() *MarketplaceClient {
	baseURL := appconfig.Current().Marketplace.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8025"
	}

	return &MarketplaceClient{
		BaseURL: baseURL,
		HTTPClient: observability.HTTPClient(&http.Client{
			Timeout: 2 * time.Minute, // Long timeout for large downloads
		}),
	}
}

// DownloadPlugin downloads a plugin package from the Marketplace
// Returns the raw ZIP file bytes
func (c *MarketplaceClient) DownloadPlugin(ctx context.Context, pluginID, versionID string) ([]byte, error) {
	url := fmt.Sprintf("%s/v1/market/plugins/%s/versions/%s/download", c.BaseURL, pluginID, versionID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("marketplace returned status %d: %s", resp.StatusCode, string(body))
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response failed: %w", err)
	}

	return data, nil
}

// GetPluginInfo fetches plugin metadata from the Marketplace
type PluginInfo struct {
	ID              string `json:"id"`
	UniqueID        string `json:"unique_identifier"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	LatestVersionID string `json:"latest_version_id,omitempty"`
	LatestVersion   string `json:"latest_version,omitempty"`
}

func (c *MarketplaceClient) GetPluginInfo(ctx context.Context, pluginID string) (*PluginInfo, error) {
	url := fmt.Sprintf("%s/v1/market/plugins/%s", c.BaseURL, pluginID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request failed: %w", err)
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("marketplace returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response - simplified, actual implementation should use proper JSON parsing
	// For now, we just return nil as we mainly need DownloadPlugin
	return nil, nil
}
