package audit

import (
	"context"
	"time"

	"github.com/zgiai/zgi/api/pkg/sql_base/guard"
)

type ClientType string

const (
	ClientTypeAPI      ClientType = "api"
	ClientTypeWorkflow ClientType = "workflow"
	ClientTypeUnknown  ClientType = "unknown"
)

type Status string

const (
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
)

type Context struct {
	OrganizationID string
	WorkspaceID    string
	DataSourceID   string
	DataSourceName string
	TableID        string
	TableName      string
	ClientType     ClientType
	WorkflowRunID  string
	NodeID         string
	CreatedBy      string
	OperationType  string
	RequestID      string
	Attempt        int
	GuardPolicy    *guard.Policy
}

type Record struct {
	Context

	SQLStatement string
	Params       []any
	RowCount     *int64
	DurationMS   int64
	Status       Status
	ErrorCode    string
	ErrorMessage string
	GuardVerdict string
	GuardAction  string
	GuardReasons []byte
	GuardPolicy  []byte
	StartTime    time.Time
	EndTime      time.Time
	ExecutedAt   time.Time
}

type Store interface {
	Insert(ctx context.Context, records []Record) error
}

type Recorder interface {
	Record(ctx context.Context, record Record)
	Close(ctx context.Context) error
}
