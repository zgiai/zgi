package workflow

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"github.com/zgiai/zgi/api/pkg/database"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const workflowConnectorConfirmationResolutionMetadataKey = "connector_confirmation_resolution"

type workflowConnectorConfirmationResolution struct {
	routeReceipt     string
	state            connectorconfirmation.State
	decisionID       string
	errorCode        string
	workflowTargetID uuid.UUID
	runID            uuid.UUID
	nodeID           string
	requestIdentity  connectorconfirmation.ConfirmationRequestIdentity
}

// RecordConnectorConfirmationResolution projects only a safe terminal onto
// the exact paused Workflow run and node named by the frozen origin. It never
// invokes a Connector or model. A completed Connector node is marked
// succeeded and its safe terminal is written into the durable pause snapshot;
// the existing resume outbox then continues only the remaining graph.
func (s *WorkflowService) RecordConnectorConfirmationResolution(
	ctx context.Context,
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) error {
	resolution, err := validateWorkflowConnectorConfirmationResolution(command, result)
	if err != nil {
		return err
	}
	db := s.workflowConnectorConfirmationDB()
	if ctx == nil || db == nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}

	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return recordWorkflowConnectorConfirmationResolutionTx(ctx, tx, command, resolution)
	})
}

func (s *WorkflowService) workflowConnectorConfirmationDB() *gorm.DB {
	if s != nil && s.workflowDB != nil {
		return s.workflowDB
	}
	return database.GetDB()
}

