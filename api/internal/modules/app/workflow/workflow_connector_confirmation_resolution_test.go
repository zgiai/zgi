package workflow

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	workflowpause "github.com/zgiai/zgi/api/internal/modules/app/workflow/pause"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	connectorconfirmation "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeconfirmation"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type workflowConnectorResolutionWorkspace struct {
	ID             uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID uuid.UUID `gorm:"type:uuid;not null"`
	Status         string    `gorm:"type:varchar(32);not null"`
}

func (workflowConnectorResolutionWorkspace) TableName() string { return "workspaces" }

type workflowConnectorResolutionAgent struct {
	ID         uuid.UUID  `gorm:"type:uuid;primaryKey"`
	TenantID   uuid.UUID  `gorm:"type:uuid;not null"`
	WorkflowID *uuid.UUID `gorm:"type:uuid"`
	DeletedAt  *time.Time
}

func (workflowConnectorResolutionAgent) TableName() string { return "agents" }

type workflowConnectorResolutionBinding struct {
	ID                uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID    uuid.UUID `gorm:"type:uuid;not null"`
	OwnerAccountID    uuid.UUID `gorm:"type:uuid;not null"`
	ConnectionID      uuid.UUID `gorm:"type:uuid;not null"`
	ConnectorID       string    `gorm:"not null"`
	TargetType        string    `gorm:"not null"`
	TargetID          string    `gorm:"not null"`
	BindingInstanceID string    `gorm:"not null"`
	Status            string    `gorm:"not null"`

	DisabledAt        *time.Time
}

func (workflowConnectorResolutionBinding) TableName() string { return "connector_target_bindings" }

type workflowConnectorResolutionConnection struct {
	ID                 uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrganizationID     uuid.UUID `gorm:"type:uuid;not null"`
	OwnerAccountID     uuid.UUID `gorm:"type:uuid;not null"`
	ConnectorID        string    `gorm:"not null"`
	State              string    `gorm:"not null"`
	UsableOperationIDs string    `gorm:"type:text;not null"`
	DeletedAt          *time.Time
}

func (workflowConnectorResolutionConnection) TableName() string {
	return "connector_connections"
}

type workflowConnectorResolutionFixture struct {
	db             *gorm.DB
	service        *WorkflowService
	organizationID uuid.UUID
	ownerID        uuid.UUID
	workspaceID    uuid.UUID
	agentID        uuid.UUID
	workflowID     uuid.UUID
	runID          uuid.UUID
	nodeID         string
	nodeLogID      uuid.UUID
	pauseID        uuid.UUID
	reasonID       uuid.UUID
	bindingID      uuid.UUID
	connectionID   uuid.UUID
	receiptID      uuid.UUID
	decisionID     uuid.UUID
	operationID    connectorcore.Identifier
	connectorID    connectorcore.Identifier
	pending        map[string]interface{}
}

