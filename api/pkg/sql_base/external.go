package sql_base

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/observability"
	"github.com/zgiai/zgi/api/pkg/logger"
	"github.com/zgiai/zgi/api/pkg/sql_base/audit"
	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
	"go.uber.org/zap"
)

type externalClient struct {
	baseURL        string
	httpClient     *http.Client
	apiKey         string
	recorder       audit.Recorder
	policyProvider GuardPolicyProvider
}

func NewExternalClient(recorder audit.Recorder, policyProvider GuardPolicyProvider) (SQLBase, error) {
	sqlBaseCfg := appconfig.Current().SQLBase
	baseURL := sqlBaseCfg.ExternalURL
	if baseURL == "" {
		return nil, fmt.Errorf("SQL_BASE_EXTERNAL_URL is required for external postgres meta client")
	}

	apiKey := sqlBaseCfg.ExternalAPIKey

	return &externalClient{
		baseURL:        baseURL,
		httpClient:     observability.HTTPClient(&http.Client{}),
		apiKey:         apiKey,
		recorder:       recorder,
		policyProvider: policyProvider,
	}, nil
}

// Schema operations
func (c *externalClient) ListSchemas(ctx context.Context, opts ListSchemasOptions) ([]Schema, error) {
	url := fmt.Sprintf("%s/schemas?include_system_schemas=%t", c.baseURL, opts.IncludeSystemSchemas)
	if opts.Limit > 0 {
		url += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		url += fmt.Sprintf("&offset=%d", opts.Offset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list schemas: %s", resp.Status)
	}

	var schemas []Schema
	if err := json.NewDecoder(resp.Body).Decode(&schemas); err != nil {
		return nil, err
	}

	return schemas, nil
}

func (c *externalClient) GetSchema(ctx context.Context, id int) (*Schema, error) {
	url := fmt.Sprintf("%s/schemas/%d", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get schema: %s", resp.Status)
	}

	var schema Schema
	if err := json.NewDecoder(resp.Body).Decode(&schema); err != nil {
		return nil, err
	}

	return &schema, nil
}

func (c *externalClient) CreateSchema(ctx context.Context, schema CreateSchemaRequest) (*Schema, error) {
	url := fmt.Sprintf("%s/schemas", c.baseURL)

	body, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to create schema: %s", resp.Status)
	}

	var createdSchema Schema
	if err := json.NewDecoder(resp.Body).Decode(&createdSchema); err != nil {
		return nil, err
	}

	return &createdSchema, nil
}

func (c *externalClient) UpdateSchema(ctx context.Context, id int, schema UpdateSchemaRequest) (*Schema, error) {
	url := fmt.Sprintf("%s/schemas/%d", c.baseURL, id)

	body, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update schema: %s", resp.Status)
	}

	var updatedSchema Schema
	if err := json.NewDecoder(resp.Body).Decode(&updatedSchema); err != nil {
		return nil, err
	}

	return &updatedSchema, nil
}

func (c *externalClient) DeleteSchema(ctx context.Context, id int, cascade bool) (*Schema, error) {
	url := fmt.Sprintf("%s/schemas/%d?cascade=%t", c.baseURL, id, cascade)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to delete schema: %s", resp.Status)
	}

	var deletedSchema Schema
	if err := json.NewDecoder(resp.Body).Decode(&deletedSchema); err != nil {
		return nil, err
	}

	return &deletedSchema, nil
}

// Table operations
func (c *externalClient) ListTables(ctx context.Context, opts ListTablesOptions) ([]Table, error) {
	url := fmt.Sprintf("%s/tables?include_system_schemas=%t", c.baseURL, opts.IncludeSystemSchemas)
	if len(opts.IncludedSchemas) > 0 {
		// Add logic to include schemas
	}
	if len(opts.ExcludedSchemas) > 0 {
		// Add logic to exclude schemas
	}
	if opts.Limit > 0 {
		url += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		url += fmt.Sprintf("&offset=%d", opts.Offset)
	}
	if opts.IncludeColumns {
		url += "&include_columns=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list tables: %s", resp.Status)
	}

	var tables []Table
	if err := json.NewDecoder(resp.Body).Decode(&tables); err != nil {
		return nil, err
	}

	return tables, nil
}

