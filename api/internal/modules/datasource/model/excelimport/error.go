package excelimport

import "time"

type ImportError struct {
	ID           string    `json:"id" gorm:"type:uuid;primaryKey"`
	JobID        string    `json:"job_id" gorm:"type:uuid;not null"`
	RowIndex     int       `json:"row_index" gorm:"not null"`
	ColumnName   *string   `json:"column_name" gorm:"type:varchar(255)"`
	RawValue     *string   `json:"raw_value" gorm:"type:text"`
	ErrorCode    string    `json:"error_code" gorm:"type:varchar(64);not null"`
	ErrorMessage string    `json:"error_message" gorm:"type:text;not null"`
	CreatedAt    time.Time `json:"created_at"`
}

func (ImportError) TableName() string {
	return "data_source_import_job_errors"
}