func newWorkflowConnectorResolutionFixture(t *testing.T) *workflowConnectorResolutionFixture {
	t.Helper()
	dsn := "file:" + strings.ReplaceAll(t.Name(), "/", "_") + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		if strings.Contains(err.Error(), "requires cgo") {
			t.Skipf("sqlite driver unavailable without cgo: %v", err)
		}
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&Workflow{}, &WorkflowRunLog{}, &WorkflowNodeRuntimeLog{},
		&workflowConnectorResolutionWorkspace{}, &workflowConnectorResolutionAgent{},
		&workflowConnectorResolutionBinding{}, &workflowConnectorResolutionConnection{},
		&workflowpause.RunPause{}, &workflowpause.RunPauseReason{},
		&workflowpause.RunEvent{}, &workflowpause.RuntimeOutbox{},
	); err != nil {
		t.Fatalf("migrate Connector resolution tables: %v", err)
	}
	if err := db.Exec("ALTER TABLE workflows ADD COLUMN deleted_at datetime").Error; err != nil {
		t.Fatalf("add Workflow soft-delete column: %v", err)
	}

	fixture := &workflowConnectorResolutionFixture{
		db: db, organizationID: uuid.New(), ownerID: uuid.New(), workspaceID: uuid.New(),
		agentID: uuid.New(), workflowID: uuid.New(), runID: uuid.New(), nodeID: "connector-node",
		nodeLogID: uuid.New(), pauseID: uuid.New(), reasonID: uuid.New(), bindingID: uuid.New(),
		connectionID: uuid.New(), receiptID: uuid.New(), decisionID: uuid.New(),
		operationID: "github.issue.create", connectorID: "github",
	}
	fixture.service = &WorkflowService{workflowDB: db}
	confirmation := connectorexecution.RuntimeConfirmation{
		RouteReceipt: connectorexecution.RouteClaimReceipt(fixture.receiptID.String()),
		ConnectorID:  fixture.connectorID, ConnectorName: "GitHub",
		OperationID: fixture.operationID, ActionName: "Create issue",
		Target: connectorcore.TargetRef{
			Type: connectorcore.TargetTypeWorkflow, ID: fixture.workflowID.String(),
			BindingInstanceID: fixture.nodeID,
		},
		Consequence: connectorexecution.RuntimeConsequence{ExternalEffect: "write", Description: "Create an issue"},
		Risk:        connectorexecution.RiskHigh,
	}
	fixture.pending = map[string]interface{}{
		"state": string(connectorexecution.ExecutionStateRequiresConfirmation), "confirmation": confirmation,
	}
	pendingJSON := mustWorkflowConnectorResolutionJSON(t, fixture.pending)
	state := workflowpause.State{
		Version: workflowpause.StateVersion, WorkflowRunID: fixture.runID.String(), WorkflowID: fixture.workflowID.String(),
		AppID: fixture.agentID.String(), TenantID: fixture.workspaceID.String(), RunType: "WORKFLOW", TriggeredFrom: "debugging",
		Request: workflowpause.RequestState{Inputs: map[string]interface{}{"query": "safe"}, ResponseMode: "streaming"},
		ExecutorState: workflowpause.ExecutorState{
			PausedNodeID: fixture.nodeID, PausedNodeIDs: []string{fixture.nodeID}, NodeQueue: []string{"existing-node"},
			CompletedNodes: map[string]bool{}, FailedNodes: map[string]string{},
			ExecutionOutputs: map[string]map[string]interface{}{fixture.nodeID: fixture.pending},
			AllNodeOutputs:   map[string]interface{}{"prior_answer": "preserved"}, NodeIndex: 2,
		},
		VariablePool: workflowpause.VariablePoolSnapshot{
			Variables:  map[string]map[string]interface{}{fixture.nodeID: fixture.pending},
			UserInputs: map[string]interface{}{},
		},
	}
	stateJSON := mustWorkflowConnectorResolutionJSON(t, state)
	now := time.Now()
	workflow := Workflow{
		ID: fixture.workflowID.String(), TenantID: fixture.workspaceID.String(), AppID: fixture.agentID.String(),
		AgentID: fixture.agentID.String(), Type: dto.WorkflowTypeWorkflow, Version: "draft", Graph: `{}`, Features: `{}`,
		CreatedBy: fixture.ownerID.String(), EnvironmentVariables: `[]`, ConversationVariables: `[]`,
	}
	run := WorkflowRunLog{
		ID: fixture.runID.String(), TenantID: fixture.workspaceID.String(), AgentID: fixture.agentID.String(),
		WorkflowID: fixture.workflowID.String(), Type: dto.WorkflowTypeWorkflow, TriggeredFrom: "debugging", Version: "draft",
		Status: dto.WorkflowRunStatusPaused, Outputs: &pendingJSON, CreatedByRole: CreatedByRoleAccount,
		CreatedBy: fixture.ownerID.String(), RuntimeProtocolVersion: workflowRuntimeProtocolVersionV2,
		ExecutionGeneration: 3,
	}
	runID := fixture.runID.String()
	node := WorkflowNodeRuntimeLog{
		ID: fixture.nodeLogID.String(), TenantID: fixture.workspaceID.String(), AgentID: fixture.agentID.String(),
		WorkflowID: fixture.workflowID.String(), TriggeredFrom: "workflow-run", WorkflowRunID: &runID,
		NodeID: fixture.nodeID, NodeType: "tools", Title: "Create issue", Status: string(dto.NodeStatusPaused),
		Outputs: &pendingJSON, CreatedByRole: string(CreatedByRoleAccount), CreatedBy: fixture.ownerID.String(),
	}
	usableJSON := mustWorkflowConnectorResolutionJSON(t, []string{fixture.operationID.String()})
	rows := []interface{}{
		&workflowConnectorResolutionWorkspace{ID: fixture.workspaceID, OrganizationID: fixture.organizationID, Status: "normal"},
		&workflowConnectorResolutionAgent{ID: fixture.agentID, TenantID: fixture.workspaceID},
		&workflow,
		&run,
		&node,
		&workflowConnectorResolutionConnection{
			ID: fixture.connectionID, OrganizationID: fixture.organizationID, OwnerAccountID: fixture.ownerID,
			ConnectorID: fixture.connectorID.String(), State: string(connectorcore.ReadinessReady), UsableOperationIDs: usableJSON,
		},
		&workflowConnectorResolutionBinding{
			ID: fixture.bindingID, OrganizationID: fixture.organizationID, OwnerAccountID: fixture.ownerID,
			ConnectionID: fixture.connectionID, ConnectorID: fixture.connectorID.String(),
			TargetType: string(connectorcore.TargetTypeWorkflow), TargetID: fixture.workflowID.String(),
			BindingInstanceID: fixture.nodeID, Status: string(connectorcore.BindingStatusActive),

		},
		&workflowpause.RunPause{
			ID: fixture.pauseID.String(), TenantID: fixture.workspaceID.String(), AppID: fixture.agentID.String(),
			WorkflowRunID: fixture.runID.String(), NodeID: fixture.nodeID,
			Reason: workflowpause.ReasonTypeConnectorConfirmationRequired, StateJSON: stateJSON,
			CreatedAt: now, Generation: 1, Status: workflowpause.RunPauseStatusPaused, Revision: 4,
		},
		&workflowpause.RunPauseReason{
			ID: fixture.reasonID.String(), PauseID: fixture.pauseID.String(),
			Type: workflowpause.ReasonTypeConnectorConfirmationRequired, NodeID: fixture.nodeID,
			Status: workflowpause.RunPauseReasonStatusPending, CreatedAt: now,
		},
	}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create Connector resolution fixture %T: %v", row, err)
		}
	}
	if err := db.Model(&WorkflowRunLog{}).Where("id = ?", run.ID).
		Updates(map[string]interface{}{"active_execution_id": nil, "execution_lease_expires_at": nil}).Error; err != nil {
		t.Fatalf("release paused run owner: %v", err)
	}
	return fixture
}

