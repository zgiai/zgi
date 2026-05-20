package workflow

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/pkg/logger"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// WorkflowType represents workflow type enum

type CreatedByRole string

const (
	CreatedByRoleAccount CreatedByRole = "account"
	CreatedByRoleEndUser CreatedByRole = "end_user"
)

//

const (
	CreatedFromServiceAPI   dto.WorkflowAppLogCreatedFrom = "service-api"
	CreatedFromWebApp       dto.WorkflowAppLogCreatedFrom = "web-app"
	CreatedFromInstalledApp dto.WorkflowAppLogCreatedFrom = "installed-app"
)

// Workflow represents workflow model
type Workflow struct {
	ID                    string           `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	TenantID              string           `gorm:"type:uuid;not null;index:idx_workflow_version" json:"tenant_id"` // Legacy tenant_id column; stores workspace scope in workflow domain.
	AppID                 string           `gorm:"type:uuid;not null;index:idx_workflow_version" json:"app_id"`
	AgentID               string           `gorm:"type:uuid;not null" json:"agent_id"`
	Type                  dto.WorkflowType `gorm:"type:varchar(255);not null" json:"type"`
	Version               string           `gorm:"type:varchar(255);not null;index:idx_workflow_version" json:"version"`
	VersionUUID           uuid.NullUUID    `gorm:"column:version_uuid;type:uuid" json:"version_uuid,omitempty"`
	Graph                 string           `gorm:"type:text" json:"graph"`
	Features              string           `gorm:"type:text" json:"features"`
	CreatedBy             string           `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt             time.Time        `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedBy             *string          `gorm:"type:uuid" json:"updated_by"`
	UpdatedAt             time.Time        `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
	EnvironmentVariables  string           `gorm:"type:text;not null;default:'[]'" json:"environment_variables"`
	ConversationVariables string           `gorm:"type:text;not null;default:'[]'" json:"conversation_variables"`
	Internal              bool             `gorm:"type:boolean;not null;default:false" json:"internal"`
}

// BeforeCreate GORM hook
func (w *Workflow) BeforeCreate(tx *gorm.DB) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}
	if w.UpdatedAt.IsZero() {
		w.UpdatedAt = time.Now()
	}
	return nil
}

// BeforeUpdate GORM hook
func (w *Workflow) BeforeUpdate(tx *gorm.DB) error {
	w.UpdatedAt = time.Now()
	return nil
}

// TableName specifies table name
func (Workflow) TableName() string {
	return "workflows"
}

// GetGraphDict returns graph as map
func (w *Workflow) GetGraphDict() map[string]interface{} {
	if w.Graph == "" {
		return make(map[string]interface{})
	}
	var graph map[string]interface{}
	json.Unmarshal([]byte(w.Graph), &graph)
	return graph
}

// GetFeaturesDict returns features as map
func (w *Workflow) GetFeaturesDict() map[string]interface{} {
	if w.Features == "" {
		return make(map[string]interface{})
	}
	var features map[string]interface{}
	json.Unmarshal([]byte(w.Features), &features)
	return features
}

// GetEnvironmentVariablesDict returns environment variables as array
func (w *Workflow) GetEnvironmentVariablesDict() []interface{} {
	if w.EnvironmentVariables == "" || w.EnvironmentVariables == "{}" {
		return make([]interface{}, 0)
	}
	var envVars []interface{}
	if err := json.Unmarshal([]byte(w.EnvironmentVariables), &envVars); err != nil {
		// If unmarshal fails, return empty array
		return make([]interface{}, 0)
	}
	return envVars
}

// GetConversationVariablesDict returns conversation variables as array
// Requirement 5.2: JSON parsing failures are logged and return empty array
func (w *Workflow) GetConversationVariablesDict() []interface{} {
	if w.ConversationVariables == "" || w.ConversationVariables == "{}" {
		return make([]interface{}, 0)
	}
	var convVars []interface{}
	if err := json.Unmarshal([]byte(w.ConversationVariables), &convVars); err != nil {
		// Requirement 5.2: Log warning and return empty array on parse failure
		logger.Warn("failed to parse workflow conversation variables json",
			zap.String("workflow_id", w.ID),
			zap.Error(err),
		)
		return make([]interface{}, 0)
	}
	return convVars
}

