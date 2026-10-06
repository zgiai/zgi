package digitalhuman

import (
	"encoding/json"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"gorm.io/datatypes"
)

type ManualInvocationRequest struct {
	RequestID       uuid.UUID  `json:"request_id"`
	Query           string     `json:"query"`
	ConversationID  *uuid.UUID `json:"conversation_id,omitempty"`
	ParentMessageID *uuid.UUID `json:"parent_message_id,omitempty"`
	FileIDs         []string   `json:"file_ids,omitempty"`
	MaxTaskTokens   int        `json:"max_task_tokens,omitempty"`
}

// RegenerateInvocationRequest replays a root user turn through the durable
// DigitalHuman runtime. The original message is replaced only after the
// ChatRuntime root-replacement guard has accepted the request; the durable run
// remains the audit and recovery boundary for the operation.
type RegenerateInvocationRequest struct {
	MessageID        uuid.UUID              `json:"-"`
	ConversationID   *uuid.UUID             `json:"-"`
	Query            *string                `json:"query,omitempty"`
	Surface          string                 `json:"surface,omitempty"`
	RuntimeContext   string                 `json:"runtime_context,omitempty"`
	OperationContext map[string]interface{} `json:"operation_context,omitempty"`
	Model            *string                `json:"model,omitempty"`
	Provider         *string                `json:"provider,omitempty"`
	Parameters       map[string]interface{} `json:"parameters,omitempty"`
	UseMemory        *bool                  `json:"use_memory,omitempty"`
}

type RunListResult struct {
	Items    []*Run `json:"items"`
	Total    int64  `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"page_size"`
	HasMore  bool   `json:"has_more"`
}

type RunCancelResult struct {
	Run *Run `json:"run"`
}

// RunRuntimeLog exposes the same safe, per-step runtime projection used by
// Agent logs while keeping the durable DigitalHuman run as the access boundary.
type RunRuntimeLog struct {
	Detail *dto.AgentRuntimeRunDetail `json:"detail,omitempty"`
	Steps  []dto.AgentRuntimeStep     `json:"steps"`
	// PendingConnectorConfirmation is populated only while the loaded run is
	// durably waiting for a server-authored Connector confirmation that matches
	// the run, message, conversation, and waiting receipt. It carries only the
	// public RuntimeConfirmation presentation fields; the frozen Preview and
	// operation input stay behind the owner-scoped receipt status endpoint.
	PendingConnectorConfirmation *connectorexecution.RuntimeConfirmation `json:"pending_connector_confirmation,omitempty"`
	// PendingWorkflowApproval is the owner-scoped approval form for a generic
	// workflow pause. It is returned only while the linked message is waiting
	// for approval and the workflow explicitly allows inline approval.
	PendingWorkflowApproval *RunWorkflowApproval `json:"pending_workflow_approval,omitempty"`
}

type RunWorkflowApproval struct {
	ApprovalToken string                 `json:"approval_token"`
	ApprovalForm  map[string]interface{} `json:"approval_form"`
}

func publicRun(run *Run) *Run {
	if run == nil {
		return nil
	}
	cloned := *run
	cloned.RunSpec = redactRunJSON(run.RunSpec)
	cloned.ExecutionSnapshot = redactRunJSON(run.ExecutionSnapshot)
	cloned.ExecutionTrace = redactRunJSON(run.ExecutionTrace)
	return &cloned
}

func publicRunList(result *RunListResult) *RunListResult {
	if result == nil {
		return nil
	}
	cloned := *result
	cloned.Items = make([]*Run, 0, len(result.Items))
	for _, run := range result.Items {
		cloned.Items = append(cloned.Items, publicRun(run))
	}
	return &cloned
}

func redactRunJSON(value datatypes.JSON) datatypes.JSON {
	if len(value) == 0 {
		return value
	}
	var decoded interface{}
	if json.Unmarshal(value, &decoded) != nil {
		return datatypes.JSON(`{}`)
	}
	redactRunValue(decoded)
	encoded, err := json.Marshal(decoded)
	if err != nil {
		return datatypes.JSON(`{}`)
	}
	return datatypes.JSON(encoded)
}

func redactRunValue(value interface{}) {
	forbidden := map[string]struct{}{
		"credential_ref": {}, "secret": {}, "password": {}, "api_key": {}, "apikey": {},
		"access_token": {}, "refresh_token": {}, "cookie": {}, "token": {},
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, item := range typed {
			if _, blocked := forbidden[strings.ToLower(strings.TrimSpace(key))]; blocked {
				delete(typed, key)
				continue
			}
			redactRunValue(item)
		}
	case []interface{}:
		for _, item := range typed {
			redactRunValue(item)
		}
	}
}
