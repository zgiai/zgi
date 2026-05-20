package client

import (
	"context"
	"fmt"
	"net/http"

	"github.com/zgiai/zgi/api/internal/modules/pluginrunner/model"
)

// ============================================
// Invoke API
// ============================================

// Invoke performs a generic invoke request
func (c *Client) Invoke(ctx context.Context, req model.InvokeRequest, opts ...RequestOption) (*model.InvokeResponse, error) {
	var result model.InvokeResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/invoke", req, &result, opts...); err != nil {
		return nil, fmt.Errorf("invoke failed: %w", err)
	}
	return &result, nil
}

// InvokeTool performs a tool invoke request
func (c *Client) InvokeTool(ctx context.Context, req model.ToolInvokeRequest, opts ...RequestOption) (*model.InvokeResponse, error) {
	var result model.InvokeResponse
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/invoke/tool", req, &result, opts...); err != nil {
		return nil, fmt.Errorf("invoke tool failed: %w", err)
	}
	return &result, nil
}

// ============================================
// Multi-Tenant API
// ============================================

// Deprecated: tenant bindings are legacy and will be removed.
// CreateWorkspace creates a new tenant.
func (c *Client) CreateWorkspace(ctx context.Context, name string, opts ...RequestOption) (*model.Tenant, error) {
	req := model.CreateTenantRequest{
		Name: name,
	}

	var result model.Tenant
	if err := c.doRequest(ctx, http.MethodPost, "/api/v1/tenants", req, &result, opts...); err != nil {
		return nil, fmt.Errorf("create tenant failed: %w", err)
	}
	return &result, nil
}

// Deprecated: tenant bindings are legacy and will be removed.
// ListPluginTenants returns all tenant bindings for a plugin.
func (c *Client) ListPluginTenants(ctx context.Context, pluginID string, opts ...RequestOption) ([]model.PluginTenantBinding, error) {
	var result []model.PluginTenantBinding
	path := fmt.Sprintf("/api/v1/plugins/%s/tenants", pluginID)
	if err := c.doRequest(ctx, http.MethodGet, path, nil, &result, opts...); err != nil {
		return nil, fmt.Errorf("list plugin tenants failed: %w", err)
	}
	return result, nil
}

// Deprecated: tenant bindings are legacy and will be removed.
// EnablePluginTenant enables a plugin for a tenant with optional config.
func (c *Client) EnablePluginTenant(ctx context.Context, pluginID string, tenantID uint, config *model.TenantConfig, opts ...RequestOption) error {
	path := fmt.Sprintf("/api/v1/plugins/%s/tenants/%d/enable", pluginID, tenantID)
	if err := c.doRequest(ctx, http.MethodPost, path, config, nil, opts...); err != nil {
		return fmt.Errorf("enable plugin tenant failed: %w", err)
	}
	return nil
}

// Deprecated: tenant bindings are legacy and will be removed.
// DisablePluginTenant disables a plugin for a tenant.
func (c *Client) DisablePluginTenant(ctx context.Context, pluginID string, tenantID uint, opts ...RequestOption) error {
	path := fmt.Sprintf("/api/v1/plugins/%s/tenants/%d/disable", pluginID, tenantID)
	if err := c.doRequest(ctx, http.MethodPost, path, nil, nil, opts...); err != nil {
		return fmt.Errorf("disable plugin tenant failed: %w", err)
	}
	return nil
}
