package dto

import (
	"encoding/json"
	"time"
)

// WorkflowType represents workflow type enum
type WorkflowType string

const (
	WorkflowTypeWorkflow WorkflowType = "workflow"
	WorkflowTypeChat     WorkflowType = "chat"
)

// WorkflowRunStatus represents workflow run status enum
type WorkflowRunStatus string

const (
	WorkflowRunStatusRunning          WorkflowRunStatus = "running"
	WorkflowRunStatusSucceeded        WorkflowRunStatus = "succeeded"
	WorkflowRunStatusFailed           WorkflowRunStatus = "failed"
	WorkflowRunStatusStopped          WorkflowRunStatus = "stopped"
	WorkflowRunStatusPaused           WorkflowRunStatus = "paused"
	WorkflowRunStatusPartialSucceeded WorkflowRunStatus = "partial-succeeded"
)

// WorkflowNodeExecutionStatus represents node execution status enum
type WorkflowNodeExecutionStatus string

const (
	NodeStatusRunning   WorkflowNodeExecutionStatus = "running"
	NodeStatusSucceeded WorkflowNodeExecutionStatus = "succeeded"
	NodeStatusFailed    WorkflowNodeExecutionStatus = "failed"
	NodeStatusException WorkflowNodeExecutionStatus = "exception"
	NodeStatusRetry     WorkflowNodeExecutionStatus = "retry"
	NodeStatusPaused    WorkflowNodeExecutionStatus = "paused"
)

// WorkflowNodeExecutionTriggeredFrom represents how node execution was triggered
type WorkflowNodeExecutionTriggeredFrom string

const (
	TriggeredFromSingleStep  WorkflowNodeExecutionTriggeredFrom = "single-step"
	TriggeredFromWorkflowRun WorkflowNodeExecutionTriggeredFrom = "workflow-run"
)

// =====================
// Workflow DTOs
// =====================

// WorkflowDetail represents detailed workflow information
type WorkflowDetail struct {
	ID                    string                 `json:"id"`
	TenantID              string                 `json:"tenant_id"`
	AgentID               string                 `json:"agent_id"`
	Type                  WorkflowType           `json:"type"`
	Version               string                 `json:"version"`
	Graph                 map[string]interface{} `json:"graph"`
	Features              map[string]interface{} `json:"features"`
	CreatedBy             string                 `json:"created_by"`
	CreatedAt             time.Time              `json:"created_at"`
	UpdatedBy             *string                `json:"updated_by,omitempty"`
	UpdatedAt             time.Time              `json:"updated_at"`
	EnvironmentVariables  []Variable             `json:"environment_variables"`
	ConversationVariables []Variable             `json:"conversation_variables"`
	UniqueHash            string                 `json:"hash"`
	ToolPublished         bool                   `json:"tool_published"`
	Internal              bool                   `json:"internal"`
}

