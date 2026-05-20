package dto

import (
	"time"
)

type SegmentStats struct {
	Total      int `json:"total"`
	Completed  int `json:"completed"`
	Processing int `json:"processing"`
	Failed     int `json:"failed"`
	Pending    int `json:"pending"`
}

type SegmentDetail struct {
	SegmentID        string    `json:"segment_id"`
	Position         int       `json:"position"`
	Status           string    `json:"status"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Error            *string   `json:"error"`
	PotentiallyStuck bool      `json:"potentially_stuck"`
}

type TaskStatusInfo struct {
	DocumentIndexingTaskID         string    `json:"document_indexing_task_id"`
	DocumentIndexingTaskStatus     string    `json:"document_indexing_task_status"`
	DocumentIndexingTaskCreatedAt  time.Time `json:"document_indexing_task_created_at"`
	DocumentIndexingTaskUpdatedAt  time.Time `json:"document_indexing_task_updated_at"`
	DocumentIndexingTaskRetryCount int       `json:"document_indexing_task_retry_count"`
}

type RedisQueueStatus struct {
	MainQueueLength       int64 `json:"main_queue_length"`
	ProcessingQueueLength int64 `json:"processing_queue_length"`
	StuckTasksCount       int   `json:"stuck_tasks_count"`
}
