package dto

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/datasource/model"
	shared_model "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/util"
)

// CreateDataSourceRequest represents the request to create a new data source
type CreateDataSourceRequest struct {
	Name           string  `json:"name" binding:"required"`
	Description    string  `json:"description"`
	Permission     string  `json:"permission"`
	WorkspaceID    *string `json:"workspace_id"`
	IconType       *string `json:"icon_type"`
	Icon           *string `json:"icon"`
	IconBackground *string `json:"icon_background"`
}

// UpdateDataSourceRequest represents the request to update an existing data source
type UpdateDataSourceRequest struct {
	Name           *string `json:"name"`
	Description    *string `json:"description"`
	Permission     *string `json:"permission"`
	WorkspaceID    *string `json:"workspace_id"`
	IconType       *string `json:"icon_type"`
	Icon           *string `json:"icon"`
	IconBackground *string `json:"icon_background"`
}

// CreateTableRequest represents the request to create a new table
// Note: Columns are not included in this request as table creation is a two-step process:
// 1. Create the table structure
// 2. Add columns separately
type CreateTableRequest struct {
	Name        string `json:"name" binding:"required"`
	Description string `json:"description"`
}

// UpdateTableRequest is the request for updating table metadata
type UpdateTableRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	UpdatedBy   string  `json:"updated_by"`
}

// UpdateTablePromptRequest is the request for updating a table prompt
type UpdateTablePromptRequest struct {
	Prompt    string `json:"prompt"`
	UpdatedBy string `json:"updated_by"`
}

// ColumnSchema represents the schema of a column
type ColumnSchema struct {
	Name       string `json:"name" binding:"required"`
	Type       string `json:"type" binding:"required"`
	IsRequired bool   `json:"is_required"`
}

// UpdateTableColumnsRequest represents the request to update table columns
// This is a full replacement operation - all existing columns will be removed
// and replaced with the provided columns
type UpdateTableColumnsRequest struct {
	Columns []TableColumn `json:"columns"`
}

// GetTableColumnsResponse represents the response for getting table columns
// Only returns the column list to match the update request structure
type GetTableColumnsResponse struct {
	Columns []TableColumn `json:"columns"`
}

// TableColumn represents a column in a table
type TableColumn struct {
	ID               string  `json:"id"` // Column ID in the database
	Name             string  `json:"name" binding:"required"`
	DisplayName      *string `json:"display_name,omitempty"`
	SourceColumnName *string `json:"source_column_name,omitempty"`
	Description      *string `json:"description"`
	Type             string  `json:"type" binding:"required"` // Data type (frontend-friendly names: int, float, double, varchar, etc.)
	IsRequired       bool    `json:"is_required"`
	IsSystemField    bool    `json:"is_system_field,omitempty"` // Indicates if this is a system field (id, uuid, created_time, updated_time)
}

// AddRecordRequest represents the request to add records to a table
type AddRecordRequest struct {
	Records []map[string]interface{} `json:"records" binding:"required,min=1"`
}

// AddRecordResponse represents the response after adding records
type AddRecordResponse struct {
	AffectedRows int64 `json:"affected_rows"`
}

// UpdateRecordRequest represents request for updating records in a table
type UpdateRecordRequest struct {
	Records []map[string]interface{} `json:"records" binding:"required,min=1"`
}

// UpdateRecordResponse represents response for updating records
type UpdateRecordResponse struct {
	AffectedRows int64 `json:"affected_rows"`
}

// DeleteRecordRequest represents request for deleting records from a table
type DeleteRecordRequest struct {
	Records []map[string]interface{} `json:"records" binding:"required,min=1"`
}

// DeleteRecordResponse represents response for deleting records
type DeleteRecordResponse struct {
	AffectedRows int64 `json:"affected_rows"`
}

// QueryRecordRequest represents the request to query table records
type QueryRecordRequest struct {
	Limit  int    `json:"limit" form:"limit,default=20"`
	Offset int    `json:"offset" form:"offset,default=0"`
	Order  string `json:"order" form:"order,default=id DESC"`
}

// QueryRecordResponse represents the response for querying table records
type QueryRecordResponse struct {
	HasMore  bool        `json:"has_more"`
	TotalNum int64       `json:"total_num"`
	Data     interface{} `json:"data"`
}