func (c *externalClient) GetTable(ctx context.Context, id int) (*Table, error) {
	url := fmt.Sprintf("%s/tables/%d", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get table: %s", resp.Status)
	}

	var table Table
	if err := json.NewDecoder(resp.Body).Decode(&table); err != nil {
		return nil, err
	}

	return &table, nil
}

func (c *externalClient) CreateTable(ctx context.Context, table CreateTableRequest) (*Table, error) {
	url := fmt.Sprintf("%s/tables", c.baseURL)

	jsonData, err := json.Marshal(table)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the response body for error details
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		responseBody := buf.String()
		logger.WarnContext(ctx, "SQL base create table failed",
			zap.Int("status", resp.StatusCode),
			zap.Int("response_body_length", len(responseBody)),
		)

		return nil, fmt.Errorf("failed to create table: %s, response body: %s", resp.Status, responseBody)
	}

	var createdTable Table
	if err := json.NewDecoder(resp.Body).Decode(&createdTable); err != nil {
		return nil, err
	}

	return &createdTable, nil
}

func (c *externalClient) UpdateTable(ctx context.Context, id int, table UpdateTableRequest) (*Table, error) {
	url := fmt.Sprintf("%s/tables/%d", c.baseURL, id)

	jsonData, err := json.Marshal(table)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to update table: %s", resp.Status)
	}

	var updatedTable Table
	if err := json.NewDecoder(resp.Body).Decode(&updatedTable); err != nil {
		return nil, err
	}

	return &updatedTable, nil
}

func (c *externalClient) DeleteTable(ctx context.Context, id int, cascade bool) (*Table, error) {
	url := fmt.Sprintf("%s/tables/%d?cascade=%t", c.baseURL, id, cascade)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to delete table: %s", resp.Status)
	}

	var deletedTable Table
	if err := json.NewDecoder(resp.Body).Decode(&deletedTable); err != nil {
		return nil, err
	}

	return &deletedTable, nil
}

// Column operations
func (c *externalClient) ListColumns(ctx context.Context, opts ListColumnsOptions) ([]Column, error) {
	url := fmt.Sprintf("%s/columns?include_system_schemas=%t", c.baseURL, opts.IncludeSystemSchemas)
	if len(opts.IncludedSchemas) > 0 {
		// Add logic to include schemas
	}
	if len(opts.ExcludedSchemas) > 0 {
		// Add logic to exclude schemas
	}
	if opts.Limit > 0 {
		url += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		url += fmt.Sprintf("&offset=%d", opts.Offset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list columns: %s", resp.Status)
	}

	var columns []Column
	if err := json.NewDecoder(resp.Body).Decode(&columns); err != nil {
		return nil, err
	}

	return columns, nil
}

func (c *externalClient) GetColumn(ctx context.Context, tableId int, ordinalPosition int) (*Column, error) {
	url := fmt.Sprintf("%s/columns/%d.%d", c.baseURL, tableId, ordinalPosition)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get column: %s", resp.Status)
	}

	var column Column
	if err := json.NewDecoder(resp.Body).Decode(&column); err != nil {
		return nil, err
	}

	return &column, nil
}

func (c *externalClient) CreateColumn(ctx context.Context, column CreateColumnRequest) (*Column, error) {
	url := fmt.Sprintf("%s/columns", c.baseURL)

	jsonData, err := json.Marshal(column)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the response body for error details
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		responseBody := buf.String()
		logger.WarnContext(ctx, "SQL base create column failed",
			zap.Int("status", resp.StatusCode),
			zap.Int("response_body_length", len(responseBody)),
		)

		return nil, fmt.Errorf("failed to create column: %s, response body: %s", resp.Status, responseBody)
	}

	var createdColumn Column
	if err := json.NewDecoder(resp.Body).Decode(&createdColumn); err != nil {
		return nil, err
	}

	return &createdColumn, nil
}

