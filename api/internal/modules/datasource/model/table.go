package model

import (
	"time"
)

// Table represents a user-created table metadata
type Table struct {
	ID                string    `json:"id"`
	OrganizationID    string    `json:"organization_id"`
	DataSourceID      string    `json:"data_source_id"`
	Name              string    `json:"name"`
	TableID           int       `json:"table_id"`                            // postgres-meta table ID
	PhysicalTableName string    `json:"table_name" gorm:"column:table_name"` // postgres-meta table name (renamed to avoid conflict with GORM's TableName method)
	Description       string    `json:"description"`
	CreatedBy         string    `json:"created_by"`
	UpdatedBy         string    `json:"updated_by"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TableName specifies the table name for GORM
func (Table) TableName() string {
	return "data_source_tables"
}

// CreateTableRequest is the request for creating a new table metadata
type CreateTableRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// UpdateTableRequest is the request for updating table metadata
type UpdateTableRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	UpdatedBy   string  `json:"updated_by"`
}