func (fixture *workflowConnectorResolutionFixture) command(decision connectorexecution.ConfirmationDecision) connectorconfirmation.Command {
	return connectorconfirmation.Command{
		OrganizationID: fixture.organizationID, OwnerAccountID: fixture.ownerID,
		RouteReceipt: connectorexecution.RouteClaimReceipt(fixture.receiptID.String()), Decision: decision,
	}
}

func (fixture *workflowConnectorResolutionFixture) result(state connectorconfirmation.State) connectorconfirmation.Result {
	result := connectorconfirmation.Result{
		State: state, Origin: connectorexecution.ConfirmationOrigin{
			Surface: connectorcore.TargetTypeWorkflow, WorkflowID: fixture.workflowID.String(),
			RunID: fixture.runID.String(), NodeID: fixture.nodeID,
		},
		RequestIdentity: connectorconfirmation.ConfirmationRequestIdentity{
			OrganizationID: fixture.organizationID, OwnerAccountID: fixture.ownerID,
			ActingPrincipalID: fixture.ownerID, BindingID: fixture.bindingID,
			ConnectionID: fixture.connectionID, ConnectorID: fixture.connectorID,
			Target: connectorcore.TargetRef{
				Type: connectorcore.TargetTypeWorkflow, ID: fixture.workflowID.String(), BindingInstanceID: fixture.nodeID,
			},
			OperationID: fixture.operationID, LogicalOperationID: uuid.NewString(),
			CorrelationID: uuid.NewString(), Interactive: true,
		},
	}
	switch state {
	case connectorconfirmation.StateCompleted, connectorconfirmation.StateCancelled:
		result.DecisionID = fixture.decisionID
	case connectorconfirmation.StateFailed:
		result.ErrorCode = connectorcore.ErrorCodeProviderUnavailable
	case connectorconfirmation.StateUnknown:
		result.ErrorCode = connectorcore.ErrorCodeOutcomeUnknown
	}
	return result
}