func (c *externalClient) UpdateColumn(ctx context.Context, id string, column UpdateColumnRequest) (*Column, error) {
	url := fmt.Sprintf("%s/columns/%s", c.baseURL, id)

	jsonData, err := json.Marshal(column)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the response body for error details
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		responseBody := buf.String()
		logger.WarnContext(ctx, "SQL base update column failed",
			zap.Int("status", resp.StatusCode),
			zap.Int("response_body_length", len(responseBody)),
		)

		return nil, fmt.Errorf("failed to update column: %s, response body: %s", resp.Status, responseBody)
	}

	var updatedColumn Column
	if err := json.NewDecoder(resp.Body).Decode(&updatedColumn); err != nil {
		return nil, err
	}

	return &updatedColumn, nil
}

func (c *externalClient) DeleteColumn(ctx context.Context, id string, cascade bool) (*Column, error) {
	url := fmt.Sprintf("%s/columns/%s?cascade=%t", c.baseURL, id, cascade)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the response body for error details
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		responseBody := buf.String()
		logger.WarnContext(ctx, "SQL base delete column failed",
			zap.Int("status", resp.StatusCode),
			zap.Int("response_body_length", len(responseBody)),
		)

		return nil, fmt.Errorf("failed to delete column: %s, response body: %s", resp.Status, responseBody)
	}

	var deletedColumn Column
	if err := json.NewDecoder(resp.Body).Decode(&deletedColumn); err != nil {
		return nil, err
	}

	return &deletedColumn, nil
}

// View operations
func (c *externalClient) ListViews(ctx context.Context, opts ListViewsOptions) ([]View, error) {
	url := fmt.Sprintf("%s/views?include_system_schemas=%t", c.baseURL, opts.IncludeSystemSchemas)
	if len(opts.IncludedSchemas) > 0 {
		// Add logic to include schemas
	}
	if len(opts.ExcludedSchemas) > 0 {
		// Add logic to exclude schemas
	}
	if opts.Limit > 0 {
		url += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		url += fmt.Sprintf("&offset=%d", opts.Offset)
	}
	if opts.IncludeColumns {
		url += "&include_columns=true"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list views: %s", resp.Status)
	}

	var views []View
	if err := json.NewDecoder(resp.Body).Decode(&views); err != nil {
		return nil, err
	}

	return views, nil
}

func (c *externalClient) GetView(ctx context.Context, id int) (*View, error) {
	url := fmt.Sprintf("%s/views/%d", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get view: %s", resp.Status)
	}

	var view View
	if err := json.NewDecoder(resp.Body).Decode(&view); err != nil {
		return nil, err
	}

	return &view, nil
}

// Materialized views operations
func (c *externalClient) ListMaterializedViews(ctx context.Context, opts ListMaterializedViewsOptions) ([]MaterializedView, error) {
	url := fmt.Sprintf("%s/materialized-views", c.baseURL)

	params := ""
	if len(opts.IncludedSchemas) > 0 {
		// Add logic to include schemas
	}
	if len(opts.ExcludedSchemas) > 0 {
		// Add logic to exclude schemas
	}
	if opts.Limit > 0 {
		params += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		params += fmt.Sprintf("&offset=%d", opts.Offset)
	}
	if opts.IncludeColumns {
		params += "&include_columns=true"
	}

	if len(params) > 0 {
		url += "?" + params[1:] // Remove leading '&'
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list materialized views: %s", resp.Status)
	}

	var materializedViews []MaterializedView
	if err := json.NewDecoder(resp.Body).Decode(&materializedViews); err != nil {
		return nil, err
	}

	return materializedViews, nil
}

func (c *externalClient) GetMaterializedView(ctx context.Context, id int) (*MaterializedView, error) {
	url := fmt.Sprintf("%s/materialized-views/%d", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get materialized view: %s", resp.Status)
	}

	var materializedView MaterializedView
	if err := json.NewDecoder(resp.Body).Decode(&materializedView); err != nil {
		return nil, err
	}

	return &materializedView, nil
}

