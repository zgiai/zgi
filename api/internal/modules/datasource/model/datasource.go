package model

import (
	"time"

	"gorm.io/datatypes"
)

// DataSourcePermissionType represents the permission levels for data sources
// Note: "all_team" means accessible by all members within the same department
// Note: "all_group" means accessible by all members within the entire organization
type DataSourcePermissionType string

const (
	// DataSourcePermissionOnlyMe restricts access to only the data source creator
	DataSourcePermissionOnlyMe DataSourcePermissionType = "only_me"

	// DataSourcePermissionAllTeam allows access to all members within the same department
	// Note: "Team" here refers to department-level organization unit, NOT entire organization
	DataSourcePermissionAllTeam DataSourcePermissionType = "all_team"

	// DataSourcePermissionAllGroup allows access to all members within the entire organization
	// Note: "Group" here refers to the top-level organization unit
	DataSourcePermissionAllGroup DataSourcePermissionType = "all_group"

	// DataSourcePermissionPartial allows access to specified members
	DataSourcePermissionPartial DataSourcePermissionType = "partial_members"
)

// IsValidDataSourcePermission checks if a given string is a valid data source permission
func IsValidDataSourcePermission(permission string) bool {
	switch DataSourcePermissionType(permission) {
	case DataSourcePermissionOnlyMe, DataSourcePermissionAllTeam, DataSourcePermissionAllGroup, DataSourcePermissionPartial:
		return true
	default:
		return false
	}
}

// OperationType represents the type of data operation
type OperationType string

const (
	// OperationTypeCreate represents a create operation
	OperationTypeCreate OperationType = "create"

	// OperationTypeUpdate represents an update operation
	OperationTypeUpdate OperationType = "update"

	// OperationTypeDelete represents a delete operation
	OperationTypeDelete OperationType = "delete"

	// OperationTypeQuery represents a query operation
	OperationTypeQuery OperationType = "query"

	// OperationTypeImport represents an import operation
	OperationTypeImport OperationType = "import"
)

// IsValidOperationType checks if a given string is a valid operation type
func IsValidOperationType(opType string) bool {
	switch OperationType(opType) {
	case OperationTypeCreate, OperationTypeUpdate, OperationTypeDelete, OperationTypeQuery, OperationTypeImport:
		return true
	default:
		return false
	}
}

// OperationStatus represents the status of a data operation
type OperationStatus string

const (
	// OperationStatusSuccess represents a successful operation
	OperationStatusSuccess OperationStatus = "success"

	// OperationStatusFailed represents a failed operation
	OperationStatusFailed OperationStatus = "failed"
)

// IsValidOperationStatus checks if a given string is a valid operation status
func IsValidOperationStatus(status string) bool {
	switch OperationStatus(status) {
	case OperationStatusSuccess, OperationStatusFailed:
		return true
	default:
		return false
	}
}

// DataSourceSQLOperation represents a user's SQL operation log for data source tables
// Stored in the data_source_sql_operations table
type DataSourceSQLOperation struct {
	ID             string     `json:"id" gorm:"type:uuid;primary_key"`
	OrganizationID string     `json:"organization_id" gorm:"type:uuid;not null"`
	WorkspaceID    *string    `json:"workspace_id" gorm:"type:uuid"`
	DataSourceID   string     `json:"data_source_id" gorm:"type:uuid;not null"`
	TableID        *string    `json:"table_id" gorm:"type:uuid"`
	DataSourceName *string    `json:"data_source_name" gorm:"type:varchar(255)"`
	TableName      *string    `json:"table_name" gorm:"type:varchar(255)"`
	SqlStatement   string     `json:"sql_statement" gorm:"type:text;not null"`
	OperationType  string     `json:"operation_type" gorm:"type:varchar(20);not null"`
	ClientType     string     `json:"client_type" gorm:"type:varchar(32);not null;default:unknown"`
	WorkflowRunID  *string    `json:"workflow_run_id" gorm:"type:varchar(255)"`
	NodeID         *string    `json:"node_id" gorm:"type:varchar(255)"`
	ParamsJSON     []byte     `json:"params_json" gorm:"type:jsonb"`
	RowCount       *int64     `json:"row_count"`
	DurationMS     *int64     `json:"duration_ms"`
	ErrorCode      *string    `json:"error_code" gorm:"type:varchar(64)"`
	ErrorMessage   *string    `json:"error_message" gorm:"type:text"`
	ExecutedAt     *time.Time `json:"executed_at" gorm:"type:timestamp"`
	RequestID      *string    `json:"request_id" gorm:"type:varchar(128)"`
	GuardVerdict   *string    `json:"guard_verdict" gorm:"type:varchar(16)"`
	GuardAction    *string    `json:"guard_action" gorm:"type:varchar(16)"`
	GuardReasons   []byte     `json:"guard_reasons" gorm:"type:jsonb"`
	GuardPolicy    []byte     `json:"guard_policy" gorm:"type:jsonb"`
	StartTime      time.Time  `json:"start_time" gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	EndTime        time.Time  `json:"end_time" gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	Status         string     `json:"status" gorm:"type:varchar(10);not null"`
	CreatedBy      string     `json:"created_by" gorm:"type:varchar(36);not null"`
	CreatedAt      time.Time  `json:"created_at" gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
}

// DataSource represents a user-created data source (PostgreSQL schema)
type DataSource struct {
	ID             string         `json:"id"`
	OrganizationID string         `json:"organization_id"`
	WorkspaceID    *string        `json:"workspace_id"`
	Name           string         `json:"name"`
	SchemaID       int            `json:"schema_id"`
	SchemaName     string         `json:"schema_name"`
	Description    string         `json:"description"`
	Permission     string         `json:"permission"`
	Status         string         `json:"status"`
	CreatedBy      string         `json:"created_by"`
	UpdatedBy      string         `json:"updated_by"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	IconType       *string        `gorm:"type:varchar(255)" json:"icon_type"`
	Icon           *string        `json:"icon" gorm:"type:varchar(255)"`
	IconBackground *string        `json:"icon_background" gorm:"type:varchar(255)"`
	GuardPolicy    datatypes.JSON `json:"guard_policy" gorm:"type:jsonb;not null"`
}