func recordWorkflowConnectorConfirmationResolutionTx(
	ctx context.Context,
	tx *gorm.DB,
	command connectorconfirmation.Command,
	resolution workflowConnectorConfirmationResolution,
) error {
	var run WorkflowRunLog
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? AND deleted_at IS NULL", resolution.runID).
		Take(&run).Error; err != nil {
		return workflowConnectorResolutionLoadError(err)
	}
	if run.RuntimeProtocolVersion < workflowRuntimeProtocolVersionV2 ||
		strings.TrimSpace(run.CreatedBy) != command.OwnerAccountID.String() {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	if err := validateWorkflowConnectorResolutionScope(
		ctx, tx, &run, command.OrganizationID, resolution.workflowTargetID,
	); err != nil {
		return err
	}
	node, pending, err := loadWorkflowConnectorResolutionNode(ctx, tx, &run, resolution)
	if err != nil {
		return err
	}
	if !pending {
		return validateExistingWorkflowConnectorTerminal(&run, node, resolution)
	}
	if run.Status != dto.WorkflowRunStatusPaused || node.Status != string(dto.NodeStatusPaused) {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}

	pauseRecord, reason, _, err := lockWorkflowConnectorConfirmationPause(ctx, tx, run.ID, resolution.nodeID)
	if err != nil {
		return err
	}
	if _, err := loadWorkflowConnectorResolutionPauseSnapshot(&run, pauseRecord, resolution); err != nil {
		return err
	}
	terminal := resolution.terminalPayload()
	now := time.Now()
	if resolution.state == connectorconfirmation.StateCompleted {
		if err := updateWorkflowConnectorResumeSnapshotTx(tx, &run, pauseRecord, resolution, terminal); err != nil {
			return err
		}
	}
	metadata, err := workflowConnectorResolutionMetadata(node, resolution)
	if err != nil {
		return err
	}
	terminalJSON, err := json.Marshal(terminal)
	if err != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}

	nodeStatus := "stopped"
	if resolution.state == connectorconfirmation.StateCompleted {
		nodeStatus = string(dto.NodeStatusSucceeded)
	} else if resolution.state == connectorconfirmation.StateFailed || resolution.state == connectorconfirmation.StateUnknown {
		nodeStatus = string(dto.NodeStatusFailed)
	}
	nodeUpdates := map[string]interface{}{
		"status": nodeStatus, "outputs": string(terminalJSON), "execution_metadata": string(metadataJSON),
		"finished_at": now,
	}
	if resolution.errorCode != "" {
		nodeUpdates["error"] = resolution.errorCode
	} else {
		nodeUpdates["error"] = nil
	}
	updatedNode := tx.Model(&WorkflowNodeRuntimeLog{}).
		Where("id = ? AND workflow_run_id = ? AND node_id = ? AND status = ?", node.ID, run.ID, resolution.nodeID, dto.NodeStatusPaused).
		Updates(nodeUpdates)
	if updatedNode.Error != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	if updatedNode.RowsAffected != 1 {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}

	if err := appendWorkflowConnectorResolutionEventsTx(
		ctx, tx, &run, pauseRecord, resolution, terminal, resolution.state != connectorconfirmation.StateCompleted, now,
	); err != nil {
		return err
	}
	if resolution.state == connectorconfirmation.StateCompleted {
		if _, _, err := workflowpause.NewService(tx).CompleteReasonsTx(ctx, tx, workflowpause.CompleteReasonsParams{
			WorkflowRunID:         run.ID,
			PauseID:               pauseRecord.ID,
			ReasonType:            workflowpause.ReasonTypeConnectorConfirmationRequired,
			ResumeInteractionType: workflowpause.ReasonTypeConnectorConfirmationRequired,
			NodeID:                resolution.nodeID,
			TriggerID:             resolution.nodeID,
			ResumeInputs:          map[string]interface{}{},
		}); err != nil {
			return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
		}
		updatedRun := tx.Model(&WorkflowRunLog{}).
			Where("id = ? AND status = ? AND execution_generation = ?", run.ID, dto.WorkflowRunStatusPaused, run.ExecutionGeneration).
			Updates(map[string]interface{}{
				"outputs": string(terminalJSON), "error": nil,
				"state_revision": gorm.Expr("state_revision + 1"),
			})
		if updatedRun.Error != nil {
			return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
		}
		if updatedRun.RowsAffected != 1 {
			return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
		}
		return nil
	}

	updatedReason := tx.Model(&workflowpause.RunPauseReason{}).
		Where("id = ? AND pause_id = ? AND status = ? AND revision = ?", reason.ID, pauseRecord.ID, workflowpause.RunPauseReasonStatusPending, reason.Revision).
		Updates(map[string]interface{}{
			"status":   workflowpause.RunPauseReasonStatusCompleted,
			"revision": gorm.Expr("revision + 1"), "completed_at": now,
		})
	if updatedReason.Error != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	if updatedReason.RowsAffected != 1 {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}

	runStatus := dto.WorkflowRunStatusStopped
	if resolution.state == connectorconfirmation.StateFailed || resolution.state == connectorconfirmation.StateUnknown {
		runStatus = dto.WorkflowRunStatusFailed
	}
	if err := closeWorkflowConnectorConfirmationPauseTx(tx, &run, pauseRecord, node.ID, runStatus, now); err != nil {
		return err
	}
	runUpdates := map[string]interface{}{
		"status": runStatus, "outputs": string(terminalJSON), "finished_at": now,
		"active_execution_id": nil, "execution_lease_expires_at": nil,
		"execution_generation": gorm.Expr("execution_generation + 1"),
		"state_revision":       gorm.Expr("state_revision + 1"),
	}
	if resolution.errorCode != "" {
		runUpdates["error"] = resolution.errorCode
		runUpdates["exceptions_count"] = gorm.Expr("exceptions_count + 1")
	} else {
		runUpdates["error"] = nil
	}
	updatedRun := tx.Model(&WorkflowRunLog{}).
		Where("id = ? AND status = ? AND execution_generation = ?", run.ID, dto.WorkflowRunStatusPaused, run.ExecutionGeneration).
		Updates(runUpdates)
	if updatedRun.Error != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	if updatedRun.RowsAffected != 1 {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	return nil
}

// validateWorkflowConnectorResolutionScope preserves the immutable ownership
// and target lineage of a submitted Connector operation without consulting its
// mutable Binding or Connection. Those records may legitimately be detached
// after the Provider accepts the operation, but a result must still belong to
// the same live Workflow, Agent, Workspace, organization, and frozen target.
func validateWorkflowConnectorResolutionScope(
	ctx context.Context,
	tx *gorm.DB,
	run *WorkflowRunLog,
	organizationID uuid.UUID,
	targetID uuid.UUID,
) error {
	if ctx == nil || tx == nil || run == nil || organizationID == uuid.Nil || targetID == uuid.Nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}

	var workflow Workflow
	result := tx.WithContext(ctx).
		Table("workflows AS workflow").
		Select("workflow.*").
		Joins("JOIN agents AS owner ON owner.id = workflow.agent_id AND owner.tenant_id = workflow.tenant_id AND owner.deleted_at IS NULL").
		Joins("JOIN workspaces AS workspace ON workspace.id = workflow.tenant_id AND workspace.status = ?", "normal").
		Where(
			"workflow.id = ? AND workflow.agent_id = ? AND workflow.tenant_id = ? AND workspace.organization_id = ? AND workflow.deleted_at IS NULL",
			run.WorkflowID, run.AgentID, run.TenantID, organizationID,
		).
		Take(&workflow)
	if result.Error != nil {
		return workflowConnectorResolutionLoadError(result.Error)
	}

	var expectedTargetID uuid.UUID
	if strings.EqualFold(strings.TrimSpace(workflow.Version), "draft") {
		parsed, err := uuid.Parse(strings.TrimSpace(workflow.ID))
		if err != nil || parsed == uuid.Nil || parsed.String() != strings.TrimSpace(workflow.ID) {
			return workflowConnectorResolutionError(connectorcore.ErrorCodeAccessDenied)
		}
		expectedTargetID = parsed
	} else {
		if !workflow.VersionUUID.Valid || workflow.VersionUUID.UUID == uuid.Nil {
			return workflowConnectorResolutionError(connectorcore.ErrorCodeAccessDenied)
		}
		expectedTargetID = workflow.VersionUUID.UUID
	}
	if expectedTargetID != targetID {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	return nil
}

func loadWorkflowConnectorResolutionNode(
	ctx context.Context,
	tx *gorm.DB,
	run *WorkflowRunLog,
	resolution workflowConnectorConfirmationResolution,
) (*WorkflowNodeRuntimeLog, bool, error) {
	var candidates []WorkflowNodeRuntimeLog
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where(
			"workflow_run_id = ? AND node_id = ? AND workflow_id = ? AND agent_id = ? AND tenant_id = ? AND deleted_at IS NULL",
			run.ID, resolution.nodeID, run.WorkflowID, run.AgentID, run.TenantID,
		).
		Order("created_at DESC, id DESC").Find(&candidates).Error; err != nil {
		return nil, false, workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	var match *WorkflowNodeRuntimeLog
	pending := false
	for index := range candidates {
		candidate := &candidates[index]
		candidatePending := workflowConnectorPendingReceipt(candidate, resolution) == resolution.routeReceipt
		candidateTerminal := workflowConnectorTerminalReceipt(candidate) == resolution.routeReceipt
		if !candidatePending && !candidateTerminal {
			continue
		}
		if match != nil {
			return nil, false, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
		}
		copy := *candidate
		match = &copy
		pending = candidatePending
	}
	if match == nil || strings.TrimSpace(match.CreatedBy) != run.CreatedBy || match.NodeType != "tools" {
		return nil, false, workflowConnectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	return match, pending, nil
}

func workflowConnectorPendingReceipt(
	node *WorkflowNodeRuntimeLog,
	resolution workflowConnectorConfirmationResolution,
) string {
	if node == nil {
		return ""
	}
	outputs, err := node.GetOutputsDict()
	if err != nil {
		return ""
	}
	confirmation, ok := workflowPausedConnectorConfirmation(outputs)
	if !ok || confirmation.Target.ID != resolution.workflowTargetID.String() ||
		confirmation.Target.BindingInstanceID != node.NodeID ||
		confirmation.ConnectorID != resolution.requestIdentity.ConnectorID ||
		confirmation.OperationID != resolution.requestIdentity.OperationID {
		return ""
	}
	return string(confirmation.RouteReceipt)
}

func workflowConnectorTerminalReceipt(node *WorkflowNodeRuntimeLog) string {
	metadata, err := workflowConnectorNodeMetadata(node)
	if err != nil {
		return ""
	}
	record, ok := metadata[workflowConnectorConfirmationResolutionMetadataKey].(map[string]interface{})
	if !ok {
		return ""
	}
	receipt, _ := record["route_receipt"].(string)
	return strings.TrimSpace(receipt)
}

func workflowConnectorNodeMetadata(node *WorkflowNodeRuntimeLog) (map[string]interface{}, error) {
	if node == nil || node.ExecutionMetadata == nil || strings.TrimSpace(*node.ExecutionMetadata) == "" {
		return map[string]interface{}{}, nil
	}
	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(*node.ExecutionMetadata), &metadata); err != nil {
		return nil, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	return metadata, nil
}

func workflowConnectorResolutionMetadata(
	node *WorkflowNodeRuntimeLog,
	resolution workflowConnectorConfirmationResolution,
) (map[string]interface{}, error) {
	metadata, err := workflowConnectorNodeMetadata(node)
	if err != nil {
		return nil, err
	}
	if _, exists := metadata[workflowConnectorConfirmationResolutionMetadataKey]; exists {
		return nil, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	metadata[workflowConnectorConfirmationResolutionMetadataKey] = resolution.eventPayload()
	return metadata, nil
}

func updateWorkflowConnectorResumeSnapshotTx(
	tx *gorm.DB,
	run *WorkflowRunLog,
	pauseRecord *workflowpause.RunPause,
	resolution workflowConnectorConfirmationResolution,
	terminal map[string]interface{},
) error {
	if tx == nil || run == nil || pauseRecord == nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	state, err := loadWorkflowConnectorResolutionPauseSnapshot(run, pauseRecord, resolution)
	if err != nil {
		return err
	}
	if state.ExecutorState.CompletedNodes == nil {
		state.ExecutorState.CompletedNodes = map[string]bool{}
	}
	if state.ExecutorState.CompletedNodes[resolution.nodeID] {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	state.ExecutorState.CompletedNodes[resolution.nodeID] = true
	if state.ExecutorState.ExecutionOutputs == nil {
		state.ExecutorState.ExecutionOutputs = map[string]map[string]interface{}{}
	}
	state.ExecutorState.ExecutionOutputs[resolution.nodeID] = copyWorkflowAnyMap(terminal)
	if state.VariablePool.Variables == nil {
		state.VariablePool.Variables = map[string]map[string]interface{}{}
	}
	state.VariablePool.Variables[resolution.nodeID] = copyWorkflowAnyMap(terminal)
	encoded, err := json.Marshal(state)
	if err != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	updated := tx.Model(&workflowpause.RunPause{}).
		Where("id = ? AND workflow_run_id = ? AND generation = ? AND revision = ? AND status = ? AND resumed_at IS NULL",
			pauseRecord.ID, run.ID, pauseRecord.Generation, pauseRecord.Revision, workflowpause.RunPauseStatusPaused).
		Update("state_json", string(encoded))
	if updated.Error != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	if updated.RowsAffected != 1 {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	pauseRecord.StateJSON = string(encoded)
	return nil
}

func loadWorkflowConnectorResolutionPauseSnapshot(
	run *WorkflowRunLog,
	pauseRecord *workflowpause.RunPause,
	resolution workflowConnectorConfirmationResolution,
) (*workflowpause.State, error) {
	if run == nil || pauseRecord == nil {
		return nil, workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	var state workflowpause.State
	if err := json.Unmarshal([]byte(pauseRecord.StateJSON), &state); err != nil {
		return nil, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	if state.Version != workflowpause.StateVersion || state.WorkflowRunID != run.ID ||
		state.WorkflowID != run.WorkflowID || state.AppID != run.AgentID || state.TenantID != run.TenantID ||
		!workflowConnectorPausedNodePresent(state.ExecutorState, resolution.nodeID) {
		return nil, workflowConnectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	pending, ok := state.ExecutorState.ExecutionOutputs[resolution.nodeID]
	if !ok {
		return nil, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	confirmation, ok := workflowPausedConnectorConfirmation(pending)
	if !ok || string(confirmation.RouteReceipt) != resolution.routeReceipt ||
		confirmation.Target.ID != resolution.workflowTargetID.String() ||
		confirmation.Target.BindingInstanceID != resolution.nodeID ||
		confirmation.ConnectorID != resolution.requestIdentity.ConnectorID ||
		confirmation.OperationID != resolution.requestIdentity.OperationID {
		return nil, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	return &state, nil
}

func workflowConnectorPausedNodePresent(state workflowpause.ExecutorState, nodeID string) bool {
	for _, candidate := range workflowResumePausedNodeIDs(state) {
		if candidate == nodeID {
			return true
		}
	}
	return false
}

func lockWorkflowConnectorConfirmationPause(
	ctx context.Context,
	tx *gorm.DB,
	workflowRunID string,
	nodeID string,
) (*workflowpause.RunPause, *workflowpause.RunPauseReason, int64, error) {
	var pauses []workflowpause.RunPause
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("workflow_run_id = ? AND resumed_at IS NULL AND status = ?", workflowRunID, workflowpause.RunPauseStatusPaused).
		Find(&pauses).Error; err != nil {
		return nil, nil, 0, workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	if len(pauses) != 1 {
		return nil, nil, 0, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	var reasons []workflowpause.RunPauseReason
	if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("pause_id = ?", pauses[0].ID).Order("created_at ASC, id ASC").Find(&reasons).Error; err != nil {
		return nil, nil, 0, workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	var match *workflowpause.RunPauseReason
	pending := int64(0)
	for index := range reasons {
		reason := &reasons[index]
		if reason.Status == workflowpause.RunPauseReasonStatusPending {
			pending++
		}
		if reason.Type == workflowpause.ReasonTypeConnectorConfirmationRequired && reason.NodeID == nodeID {
			if match != nil || reason.Status != workflowpause.RunPauseReasonStatusPending {
				return nil, nil, 0, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
			}
			copy := *reason
			match = &copy
		}
	}
	if match == nil || pending == 0 {
		return nil, nil, 0, workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	return &pauses[0], match, pending, nil
}

func appendWorkflowConnectorResolutionEventsTx(
	ctx context.Context,
	tx *gorm.DB,
	run *WorkflowRunLog,
	pauseRecord *workflowpause.RunPause,
	resolution workflowConnectorConfirmationResolution,
	terminal map[string]interface{},
	terminateRun bool,
	now time.Time,
) error {
	pauseGeneration := pauseRecord.Generation
	pauseRevision := pauseRecord.Revision
	events := []workflowpause.EventDraft{{
		EventType: workflowpause.EventConnectorConfirmationResolved,
		Category:  workflowpause.EventCategoryInteraction,
		PauseID:   pauseRecord.ID, PauseGeneration: &pauseGeneration,
		IdempotencyKey: "connector-confirmation:" + resolution.routeReceipt + ":" + string(resolution.state),
		OccurredAt:     now, EventData: resolution.eventPayload(),
	}}
	if terminateRun {
		runStatus := string(dto.WorkflowRunStatusStopped)
		if resolution.state == connectorconfirmation.StateFailed || resolution.state == connectorconfirmation.StateUnknown {
			runStatus = string(dto.WorkflowRunStatusFailed)
		}
		finishData := map[string]interface{}{
			"id": run.ID, "workflow_run_id": run.ID, "workflow_id": run.WorkflowID,
			"status": runStatus, "outputs": terminal, "finished_at": now.Unix(),
		}
		if resolution.errorCode != "" {
			finishData["error"] = resolution.errorCode
		}
		events = append(events, workflowpause.EventDraft{
			EventType: workflowpause.EventWorkflowFinished, Category: workflowpause.EventCategoryControl,
			PauseID: pauseRecord.ID, PauseGeneration: &pauseGeneration,
			IdempotencyKey: "connector-confirmation:" + resolution.routeReceipt + ":workflow-finished",
			OccurredAt:     now, EventData: finishData,
		})
	}
	_, err := workflowpause.NewService(tx).AppendEventBatchTx(ctx, tx, workflowpause.AppendEventBatchRequest{
		TenantID: run.TenantID, AppID: run.AgentID, WorkflowRunID: run.ID,
		FlushReason: "connector_confirmation_resolution",
		Fence: workflowpause.RuntimeFence{
			ExpectedPauseID: pauseRecord.ID, ExpectedPauseGeneration: &pauseGeneration, ExpectedPauseRevision: &pauseRevision,
		},
		Events: events,
	})
	if err != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	return nil
}

func closeWorkflowConnectorConfirmationPauseTx(
	tx *gorm.DB,
	run *WorkflowRunLog,
	pauseRecord *workflowpause.RunPause,
	resolvedNodeLogID string,
	runStatus dto.WorkflowRunStatus,
	now time.Time,
) error {
	otherStatus := "stopped"
	if runStatus == dto.WorkflowRunStatusFailed {
		otherStatus = "stopped"
	}
	if err := tx.Model(&WorkflowNodeRuntimeLog{}).
		Where("workflow_run_id = ? AND id <> ? AND deleted_at IS NULL AND status IN ?", run.ID, resolvedNodeLogID, []string{"running", "paused"}).
		Updates(map[string]interface{}{"status": otherStatus, "finished_at": now}).Error; err != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	closed := tx.Model(&workflowpause.RunPause{}).
		Where("id = ? AND workflow_run_id = ? AND status = ? AND revision = ?", pauseRecord.ID, run.ID, workflowpause.RunPauseStatusPaused, pauseRecord.Revision).
		Updates(map[string]interface{}{
			"status": workflowpause.RunPauseStatusClosed, "revision": gorm.Expr("revision + 1"),
			"resumed_at": now, "resume_execution_id": nil, "lease_expires_at": nil,
		})
	if closed.Error != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	if closed.RowsAffected != 1 {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	if err := tx.Model(&workflowpause.RuntimeOutbox{}).
		Where("workflow_run_id = ? AND kind = ? AND status IN ?", run.ID, workflowpause.RuntimeOutboxKindResume, []string{workflowpause.RuntimeOutboxPending, workflowpause.RuntimeOutboxPublished}).
		Updates(map[string]interface{}{"status": workflowpause.RuntimeOutboxObsolete, "updated_at": now}).Error; err != nil {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
	}
	return nil
}

func validateExistingWorkflowConnectorTerminal(
	run *WorkflowRunLog,
	node *WorkflowNodeRuntimeLog,
	resolution workflowConnectorConfirmationResolution,
) error {
	metadata, err := workflowConnectorNodeMetadata(node)
	if err != nil {
		return err
	}
	record, ok := metadata[workflowConnectorConfirmationResolutionMetadataKey].(map[string]interface{})
	if !ok || !resolution.matchesEvent(record) {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	outputs, err := node.GetOutputsDict()
	if err != nil || !workflowConnectorTerminalPayloadMatches(outputs, resolution.terminalPayload()) {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	expectedNodeStatus := "stopped"
	if resolution.state == connectorconfirmation.StateCompleted {
		expectedNodeStatus = string(dto.NodeStatusSucceeded)
	} else if resolution.state == connectorconfirmation.StateFailed || resolution.state == connectorconfirmation.StateUnknown {
		expectedNodeStatus = string(dto.NodeStatusFailed)
	}
	if node.Status != expectedNodeStatus {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	if resolution.state == connectorconfirmation.StateCompleted {
		switch run.Status {
		case dto.WorkflowRunStatusPaused, dto.WorkflowRunStatusRunning,
			dto.WorkflowRunStatusSucceeded, dto.WorkflowRunStatusFailed,
			dto.WorkflowRunStatusStopped, dto.WorkflowRunStatusPartialSucceeded:
			return nil
		default:
			return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
		}
	}
	expectedRunStatus := dto.WorkflowRunStatusStopped
	if resolution.state == connectorconfirmation.StateFailed || resolution.state == connectorconfirmation.StateUnknown {
		expectedRunStatus = dto.WorkflowRunStatusFailed
	}
	if run.Status != expectedRunStatus {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeConflict)
	}
	return nil
}

func validateWorkflowConnectorConfirmationResolution(
	command connectorconfirmation.Command,
	result connectorconfirmation.Result,
) (workflowConnectorConfirmationResolution, error) {
	if command.OrganizationID == uuid.Nil || command.OwnerAccountID == uuid.Nil || result.ExecutionResult != nil ||
		(command.Decision != connectorexecution.ConfirmationDecisionConfirmed &&
			command.Decision != connectorexecution.ConfirmationDecisionCancelled) {
		return workflowConnectorConfirmationResolution{}, workflowConnectorResolutionError(connectorcore.ErrorCodeInvalidInput)
	}
	receipt, receiptOK := canonicalWorkflowConnectorResolutionUUID(string(command.RouteReceipt))
	targetID, targetOK := canonicalWorkflowConnectorResolutionUUID(result.Origin.WorkflowID)
	runID, runOK := canonicalWorkflowConnectorResolutionUUID(result.Origin.RunID)
	nodeID := strings.TrimSpace(result.Origin.NodeID)
	identity := result.RequestIdentity
	if !receiptOK || !targetOK || !runOK || nodeID == "" || nodeID != result.Origin.NodeID || len(nodeID) > 255 ||
		result.Origin.Surface != connectorcore.TargetTypeWorkflow || result.Origin.ConversationID != "" ||
		result.Origin.MessageID != "" || result.Origin.AgentID != "" ||
		identity.OrganizationID != command.OrganizationID || identity.OwnerAccountID != command.OwnerAccountID ||
		identity.ActingPrincipalID != command.OwnerAccountID || identity.BindingID == uuid.Nil ||
		identity.ConnectionID == uuid.Nil || !identity.ConnectorID.Valid() || !identity.OperationID.Valid() ||
		identity.Target.Type != connectorcore.TargetTypeWorkflow || identity.Target.ID != targetID.String() ||
		identity.Target.BindingInstanceID != nodeID || !identity.Interactive ||
		!validWorkflowConnectorResolutionTrace(identity.LogicalOperationID) ||
		!validWorkflowConnectorResolutionTrace(identity.CorrelationID) {
		return workflowConnectorConfirmationResolution{}, workflowConnectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	resolution := workflowConnectorConfirmationResolution{
		routeReceipt: receipt.String(), state: result.State,
		workflowTargetID: targetID, runID: runID, nodeID: nodeID, requestIdentity: identity,
	}
	if result.DecisionID != uuid.Nil {
		resolution.decisionID = result.DecisionID.String()
	}
	if result.ErrorCode != "" {
		resolution.errorCode = string(result.ErrorCode)
	}
	valid := false
	switch result.State {
	case connectorconfirmation.StateCancelled:
		valid = command.Decision == connectorexecution.ConfirmationDecisionCancelled && result.DecisionID != uuid.Nil && result.ErrorCode == ""
	case connectorconfirmation.StateCompleted:
		valid = command.Decision == connectorexecution.ConfirmationDecisionConfirmed && result.DecisionID != uuid.Nil && result.ErrorCode == ""
	case connectorconfirmation.StateFailed:
		valid = command.Decision == connectorexecution.ConfirmationDecisionConfirmed && result.ErrorCode.Valid()
	case connectorconfirmation.StateUnknown:
		valid = command.Decision == connectorexecution.ConfirmationDecisionConfirmed && result.ErrorCode == connectorcore.ErrorCodeOutcomeUnknown
	}
	if !valid {
		return workflowConnectorConfirmationResolution{}, workflowConnectorResolutionError(connectorcore.ErrorCodeInvalidInput)
	}
	return resolution, nil
}

func validWorkflowConnectorResolutionTrace(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && len(value) <= 128 &&
		!strings.ContainsAny(value, "\x00\r\n")
}

func (resolution workflowConnectorConfirmationResolution) terminalPayload() map[string]interface{} {
	payload := map[string]interface{}{"state": string(resolution.state)}
	if resolution.decisionID != "" {
		payload["decision_id"] = resolution.decisionID
	}
	if resolution.errorCode != "" {
		payload["error_code"] = resolution.errorCode
	}
	return payload
}

func (resolution workflowConnectorConfirmationResolution) eventPayload() map[string]interface{} {
	payload := resolution.terminalPayload()
	payload["event_type"] = workflowpause.EventConnectorConfirmationResolved
	payload["route_receipt"] = resolution.routeReceipt
	payload["surface"] = string(connectorcore.TargetTypeWorkflow)
	payload["workflow_id"] = resolution.workflowTargetID.String()
	payload["workflow_run_id"] = resolution.runID.String()
	payload["node_id"] = resolution.nodeID
	return payload
}

func (resolution workflowConnectorConfirmationResolution) matchesEvent(event map[string]interface{}) bool {
	return strings.TrimSpace(workflowString(event, "event_type")) == workflowpause.EventConnectorConfirmationResolved &&
		strings.TrimSpace(workflowString(event, "route_receipt")) == resolution.routeReceipt &&
		strings.TrimSpace(workflowString(event, "surface")) == string(connectorcore.TargetTypeWorkflow) &&
		strings.TrimSpace(workflowString(event, "workflow_id")) == resolution.workflowTargetID.String() &&
		strings.TrimSpace(workflowString(event, "workflow_run_id")) == resolution.runID.String() &&
		strings.TrimSpace(workflowString(event, "node_id")) == resolution.nodeID &&
		strings.TrimSpace(workflowString(event, "state")) == string(resolution.state) &&
		strings.TrimSpace(workflowString(event, "decision_id")) == resolution.decisionID &&
		strings.TrimSpace(workflowString(event, "error_code")) == resolution.errorCode
}

func workflowConnectorTerminalPayloadMatches(left, right map[string]interface{}) bool {
	if len(left) != len(right) {
		return false
	}
	for key, rightValue := range right {
		if leftValue, ok := left[key]; !ok || leftValue != rightValue {
			return false
		}
	}
	return true
}

func canonicalWorkflowConnectorResolutionUUID(value string) (uuid.UUID, bool) {
	if value == "" || value != strings.TrimSpace(value) {
		return uuid.Nil, false
	}
	parsed, err := uuid.Parse(value)
	return parsed, err == nil && parsed != uuid.Nil && parsed.String() == value
}

func workflowConnectorResolutionLoadError(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return workflowConnectorResolutionError(connectorcore.ErrorCodeAccessDenied)
	}
	return workflowConnectorResolutionError(connectorcore.ErrorCodeInternal)
}

func workflowConnectorResolutionError(code connectorcore.ErrorCode) error {
	return connectorcore.NewError(code, false, nil)
}
