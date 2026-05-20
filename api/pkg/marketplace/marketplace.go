package marketplace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	appconfig "github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/observability"
	"github.com/zgiai/ginext/pkg/logger"
)

// MarketplacePluginDeclaration represents plugin information from marketplace
type MarketplacePluginDeclaration struct {
	ID                      string          `json:"id"`
	PluginID                string          `json:"plugin_id"`
	Name                    string          `json:"name"`
	Org                     string          `json:"org"`
	AgentStrategy           json.RawMessage `json:"agent_strategy"`
	Badges                  json.RawMessage `json:"badges"`
	Brief                   json.RawMessage `json:"brief"`
	Category                string          `json:"category"`
	Endpoint                json.RawMessage `json:"endpoint"`
	Icon                    string          `json:"icon"`
	IndexID                 string          `json:"index_id"`
	InstallCount            int64           `json:"install_count"`
	Introduction            string          `json:"introduction"`
	Label                   json.RawMessage `json:"label"`
	LatestPackageIdentifier string          `json:"latest_package_identifier"`
	LatestVersion           string          `json:"latest_version"`
	Model                   json.RawMessage `json:"model"`
	Plugins                 json.RawMessage `json:"plugins"`
	PrivacyOptions          string          `json:"privacy_options"`
	PrivacyPolicy           string          `json:"privacy_policy"`
	Repository              string          `json:"repository"`
	Resource                json.RawMessage `json:"resource"`
	Status                  string          `json:"status"`
	Tags                    json.RawMessage `json:"tags"`
	Tool                    json.RawMessage `json:"tool"`
	Type                    string          `json:"type"`
	CreatedAt               string          `json:"created_at,omitempty"`
	UpdatedAt               string          `json:"updated_at,omitempty"`
	VersionUpdatedAt        string          `json:"version_updated_at,omitempty"`
}

// BatchFetchPluginManifests fetches plugin manifests from marketplace
func BatchFetchPluginManifests(ctx context.Context, pluginIDs []string) ([]MarketplacePluginDeclaration, error) {
	if len(pluginIDs) == 0 {
		return []MarketplacePluginDeclaration{}, nil
	}

	marketplaceCfg := appconfig.Current().Marketplace
	marketplaceAPIURL := marketplaceCfg.APIURL
	if marketplaceAPIURL == "" {
		marketplaceAPIURL = "https://market.zgiai.com"
	}

	marketplaceSource := marketplaceCfg.Source
	if marketplaceSource == "" {
		marketplaceSource = "zgi"
	}

	// Build request URL
	url := fmt.Sprintf("%s/api/v1/plugins/batch", marketplaceAPIURL)

	// Build request body
	requestBody := map[string]interface{}{
		"plugin_ids": pluginIDs,
		"source":     marketplaceSource,
	}

	jsonData, err := json.Marshal(requestBody)
	if err != nil {
		logger.Error("Failed to marshal marketplace request body", err)
		return nil, fmt.Errorf("failed to marshal request body: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		logger.Error("Failed to create marketplace request", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	// Create HTTP client with timeout
	client := observability.HTTPClient(&http.Client{
		Timeout: 30 * time.Second,
	})

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to execute marketplace request", err)
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read marketplace response body", err)
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		logger.Error("Marketplace returned non-200 status",
			"status", resp.StatusCode,
			"body_bytes", len(bodyBytes),
		)
		return nil, fmt.Errorf("marketplace returned status %d", resp.StatusCode)
	}

	// Parse response
	var response struct {
		Data struct {
			Plugins []MarketplacePluginDeclaration `json:"plugins"`
		} `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &response); err != nil {
		logger.Error("Failed to unmarshal marketplace response", err)
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return response.Data.Plugins, nil
}

// DownloadPluginPkg downloads plugin package from marketplace
func DownloadPluginPkg(ctx context.Context, pluginUniqueIdentifier string) ([]byte, error) {
	marketplaceAPIURL := appconfig.Current().Marketplace.APIURL
	if marketplaceAPIURL == "" {
		marketplaceAPIURL = "https://market.zgiai.com"
	}

	// Build download URL
	url := fmt.Sprintf("%s/api/v1/plugins/download?unique_identifier=%s", marketplaceAPIURL, pluginUniqueIdentifier)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		logger.Error("Failed to create plugin download request", err)
		return nil, fmt.Errorf("failed to create download request: %w", err)
	}

	// Create HTTP client with timeout
	// Plugin packages can be large (up to 100MB+), so use a longer timeout
	client := observability.HTTPClient(&http.Client{
		Timeout: 5 * time.Minute, // 5 minutes timeout for downloading large plugin packages
	})

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		logger.Error("Failed to execute plugin download request", err)
		return nil, fmt.Errorf("failed to execute download request: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		logger.Error("Marketplace download returned non-200 status",
			"status", resp.StatusCode,
			"body_bytes", len(bodyBytes),
		)
		return nil, fmt.Errorf("marketplace download returned status %d", resp.StatusCode)
	}

	// Read package content
	pkgBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read plugin package content", err)
		return nil, fmt.Errorf("failed to read package content: %w", err)
	}

	return pkgBytes, nil
}