func TestWorkflowConnectorConfirmationCompletedDurablyResumesRemainingGraph(t *testing.T) {
	fixture := newWorkflowConnectorResolutionFixture(t)
	result := fixture.result(connectorconfirmation.StateCompleted)
	if err := fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed), result,
	); err != nil {
		t.Fatalf("RecordConnectorConfirmationResolution() error = %v", err)
	}

	var node WorkflowNodeRuntimeLog
	if err := fixture.db.First(&node, "id = ?", fixture.nodeLogID).Error; err != nil {
		t.Fatal(err)
	}
	terminal := map[string]interface{}{"state": "completed", "decision_id": fixture.decisionID.String()}
	assertWorkflowConnectorResolutionMap(t, node.GetOutputsDict, terminal)
	if node.Status != string(dto.NodeStatusSucceeded) || node.FinishedAt == nil {
		t.Fatalf("node terminal = status %q finished %v", node.Status, node.FinishedAt)
	}

	var run WorkflowRunLog
	if err := fixture.db.First(&run, "id = ?", fixture.runID).Error; err != nil {
		t.Fatal(err)
	}
	if run.Status != dto.WorkflowRunStatusPaused || run.FinishedAt != nil {
		t.Fatalf("run projection = status %q finished %v, want paused and resumable", run.Status, run.FinishedAt)
	}
	assertWorkflowConnectorResolutionMapValue(t, run.GetOutputsDict(), terminal)

	var pauseRecord workflowpause.RunPause
	if err := fixture.db.First(&pauseRecord, "id = ?", fixture.pauseID).Error; err != nil {
		t.Fatal(err)
	}
	if pauseRecord.Status != workflowpause.RunPauseStatusResumeReady || pauseRecord.ResumedAt != nil {
		t.Fatalf("pause = status %q resumed %v, want resume_ready", pauseRecord.Status, pauseRecord.ResumedAt)
	}
	var state workflowpause.State
	if err := json.Unmarshal([]byte(pauseRecord.StateJSON), &state); err != nil {
		t.Fatal(err)
	}
	if !state.ExecutorState.CompletedNodes[fixture.nodeID] {
		t.Fatal("resolved Connector node was not marked completed in pause snapshot")
	}
	assertWorkflowConnectorResolutionMapValue(t, state.ExecutorState.ExecutionOutputs[fixture.nodeID], terminal)
	assertWorkflowConnectorResolutionMapValue(t, state.ExecutorState.AllNodeOutputs, map[string]interface{}{"prior_answer": "preserved"})
	assertWorkflowConnectorResolutionMapValue(t, state.VariablePool.Variables[fixture.nodeID], terminal)

	var reason workflowpause.RunPauseReason
	if err := fixture.db.First(&reason, "id = ?", fixture.reasonID).Error; err != nil {
		t.Fatal(err)
	}
	if reason.Status != workflowpause.RunPauseReasonStatusCompleted || reason.CompletedAt == nil {
		t.Fatalf("pause reason = status %q completed %v", reason.Status, reason.CompletedAt)
	}
	var outboxes []workflowpause.RuntimeOutbox
	if err := fixture.db.Find(&outboxes).Error; err != nil || len(outboxes) != 1 {
		t.Fatalf("resume outboxes = %d error = %v, want 1", len(outboxes), err)
	}
	var payload workflowpause.RuntimeOutboxPayload
	if err := json.Unmarshal([]byte(outboxes[0].PayloadJSON), &payload); err != nil {
		t.Fatal(err)
	}
	if payload.WorkflowRunID != fixture.runID.String() || payload.InteractionType != workflowpause.ReasonTypeConnectorConfirmationRequired {
		t.Fatalf("outbox payload = %#v", payload)
	}
	var events []workflowpause.RunEvent
	if err := fixture.db.Order("sequence ASC").Find(&events).Error; err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventType != workflowpause.EventConnectorConfirmationResolved {
		t.Fatalf("events = %#v, want one Connector resolution", events)
	}

	if err := fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed), result,
	); err != nil {
		t.Fatalf("exact duplicate error = %v", err)
	}
	var eventCount, outboxCount int64
	fixture.db.Model(&workflowpause.RunEvent{}).Count(&eventCount)
	fixture.db.Model(&workflowpause.RuntimeOutbox{}).Count(&outboxCount)
	if eventCount != 1 || outboxCount != 1 {
		t.Fatalf("duplicate counts = events %d outboxes %d, want 1/1", eventCount, outboxCount)
	}
}