// Variable represents a workflow variable
type Variable struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Label       string                 `json:"label"`
	Type        string                 `json:"type"`
	Required    bool                   `json:"required"`
	Options     []string               `json:"options,omitempty"`
	Description string                 `json:"description,omitempty"`
	Value       interface{}            `json:"value,omitempty"`
	Selector    []string               `json:"selector,omitempty"`
	ValueType   string                 `json:"value_type"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// SyncDraftWorkflowRequest represents request to sync draft workflow
type SyncDraftWorkflowRequest struct {
	Graph                 map[string]interface{} `json:"graph" validate:"required"`
	Features              map[string]interface{} `json:"features"`
	Hash                  string                 `json:"hash,omitempty"`
	EnvironmentVariables  []Variable             `json:"environment_variables,omitempty"`
	ConversationVariables []Variable             `json:"conversation_variables,omitempty"`
	Type                  WorkflowType           `json:"type"`
	Internal              *bool                  `json:"internal,omitempty"`
}

// SyncDraftWorkflowResponse represents response for sync draft workflow
type SyncDraftWorkflowResponse struct {
	Result    string    `json:"result"`
	Hash      string    `json:"hash"`
	UpdatedAt time.Time `json:"updated_at"`
}

// WorkflowParallelConfigResponse represents workflow parallel configuration
type WorkflowParallelConfigResponse struct {
	ParallelDepthLimit int `json:"parallel_depth_limit"`
}

// WorkflowConfigResponse represents workflow configuration for web app
type WorkflowConfigResponse struct {
	Variables []map[string]interface{} `json:"variables"`
	Features  map[string]interface{}   `json:"features"`
	Config    *AppConfig               `json:"config"`
}

// AppConfig represents app configuration metadata
type AppConfig struct {
	AgentID  string `json:"agent_id"`
	Type     string `json:"type"`
	Icon     string `json:"icon"`
	IconType string `json:"icon_type"`
	IconURL  string `json:"icon_url,omitempty"`
	Title    string `json:"title"`
}

// AgentMetadata is deprecated, use AppConfig instead
type AgentMetadata = AppConfig

// =====================
// Workflow Run DTOs
// =====================

// DraftWorkflowRunRequest represents request to run draft workflow
type DraftWorkflowRunRequest struct {
	Inputs       map[string]interface{} `json:"inputs" validate:"required"`
	UserID       string                 `json:"user_id,omitempty"`
	StreamMode   bool                   `json:"stream_mode,omitempty"`
	ResponseMode string                 `json:"response_mode,omitempty"` // streaming, blocking
	Files        []FileInfo             `json:"files,omitempty"`
}

// AdvancedChatDraftWorkflowRunRequest represents advanced chat workflow run request
type AdvancedChatDraftWorkflowRunRequest struct {
	Query             string                 `json:"query" validate:"required"`
	Inputs            map[string]interface{} `json:"inputs"`
	ResponseMode      string                 `json:"response_mode,omitempty"` // streaming, blocking
	UserID            string                 `json:"user_id,omitempty"`
	ConversationID    string                 `json:"conversation_id,omitempty"`
	HistoryWindowSize *int                   `json:"history_window_size,omitempty"` // deprecated: accepted for compatibility, LLM node conversation_history controls history
	Files             []FileInfo             `json:"files,omitempty"`
}

// FileInfo represents file information in requests
type FileInfo struct {
	Type           string `json:"type"`
	TransferMethod string `json:"transfer_method"`
	URL            string `json:"url,omitempty"`
	UploadFileID   string `json:"upload_file_id,omitempty"`
}

// WorkflowRunResponse represents workflow run response
type WorkflowRunResponse struct {
	TaskID        string `json:"task_id"`
	WorkflowRunID string `json:"workflow_run_id"`
}

// WorkflowTaskStopResponse represents workflow task stop response
type WorkflowTaskStopResponse struct {
	Result string `json:"result"`
}

// =====================
// Node Execution DTOs
// =====================

// DraftWorkflowNodeRunRequest represents request to run draft workflow node
type DraftWorkflowNodeRunRequest struct {
	Inputs map[string]interface{} `json:"inputs" validate:"required"`
}

// WorkflowNodeExecutionDetail represents detailed node execution information
type WorkflowNodeExecutionDetail struct {
	ID                string                             `json:"id"`
	TenantID          string                             `json:"tenant_id"`
	AgentID           string                             `json:"agent_id"`
	WorkflowID        string                             `json:"workflow_id"`
	TriggeredFrom     WorkflowNodeExecutionTriggeredFrom `json:"triggered_from"`
	WorkflowRunID     *string                            `json:"workflow_run_id,omitempty"`
	Index             int                                `json:"index"`
	PredecessorNodeID *string                            `json:"predecessor_node_id,omitempty"`
	NodeExecutionID   *string                            `json:"node_execution_id,omitempty"`
	NodeID            string                             `json:"node_id"`
	NodeType          string                             `json:"node_type"`
	Title             string                             `json:"title"`
	Inputs            map[string]interface{}             `json:"inputs,omitempty"`
	ProcessData       map[string]interface{}             `json:"process_data,omitempty"`
	Outputs           map[string]interface{}             `json:"outputs,omitempty"`
	Status            WorkflowNodeExecutionStatus        `json:"status"`
	Error             *string                            `json:"error,omitempty"`
	ElapsedTime       float64                            `json:"elapsed_time"`
	ExecutionMetadata map[string]interface{}             `json:"execution_metadata,omitempty"`
	CreatedAt         time.Time                          `json:"created_at"`
	CreatedByRole     string                             `json:"created_by_role"`
	CreatedBy         string                             `json:"created_by"`
	FinishedAt        *time.Time                         `json:"finished_at,omitempty"`
}

// =====================
// Workflow Publishing DTOs
// =====================

// PublishWorkflowRequest represents request to publish workflow
type PublishWorkflowRequest struct {
	MarkedName    string `json:"marked_name,omitempty"`
	MarkedComment string `json:"marked_comment,omitempty"`
}

// PublishWorkflowResponse represents response for publish workflow
type PublishWorkflowResponse struct {
	WorkflowID string    `json:"workflow_id"`
	CreatedAt  time.Time `json:"created_at"`
}

// PublishedWorkflowsRequest represents request for published workflows list
type PublishedWorkflowsRequest struct {
	Page  int `form:"page" validate:"min=1"`
	Limit int `form:"limit" validate:"min=1,max=100"`
}

// PublishedWorkflowsResponse represents response for published workflows list
type PublishedWorkflowsResponse struct {
	Data    []WorkflowDetail `json:"data"`
	HasMore bool             `json:"has_more"`
	Limit   int              `json:"limit"`
}

// =====================
// Block Configs DTOs
// =====================

// DefaultBlockConfigsResponse represents response for default block configs
type DefaultBlockConfigsResponse struct {
	Data []BlockConfig `json:"data"`
}

// BlockConfig represents a workflow block configuration
type BlockConfig struct {
	BlockType  string                 `json:"block_type"`
	Config     map[string]interface{} `json:"config"`
	IsListType bool                   `json:"is_list_type"`
}

// DefaultBlockConfigResponse represents response for single block config
type DefaultBlockConfigResponse struct {
	BlockConfig
}

// =====================
// Conversion DTOs
// =====================

// ConvertToWorkflowRequest represents request to convert app to workflow
type ConvertToWorkflowRequest struct {
	// Usually empty body for this operation
}

// ConvertToWorkflowResponse represents response for conversion
type ConvertToWorkflowResponse struct {
	Result string `json:"result"`
}

// =====================
// Workflow Run Detail DTOs
// =====================

// WorkflowRunDetail represents detailed workflow run information
type WorkflowRunDetail struct {
	ID             string                        `json:"id"`
	TenantID       string                        `json:"tenant_id"`
	AgentID        string                        `json:"agent_id"`
	SequenceNumber int                           `json:"sequence_number"`
	WorkflowID     string                        `json:"workflow_id"`
	Type           WorkflowType                  `json:"type"`
	TriggeredFrom  string                        `json:"triggered_from"`
	Version        string                        `json:"version"`
	Graph          map[string]interface{}        `json:"graph,omitempty"`
	Inputs         map[string]interface{}        `json:"inputs,omitempty"`
	Status         WorkflowRunStatus             `json:"status"`
	Outputs        map[string]interface{}        `json:"outputs,omitempty"`
	Error          *string                       `json:"error,omitempty"`
	ElapsedTime    float64                       `json:"elapsed_time"`
	TotalTokens    int64                         `json:"total_tokens"`
	TotalSteps     int                           `json:"total_steps"`
	CreatedByRole  string                        `json:"created_by_role"`
	CreatedBy      string                        `json:"created_by"`
	CreatedAt      time.Time                     `json:"created_at"`
	FinishedAt     *time.Time                    `json:"finished_at,omitempty"`
	NodeExecutions []WorkflowNodeExecutionDetail `json:"node_executions,omitempty"`
}

// =====================
// Loop and Iteration DTOs
// =====================

// DraftWorkflowIterationNodeRunRequest represents iteration node run request
type DraftWorkflowIterationNodeRunRequest struct {
	Inputs map[string]interface{} `json:"inputs" validate:"required"`
}

// DraftWorkflowLoopNodeRunRequest represents loop node run request
type DraftWorkflowLoopNodeRunRequest struct {
	Inputs map[string]interface{} `json:"inputs" validate:"required"`
}

// =====================
// Pagination DTOs
// =====================

// WorkflowPagination represents workflow pagination response
type WorkflowPagination struct {
	Data    []WorkflowDetail `json:"data"`
	Page    int              `json:"page"`
	Limit   int              `json:"limit"`
	Total   int64            `json:"total"`
	HasMore bool             `json:"has_more"`
}

// =====================
// Variable Management DTOs
// =====================

// WorkflowVariableType represents the variable type enum
type WorkflowVariableType string

const (
	VariableTypeWorkflow     WorkflowVariableType = "workflow"
	VariableTypeConversation WorkflowVariableType = "conversation"
	VariableTypeSystem       WorkflowVariableType = "system"
	VariableTypeEnvironment  WorkflowVariableType = "environment"
	VariableTypeNode         WorkflowVariableType = "node"
)

// WorkflowVariable represents a workflow variable with all details
type WorkflowVariable struct {
	ID          string               `json:"id"`
	Type        WorkflowVariableType `json:"type"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Selector    []string             `json:"selector"`
	ValueType   string               `json:"value_type"` // string, number, select, secret, file, array_file, array_string, array_number
	Value       interface{}          `json:"value,omitempty"`
	Edited      bool                 `json:"edited"`
	Visible     bool                 `json:"visible"`
	Editable    bool                 `json:"editable,omitempty"`
}

