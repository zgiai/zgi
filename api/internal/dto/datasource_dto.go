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
	Content string        `json:"content,omitempty"`
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

// ParseFileForTableIngestRequest defines the request for parsing file content
// before table field recognition.
type ParseFileForTableIngestRequest struct {
	FileID  string `json:"file_id" binding:"required"`
	TableID string `json:"table_id" binding:"required"`
}

// ParseFileForTableIngestResponse defines the file parsing stage response.
type ParseFileForTableIngestResponse struct {
	FileID     string                    `json:"file_id,omitempty"`
	FileName   string                    `json:"file_name,omitempty"`
	Message    string                    `json:"message"`
	Content    string                    `json:"content,omitempty"`
	Extraction *FileIngestExtractionInfo `json:"extraction,omitempty"`
	Stage      string                    `json:"stage,omitempty"`
	Error      *string                   `json:"error,omitempty"`
}

// ExtractTextToTableRecordsRequest defines the request for recognizing table
// records from already parsed text content.
type ExtractTextToTableRecordsRequest struct {
	FileID      string     `json:"file_id,omitempty"`
	TableID     string     `json:"table_id" binding:"required"`
	Content     string     `json:"content"`
	ContentHash string     `json:"content_hash,omitempty"`
	Prompt      *string    `json:"prompt,omitempty"`
	Model       *ModelSpec `json:"model,omitempty"`
}

// ExtractTextToTableRecordsResponse defines the text recognition stage response.
type ExtractTextToTableRecordsResponse struct {
	FileID          string                     `json:"file_id,omitempty"`
	Records         []map[string]interface{}   `json:"records"`
	Columns         []TableColumn              `json:"columns"`
	Message         string                     `json:"message"`
	FieldExtraction *FileIngestFieldExtraction `json:"field_extraction,omitempty"`
	ContentHash     string                     `json:"content_hash,omitempty"`
	Stage           string                     `json:"stage,omitempty"`
	Error           *string                    `json:"error,omitempty"`
}

// IngestFileToTableResponse defines the response for ingesting file to table
type IngestFileToTableResponse struct {
	FileID          string                     `json:"file_id,omitempty"`
	FileName        string                     `json:"file_name,omitempty"`
	Records         []map[string]interface{}   `json:"records"`
	Columns         []TableColumn              `json:"columns"`
	Message         string                     `json:"message"`
	Content         string                     `json:"content,omitempty"`
	Extraction      *FileIngestExtractionInfo  `json:"extraction,omitempty"`
	FieldExtraction *FileIngestFieldExtraction `json:"field_extraction,omitempty"`
	Stage           string                     `json:"stage,omitempty"`
	Error           *string                    `json:"error,omitempty"`
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
	Results      map[string]FileIngestResult `json:"results"`
	Columns      []TableColumn               `json:"columns"`
	Message      string                      `json:"message"`
	TotalCount   int                         `json:"total_count"`
	SuccessCount int                         `json:"success_count"`
	FailedCount  int                         `json:"failed_count"`
}

// FileIngestResult represents the result of ingesting a single file
type FileIngestResult struct {
	FileID          string                     `json:"file_id"`
	FileName        string                     `json:"file_name"`
	Records         []map[string]interface{}   `json:"records"`
	Message         string                     `json:"message"`
	Content         string                     `json:"content,omitempty"`
	Extraction      *FileIngestExtractionInfo  `json:"extraction,omitempty"`
	FieldExtraction *FileIngestFieldExtraction `json:"field_extraction,omitempty"`
	Stage           string                     `json:"stage,omitempty"`
	Error           *string                    `json:"error,omitempty"`
}

// FileIngestExtractionInfo describes the parser path used before field extraction.
type FileIngestExtractionInfo struct {
	PrimaryStrategy string              `json:"primary_strategy,omitempty"`
	ActualStrategy  string              `json:"actual_strategy,omitempty"`
	FallbackReason  string              `json:"fallback_reason,omitempty"`
	SourceType      string              `json:"source_type,omitempty"`
	ContentHash     string              `json:"content_hash,omitempty"`
	Attempts        []FileIngestAttempt `json:"attempts,omitempty"`
}

// FileIngestAttempt describes one user-visible extraction attempt in a file ingest run.
type FileIngestAttempt struct {
	Method      string `json:"method"`
	Status      string `json:"status"`
	Result      string `json:"result,omitempty"`
	Reason      string `json:"reason,omitempty"`
	DurationMS  int64  `json:"duration_ms,omitempty"`
	RecordCount int    `json:"record_count,omitempty"`
}

// FileIngestFieldExtraction contains schema-aware field matches produced after
// file parsing. It is additive to Records so existing ingest clients can keep
// using table field names while review UIs can inspect evidence and confidence.
type FileIngestFieldExtraction struct {
	Records []FileIngestRecordExtraction `json:"records,omitempty"`
}

