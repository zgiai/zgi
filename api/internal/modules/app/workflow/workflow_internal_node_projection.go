package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type internalNodeProjectionEvent struct {
	eventType string
	data      map[string]interface{}
	stored    *workflowpause.RunEventPayload
}

func projectInternalNodeExecution(ctx context.Context, tx *gorm.DB, workflowRunID, eventType string, data map[string]interface{}, stored *workflowpause.RunEventPayload) error {
	return projectInternalNodeExecutions(ctx, tx, workflowRunID, []internalNodeProjectionEvent{{
		eventType: eventType,
		data:      data,
		stored:    stored,
	}})
}

// projectInternalNodeExecutions projects a committed event batch with one run
// scope read and at most two bulk UPSERT statements (start, then finish).
func projectInternalNodeExecutions(ctx context.Context, tx *gorm.DB, workflowRunID string, events []internalNodeProjectionEvent) error {
	if tx == nil || len(events) == 0 {
		return nil
	}
	filtered := make([]internalNodeProjectionEvent, 0, len(events))
	for _, event := range events {
		if event.stored == nil || (event.eventType != workflowpause.EventNodeStarted && event.eventType != workflowpause.EventNodeFinished) {
			continue
		}
		if internalNodeProjectionIdentity(event.data) == "" {
			continue
		}
		filtered = append(filtered, event)
	}
	if len(filtered) == 0 {
		return nil
	}

	var run WorkflowRunLog
	if err := tx.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", workflowRunID).First(&run).Error; err != nil {
		return fmt.Errorf("load workflow run for internal node projection: %w", err)
	}

	starts := make([]WorkflowNodeRuntimeLog, 0, len(filtered))
	finishes := make([]WorkflowNodeRuntimeLog, 0, len(filtered))
	for _, event := range filtered {
		log := internalNodeProjectionLog(run, workflowRunID, event)
		if event.eventType == workflowpause.EventNodeStarted {
			starts = append(starts, log)
		} else {
			finishes = append(finishes, log)
		}
	}
	if err := upsertInternalNodeStarts(ctx, tx, starts); err != nil {
		return err
	}
	return upsertInternalNodeFinishes(ctx, tx, finishes)
}

func internalNodeProjectionIdentity(data map[string]interface{}) string {
	containerID := strings.TrimSpace(workflowEventString(firstWorkflowValue(data["container_id"], data["loop_id"], data["iteration_id"])))
	nodeExecutionID := strings.TrimSpace(workflowEventString(firstWorkflowValue(data["node_execution_id"], data["id"])))
	if containerID == "" || nodeExecutionID == "" {
		return ""
	}
	return containerID + ":" + nodeExecutionID
}

func internalNodeProjectionLog(run WorkflowRunLog, workflowRunID string, event internalNodeProjectionEvent) WorkflowNodeRuntimeLog {
	data := event.data
	containerID := strings.TrimSpace(workflowEventString(firstWorkflowValue(data["container_id"], data["loop_id"], data["iteration_id"])))
	nodeExecutionID := strings.TrimSpace(workflowEventString(firstWorkflowValue(data["node_execution_id"], data["id"])))
	roundIndex, _ := workflowEventInt(firstWorkflowValue(data["round_index"], data["loop_index"], data["iteration_index"]))
	containerType := strings.TrimSpace(workflowEventString(data["container_type"]))
	if containerType == "" {
		if workflowEventString(data["loop_id"]) != "" {
			containerType = "loop"
		} else {
			containerType = "iteration"
		}
	}
	parentExecutionID := strings.TrimSpace(workflowEventString(firstWorkflowValue(data["parent_execution_id"], data["round_execution_id"])))
	if parentExecutionID == "" {
		parentExecutionID = fmt.Sprintf("round-%s-%d", containerID, roundIndex)
	}
	attempt, ok := workflowEventInt(data["attempt"])
	if !ok || attempt <= 0 {
		attempt = 1
	}
	sequence := int64(event.stored.Sequence)
	createdAt := workflowRuntimeEventTime(data, "created_at_ms", "created_at")
	status := "running"
	if event.eventType == workflowpause.EventNodeFinished {
		status = workflowEventString(data["status"])
	}
	log := WorkflowNodeRuntimeLog{
		TenantID:          run.TenantID,
		AgentID:           run.AgentID,
		WorkflowID:        run.WorkflowID,
		TriggeredFrom:     run.TriggeredFrom,
		WorkflowRunID:     &workflowRunID,
		Index:             workflowEventIntOrDefault(data["index"], 1),
		NodeExecutionID:   &nodeExecutionID,
		NodeID:            workflowEventString(data["node_id"]),
		NodeType:          workflowEventString(data["node_type"]),
		Title:             workflowEventString(data["title"]),
		Inputs:            marshalWorkflowRuntimeValue(data["inputs"]),
		Status:            status,
		CreatedAt:         createdAt,
		CreatedByRole:     "account",
		CreatedBy:         run.CreatedBy,
		ParentExecutionID: &parentExecutionID,
		ContainerID:       &containerID,
		ContainerType:     &containerType,
		RoundIndex:        &roundIndex,
		Attempt:           attempt,
	}
	if event.eventType == workflowpause.EventNodeStarted {
		log.StartedEventSequence = &sequence
		return log
	}
	finishedAt := workflowRuntimeEventTime(data, "finished_at_ms", "finished_at")
	log.Outputs = marshalWorkflowRuntimeValue(data["outputs"])
	log.ProcessData = marshalWorkflowRuntimeValue(data["process_data"])
	log.ExecutionMetadata = marshalWorkflowRuntimeValue(data["execution_metadata"])
	log.ElapsedTime = workflowEventFloat(data["elapsed_time"])
	log.FinishedAt = &finishedAt
	log.FinishedEventSequence = &sequence
	if errorText := strings.TrimSpace(workflowEventString(data["error"])); errorText != "" {
		log.Error = &errorText
	}
	return log
}