// WorkflowVariableWithoutValue represents a workflow variable without value (for list views)
type WorkflowVariableWithoutValue struct {
	ID          string               `json:"id"`
	Type        WorkflowVariableType `json:"type"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Selector    []string             `json:"selector"`
	ValueType   string               `json:"value_type"`
	Edited      bool                 `json:"edited"`
	Visible     bool                 `json:"visible"`
}

// VariableListRequest represents request for getting variable lists with pagination
type VariableListRequest struct {
	Page  int `form:"page" validate:"min=1"`
	Limit int `form:"limit" validate:"min=1,max=100"`
}

// WorkflowVariableListResponse represents paginated workflow variable list without values
type WorkflowVariableListResponse struct {
	Items []WorkflowVariableWithoutValue `json:"items"`
	Total int64                          `json:"total"`
}

// NodeVariableListResponse represents node variable list with values
type NodeVariableListResponse struct {
	Items []WorkflowVariable `json:"items"`
}

// ConversationVariableListResponse represents conversation variable list
type ConversationVariableListResponse struct {
	Items []WorkflowVariable `json:"items"`
}

// SystemVariableListResponse represents system variable list
type SystemVariableListResponse struct {
	Items []WorkflowVariable `json:"items"`
}

// EnvironmentVariableListResponse represents environment variable list
type EnvironmentVariableListResponse struct {
	Items []WorkflowVariable `json:"items"`
}

// VariableUpdateRequest represents request to update a variable
type VariableUpdateRequest struct {
	Name  *string     `json:"name,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

// FileVariableValue represents file variable value structure
type FileVariableValue struct {
	Type           string `json:"type"`            // "image", "document", etc.
	TransferMethod string `json:"transfer_method"` // "local_file", "remote_url"
	URL            string `json:"url,omitempty"`
	UploadFileID   string `json:"upload_file_id,omitempty"`
}

// Note: StandardResponse and ErrorResponse are defined in app_dto.go

// WorkflowRunForListResponse represents a workflow run for list display
type WorkflowRunForListResponse struct {
	ID               string     `json:"id"`
	Version          string     `json:"version"`
	Status           string     `json:"status"`
	ElapsedTime      float64    `json:"elapsed_time"`
	TotalTokens      int64      `json:"total_tokens"`
	TotalSteps       int        `json:"total_steps"`
	CreatedByAccount *Account   `json:"created_by_account,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	ExceptionsCount  int        `json:"exceptions_count"`
	RetryIndex       int        `json:"retry_index"`
}

// AdvancedChatWorkflowRunForListResponse represents an advanced chat workflow run for list display
type AdvancedChatWorkflowRunForListResponse struct {
	ID               string     `json:"id"`
	ConversationID   string     `json:"conversation_id"`
	MessageID        string     `json:"message_id"`
	Version          string     `json:"version"`
	Status           string     `json:"status"`
	ElapsedTime      float64    `json:"elapsed_time"`
	TotalTokens      int64      `json:"total_tokens"`
	TotalSteps       int        `json:"total_steps"`
	CreatedByAccount *Account   `json:"created_by_account,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	FinishedAt       *time.Time `json:"finished_at,omitempty"`
	ExceptionsCount  int        `json:"exceptions_count"`
	RetryIndex       int        `json:"retry_index"`
}

// Account represents account information
type Account struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Avatar string `json:"avatar,omitempty"`
}

