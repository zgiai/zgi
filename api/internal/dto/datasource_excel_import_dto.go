package dto

import "time"

type ExcelImportStatus string

const (
	ExcelImportStatusAnalyzing     ExcelImportStatus = "analyzing"
	ExcelImportStatusNeedsReview   ExcelImportStatus = "needs_review"
	ExcelImportStatusImporting     ExcelImportStatus = "importing"
	ExcelImportStatusCompleted     ExcelImportStatus = "completed"
	ExcelImportStatusFailed        ExcelImportStatus = "failed"
	ExcelImportStatusPartialFailed ExcelImportStatus = "partial_failed"
)

type AnalyzeExcelImportRequest struct {
	UploadFileID string  `json:"upload_file_id" binding:"required"`
	SheetName    *string `json:"sheet_name,omitempty"`
	HeaderRow    *int    `json:"header_row,omitempty"`
	SampleSize   *int    `json:"sample_size,omitempty"`
}

type ExcelImportSheet struct {
	Name        string `json:"name"`
	RowCount    int    `json:"row_count"`
	ColumnCount int    `json:"column_count"`
	Hidden      bool   `json:"hidden"`
	Recommended bool   `json:"recommended"`
}

type ExcelImportWarning struct {
	Code       string  `json:"code"`
	Message    string  `json:"message"`
	RowIndex   *int    `json:"row_index,omitempty"`
	ColumnName *string `json:"column_name,omitempty"`
}

type InferredExcelColumn struct {
	SourceColumn      string               `json:"source_column"`
	SourceColumnIndex int                  `json:"source_column_index"`
	Name              string               `json:"name"`
	DisplayName       string               `json:"display_name"`
	Type              string               `json:"type"`
	IsRequired        bool                 `json:"is_required"`
	Description       string               `json:"description"`
	Confidence        float64              `json:"confidence"`
	SampleValues      []string             `json:"sample_values"`
	Warnings          []ExcelImportWarning `json:"warnings"`
	Enabled           *bool                `json:"enabled,omitempty"`
}

type ExcelImportPreviewRow struct {
	RowIndex int                    `json:"row_index"`
	Values   map[string]interface{} `json:"values"`
}

type AnalyzeExcelImportData struct {
	JobID  string `json:"job_id"`
	Source struct {
		FileName   string             `json:"file_name"`
		SourceType string             `json:"source_type"`
		Sheets     []ExcelImportSheet `json:"sheets"`
	} `json:"source"`
	Selection struct {
		SheetName string `json:"sheet_name"`
		HeaderRow int    `json:"header_row"`
		StartRow  int    `json:"start_row"`
	} `json:"selection"`
	Columns     []InferredExcelColumn   `json:"columns"`
	PreviewRows []ExcelImportPreviewRow `json:"preview_rows"`
	Warnings    []ExcelImportWarning    `json:"warnings"`
}

type ConfirmExcelImportRequest struct {
	Table struct {
		Name        string  `json:"name" binding:"required"`
		Description *string `json:"description,omitempty"`
	} `json:"table" binding:"required"`
	Selection struct {
		SheetName string `json:"sheet_name" binding:"required"`
		HeaderRow int    `json:"header_row" binding:"required,min=1"`
		StartRow  int    `json:"start_row" binding:"required,min=1"`
	} `json:"selection" binding:"required"`
	Columns []InferredExcelColumn `json:"columns" binding:"required,min=1"`
	Options struct {
		ErrorPolicy    string `json:"error_policy"`
		EmptyRowPolicy string `json:"empty_row_policy"`
		BatchSize      int    `json:"batch_size"`
	} `json:"options"`
}

type RecognizeExcelImportTable struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type RecognizeExcelImportSource struct {
	FileName  string `json:"file_name,omitempty"`
	SheetName string `json:"sheet_name,omitempty"`
}

type RecognizeExcelImportRequest struct {
	Table            RecognizeExcelImportTable  `json:"table" binding:"required"`
	Source           RecognizeExcelImportSource `json:"source,omitempty"`
	Columns          []InferredExcelColumn      `json:"columns" binding:"required,min=1"`
	Model            *ModelSpec                 `json:"model" binding:"required"`
	OperatorLanguage string                     `json:"operator_language,omitempty"`
}

type RecognizeExcelImportData struct {
	Table   RecognizeExcelImportTable `json:"table"`
	Columns []InferredExcelColumn     `json:"columns"`
}

type ExcelImportFailedItem struct {
	RowIndex     int     `json:"row_index"`
	ColumnName   *string `json:"column_name,omitempty"`
	RawValue     *string `json:"raw_value,omitempty"`
	ErrorCode    string  `json:"error_code"`
	ErrorMessage string  `json:"error_message"`
}

type ConfirmExcelImportData struct {
	JobID        string                  `json:"job_id"`
	TableID      string                  `json:"table_id"`
	Status       ExcelImportStatus       `json:"status"`
	TotalRows    int                     `json:"total_rows"`
	ImportedRows int                     `json:"imported_rows"`
	FailedRows   int                     `json:"failed_rows"`
	FailedItems  []ExcelImportFailedItem `json:"failed_items"`
}

type ExcelImportJobResponse struct {
	ID             string            `json:"id"`
	OrganizationID string            `json:"organization_id"`
	WorkspaceID    *string           `json:"workspace_id,omitempty"`
	DataSourceID   string            `json:"data_source_id"`
	TableID        *string           `json:"table_id,omitempty"`
	UploadFileID   *string           `json:"upload_file_id,omitempty"`
	SourceType     string            `json:"source_type"`
	SourceFileName string            `json:"source_file_name"`
	Status         ExcelImportStatus `json:"status"`
	TotalRows      int               `json:"total_rows"`
	ValidRows      int               `json:"valid_rows"`
	ImportedRows   int               `json:"imported_rows"`
	FailedRows     int               `json:"failed_rows"`
	SheetName      *string           `json:"sheet_name,omitempty"`
	HeaderRow      *int              `json:"header_row,omitempty"`
	StartRow       *int              `json:"start_row,omitempty"`
	CreatedBy      string            `json:"created_by"`
	UpdatedBy      string            `json:"updated_by"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

type ExcelImportErrorList struct {
	Data     []ExcelImportFailedItem `json:"data"`
	HasMore  bool                    `json:"has_more"`
	Limit    int                     `json:"limit"`
	Offset   int                     `json:"offset"`
	TotalNum int64                   `json:"total_num"`
}
