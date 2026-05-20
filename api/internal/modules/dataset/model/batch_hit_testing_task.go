package model

import (
	"database/sql/driver"
	"encoding/json"
	"time"

	"github.com/zgiai/ginext/internal/dto"
)

// BatchHitTestingTask represents a batch hit testing task in database
type BatchHitTestingTask struct {
	TaskID         string    `json:"task_id" gorm:"primaryKey;type:varchar(36);not null"`
	DatasetID      string    `json:"dataset_id" gorm:"type:uuid;not null;index:idx_batch_hit_testing_tasks_dataset_id"`
	AccountID      string    `json:"account_id" gorm:"type:uuid;not null"`
	OrganizationID string    `json:"organization_id" gorm:"type:uuid;not null;index:idx_batch_hit_testing_tasks_organization_id"`
	Status         string    `json:"status" gorm:"type:varchar(20);not null;default:'pending'"`
	Progress       int       `json:"progress" gorm:"type:integer;not null;default:0"`
	Total          int       `json:"total" gorm:"type:integer;not null"`
	Completed      int       `json:"completed" gorm:"type:integer;not null;default:0"`
	Failed         int       `json:"failed" gorm:"type:integer;not null;default:0"`
	CreatedAt      time.Time `json:"created_at" gorm:"not null;default:CURRENT_TIMESTAMP(0)"`
	StartedAt      time.Time `json:"started_at" gorm:"default:null"`
	FinishedAt     time.Time `json:"finished_at" gorm:"default:null"`

	// Query tasks as JSON field
	Queries JSONQueryTasks `json:"queries" gorm:"type:jsonb"`

	// Calculated fields
	CreatedAtTimestamp  int64  `json:"created_at_timestamp" gorm:"-"`
	StartedAtTimestamp  *int64 `json:"started_at_timestamp" gorm:"-"`
	FinishedAtTimestamp *int64 `json:"finished_at_timestamp" gorm:"-"`
}

// JSONQueryTasks is a custom type for storing query tasks as JSON in database
type JSONQueryTasks []QueryTask

// Value implements driver.Valuer interface
func (j JSONQueryTasks) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return json.Marshal(j)
}

// Scan implements sql.Scanner interface
func (j *JSONQueryTasks) Scan(value interface{}) error {
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

// QueryTask represents a single query task in database
type QueryTask struct {
	Query      string                  `json:"query"`
	Status     string                  `json:"status"` // pending, processing, completed, failed
	Result     *dto.HitTestingResponse `json:"result,omitempty"`
	Error      *string                 `json:"error,omitempty"`
	StartedAt  *time.Time              `json:"started_at,omitempty"`
	FinishedAt *time.Time              `json:"finished_at,omitempty"`
}

// TableName specifies table name
func (BatchHitTestingTask) TableName() string {
	return "batch_hit_testing_tasks"
}

// ToDTO converts a BatchHitTestingTask to DTO
func (t *BatchHitTestingTask) ToDTO() *dto.BatchHitTestingTaskStatus {
	dtoTask := &dto.BatchHitTestingTaskStatus{
		TaskID:    t.TaskID,
		Status:    t.Status,
		Progress:  t.Progress,
		Total:     t.Total,
		Completed: t.Completed,
		Failed:    t.Failed,
		CreatedAt: t.CreatedAt.Unix(),
	}

	// Set timestamps
	if !t.StartedAt.IsZero() {
		timestamp := t.StartedAt.Unix()
		dtoTask.StartedAt = &timestamp
	}

	if !t.FinishedAt.IsZero() {
		timestamp := t.FinishedAt.Unix()
		dtoTask.FinishedAt = &timestamp
	}

	// Add query results if task is completed or failed
	if t.Status == "completed" || t.Status == "failed" {
		var results []dto.QueryResult
		for _, queryTask := range t.Queries {
			var qStartedAt *int64
			if queryTask.StartedAt != nil && !queryTask.StartedAt.IsZero() {
				unix := queryTask.StartedAt.Unix()
				qStartedAt = &unix
			}

			var qFinishedAt *int64
			if queryTask.FinishedAt != nil && !queryTask.FinishedAt.IsZero() {
				unix := queryTask.FinishedAt.Unix()
				qFinishedAt = &unix
			}

			result := dto.QueryResult{
				Query:      queryTask.Query,
				Status:     queryTask.Status,
				Result:     queryTask.Result,
				Error:      queryTask.Error,
				StartedAt:  qStartedAt,
				FinishedAt: qFinishedAt,
			}
			results = append(results, result)
		}
		dtoTask.Results = results
	}

	return dtoTask
}
