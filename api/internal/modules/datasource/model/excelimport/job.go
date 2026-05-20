package excelimport

import (
	"time"

	"gorm.io/datatypes"
)

type ImportJob struct {
	ID              string         `json:"id" gorm:"type:uuid;primaryKey"`
	OrganizationID  string         `json:"organization_id" gorm:"type:uuid;not null"`
	WorkspaceID     *string        `json:"workspace_id" gorm:"type:uuid"`
	DataSourceID    string         `json:"data_source_id" gorm:"type:uuid;not null"`
	TableID         *string        `json:"table_id" gorm:"type:uuid"`
	UploadFileID    *string        `json:"upload_file_id" gorm:"type:uuid"`
	SourceType      string         `json:"source_type" gorm:"type:varchar(20);not null"`
	SourceFileName  string         `json:"source_file_name" gorm:"type:varchar(512);not null"`
	Status          string         `json:"status" gorm:"type:varchar(32);not null"`
	TotalRows       int            `json:"total_rows" gorm:"not null;default:0"`
	ValidRows       int            `json:"valid_rows" gorm:"not null;default:0"`
	ImportedRows    int            `json:"imported_rows" gorm:"not null;default:0"`
	FailedRows      int            `json:"failed_rows" gorm:"not null;default:0"`
	SheetName       *string        `json:"sheet_name" gorm:"type:varchar(255)"`
	HeaderRow       *int           `json:"header_row"`
	StartRow        *int           `json:"start_row"`
	SchemaSnapshot  datatypes.JSON `json:"schema_snapshot" gorm:"type:jsonb"`
	PreviewSnapshot datatypes.JSON `json:"preview_snapshot" gorm:"type:jsonb"`
	ErrorSummary    datatypes.JSON `json:"error_summary" gorm:"type:jsonb"`
	CreatedBy       string         `json:"created_by" gorm:"type:varchar(36);not null"`
	UpdatedBy       string         `json:"updated_by" gorm:"type:varchar(36);not null"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func (ImportJob) TableName() string {
	return "data_source_import_jobs"
}