// Function operations
func (c *externalClient) ListFunctions(ctx context.Context) ([]Function, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) GetFunction(ctx context.Context, id string) (*Function, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) CreateFunction(ctx context.Context, function Function) (*Function, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) UpdateFunction(ctx context.Context, id string, function Function) (*Function, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) DeleteFunction(ctx context.Context, id string) (*Function, error) {
	return nil, fmt.Errorf("not implemented")
}

// Trigger operations
func (c *externalClient) ListTriggers(ctx context.Context) ([]Trigger, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) GetTrigger(ctx context.Context, id string) (*Trigger, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) CreateTrigger(ctx context.Context, trigger Trigger) (*Trigger, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) UpdateTrigger(ctx context.Context, id string, trigger Trigger) (*Trigger, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) DeleteTrigger(ctx context.Context, id string) (*Trigger, error) {
	return nil, fmt.Errorf("not implemented")
}

// Role operations
func (c *externalClient) ListRoles(ctx context.Context, opts ListRolesOptions) ([]Role, error) {
	url := fmt.Sprintf("%s/roles?include_system_schemas=%t", c.baseURL, opts.IncludeSystemSchemas)
	if opts.Limit > 0 {
		url += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		url += fmt.Sprintf("&offset=%d", opts.Offset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list roles: %s", resp.Status)
	}

	var roles []Role
	if err := json.NewDecoder(resp.Body).Decode(&roles); err != nil {
		return nil, err
	}

	return roles, nil
}

func (c *externalClient) GetRole(ctx context.Context, id int) (*Role, error) {
	url := fmt.Sprintf("%s/roles/%d", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get role: %s", resp.Status)
	}

	var role Role
	if err := json.NewDecoder(resp.Body).Decode(&role); err != nil {
		return nil, err
	}

	return &role, nil
}

func (c *externalClient) CreateRole(ctx context.Context, role CreateRoleRequest) (*Role, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) UpdateRole(ctx context.Context, id int, role UpdateRoleRequest) (*Role, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) DeleteRole(ctx context.Context, id int, cascade bool) (*Role, error) {
	return nil, fmt.Errorf("not implemented")
}

// Extension operations
func (c *externalClient) ListExtensions(ctx context.Context) ([]Extension, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) GetExtension(ctx context.Context, name string) (*Extension, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) CreateExtension(ctx context.Context, extension Extension) (*Extension, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) UpdateExtension(ctx context.Context, name string, extension Extension) (*Extension, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) DeleteExtension(ctx context.Context, name string) (*Extension, error) {
	return nil, fmt.Errorf("not implemented")
}

// Policy operations
func (c *externalClient) ListPolicies(ctx context.Context) ([]Policy, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) GetPolicy(ctx context.Context, id string) (*Policy, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) CreatePolicy(ctx context.Context, policy Policy) (*Policy, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) UpdatePolicy(ctx context.Context, id string, policy Policy) (*Policy, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) DeletePolicy(ctx context.Context, id string) (*Policy, error) {
	return nil, fmt.Errorf("not implemented")
}

// Publication operations
func (c *externalClient) ListPublications(ctx context.Context) ([]Publication, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) GetPublication(ctx context.Context, id string) (*Publication, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) CreatePublication(ctx context.Context, publication Publication) (*Publication, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) UpdatePublication(ctx context.Context, id string, publication Publication) (*Publication, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) DeletePublication(ctx context.Context, id string) (*Publication, error) {
	return nil, fmt.Errorf("not implemented")
}

// Foreign table operations
func (c *externalClient) ListForeignTables(ctx context.Context, opts ListForeignTablesOptions) ([]ForeignTable, error) {
	url := fmt.Sprintf("%s/foreign-tables", c.baseURL)

	params := ""
	if opts.Limit > 0 {
		params += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		params += fmt.Sprintf("&offset=%d", opts.Offset)
	}
	if opts.IncludeColumns {
		params += "&include_columns=true"
	}

	if len(params) > 0 {
		url += "?" + params[1:] // Remove leading '&'
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list foreign tables: %s", resp.Status)
	}

	var foreignTables []ForeignTable
	if err := json.NewDecoder(resp.Body).Decode(&foreignTables); err != nil {
		return nil, err
	}

	return foreignTables, nil
}

func (c *externalClient) GetForeignTable(ctx context.Context, id int) (*ForeignTable, error) {
	url := fmt.Sprintf("%s/foreign-tables/%d", c.baseURL, id)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get foreign table: %s", resp.Status)
	}

	var foreignTable ForeignTable
	if err := json.NewDecoder(resp.Body).Decode(&foreignTable); err != nil {
		return nil, err
	}

	return &foreignTable, nil
}

