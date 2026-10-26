// Package workjournal provides durable, business-independent employee records
// and a single-dispatch journal. Business workflows belong in prompts or Skills.
package workjournal

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
)

const (
	StatePending    = "pending"
	StateExecuting  = "executing"
	StateSucceeded  = "succeeded"
	StateFailed     = "failed"
	StateUnknown    = "unknown"
	MaxPayloadBytes = 48 << 10
)

type Scope struct {
	OrganizationID uuid.UUID `json:"organization_id"`
	WorkspaceID    uuid.UUID `json:"workspace_id"`
	OwnerID        uuid.UUID `json:"owner_id"`
	EmployeeID     uuid.UUID `json:"employee_id"`
}
type Record struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Scope     `gorm:"embedded"`
	ScopeKey  string         `gorm:"size:64;uniqueIndex:idx_work_records_key,priority:1" json:"-"`
	Kind      string         `gorm:"size:128;uniqueIndex:idx_work_records_key,priority:2" json:"kind"`
	Key       string         `gorm:"size:128;uniqueIndex:idx_work_records_key,priority:3" json:"key"`
	State     string         `gorm:"size:128" json:"state"`
	Content   datatypes.JSON `json:"content"`
	Sources   datatypes.JSON `json:"sources"`
	Revision  uint64         `json:"revision"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

func (Record) TableName() string { return "employee_work_records" }

type RecordVersion struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Scope     `gorm:"embedded"`
	RecordID  uuid.UUID      `gorm:"type:uuid;uniqueIndex:idx_work_record_revision,priority:1" json:"record_id"`
	Revision  uint64         `gorm:"uniqueIndex:idx_work_record_revision,priority:2" json:"revision"`
	Kind      string         `json:"kind"`
	Key       string         `json:"key"`
	State     string         `json:"state"`
	Content   datatypes.JSON `json:"content"`
	Sources   datatypes.JSON `json:"sources"`
	CreatedAt time.Time      `json:"created_at"`
}

func (RecordVersion) TableName() string { return "employee_work_record_versions" }

type SaveRecordRequest struct {
	Kind             string         `json:"kind"`
	Key              string         `json:"key"`
	State            string         `json:"state"`
	Content          datatypes.JSON `json:"content"`
	Sources          datatypes.JSON `json:"sources"`
	ExpectedRevision uint64         `json:"expected_revision"`
}
type PrepareRequest struct {
	WorkKey           string
	EmployeeVersionID uuid.UUID
	ConnectorID       string
	ConnectionID      uuid.UUID
	OperationID       string
	Input             datatypes.JSON
}
type Operation struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey" json:"id"`
	Scope             `gorm:"embedded"`
	ScopeKey          string    `gorm:"size:64;uniqueIndex:idx_work_operation_key,priority:1" json:"-"`
	WorkKey           string    `gorm:"size:128;uniqueIndex:idx_work_operation_key,priority:2" json:"work_key"`
	EmployeeVersionID uuid.UUID `gorm:"type:uuid" json:"employee_version_id"`
	ConnectorID       string    `gorm:"size:128" json:"connector_id"`
	ConnectionID      uuid.UUID `gorm:"type:uuid" json:"connection_id"`
	OperationID       string    `gorm:"size:128" json:"operation_id"`
	// Input is frozen JSON text: PostgreSQL jsonb would rewrite numeric lexemes
	// and break the persisted digest and exact request passed to the gateway.
	Input  datatypes.JSON `gorm:"type:text" json:"input"`
	Digest string         `gorm:"size:64" json:"digest"`
	// A nullable unique fingerprint prevents a new work key or version from
	// bypassing a pending, executing or unknown operation.
	UnresolvedFingerprint *string        `gorm:"size:64;uniqueIndex:idx_work_operation_unresolved" json:"-"`
	State                 string         `gorm:"size:16" json:"state"`
	Attempts              uint32         `json:"attempts"`
	Receipt               datatypes.JSON `json:"receipt"`
	EvidenceID            *uuid.UUID     `gorm:"type:uuid" json:"evidence_id,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
}

func (Operation) TableName() string   { return "employee_work_operations" }
func (o Operation) LogicalID() string { return "workjournal:" + o.ID.String() }

// Gateway is a server-owned boundary. Recover may only inspect native audit
// evidence; it must never submit a remote operation or accept model receipts.
type Gateway interface {
	Execute(context.Context, Operation) (DispatchResult, error)
	Recover(context.Context, Operation) (DispatchResult, error)
}
type DispatchResult struct {
	State      string
	Receipt    datatypes.JSON
	EvidenceID *uuid.UUID
}