// WorkflowRun represents workflow run model
type WorkflowRun struct {
	ID              string                `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	TenantID        string                `gorm:"type:uuid;not null;index:idx_workflow_run_triggered_from" json:"tenant_id"` // Legacy tenant_id column; stores workspace scope in workflow domain.
	AppID           string                `gorm:"type:uuid;not null;index:idx_workflow_run_triggered_from" json:"app_id"`
	SequenceNumber  int                   `gorm:"not null;index:idx_workflow_run_tenant_app_sequence" json:"sequence_number"`
	WorkflowID      string                `gorm:"type:uuid;not null" json:"workflow_id"`
	Type            dto.WorkflowType      `gorm:"type:varchar(255);not null" json:"type"`
	TriggeredFrom   string                `gorm:"type:varchar(255);not null;index:idx_workflow_run_triggered_from" json:"triggered_from"`
	Version         string                `gorm:"type:varchar(255);not null" json:"version"`
	Graph           *string               `gorm:"type:text" json:"graph"`
	Inputs          *string               `gorm:"type:text" json:"inputs"`
	Status          dto.WorkflowRunStatus `gorm:"type:varchar(255);not null" json:"status"`
	Outputs         *string               `gorm:"type:text;default:'{}'" json:"outputs"`
	Error           *string               `gorm:"type:text" json:"error"`
	ElapsedTime     float64               `gorm:"not null;default:0" json:"elapsed_time"`
	TotalTokens     int64                 `gorm:"not null;default:0" json:"total_tokens"`
	TotalSteps      int                   `gorm:"not null;default:0" json:"total_steps"`
	CreatedByRole   CreatedByRole         `gorm:"type:varchar(255);not null" json:"created_by_role"`
	CreatedBy       string                `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt       time.Time             `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	FinishedAt      *time.Time            `json:"finished_at"`
	ExceptionsCount int                   `gorm:"not null;default:0" json:"exceptions_count"`
}

// BeforeCreate GORM hook
func (wr *WorkflowRun) BeforeCreate(tx *gorm.DB) error {
	if wr.ID == "" {
		wr.ID = uuid.New().String()
	}
	if wr.CreatedAt.IsZero() {
		wr.CreatedAt = time.Now()
	}
	return nil
}

// TableName specifies table name
func (WorkflowRun) TableName() string {
	return "workflow_runs"
}

// GetGraphDict returns graph as map
func (wr *WorkflowRun) GetGraphDict() map[string]interface{} {
	if wr.Graph == nil || *wr.Graph == "" {
		return make(map[string]interface{})
	}
	var graph map[string]interface{}
	json.Unmarshal([]byte(*wr.Graph), &graph)
	return graph
}

// GetInputsDict returns inputs as map
func (wr *WorkflowRun) GetInputsDict() map[string]interface{} {
	if wr.Inputs == nil || *wr.Inputs == "" {
		return make(map[string]interface{})
	}
	var inputs map[string]interface{}
	json.Unmarshal([]byte(*wr.Inputs), &inputs)
	return inputs
}

// GetOutputsDict returns outputs as map
func (wr *WorkflowRun) GetOutputsDict() map[string]interface{} {
	if wr.Outputs == nil || *wr.Outputs == "" {
		return make(map[string]interface{})
	}
	var outputs map[string]interface{}
	json.Unmarshal([]byte(*wr.Outputs), &outputs)
	return outputs
}

// WorkflowNodeExecution represents workflow node execution model
type WorkflowNodeExecution struct {
	ID                string                                 `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	TenantID          string                                 `gorm:"type:uuid;not null;index:idx_workflow_node_execution_workflow_run" json:"tenant_id"` // Legacy tenant_id column; stores workspace scope in workflow domain.
	AppID             string                                 `gorm:"type:uuid;not null;index:idx_workflow_node_execution_workflow_run" json:"app_id"`
	WorkflowID        string                                 `gorm:"type:uuid;not null;index:idx_workflow_node_execution_workflow_run" json:"workflow_id"`
	TriggeredFrom     dto.WorkflowNodeExecutionTriggeredFrom `gorm:"type:varchar(255);not null;index:idx_workflow_node_execution_workflow_run" json:"triggered_from"`
	WorkflowRunID     *string                                `gorm:"type:uuid;index:idx_workflow_node_execution_workflow_run" json:"workflow_run_id"`
	Index             int                                    `gorm:"not null" json:"index"`
	PredecessorNodeID *string                                `gorm:"type:varchar(255)" json:"predecessor_node_id"`
	NodeExecutionID   *string                                `gorm:"type:varchar(255);index:idx_workflow_node_execution_id" json:"node_execution_id"`
	NodeID            string                                 `gorm:"type:varchar(255);not null;index:idx_workflow_node_execution_node_run" json:"node_id"`
	NodeType          string                                 `gorm:"type:varchar(255);not null" json:"node_type"`
	Title             string                                 `gorm:"type:varchar(255);not null" json:"title"`
	Inputs            *string                                `gorm:"type:text" json:"inputs"`
	ProcessData       *string                                `gorm:"type:text" json:"process_data"`
	Outputs           *string                                `gorm:"type:text" json:"outputs"`
	Status            dto.WorkflowNodeExecutionStatus        `gorm:"type:varchar(255);not null" json:"status"`
	Error             *string                                `gorm:"type:text" json:"error"`
	ElapsedTime       float64                                `gorm:"not null;default:0" json:"elapsed_time"`
	ExecutionMetadata *string                                `gorm:"type:text" json:"execution_metadata"`
	CreatedAt         time.Time                              `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	CreatedByRole     CreatedByRole                          `gorm:"type:varchar(255);not null" json:"created_by_role"`
	CreatedBy         string                                 `gorm:"type:uuid;not null" json:"created_by"`
	FinishedAt        *time.Time                             `json:"finished_at"`
}

// BeforeCreate GORM hook
func (wne *WorkflowNodeExecution) BeforeCreate(tx *gorm.DB) error {
	if wne.ID == "" {
		wne.ID = uuid.New().String()
	}
	if wne.CreatedAt.IsZero() {
		wne.CreatedAt = time.Now()
	}
	return nil
}

// TableName specifies table name
func (WorkflowNodeExecution) TableName() string {
	return "workflow_node_executions"
}

// GetInputsDict returns inputs as map
func (wne *WorkflowNodeExecution) GetInputsDict() map[string]interface{} {
	if wne.Inputs == nil || *wne.Inputs == "" {
		return make(map[string]interface{})
	}
	var inputs map[string]interface{}
	json.Unmarshal([]byte(*wne.Inputs), &inputs)
	return inputs
}

// GetOutputsDict returns outputs as map
func (wne *WorkflowNodeExecution) GetOutputsDict() map[string]interface{} {
	if wne.Outputs == nil || *wne.Outputs == "" {
		return make(map[string]interface{})
	}
	var outputs map[string]interface{}
	json.Unmarshal([]byte(*wne.Outputs), &outputs)
	return outputs
}

// GetProcessDataDict returns process data as map
func (wne *WorkflowNodeExecution) GetProcessDataDict() map[string]interface{} {
	if wne.ProcessData == nil || *wne.ProcessData == "" {
		return make(map[string]interface{})
	}
	var processData map[string]interface{}
	json.Unmarshal([]byte(*wne.ProcessData), &processData)
	return processData
}

// GetExecutionMetadataDict returns execution metadata as map
func (wne *WorkflowNodeExecution) GetExecutionMetadataDict() map[string]interface{} {
	if wne.ExecutionMetadata == nil || *wne.ExecutionMetadata == "" {
		return make(map[string]interface{})
	}
	var metadata map[string]interface{}
	json.Unmarshal([]byte(*wne.ExecutionMetadata), &metadata)
	return metadata
}

// WorkflowAppLog represents workflow app log model
type WorkflowAppLog struct {
	ID            string                        `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	TenantID      string                        `gorm:"type:uuid;not null;index:idx_workflow_app_log_app" json:"tenant_id"` // Legacy tenant_id column; stores workspace scope in workflow domain.
	AppID         string                        `gorm:"type:uuid;not null;index:idx_workflow_app_log_app" json:"app_id"`
	WorkflowID    string                        `gorm:"type:uuid;not null" json:"workflow_id"`
	WorkflowRunID string                        `gorm:"type:uuid;not null" json:"workflow_run_id"`
	CreatedFrom   dto.WorkflowAppLogCreatedFrom `gorm:"type:varchar(255);not null" json:"created_from"`
	CreatedByRole CreatedByRole                 `gorm:"type:varchar(255);not null" json:"created_by_role"`
	CreatedBy     string                        `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt     time.Time                     `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
}