// EndUser represents end user information
type EndUser struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Type        string  `json:"type"`
	IsAnonymous bool    `json:"is_anonymous"`
	SessionID   string  `json:"session_id"`
}

// WorkflowRunPaginationResponse represents paginated workflow runs
type WorkflowRunPaginationResponse struct {
	Limit   int                          `json:"limit"`
	HasMore bool                         `json:"has_more"`
	Data    []WorkflowRunForListResponse `json:"data"`
}

// AdvancedChatWorkflowRunPaginationResponse represents paginated advanced chat workflow runs
type AdvancedChatWorkflowRunPaginationResponse struct {
	Limit   int                                      `json:"limit"`
	HasMore bool                                     `json:"has_more"`
	Data    []AdvancedChatWorkflowRunForListResponse `json:"data"`
}

// WorkflowRunNodeExecutionResponse represents a workflow node execution
type WorkflowRunNodeExecutionResponse struct {
	ID                string          `json:"id"`
	Index             int             `json:"index"`
	PredecessorNodeID string          `json:"predecessor_node_id,omitempty"`
	NodeID            string          `json:"node_id"`
	NodeType          string          `json:"node_type"`
	Title             string          `json:"title"`
	TriggeredFrom     string          `json:"triggered_from"`
	Inputs            json.RawMessage `json:"inputs"`
	ProcessData       json.RawMessage `json:"process_data"`
	Outputs           json.RawMessage `json:"outputs"`
	Graph             json.RawMessage `json:"graph"`
	Features          json.RawMessage `json:"features"`
	Status            string          `json:"status"`
	Error             string          `json:"error,omitempty"`
	ErrorType         string          `json:"error_type,omitempty"`
	DiagnosisResult   string          `json:"diagnosis_result,omitempty"`
	IsLLMDiagnosed    bool            `json:"is_llm_diagnosed"`
	ElapsedTime       float64         `json:"elapsed_time"`
	ExecutionMetadata json.RawMessage `json:"execution_metadata"`
	Extras            json.RawMessage `json:"extras,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	CreatedByRole     string          `json:"created_by_role"`
	CreatedByAccount  *Account        `json:"created_by_account,omitempty"`
	CreatedByEndUser  *EndUser        `json:"created_by_end_user,omitempty"`
	FinishedAt        *time.Time      `json:"finished_at,omitempty"`
}

// WorkflowRunNodeExecutionListResponse represents a list of workflow node executions
type WorkflowRunNodeExecutionListResponse struct {
	Data []WorkflowRunNodeExecutionResponse `json:"data"`
}

// WorkflowRunListRequest represents the request parameters for workflow run list
type WorkflowRunListRequest struct {
	LastID string `form:"last_id" binding:"omitempty,uuid"`
	Limit  int    `form:"limit" binding:"omitempty,min=1,max=100"`
}

// WorkflowStatisticRequest represents the common request parameters for workflow statistics
type WorkflowStatisticRequest struct {
	Start *time.Time `form:"start" time_format:"2006-01-02 15:04"`
	End   *time.Time `form:"end" time_format:"2006-01-02 15:04"`
}

// WorkflowDailyRunsResponse represents daily workflow runs statistics response
type WorkflowDailyRunsResponse struct {
	Data []WorkflowDailyRunsData `json:"data"`
}

// WorkflowDailyRunsData represents daily runs data
type WorkflowDailyRunsData struct {
	Date string `json:"date"`
	Runs int    `json:"runs"`
}

// WorkflowDailyTerminalsResponse represents daily terminals statistics response
type WorkflowDailyTerminalsResponse struct {
	Data []WorkflowDailyTerminalsData `json:"data"`
}

// WorkflowDailyTerminalsData represents daily terminals data
type WorkflowDailyTerminalsData struct {
	Date          string `json:"date"`
	TerminalCount int    `json:"terminal_count"`
}

// WorkflowDailyTokenCostResponse represents daily token cost statistics response
type WorkflowDailyTokenCostResponse struct {
	Data []WorkflowDailyTokenCostData `json:"data"`
}

// WorkflowDailyTokenCostData represents daily token cost data
type WorkflowDailyTokenCostData struct {
	Date       string `json:"date"`
	TokenCount int64  `json:"token_count"`
}

// WorkflowAverageAppInteractionResponse represents average app interaction statistics response
type WorkflowAverageAppInteractionResponse struct {
	Data []WorkflowAverageAppInteractionData `json:"data"`
}

// WorkflowAverageAppInteractionData represents average interaction data
type WorkflowAverageAppInteractionData struct {
	Date         string  `json:"date"`
	Interactions float64 `json:"interactions"`
}

// WorkflowAppLogRequest represents the request parameters for workflow app logs
type WorkflowAppLogRequest struct {
	Keyword                   *string    `form:"keyword"`
	Status                    *string    `form:"status" binding:"omitempty,oneof=succeeded failed stopped paused"`
	CreatedAtBefore           *time.Time `form:"created_at__before" time_format:"2006-01-02T15:04:05Z07:00"`
	CreatedAtAfter            *time.Time `form:"created_at__after" time_format:"2006-01-02T15:04:05Z07:00"`
	CreatedByEndUserSessionID *string    `form:"created_by_end_user_session_id"`
	CreatedByAccount          *string    `form:"created_by_account"`
	Page                      int        `form:"page" binding:"min=1,max=99999" default:"1"`
	Limit                     int        `form:"limit" binding:"min=1,max=100" default:"20"`
}

// WorkflowAppLogResponse represents a single workflow app log entry
type WorkflowAppLogResponse struct {
	ID               string                     `json:"id"`
	WorkflowRun      *WorkflowRunForLogResponse `json:"workflow_run"`
	CreatedFrom      string                     `json:"created_from"`
	CreatedByRole    string                     `json:"created_by_role"`
	CreatedByAccount *Account                   `json:"created_by_account"`
	CreatedByEndUser *EndUser                   `json:"created_by_end_user"`
	CreatedAt        time.Time                  `json:"created_at"`
}

// WorkflowRunForLogResponse represents workflow run info for log display
type WorkflowRunForLogResponse struct {
	ID          string                 `json:"id"`
	WorkflowID  string                 `json:"workflow_id"`
	Status      string                 `json:"status"`
	Inputs      map[string]interface{} `json:"inputs"`
	Outputs     map[string]interface{} `json:"outputs"`
	Error       *string                `json:"error"`
	TotalSteps  *int                   `json:"total_steps"`
	TotalTokens int64                  `json:"total_tokens"`
	CreatedAt   time.Time              `json:"created_at"`
	FinishedAt  *time.Time             `json:"finished_at"`
	ElapsedTime float64                `json:"elapsed_time"`
}

// WorkflowAppLogPaginationResponse represents paginated workflow app logs response
type WorkflowAppLogPaginationResponse struct {
	Page    int                      `json:"page"`
	Limit   int                      `json:"limit"`
	Total   int64                    `json:"total"`
	HasMore bool                     `json:"has_more"`
	Data    []WorkflowAppLogResponse `json:"data"`
}

// WorkflowAppLogCreatedFrom represents the source where workflow app log was created
type WorkflowAppLogCreatedFrom string

const (
	WorkflowAppLogCreatedFromServiceAPI   WorkflowAppLogCreatedFrom = "service-api"
	WorkflowAppLogCreatedFromWebApp       WorkflowAppLogCreatedFrom = "web-app"
	WorkflowAppLogCreatedFromInstalledApp WorkflowAppLogCreatedFrom = "installed-app"
	WorkflowAppLogCreatedFromExternalAPI  WorkflowAppLogCreatedFrom = "external-api"
)

// WorkflowExecutionStatus represents workflow execution status
type WorkflowExecutionStatus string

const (
	WorkflowExecutionStatusSucceeded WorkflowExecutionStatus = "succeeded"
	WorkflowExecutionStatusFailed    WorkflowExecutionStatus = "failed"
	WorkflowExecutionStatusStopped   WorkflowExecutionStatus = "stopped"
)

// =====================
// Workflow Runs DTOs
// =====================

// WorkflowRunsRequest represents the request parameters for workflow runs
type WorkflowRunsRequest struct {
	Page          int    `form:"page" binding:"min=1,max=99999" default:"1"`
	Limit         int    `form:"limit" binding:"min=1,max=100" default:"20"`
	Status        string `form:"status" binding:"omitempty,oneof=succeeded failed stopped running paused"`
	Keyword       string `form:"keyword"`
	TriggeredFrom string `form:"triggered_from" binding:"omitempty"`
}

// WorkflowRunsResponse represents the response for workflow runs
type WorkflowRunsResponse struct {
	Limit   int                      `json:"limit"`
	HasMore bool                     `json:"has_more"`
	Data    []WorkflowRunLogResponse `json:"data"`
}

// WorkflowRunLogResponse represents a single workflow run log entry
type WorkflowRunLogResponse struct {
	ID               string                    `json:"id"`
	SequenceNumber   int                       `json:"sequence_number"`
	Version          string                    `json:"version"`
	TriggeredFrom    string                    `json:"triggered_from"`
	Status           string                    `json:"status"`
	ElapsedTime      float64                   `json:"elapsed_time"`
	TotalTokens      int64                     `json:"total_tokens"`
	TotalSteps       int                       `json:"total_steps"`
	ConversationID   *string                   `json:"conversation_id,omitempty"`
	MessageID        *string                   `json:"message_id,omitempty"`
	CreatedByAccount *CreatedByAccountResponse `json:"created_by_account"`
	CreatedAt        int64                     `json:"created_at"`
	FinishedAt       *int64                    `json:"finished_at"`
	ExceptionsCount  int                       `json:"exceptions_count"`
	RetryIndex       int                       `json:"retry_index"`
}

// CreatedByAccountResponse represents the account information for workflow run creator
type CreatedByAccountResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// =====================
// Workflow Run Detail DTOs
// =====================

// WorkflowRunDetailResponse represents the response for workflow run detail
type WorkflowRunDetailResponse struct {
	ID               string                    `json:"id"`
	SequenceNumber   int                       `json:"sequence_number"`
	Version          string                    `json:"version"`
	Graph            map[string]interface{}    `json:"graph"`
	Features         map[string]interface{}    `json:"features"`
	Inputs           map[string]interface{}    `json:"inputs"`
	Status           string                    `json:"status"`
	Outputs          map[string]interface{}    `json:"outputs"`
	Error            string                    `json:"error"`
	ElapsedTime      float64                   `json:"elapsed_time"`
	TotalTokens      int64                     `json:"total_tokens"`
	TotalSteps       int                       `json:"total_steps"`
	ConversationID   *string                   `json:"conversation_id,omitempty"`
	MessageID        *string                   `json:"message_id,omitempty"`
	CreatedByRole    string                    `json:"created_by_role"`
	CreatedByAccount *CreatedByAccountResponse `json:"created_by_account"`
	CreatedByEndUser *CreatedByAccountResponse `json:"created_by_end_user"`
	CreatedAt        int64                     `json:"created_at"`
	FinishedAt       *int64                    `json:"finished_at"`
	ExceptionsCount  int                       `json:"exceptions_count"`
}

// =====================
// User Migration DTOs
// =====================

// MigrateUserResponse represents the response for user migration
type MigrateUserResponse struct {
	ConversationsMigrated   int    `json:"conversations_migrated"`
	MessagesMigrated        int    `json:"messages_migrated"`
	WorkflowRunLogsMigrated int    `json:"workflow_run_logs_migrated"`
	NodeRuntimeLogsMigrated int    `json:"node_runtime_logs_migrated"`
	AuthenticatedAccountID  string `json:"authenticated_account_id"`
}