func TestWorkflowConnectorConfirmationTerminalStatesFailClosed(t *testing.T) {
	tests := []struct {
		name       string
		state      connectorconfirmation.State
		decision   connectorexecution.ConfirmationDecision
		nodeStatus string
		runStatus  dto.WorkflowRunStatus
		errorCode  string
	}{
		{name: "cancelled", state: connectorconfirmation.StateCancelled, decision: connectorexecution.ConfirmationDecisionCancelled, nodeStatus: "stopped", runStatus: dto.WorkflowRunStatusStopped},
		{name: "failed", state: connectorconfirmation.StateFailed, decision: connectorexecution.ConfirmationDecisionConfirmed, nodeStatus: string(dto.NodeStatusFailed), runStatus: dto.WorkflowRunStatusFailed, errorCode: string(connectorcore.ErrorCodeProviderUnavailable)},
		{name: "unknown", state: connectorconfirmation.StateUnknown, decision: connectorexecution.ConfirmationDecisionConfirmed, nodeStatus: string(dto.NodeStatusFailed), runStatus: dto.WorkflowRunStatusFailed, errorCode: string(connectorcore.ErrorCodeOutcomeUnknown)},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newWorkflowConnectorResolutionFixture(t)
			if err := fixture.service.RecordConnectorConfirmationResolution(
				context.Background(), fixture.command(testCase.decision), fixture.result(testCase.state),
			); err != nil {
				t.Fatalf("RecordConnectorConfirmationResolution() error = %v", err)
			}
			var node WorkflowNodeRuntimeLog
			var run WorkflowRunLog
			var pauseRecord workflowpause.RunPause
			fixture.db.First(&node, "id = ?", fixture.nodeLogID)
			fixture.db.First(&run, "id = ?", fixture.runID)
			fixture.db.First(&pauseRecord, "id = ?", fixture.pauseID)
			if node.Status != testCase.nodeStatus || run.Status != testCase.runStatus || pauseRecord.Status != workflowpause.RunPauseStatusClosed {
				t.Fatalf("terminal projection = node %q run %q pause %q", node.Status, run.Status, pauseRecord.Status)
			}
			if testCase.errorCode != "" && (node.Error == nil || *node.Error != testCase.errorCode || run.Error == nil || *run.Error != testCase.errorCode) {
				t.Fatalf("terminal errors = node %v run %v, want %q", node.Error, run.Error, testCase.errorCode)
			}
			var outboxCount int64
			fixture.db.Model(&workflowpause.RuntimeOutbox{}).Count(&outboxCount)
			if outboxCount != 0 {
				t.Fatalf("terminal %s created %d resume outboxes", testCase.state, outboxCount)
			}
		})
	}
}

func TestWorkflowConnectorConfirmationProjectsExistingTerminalAfterBindingDetach(t *testing.T) {
	fixture := newWorkflowConnectorResolutionFixture(t)
	if err := fixture.db.Delete(&workflowConnectorResolutionBinding{}, "id = ?", fixture.bindingID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Delete(&workflowConnectorResolutionConnection{}, "id = ?", fixture.connectionID).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed),
		fixture.result(connectorconfirmation.StateCompleted),
	); err != nil {
		t.Fatalf("post-detach terminal projection error = %v", err)
	}
	var node WorkflowNodeRuntimeLog
	if err := fixture.db.First(&node, "id = ?", fixture.nodeLogID).Error; err != nil {
		t.Fatal(err)
	}
	if node.Status != string(dto.NodeStatusSucceeded) {
		t.Fatalf("post-detach node status = %q, want succeeded", node.Status)
	}
}