// Index operations
func (c *externalClient) ListIndexes(ctx context.Context) ([]Index, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) GetIndex(ctx context.Context, id string) (*Index, error) {
	return nil, fmt.Errorf("not implemented")
}

// Type operations
func (c *externalClient) ListTypes(ctx context.Context) ([]Type, error) {
	return nil, fmt.Errorf("not implemented")
}

// Query operations
func (c *externalClient) ExecuteSQL(ctx context.Context, query string, params []interface{}, auditCtx *audit.Context) (*QueryResult, error) {
	start := time.Now()
	guardResult, guarded, guardErr := checkSQLGuard(ctx, query, auditCtx, c.policyProvider)
	var result *QueryResult
	var err error
	if guardErr != nil {
		err = guardErr
	} else {
		result, err = c.executeSQL(ctx, query, params)
	}
	end := time.Now()
	c.recordAudit(ctx, auditCtx, query, params, result, err, start, end, guardResult, guarded)
	return result, err
}

func (c *externalClient) executeSQL(ctx context.Context, query string, params []interface{}) (*QueryResult, error) {
	url := fmt.Sprintf("%s/query", c.baseURL)

	// Prepare the request body
	requestBody := map[string]interface{}{
		"query": query,
	}

	// Add parameters if provided
	if len(params) > 0 {
		requestBody["parameters"] = params
	}

	body, err := json.Marshal(requestBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read the response body for error details
		buf := new(bytes.Buffer)
		buf.ReadFrom(resp.Body)
		responseBody := buf.String()
		return nil, fmt.Errorf("failed to execute query: %s, response body: %s", resp.Status, responseBody)
	}

	// Try to read and log the raw response for debugging
	buf := new(bytes.Buffer)
	buf.ReadFrom(resp.Body)
	rawResponse := buf.Bytes()

	// Handle empty response case
	if len(rawResponse) == 0 {
		// For queries that don't return data (INSERT, UPDATE, DELETE), return empty result
		return &QueryResult{
			RowsAffected: 0,
			Columns:      []string{},
			Rows:         [][]interface{}{},
		}, nil
	}

	// Restore the response body for further processing
	resp.Body = io.NopCloser(bytes.NewBuffer(rawResponse))

	// First try to decode as a direct array of results (postgres-meta format)
	var resultArray []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&resultArray); err == nil {
		// Handle empty array response (INSERT/UPDATE/DELETE)
		if len(resultArray) == 0 {
			return &QueryResult{
				RowsAffected: 0,
				Columns:      []string{},
				Rows:         [][]interface{}{},
			}, nil
		}

		// Extract column names from the first row
		columns := make([]string, 0, len(resultArray[0]))
		for k := range resultArray[0] {
			columns = append(columns, k)
		}

		// Build rows data
		rows := make([][]interface{}, 0, len(resultArray))
		for _, row := range resultArray {
			rowData := make([]interface{}, 0, len(columns))
			for _, col := range columns {
				rowData = append(rowData, row[col])
			}
			rows = append(rows, rowData)
		}

		result := &QueryResult{
			RowsAffected: int64(len(resultArray)),
			Columns:      columns,
			Rows:         rows,
		}

		return result, nil
	}

	// Restore the response body again
	resp.Body = io.NopCloser(bytes.NewBuffer(rawResponse))

	// If array of results decoding failed, try to decode directly as QueryResult
	var result QueryResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response as QueryResult: %w, raw response: %s", err, string(rawResponse))
	}

	return &result, nil
}