// AnalyzeFileForTableResponse represents the response for analyzing a file to infer table structure
type AnalyzeFileForTableResponse struct {
	Columns []TableColumn `json:"columns"`
}

// ModelSpec represents the specification of a model to use
type ModelSpec struct {
	Provider string `json:"provider" binding:"required"`
	Name     string `json:"name" binding:"required"`
}

// AnalyzeFileForTableRequest represents the request for analyzing a file to infer table structure
type AnalyzeFileForTableRequest struct {
	DataSourceID string     `json:"data_source_id" binding:"required"`
	FileID       *string    `json:"file_id,omitempty"`
	Prompt       *string    `json:"prompt,omitempty"`
	Model        *ModelSpec `json:"model,omitempty"`
}

// IngestFileToTableRequest defines the request for ingesting file content into a table
type IngestFileToTableRequest struct {
	FileID  string     `json:"file_id" binding:"required"`
	TableID string     `json:"table_id" binding:"required"`
	Prompt  *string    `json:"prompt,omitempty"`
	Model   *ModelSpec `json:"model,omitempty"`
}

// IngestFileToTableResponse defines the response for ingesting file to table
type IngestFileToTableResponse struct {
	Records []map[string]interface{} `json:"records"`
	Columns []TableColumn            `json:"columns"`
	Message string                   `json:"message"`
}

// BatchIngestFileToTableRequest defines the request for ingesting multiple files content into a table
type BatchIngestFileToTableRequest struct {
	FileIDs []string   `json:"file_ids" binding:"required"`
	TableID string     `json:"table_id" binding:"required"`
	Prompt  *string    `json:"prompt,omitempty"`
	Model   *ModelSpec `json:"model,omitempty"`
}

// BatchIngestFileToTableResponse defines the response for ingesting multiple files to table
type BatchIngestFileToTableResponse struct {
	Results map[string]FileIngestResult `json:"results"`
	Columns []TableColumn               `json:"columns"`
	Message string                      `json:"message"`
}

// FileIngestResult represents the result of ingesting a single file
type FileIngestResult struct {
	FileID   string                   `json:"file_id"`
	FileName string                   `json:"file_name"`
	Records  []map[string]interface{} `json:"records"`
	Message  string                   `json:"message"`
	Error    *string                  `json:"error,omitempty"`
}