func TestWorkflowConnectorConfirmationRejectsCrossOrganizationProjection(t *testing.T) {
	fixture := newWorkflowConnectorResolutionFixture(t)
	if err := fixture.db.Model(&workflowConnectorResolutionWorkspace{}).
		Where("id = ?", fixture.workspaceID).
		Update("organization_id", uuid.New()).Error; err != nil {
		t.Fatal(err)
	}

	err := fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed),
		fixture.result(connectorconfirmation.StateCompleted),
	)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeAccessDenied) {
		t.Fatalf("cross-organization projection error = %v, want access denied", err)
	}
	assertWorkflowConnectorResolutionStillPending(t, fixture)
}

func TestWorkflowConnectorConfirmationRejectsWrongPublishedTarget(t *testing.T) {
	fixture := newWorkflowConnectorResolutionFixture(t)
	publishedTargetID := uuid.New()
	if err := fixture.db.Model(&Workflow{}).
		Where("id = ?", fixture.workflowID).
		Updates(map[string]interface{}{
			"version": "20260902000000", "version_uuid": publishedTargetID,
		}).Error; err != nil {
		t.Fatal(err)
	}
	if err := fixture.db.Model(&WorkflowRunLog{}).
		Where("id = ?", fixture.runID).
		Update("version", "20260902000000").Error; err != nil {
		t.Fatal(err)
	}

	err := fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed),
		fixture.result(connectorconfirmation.StateCompleted),
	)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeAccessDenied) {
		t.Fatalf("wrong published target error = %v, want access denied", err)
	}
	assertWorkflowConnectorResolutionStillPending(t, fixture)
}

func TestWorkflowConnectorConfirmationRejectsInactiveWorkflowLineage(t *testing.T) {
	tests := []struct {
		name       string
		invalidate func(*workflowConnectorResolutionFixture) error
	}{
		{
			name: "deleted workflow",
			invalidate: func(fixture *workflowConnectorResolutionFixture) error {
				return fixture.db.Exec(
					"UPDATE workflows SET deleted_at = ? WHERE id = ?", time.Now(), fixture.workflowID,
				).Error
			},
		},
		{
			name: "deleted agent",
			invalidate: func(fixture *workflowConnectorResolutionFixture) error {
				return fixture.db.Model(&workflowConnectorResolutionAgent{}).
					Where("id = ?", fixture.agentID).
					Update("deleted_at", time.Now()).Error
			},
		},
		{
			name: "archived workspace",
			invalidate: func(fixture *workflowConnectorResolutionFixture) error {
				return fixture.db.Model(&workflowConnectorResolutionWorkspace{}).
					Where("id = ?", fixture.workspaceID).
					Update("status", "archived").Error
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newWorkflowConnectorResolutionFixture(t)
			if err := testCase.invalidate(fixture); err != nil {
				t.Fatal(err)
			}
			err := fixture.service.RecordConnectorConfirmationResolution(
				context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed),
				fixture.result(connectorconfirmation.StateCompleted),
			)
			if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeAccessDenied) {
				t.Fatalf("inactive %s lineage error = %v, want access denied", testCase.name, err)
			}
			assertWorkflowConnectorResolutionStillPending(t, fixture)
		})
	}
}

func TestWorkflowConnectorConfirmationRejectsTamperedRequestIdentity(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(*connectorconfirmation.Result)
	}{
		{
			name: "target",
			tamper: func(result *connectorconfirmation.Result) {
				tamperedTargetID := uuid.NewString()
				result.Origin.WorkflowID = tamperedTargetID
				result.RequestIdentity.Target.ID = tamperedTargetID
			},
		},
		{
			name: "connector",
			tamper: func(result *connectorconfirmation.Result) {
				result.RequestIdentity.ConnectorID = connectorcore.Identifier("slack")
			},
		},
		{
			name: "operation",
			tamper: func(result *connectorconfirmation.Result) {
				result.RequestIdentity.OperationID = connectorcore.Identifier("github.issue.delete")
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newWorkflowConnectorResolutionFixture(t)
			result := fixture.result(connectorconfirmation.StateCompleted)
			testCase.tamper(&result)
			err := fixture.service.RecordConnectorConfirmationResolution(
				context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed), result,
			)
			if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeAccessDenied) {
				t.Fatalf("tampered %s identity error = %v, want access denied", testCase.name, err)
			}
			assertWorkflowConnectorResolutionStillPending(t, fixture)
		})
	}
}