func (c *externalClient) recordAudit(ctx context.Context, auditCtx *audit.Context, query string, params []interface{}, result *QueryResult, execErr error, start, end time.Time, guardResult guard.Result, guarded bool) {
	if c.recorder == nil || auditCtx == nil {
		return
	}

	record := audit.Record{
		Context:      *auditCtx,
		SQLStatement: query,
		Params:       append([]any(nil), params...),
		DurationMS:   end.Sub(start).Milliseconds(),
		Status:       audit.StatusSuccess,
		StartTime:    start,
		EndTime:      end,
		ExecutedAt:   start,
	}
	applyGuardAudit(&record, guardResult, guarded)
	if record.ClientType == "" {
		record.ClientType = audit.ClientTypeUnknown
	}
	if result != nil {
		rowCount := result.RowsAffected
		record.RowCount = &rowCount
	}
	if execErr != nil {
		record.Status = audit.StatusFailed
		record.RowCount = nil
		record.ErrorMessage = execErr.Error()
	}

	c.recorder.Record(ctx, record)
}

func (c *externalClient) FormatQuery(ctx context.Context, req FormatQueryRequest) (string, error) {
	return "", fmt.Errorf("not implemented")
}

func (c *externalClient) ParseQuery(ctx context.Context, req ParseQueryRequest) (*ParsedQuery, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) DeparseQuery(ctx context.Context, req DeparseQueryRequest) (string, error) {
	return "", fmt.Errorf("not implemented")
}

// Table privileges
func (c *externalClient) ListTablePrivileges(ctx context.Context, opts ListTablePrivilegesOptions) ([]TablePrivilege, error) {
	url := fmt.Sprintf("%s/table-privileges?include_system_schemas=%t", c.baseURL, opts.IncludeSystemSchemas)
	if len(opts.IncludedSchemas) > 0 {
		// Add logic to include schemas
	}
	if len(opts.ExcludedSchemas) > 0 {
		// Add logic to exclude schemas
	}
	if opts.Limit > 0 {
		url += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		url += fmt.Sprintf("&offset=%d", opts.Offset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list table privileges: %s", resp.Status)
	}

	var tablePrivileges []TablePrivilege
	if err := json.NewDecoder(resp.Body).Decode(&tablePrivileges); err != nil {
		return nil, err
	}

	return tablePrivileges, nil
}

func (c *externalClient) GrantTablePrivileges(ctx context.Context, privileges []TablePrivilegeGrant) ([]TablePrivilege, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) RevokeTablePrivileges(ctx context.Context, privileges []TablePrivilegeRevoke) ([]TablePrivilege, error) {
	return nil, fmt.Errorf("not implemented")
}

// Column privileges
func (c *externalClient) ListColumnPrivileges(ctx context.Context, opts ListColumnPrivilegesOptions) ([]ColumnPrivilege, error) {
	url := fmt.Sprintf("%s/column-privileges?include_system_schemas=%t", c.baseURL, opts.IncludeSystemSchemas)
	if len(opts.IncludedSchemas) > 0 {
		// Add logic to include schemas
	}
	if len(opts.ExcludedSchemas) > 0 {
		// Add logic to exclude schemas
	}
	if opts.Limit > 0 {
		url += fmt.Sprintf("&limit=%d", opts.Limit)
	}
	if opts.Offset > 0 {
		url += fmt.Sprintf("&offset=%d", opts.Offset)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if c.apiKey != "" {
		req.Header.Set("apikey", c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to list column privileges: %s", resp.Status)
	}

	var columnPrivileges []ColumnPrivilege
	if err := json.NewDecoder(resp.Body).Decode(&columnPrivileges); err != nil {
		return nil, err
	}

	return columnPrivileges, nil
}

func (c *externalClient) GrantColumnPrivileges(ctx context.Context, privileges []ColumnPrivilegeGrant) ([]ColumnPrivilege, error) {
	return nil, fmt.Errorf("not implemented")
}

func (c *externalClient) RevokeColumnPrivileges(ctx context.Context, privileges []ColumnPrivilegeRevoke) ([]ColumnPrivilege, error) {
	return nil, fmt.Errorf("not implemented")
}
