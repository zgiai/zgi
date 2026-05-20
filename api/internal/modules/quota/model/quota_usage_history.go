package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// QuotaUsageHistory Quota usage history record
type QuotaUsageHistory struct {
	ID            string        `json:"id" gorm:"primaryKey;type:varchar(255)" db:"id"`
	GroupID       uuid.UUID     `json:"group_id" gorm:"type:uuid;not null;index:idx_quota_history_group" db:"group_id"`
	AccountID     uuid.UUID     `json:"account_id" gorm:"type:uuid;not null;index:idx_quota_history_account" db:"account_id"`
	TenantID      *uuid.UUID    `json:"tenant_id,omitempty" gorm:"type:uuid;index:idx_quota_history_tenant" db:"tenant_id"`
	ResourceType  ResourceType  `json:"resource_type" gorm:"type:varchar(50);not null;index:idx_quota_history_resource_type,idx_quota_history_group_resource" db:"resource_type"`
	OperationType OperationType `json:"operation_type" gorm:"type:varchar(20);not null" db:"operation_type"`
	Delta         int64         `json:"delta" gorm:"not null" db:"delta"`
	ValueBefore   int64         `json:"value_before" gorm:"not null" db:"value_before"`
	ValueAfter    int64         `json:"value_after" gorm:"not null" db:"value_after"`
	ResourceID    *string       `json:"resource_id,omitempty" gorm:"type:varchar(255)" db:"resource_id"`
	ResourceName  *string       `json:"resource_name,omitempty" gorm:"type:varchar(500)" db:"resource_name"`
	Metadata      *JSONMap      `json:"metadata,omitempty" gorm:"type:jsonb" db:"metadata"`
	CreatedAt     time.Time     `json:"created_at" gorm:"autoCreateTime;index:idx_quota_history_created_at,idx_quota_history_group_created" db:"created_at"`
}

// JSONMap Custom JSON type for storing metadata
type JSONMap map[string]interface{}

// Value Implement driver.Valuer interface
func (j JSONMap) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan Implement sql.Scanner interface
func (j *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}

	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return nil
	}

	return json.Unmarshal(bytes, j)
}

// ResourceType Resource type enum
type ResourceType string

const (
	ResourceTypeSeats              ResourceType = "seats"               // Seats
	ResourceTypeStorage            ResourceType = "storage"             // Storage space (bytes)
	ResourceTypeDBRows             ResourceType = "db_rows"             // Database rows
	ResourceTypeKnowledgeBases     ResourceType = "knowledge_bases"     // Knowledge base count
	ResourceTypeAIAgents           ResourceType = "ai_agents"           // AI agent count
	ResourceTypeWorkflows          ResourceType = "workflows"           // Workflow count
	ResourceTypeWorkflowExecutions ResourceType = "workflow_executions" // Workflow execution count
	ResourceTypeOCRPages           ResourceType = "ocr_pages"           // OCR page count
)

// OperationType Operation type enum
type OperationType string

const (
	OperationTypeIncrease OperationType = "increase" // Increase
	OperationTypeDecrease OperationType = "decrease" // Decrease
)

// TableName Specify table name
func (QuotaUsageHistory) TableName() string {
	return "quota_usage_history"
}

// GetResourceTypeDisplayName Get display name of resource type
func (rt ResourceType) GetDisplayName() string {
	switch rt {
	case ResourceTypeSeats:
		return "Seats"
	case ResourceTypeStorage:
		return "Storage"
	case ResourceTypeDBRows:
		return "Database Rows"
	case ResourceTypeKnowledgeBases:
		return "Knowledge Bases"
	case ResourceTypeAIAgents:
		return "AI Agents"
	case ResourceTypeWorkflows:
		return "Workflows"
	case ResourceTypeWorkflowExecutions:
		return "Workflow Executions"
	case ResourceTypeOCRPages:
		return "OCR Pages"
	default:
		return string(rt)
	}
}

// GetOperationTypeDisplayName Get display name of operation type
func (ot OperationType) GetDisplayName() string {
	switch ot {
	case OperationTypeIncrease:
		return "Increase"
	case OperationTypeDecrease:
		return "Decrease"
	default:
		return string(ot)
	}
}

// IsValidResourceType Check if resource type is valid
func IsValidResourceType(rt ResourceType) bool {
	switch rt {
	case ResourceTypeSeats, ResourceTypeStorage, ResourceTypeDBRows,
		ResourceTypeKnowledgeBases, ResourceTypeAIAgents, ResourceTypeWorkflows,
		ResourceTypeWorkflowExecutions, ResourceTypeOCRPages:
		return true
	default:
		return false
	}
}

// IsValidOperationType Check if operation type is valid
func IsValidOperationType(ot OperationType) bool {
	switch ot {
	case OperationTypeIncrease, OperationTypeDecrease:
		return true
	default:
		return false
	}
}

// GetResourceUnit Get unit of resource
func (rt ResourceType) GetUnit() string {
	switch rt {
	case ResourceTypeSeats:
		return "count"
	case ResourceTypeStorage:
		return "bytes"
	case ResourceTypeDBRows:
		return "rows"
	case ResourceTypeKnowledgeBases:
		return "count"
	case ResourceTypeAIAgents:
		return "count"
	case ResourceTypeWorkflows:
		return "count"
	case ResourceTypeWorkflowExecutions:
		return "times"
	case ResourceTypeOCRPages:
		return "pages"
	default:
		return "count"
	}
}