type FileIngestRecordExtraction struct {
	Fields []FileIngestFieldMatch `json:"fields,omitempty"`
}

type FileIngestFieldMatch struct {
	ColumnID            string      `json:"column_id"`
	ColumnName          string      `json:"column_name,omitempty"`
	Value               interface{} `json:"value,omitempty"`
	RawValue            interface{} `json:"raw_value,omitempty"`
	NormalizedValue     interface{} `json:"normalized_value,omitempty"`
	NormalizationStatus string      `json:"normalization_status,omitempty"`
	NormalizationReason string      `json:"normalization_reason,omitempty"`
	Evidence            string      `json:"evidence,omitempty"`
	Confidence          *float64    `json:"confidence,omitempty"`
	Reason              string      `json:"reason,omitempty"`
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

type SQLAuditFilter struct {
	DataSourceID  *string
	TableID       *string
	ClientType    *string
	WorkflowRunID *string
	NodeID        *string
	CreatedBy     *string
	OperationType *string
	Status        *string
	StartTime     *time.Time
	EndTime       *time.Time
}

type ListSQLAuditRequest struct {
	Page          int        `form:"page" binding:"omitempty,min=1"`
	Limit         int        `form:"limit" binding:"omitempty,min=1,max=100"`
	DataSourceID  *string    `form:"data_source_id"`
	TableID       *string    `form:"table_id"`
	ClientType    *string    `form:"client_type"`
	WorkflowRunID *string    `form:"workflow_run_id"`
	NodeID        *string    `form:"node_id"`
	CreatedBy     *string    `form:"created_by"`
	OperationType *string    `form:"operation_type"`
	Status        *string    `form:"status"`
	StartTime     *time.Time `form:"start_time"`
	EndTime       *time.Time `form:"end_time"`
}

type SQLAuditListItem struct {
	ID             string     `json:"id"`
	OrganizationID string     `json:"organization_id"`
	WorkspaceID    *string    `json:"workspace_id"`
	DataSourceID   string     `json:"data_source_id"`
	DataSourceName *string    `json:"data_source_name"`
	TableID        *string    `json:"table_id"`
	TableName      *string    `json:"table_name"`
	ClientType     string     `json:"client_type"`
	WorkflowRunID  *string    `json:"workflow_run_id,omitempty"`
	NodeID         *string    `json:"node_id,omitempty"`
	OperationType  string     `json:"operation_type"`
	Status         string     `json:"status"`
	RowCount       *int64     `json:"row_count"`
	DurationMS     *int64     `json:"duration_ms"`
	CreatedBy      string     `json:"created_by"`
	ExecutedAt     *time.Time `json:"executed_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type SQLAuditDetailResponse struct {
	SQLAuditListItem
	SQLStatement string          `json:"sql_statement"`
	ParamsJSON   json.RawMessage `json:"params_json,omitempty"`
	ErrorCode    *string         `json:"error_code,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
	RequestID    *string         `json:"request_id,omitempty"`
	StartTime    time.Time       `json:"start_time"`
	EndTime      time.Time       `json:"end_time"`
}

type ListSQLAuditResponse struct {
	Data    []SQLAuditListItem `json:"data"`
	HasMore bool               `json:"has_more"`
	Limit   int                `json:"limit"`
	Total   int64              `json:"total"`
	Page    int                `json:"page"`
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

func ConvertSQLOperationModelToAuditListItem(op *model.DataSourceSQLOperation) SQLAuditListItem {
	return SQLAuditListItem{
		ID:             op.ID,
		OrganizationID: op.OrganizationID,
		WorkspaceID:    op.WorkspaceID,
		DataSourceID:   op.DataSourceID,
		DataSourceName: op.DataSourceName,
		TableID:        op.TableID,
		TableName:      op.TableName,
		ClientType:     op.ClientType,
		WorkflowRunID:  op.WorkflowRunID,
		NodeID:         op.NodeID,
		OperationType:  op.OperationType,
		Status:         op.Status,
		RowCount:       op.RowCount,
		DurationMS:     op.DurationMS,
		CreatedBy:      op.CreatedBy,
		ExecutedAt:     op.ExecutedAt,
		CreatedAt:      op.CreatedAt,
	}
}

func ConvertSQLOperationModelToAuditDetail(op *model.DataSourceSQLOperation) SQLAuditDetailResponse {
	return SQLAuditDetailResponse{
		SQLAuditListItem: ConvertSQLOperationModelToAuditListItem(op),
		SQLStatement:     op.SqlStatement,
		ParamsJSON:       json.RawMessage(op.ParamsJSON),
		ErrorCode:        op.ErrorCode,
		ErrorMessage:     op.ErrorMessage,
		RequestID:        op.RequestID,
		StartTime:        op.StartTime,
		EndTime:          op.EndTime,
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