func TestWorkflowConnectorConfirmationResolutionRollsBackAtomically(t *testing.T) {
	fixture := newWorkflowConnectorResolutionFixture(t)
	if err := fixture.db.Migrator().DropTable(&workflowpause.RuntimeOutbox{}); err != nil {
		t.Fatal(err)
	}
	err := fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed),
		fixture.result(connectorconfirmation.StateCompleted),
	)
	if err == nil {
		t.Fatal("missing outbox table unexpectedly committed resolution")
	}
	assertWorkflowConnectorResolutionStillPending(t, fixture)
}

func TestWorkflowConnectorConfirmationRejectsUnsafeOrConflictingProjection(t *testing.T) {
	fixture := newWorkflowConnectorResolutionFixture(t)
	unsafe := fixture.result(connectorconfirmation.StateCompleted)
	unsafe.ExecutionResult = &connectorexecution.ExecutionResult{Value: map[string]interface{}{"secret": "must-not-persist"}}
	err := fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed), unsafe,
	)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeInvalidInput) {
		t.Fatalf("unsafe ExecutionResult error = %v, want invalid input", err)
	}
	assertWorkflowConnectorResolutionStillPending(t, fixture)

	completed := fixture.result(connectorconfirmation.StateCompleted)
	if err := fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionConfirmed), completed,
	); err != nil {
		t.Fatal(err)
	}
	conflict := fixture.result(connectorconfirmation.StateCancelled)
	err = fixture.service.RecordConnectorConfirmationResolution(
		context.Background(), fixture.command(connectorexecution.ConfirmationDecisionCancelled), conflict,
	)
	if !connectorcore.IsErrorCode(err, connectorcore.ErrorCodeConflict) {
		t.Fatalf("conflicting terminal error = %v, want conflict", err)
	}
}

func mustWorkflowConnectorResolutionJSON(t *testing.T, value interface{}) string {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func assertWorkflowConnectorResolutionMap(
	t *testing.T,
	load func() (map[string]interface{}, error),
	want map[string]interface{},
) {
	t.Helper()
	got, err := load()
	if err != nil {
		t.Fatal(err)
	}
	assertWorkflowConnectorResolutionMapValue(t, got, want)
}

func assertWorkflowConnectorResolutionMapValue(t *testing.T, got, want map[string]interface{}) {
	t.Helper()
	gotJSON := mustWorkflowConnectorResolutionJSON(t, got)
	wantJSON := mustWorkflowConnectorResolutionJSON(t, want)
	if gotJSON != wantJSON {
		t.Fatalf("map = %s, want %s", gotJSON, wantJSON)
	}
}

func assertWorkflowConnectorResolutionStillPending(t *testing.T, fixture *workflowConnectorResolutionFixture) {
	t.Helper()
	var node WorkflowNodeRuntimeLog
	var run WorkflowRunLog
	var pauseRecord workflowpause.RunPause
	var reason workflowpause.RunPauseReason
	fixture.db.First(&node, "id = ?", fixture.nodeLogID)
	fixture.db.First(&run, "id = ?", fixture.runID)
	fixture.db.First(&pauseRecord, "id = ?", fixture.pauseID)
	fixture.db.First(&reason, "id = ?", fixture.reasonID)
	if node.Status != string(dto.NodeStatusPaused) || run.Status != dto.WorkflowRunStatusPaused ||
		pauseRecord.Status != workflowpause.RunPauseStatusPaused || reason.Status != workflowpause.RunPauseReasonStatusPending {
		t.Fatalf("rollback projection = node %q run %q pause %q reason %q", node.Status, run.Status, pauseRecord.Status, reason.Status)
	}
}