// BeforeCreate GORM hook
func (wal *WorkflowAppLog) BeforeCreate(tx *gorm.DB) error {
	if wal.ID == "" {
		wal.ID = uuid.New().String()
	}
	if wal.CreatedAt.IsZero() {
		wal.CreatedAt = time.Now()
	}
	return nil
}

// TableName specifies table name
func (WorkflowAppLog) TableName() string {
	return "workflow_app_logs"
}

// ConversationVariable represents conversation variable model
type ConversationVariable struct {
	ID             string    `gorm:"type:uuid;primary_key" json:"id"`
	ConversationID string    `gorm:"type:uuid;not null;primary_key" json:"conversation_id"`
	AppID          string    `gorm:"type:uuid;not null;index:idx_conversation_variables_app_id" json:"app_id"`
	Data           string    `gorm:"type:text;not null" json:"data"`
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP;index:idx_conversation_variables_created_at" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

// TableName specifies table name
func (ConversationVariable) TableName() string {
	return "workflow_conversation_variables"
}

// WorkflowRunLog represents workflow run log model - for workflow execution logging
type WorkflowRunLog struct {
	ID              string                `gorm:"type:uuid;primary_key;default:uuid_generate_v4()" json:"id"`
	TenantID        string                `gorm:"type:uuid;not null;index:idx_workflow_run_logs_triggered_from" json:"tenant_id"` // Legacy tenant_id column; stores workspace scope in workflow domain.
	AgentID         string                `gorm:"type:uuid;not null;index:idx_workflow_run_logs_triggered_from" json:"agent_id"`
	SequenceNumber  int                   `gorm:"not null;index:idx_workflow_run_logs_tenant_agent_sequence" json:"sequence_number"`
	WorkflowID      string                `gorm:"type:uuid;not null" json:"workflow_id"`
	Type            dto.WorkflowType      `gorm:"type:varchar(255);not null" json:"type"`
	TriggeredFrom   string                `gorm:"type:varchar(255);not null;index:idx_workflow_run_logs_triggered_from" json:"triggered_from"`
	Version         string                `gorm:"type:varchar(255);not null" json:"version"`
	WebAppID        *string               `gorm:"type:uuid" json:"web_app_id"` // Track which web application was used
	Graph           *string               `gorm:"type:text" json:"graph"`
	Features        *string               `gorm:"type:text" json:"features"`
	Inputs          *string               `gorm:"type:text" json:"inputs"`
	Status          dto.WorkflowRunStatus `gorm:"type:varchar(255);not null" json:"status"`
	Outputs         *string               `gorm:"type:text;default:'{}'" json:"outputs"`
	Error           *string               `gorm:"type:text" json:"error"`
	ElapsedTime     float64               `gorm:"not null;default:0" json:"elapsed_time"`
	TotalTokens     int64                 `gorm:"not null;default:0" json:"total_tokens"`
	TotalSteps      int                   `gorm:"not null;default:0" json:"total_steps"`
	CreatedByRole   CreatedByRole         `gorm:"type:varchar(255);not null" json:"created_by_role"`
	CreatedBy       string                `gorm:"type:uuid;not null" json:"created_by"`
	CreatedAt       time.Time             `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	FinishedAt      *time.Time            `json:"finished_at"`
	DeletedAt       *time.Time            `json:"deleted_at"`
	DeletedBy       *string               `gorm:"type:uuid" json:"deleted_by"`
	ExceptionsCount int                   `gorm:"not null;default:0" json:"exceptions_count"`
}

// BeforeCreate GORM hook
func (wrl *WorkflowRunLog) BeforeCreate(tx *gorm.DB) error {
	if wrl.ID == "" {
		wrl.ID = uuid.New().String()
	}
	if wrl.CreatedAt.IsZero() {
		wrl.CreatedAt = time.Now()
	}
	return nil
}

// TableName specifies table name
func (WorkflowRunLog) TableName() string {
	return "workflow_run_logs"
}

// WorkflowNodeRuntimeLog represents the workflow_node_runtime_logs table
type WorkflowNodeRuntimeLog struct {
	ID                       string     `json:"id" gorm:"type:uuid;primary_key;default:uuid_generate_v4()"`
	TenantID                 string     `json:"tenant_id" gorm:"type:uuid;not null;index:workflow_node_runtime_logs_workflow_run_idx,priority:1;index:workflow_node_runtime_logs_node_run_idx,priority:1;index:workflow_node_runtime_logs_id_idx,priority:1"` // Legacy tenant_id column; stores workspace scope in workflow domain.
	AgentID                  string     `json:"agent_id" gorm:"type:uuid;not null;index:workflow_node_runtime_logs_workflow_run_idx,priority:2;index:workflow_node_runtime_logs_node_run_idx,priority:2;index:workflow_node_runtime_logs_id_idx,priority:2"`
	WorkflowID               string     `json:"workflow_id" gorm:"type:uuid;not null;index:workflow_node_runtime_logs_workflow_run_idx,priority:3;index:workflow_node_runtime_logs_node_run_idx,priority:3;index:workflow_node_runtime_logs_id_idx,priority:3"`
	TriggeredFrom            string     `json:"triggered_from" gorm:"type:varchar(255);not null;index:workflow_node_runtime_logs_workflow_run_idx,priority:4;index:workflow_node_runtime_logs_node_run_idx,priority:4;index:workflow_node_runtime_logs_id_idx,priority:4"`
	WorkflowRunID            *string    `json:"workflow_run_id" gorm:"type:uuid;index:workflow_node_runtime_logs_workflow_run_idx,priority:5"`
	WebAppID                 *string    `json:"web_app_id" gorm:"type:uuid"` // Track which web application was used
	Index                    int        `json:"index" gorm:"not null"`
	PredecessorNodeID        *string    `json:"predecessor_node_id" gorm:"type:varchar(255)"`
	NodeExecutionID          *string    `json:"node_execution_id" gorm:"type:varchar(255);index:workflow_node_runtime_logs_id_idx,priority:5"`
	NodeID                   string     `json:"node_id" gorm:"type:varchar(255);not null;index:workflow_node_runtime_logs_node_run_idx,priority:5"`
	NodeType                 string     `json:"node_type" gorm:"type:varchar(255);not null"`
	Title                    string     `json:"title" gorm:"type:varchar(255);not null"`
	Inputs                   *string    `json:"inputs" gorm:"type:text"`
	ProcessData              *string    `json:"process_data" gorm:"type:text"`
	Outputs                  *string    `json:"outputs" gorm:"type:text"`
	Graph                    *string    `json:"graph" gorm:"type:text"`
	Features                 *string    `json:"features" gorm:"type:text"`
	Status                   string     `json:"status" gorm:"type:varchar(255);not null"`
	Error                    *string    `json:"error" gorm:"type:text"`
	ErrorType                *string    `json:"error_type" gorm:"type:varchar(64)"`
	ErrorStack               *string    `json:"error_stack" gorm:"type:text"`
	DiagnosisResult          *string    `json:"diagnosis_result" gorm:"type:text"`
	DiagnosisModel           *string    `json:"diagnosis_model" gorm:"type:varchar(128)"`
	DiagnosisTokens          int        `json:"diagnosis_tokens" gorm:"type:integer;default:0"`
	DiagnosisLatencyMs       int        `json:"diagnosis_latency_ms" gorm:"type:integer;default:0"`
	IsLLMDiagnosed           bool       `json:"is_llm_diagnosed" gorm:"type:boolean;default:false"`
	DiagnosisNodeConfig      *string    `json:"diagnosis_node_config" gorm:"type:jsonb"`
	DiagnosisUpstreamConfig  *string    `json:"diagnosis_upstream_config" gorm:"type:jsonb"`
	DiagnosisInputSnapshot   *string    `json:"diagnosis_input_snapshot" gorm:"type:jsonb"`
	DiagnosisUpstreamOutputs *string    `json:"diagnosis_upstream_outputs" gorm:"type:jsonb"`
	ElapsedTime              float64    `json:"elapsed_time" gorm:"type:double precision;not null;default:0"`
	ExecutionMetadata        *string    `json:"execution_metadata" gorm:"type:text"`
	CreatedAt                time.Time  `json:"created_at" gorm:"type:timestamp;not null;default:CURRENT_TIMESTAMP"`
	CreatedByRole            string     `json:"created_by_role" gorm:"type:varchar(255);not null"`
	CreatedBy                string     `json:"created_by" gorm:"type:uuid;not null"`
	DeletedAt                *time.Time `json:"deleted_at" gorm:"type:timestamp"`
	DeletedBy                *string    `json:"deleted_by" gorm:"type:uuid"`
	FinishedAt               *time.Time `json:"finished_at" gorm:"type:timestamp"`
}

// BeforeCreate hook for WorkflowNodeRuntimeLog
func (w *WorkflowNodeRuntimeLog) BeforeCreate(tx *gorm.DB) error {
	if w.ID == "" {
		w.ID = uuid.New().String()
	}
	if w.CreatedAt.IsZero() {
		w.CreatedAt = time.Now()
	}
	return nil
}

// TableName returns the table name for WorkflowNodeRuntimeLog
func (w *WorkflowNodeRuntimeLog) TableName() string {
	return "workflow_node_runtime_logs"
}

// GetInputsDict parses inputs JSON string to map
func (w *WorkflowNodeRuntimeLog) GetInputsDict() (map[string]interface{}, error) {
	if w.Inputs == nil || *w.Inputs == "" {
		return make(map[string]interface{}), nil
	}

	var inputs map[string]interface{}
	if err := json.Unmarshal([]byte(*w.Inputs), &inputs); err != nil {
		return nil, err
	}
	return inputs, nil
}

// GetProcessDataDict parses process_data JSON string to map
func (w *WorkflowNodeRuntimeLog) GetProcessDataDict() (map[string]interface{}, error) {
	if w.ProcessData == nil || *w.ProcessData == "" {
		return make(map[string]interface{}), nil
	}

	var processData map[string]interface{}
	if err := json.Unmarshal([]byte(*w.ProcessData), &processData); err != nil {
		return nil, err
	}
	return processData, nil
}

// GetOutputsDict parses outputs JSON string to map
func (w *WorkflowNodeRuntimeLog) GetOutputsDict() (map[string]interface{}, error) {
	if w.Outputs == nil || *w.Outputs == "" {
		return make(map[string]interface{}), nil
	}

	var outputs map[string]interface{}
	if err := json.Unmarshal([]byte(*w.Outputs), &outputs); err != nil {
		return nil, err
	}
	return outputs, nil
}

// GetExecutionMetadataDict parses execution_metadata JSON string to map
func (w *WorkflowNodeRuntimeLog) GetExecutionMetadataDict() (map[string]interface{}, error) {
	if w.ExecutionMetadata == nil || *w.ExecutionMetadata == "" {
		return make(map[string]interface{}), nil
	}

	var metadata map[string]interface{}
	if err := json.Unmarshal([]byte(*w.ExecutionMetadata), &metadata); err != nil {
		return nil, err
	}
	return metadata, nil
}

// GetGraphDict returns graph as map
func (wrl *WorkflowRunLog) GetGraphDict() map[string]interface{} {
	if wrl.Graph == nil || *wrl.Graph == "" {
		return make(map[string]interface{})
	}
	var graph map[string]interface{}
	json.Unmarshal([]byte(*wrl.Graph), &graph)
	return graph
}

// GetInputsDict returns inputs as map
func (wrl *WorkflowRunLog) GetInputsDict() map[string]interface{} {
	if wrl.Inputs == nil || *wrl.Inputs == "" {
		return make(map[string]interface{})
	}
	var inputs map[string]interface{}
	json.Unmarshal([]byte(*wrl.Inputs), &inputs)
	return inputs
}

// GetOutputsDict returns outputs as map
func (wrl *WorkflowRunLog) GetOutputsDict() map[string]interface{} {
	if wrl.Outputs == nil || *wrl.Outputs == "" {
		return make(map[string]interface{})
	}
	var outputs map[string]interface{}
	json.Unmarshal([]byte(*wrl.Outputs), &outputs)
	return outputs
}