func internalNodeProjectionConflict() clause.OnConflict {
	return clause.OnConflict{
		Columns: []clause.Column{{Name: "workflow_run_id"}, {Name: "node_execution_id"}},
		TargetWhere: clause.Where{Exprs: []clause.Expression{clause.Expr{
			SQL: "workflow_run_id IS NOT NULL AND node_execution_id IS NOT NULL AND deleted_at IS NULL",
		}}},
	}
}

func upsertInternalNodeStarts(ctx context.Context, tx *gorm.DB, logs []WorkflowNodeRuntimeLog) error {
	if len(logs) == 0 {
		return nil
	}
	conflict := internalNodeProjectionConflict()
	conflict.DoUpdates = clause.AssignmentColumns([]string{
		"inputs", "started_event_sequence", "node_id", "node_type", "title",
		"parent_execution_id", "container_id", "container_type", "round_index", "attempt",
	})
	if err := tx.WithContext(ctx).Clauses(conflict).Create(&logs).Error; err != nil {
		return fmt.Errorf("upsert internal node start projections: %w", err)
	}
	return nil
}

func upsertInternalNodeFinishes(ctx context.Context, tx *gorm.DB, logs []WorkflowNodeRuntimeLog) error {
	if len(logs) == 0 {
		return nil
	}
	conflict := internalNodeProjectionConflict()
	conflict.DoUpdates = clause.AssignmentColumns([]string{
		"status", "outputs", "process_data", "execution_metadata", "elapsed_time",
		"finished_at", "finished_event_sequence", "error", "node_id", "node_type", "title",
		"parent_execution_id", "container_id", "container_type", "round_index", "attempt",
	})
	if err := tx.WithContext(ctx).Clauses(conflict).Create(&logs).Error; err != nil {
		return fmt.Errorf("upsert internal node finish projections: %w", err)
	}
	return nil
}

func marshalWorkflowRuntimeValue(value interface{}) *string {
	if value == nil {
		return nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	encoded := string(raw)
	return &encoded
}

func workflowRuntimeEventTime(data map[string]interface{}, millisecondsKey, secondsKey string) time.Time {
	if milliseconds, ok := workflowEventInt(data[millisecondsKey]); ok && milliseconds > 0 {
		return time.UnixMilli(int64(milliseconds))
	}
	if seconds, ok := workflowEventInt(data[secondsKey]); ok && seconds > 0 {
		return time.Unix(int64(seconds), 0)
	}
	return time.Now()
}

func workflowEventIntOrDefault(value interface{}, fallback int) int {
	if parsed, ok := workflowEventInt(value); ok {
		return parsed
	}
	return fallback
}

func workflowEventFloat(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	default:
		return 0
	}
}