// DataSourceResponse represents data source response DTO
type DataSourceResponse struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	WorkspaceID    *string   `json:"workspace_id"`
	Name           string    `json:"name"`
	SchemaID       int       `json:"schema_id"`
	SchemaName     string    `json:"schema_name"`
	Description    string    `json:"description"`
	Permission     string    `json:"permission"`
	Status         string    `json:"status"`
	IconType       *string   `json:"icon_type"`
	Icon           *string   `json:"icon"`
	IconBackground *string   `json:"icon_background"`
	CreatedBy      string    `json:"created_by"`
	UpdatedBy      string    `json:"updated_by"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	CanEdit        bool      `json:"can_edit"` // NEW: Indicates if current user can edit this datasource
}

// MarshalJSON implements custom JSON marshaling to generate icon URLs
func (d *DataSourceResponse) MarshalJSON() ([]byte, error) {
	// Generate icon URLs if needed
	icon := d.Icon
	iconType := d.IconType
	var iconUrl *string

	if icon != nil && iconType != nil && *iconType == string(shared_model.IconTypeImage) {
		// Icon is a file ID, generate signed preview URL
		signedURL, err := util.GetSignedFileURL(*icon)
		if err == nil {
			iconUrl = &signedURL
		} else {
			// Fallback: use simple URL without signature
			if config.GlobalConfig != nil && config.GlobalConfig.Console.APIURL != "" {
				consoleAPIURL := config.GlobalConfig.Console.APIURL
				iconUrlStr := fmt.Sprintf("%s/console/api/files/%s/file-preview", consoleAPIURL, *icon)
				iconUrl = &iconUrlStr
			}
		}
	}

	// Create alias to avoid infinite recursion
	type Alias DataSourceResponse
	return json.Marshal(&struct {
		*Alias
		IconUrl *string `json:"icon_url"`
	}{
		Alias:   (*Alias)(d),
		IconUrl: iconUrl,
	})
}

// ConvertDataSourceModelToResponse converts a data source model to response DTO
func ConvertDataSourceModelToResponse(ds *model.DataSource) *DataSourceResponse {
	return &DataSourceResponse{
		ID:             ds.ID,
		OrganizationID: ds.OrganizationID,
		WorkspaceID:    ds.WorkspaceID,
		Name:           ds.Name,
		SchemaID:       ds.SchemaID,
		SchemaName:     ds.SchemaName,
		Description:    ds.Description,
		Permission:     ds.Permission,
		Status:         ds.Status,
		IconType:       ds.IconType,
		Icon:           ds.Icon,
		IconBackground: ds.IconBackground,
		CreatedBy:      ds.CreatedBy,
		UpdatedBy:      ds.UpdatedBy,
		CreatedAt:      ds.CreatedAt,
		UpdatedAt:      ds.UpdatedAt,
	}
}

// SQLOperationResponse represents the response for an SQL operation log
type SQLOperationResponse struct {
	ID             string    `json:"id"`
	OrganizationID string    `json:"organization_id"`
	DataSourceID   string    `json:"data_source_id"`
	TableID        *string   `json:"table_id"`
	TableName      *string   `json:"table_name"`
	DataSourceName *string   `json:"data_source_name"`
	SqlStatement   string    `json:"sql_statement"`
	OperationType  string    `json:"operation_type"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Status         string    `json:"status"`
	CreatedBy      string    `json:"created_by"`
	CreatedByName  *string   `json:"created_by_name,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ListSQLOperationsRequest represents the request for listing SQL operations
type ListSQLOperationsRequest struct {
	Page          int        `form:"page" binding:"omitempty,min=1"`
	Limit         int        `form:"limit" binding:"omitempty,min=1,max=100"`
	TableID       *string    `form:"table_id"`
	CreatedBy     *string    `form:"created_by"`
	OperationType *string    `form:"operation_type"`
	Status        *string    `form:"status"`
	StartTime     *time.Time `form:"start_time"`
	EndTime       *time.Time `form:"end_time"`
}

// SQLOperationFilter represents the filter criteria for SQL operations
type SQLOperationFilter struct {
	TableID       *string
	CreatedBy     *string
	OperationType *string
	Status        *string
	CreatedAtGTE  *time.Time
	CreatedAtLTE  *time.Time
}

// ListSQLOperationsByDataSourceIDResponse represents the response for listing SQL operations by data source ID
type ListSQLOperationsByDataSourceIDResponse struct {
	Data    []SQLOperationResponse `json:"data"`
	HasMore bool                   `json:"has_more"`
	Limit   int                    `json:"limit"`
	Total   int64                  `json:"total"`
	Page    int                    `json:"page"`
}

// ConvertSQLOperationModelToResponse converts an SQL operation model to response DTO
func ConvertSQLOperationModelToResponse(op *model.DataSourceSQLOperation) *SQLOperationResponse {
	return &SQLOperationResponse{
		ID:             op.ID,
		OrganizationID: op.OrganizationID,
		DataSourceID:   op.DataSourceID,
		TableID:        op.TableID,
		TableName:      op.TableName,
		DataSourceName: op.DataSourceName,
		SqlStatement:   op.SqlStatement,
		OperationType:  op.OperationType,
		StartTime:      op.StartTime,
		EndTime:        op.EndTime,
		Status:         op.Status,
		CreatedBy:      op.CreatedBy,
		CreatedAt:      op.CreatedAt,
	}
}

type ImportRecordResponse struct {
	AffectedRows int                `json:"affected_rows"`
	FailedCount  int                `json:"failed_count"`
	TotalCount   int                `json:"total_count"`
	FailedItems  []ImportFailedItem `json:"failed_items,omitempty"`
}

type ImportRecordRequest struct {
	UploadFileID string `json:"upload_file_id" binding:"required"`
}

type ImportFailedItem struct {
	RowIndex int    `json:"row_index"`
	Error    string `json:"error"`
}
