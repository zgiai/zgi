package client

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"

	"github.com/zgiai/ginext/internal/modules/pluginrunner/model"
)

// ============================================
// Plugin Management API
// ============================================

// RegisterPlugin registers a new plugin with the given manifest
func (c *Client) RegisterPlugin(ctx context.Context, manifest model.PluginManifest, opts ...RequestOption) (*model.Plugin, error) {
	req := model.RegisterPluginRequest{
		Manifest: manifest,
	}

	var result model.Plugin
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/plugins", req, &result, opts...); err != nil {
		return nil, fmt.Errorf("register plugin failed: %w", err)
	}
	return &result, nil
}

// ListPlugins returns all registered plugins
func (c *Client) ListPlugins(ctx context.Context, opts ...RequestOption) ([]model.Plugin, error) {
	var result []model.Plugin
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/plugins", nil, &result, opts...); err != nil {
		return nil, fmt.Errorf("list plugins failed: %w", err)
	}
	return result, nil
}

// GetPlugin returns the details of a specific plugin
func (c *Client) GetPlugin(ctx context.Context, pluginID string, opts ...RequestOption) (*model.Plugin, error) {
	var result model.Plugin
	path := fmt.Sprintf("/api/v1/plugins/%s", pluginID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &result, opts...); err != nil {
		return nil, fmt.Errorf("get plugin failed: %w", err)
	}
	return &result, nil
}

// DeletePlugin deletes a plugin by ID
func (c *Client) DeletePlugin(ctx context.Context, pluginID string, opts ...RequestOption) error {
	path := fmt.Sprintf("/api/v1/plugins/%s", pluginID)
	if err := c.doRequest(ctx, http.MethodDelete, path, nil, nil, opts...); err != nil {
		return fmt.Errorf("delete plugin failed: %w", err)
	}
	return nil
}

// InstallPluginWithFile installs a plugin package using multipart file upload
func (c *Client) InstallPluginWithFile(ctx context.Context, pluginID string, packageContent []byte, force bool, opts ...RequestOption) (*model.Installation, error) {
	path := fmt.Sprintf("/api/v1/plugins/%s/install", pluginID)

	formFields := map[string]string{}
	if force {
		formFields["force"] = "true"
	} else {
		formFields["force"] = "false"
	}

	// FIXME: When user A installs a plugin, and user B also needs to install this plugin,
	// we should first check if the plugin is already installed. If it is already installed,
	// then record it in the zgi api database.
	// If the same user performs repeated installation, directly prompt that the plugin is already installed.
	var result model.Installation
	if err := c.doMultipartRequest(ctx, path, "file", packageContent, "plugin.zip", formFields, &result, opts...); err != nil {
		return nil, fmt.Errorf("install plugin failed: %w", err)
	}
	return &result, nil
}

// InstallPluginWithBase64 installs a plugin package using base64-encoded content
func (c *Client) InstallPluginWithBase64(ctx context.Context, pluginID string, packageContent []byte, force bool, opts ...RequestOption) (*model.Installation, error) {
	path := fmt.Sprintf("/api/v1/plugins/%s/install", pluginID)

	req := model.InstallPluginRequest{
		PackageBase64: base64.StdEncoding.EncodeToString(packageContent),
		Force:         force,
	}

	var result model.Installation
	if err := c.doRequest(ctx, http.MethodPost, path, req, &result, opts...); err != nil {
		return nil, fmt.Errorf("install plugin failed: %w", err)
	}
	return &result, nil
}

// ListInstalledPlugins returns all installed plugins
func (c *Client) ListInstalledPlugins(ctx context.Context, opts ...RequestOption) ([]model.Installation, error) {
	var result []model.Installation
	if err := c.doRequest(ctx, http.MethodGet, "/api/v1/plugins/installed", nil, &result, opts...); err != nil {
		return nil, fmt.Errorf("list installed plugins failed: %w", err)
	}
	return result, nil
}
