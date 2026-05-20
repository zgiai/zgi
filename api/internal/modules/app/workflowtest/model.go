package workflowtest

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

const (
	CaseStatusEnabled  = "enabled"
	CaseStatusDisabled = "disabled"

	CaseTypeCore      = "core"
	CaseTypeExtension = "extension"
	CaseTypeFuzzy     = "fuzzy"

	BatchStatusQueued    = "queued"
	BatchStatusRunning   = "running"
	BatchStatusCompleted = "completed"
	BatchStatusStopped   = "stopped"
	BatchStatusCanceled  = "canceled"
)

const DefaultJudgePromptTemplate = `你是工作流自动化测试的 AI 评分助手。请根据测试问题、智能体回复、期望要点和业务场景，判断本次回答是否有效解决用户问题。

评分结果只能归为以下三类：
1. 通过：回答满足用户核心诉求，没有明显事实错误或流程错误。
2. 不通过：回答未解决核心诉求，或存在明显错误、遗漏、与业务流程冲突。
3. 需复核：信息不足、判断边界模糊，或需要人工结合业务上下文确认。

请输出判断结果、简要理由、改进建议。`

type Setting struct {
	ID                  string    `gorm:"type:uuid;primaryKey" json:"id"`
	AgentID             string    `gorm:"type:uuid;not null;uniqueIndex:idx_workflow_test_settings_agent" json:"agent_id"`
	JudgePromptTemplate string    `gorm:"type:text;not null" json:"judge_prompt_template"`
	JudgeModelProvider  string    `gorm:"type:varchar(100);not null;default:''" json:"judge_model_provider"`
	JudgeModelName      string    `gorm:"type:varchar(160);not null;default:''" json:"judge_model_name"`
	CreatedAt           time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt           time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Setting) TableName() string {
	return "workflow_test_settings"
}

type Scenario struct {
	ID          string    `gorm:"type:uuid;primaryKey" json:"id"`
	AgentID     string    `gorm:"type:uuid;not null;index:idx_workflow_test_scenarios_agent" json:"agent_id"`
	Name        string    `gorm:"type:varchar(120);not null" json:"name"`
	Description string    `gorm:"type:text;not null;default:''" json:"description"`
	Source      string    `gorm:"type:varchar(32);not null;default:'manual'" json:"source"`
	CaseCount   int       `gorm:"type:int;not null;default:0" json:"case_count"`
	CreatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt   time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Scenario) TableName() string {
	return "workflow_test_scenarios"
}

type CaseAttachment struct {
	Type           string `json:"type"`
	TransferMethod string `json:"transfer_method"`
	URL            string `json:"url,omitempty"`
	UploadFileID   string `json:"upload_file_id,omitempty"`
	Name           string `json:"name,omitempty"`
}

type CaseTurn struct {
	Role        string           `json:"role"`
	Content     string           `json:"content"`
	Attachments []CaseAttachment `json:"attachments,omitempty"`
	Inputs      JSONMap          `json:"inputs,omitempty"`
}

type CaseTurns []CaseTurn

func (t CaseTurns) Value() (driver.Value, error) {
	if t == nil {
		return "[]", nil
	}
	data, err := json.Marshal(t)
	return string(data), err
}

func (t *CaseTurns) Scan(value interface{}) error {
	if value == nil {
		*t = CaseTurns{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported CaseTurns scan type %T", value)
	}
	if len(bytes) == 0 {
		*t = CaseTurns{}
		return nil
	}
	return json.Unmarshal(bytes, t)
}

type Case struct {
	ID             string    `gorm:"type:uuid;primaryKey" json:"id"`
	AgentID        string    `gorm:"type:uuid;not null;index:idx_workflow_test_cases_agent" json:"agent_id"`
	ScenarioID     *string   `gorm:"type:uuid;index:idx_workflow_test_cases_scenario" json:"scenario_id,omitempty"`
	Content        string    `gorm:"type:text;not null" json:"content"`
	ExpectedResult string    `gorm:"type:text;not null;default:''" json:"expected_result"`
	QuestionType   string    `gorm:"type:varchar(32);not null;default:'core'" json:"question_type"`
	Status         string    `gorm:"type:varchar(32);not null;default:'enabled'" json:"status"`
	Turns          CaseTurns `gorm:"type:jsonb;not null" json:"turns"`
	CreatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt      time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Case) TableName() string {
	return "workflow_test_cases"
}

type CaseSnapshot struct {
	ID             string    `json:"id"`
	ScenarioID     *string   `json:"scenario_id,omitempty"`
	Content        string    `json:"content"`
	ExpectedResult string    `json:"expected_result"`
	QuestionType   string    `json:"question_type"`
	Turns          CaseTurns `json:"turns"`
}

type BatchItemStatus string

const (
	BatchItemStatusPending  BatchItemStatus = "pending"
	BatchItemStatusRunning  BatchItemStatus = "running"
	BatchItemStatusPassed   BatchItemStatus = "passed"
	BatchItemStatusFailed   BatchItemStatus = "failed"
	BatchItemStatusReview   BatchItemStatus = "review"
	BatchItemStatusCanceled BatchItemStatus = "canceled"
)

type BatchItem struct {
	ID              string           `gorm:"type:uuid;primaryKey" json:"id"`
	AgentID         string           `gorm:"type:uuid;not null;index:idx_workflow_test_batch_items_agent" json:"agent_id"`
	BatchID         string           `gorm:"type:uuid;not null;index:idx_workflow_test_batch_items_batch" json:"batch_id"`
	CaseID          string           `gorm:"type:uuid;not null;index:idx_workflow_test_batch_items_case" json:"case_id"`
	CaseSnapshot    JSONCaseSnapshot `gorm:"type:jsonb;not null" json:"case_snapshot"`
	Status          string           `gorm:"type:varchar(32);not null;default:'pending'" json:"status"`
	WorkflowRunID   string           `gorm:"type:varchar(120);not null;default:''" json:"workflow_run_id"`
	Outputs         JSONMap          `gorm:"type:jsonb;not null" json:"outputs"`
	Error           string           `gorm:"type:text;not null;default:''" json:"error"`
	JudgeReason     string           `gorm:"type:text;not null;default:''" json:"judge_reason"`
	JudgeSuggestion string           `gorm:"type:text;not null;default:''" json:"judge_suggestion"`
	JudgeConfidence float64          `gorm:"type:decimal(5,4);not null;default:0" json:"judge_confidence"`
	CreatedAt       time.Time        `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt       time.Time        `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (BatchItem) TableName() string {
	return "workflow_test_batch_items"
}

type JSONCaseSnapshot CaseSnapshot

func (s JSONCaseSnapshot) Value() (driver.Value, error) {
	data, err := json.Marshal(s)
	return string(data), err
}

func (s *JSONCaseSnapshot) Scan(value interface{}) error {
	if value == nil {
		*s = JSONCaseSnapshot{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported JSONCaseSnapshot scan type %T", value)
	}
	if len(bytes) == 0 {
		*s = JSONCaseSnapshot{}
		return nil
	}
	return json.Unmarshal(bytes, s)
}

type JSONMap map[string]interface{}

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	data, err := json.Marshal(m)
	return string(data), err
}

func (m *JSONMap) Scan(value interface{}) error {
	if value == nil {
		*m = JSONMap{}
		return nil
	}
	var bytes []byte
	switch v := value.(type) {
	case []byte:
		bytes = v
	case string:
		bytes = []byte(v)
	default:
		return fmt.Errorf("unsupported JSONMap scan type %T", value)
	}
	if len(bytes) == 0 {
		*m = JSONMap{}
		return nil
	}
	return json.Unmarshal(bytes, m)
}

type Batch struct {
	ID                         string    `gorm:"type:uuid;primaryKey" json:"id"`
	AgentID                    string    `gorm:"type:uuid;not null;index:idx_workflow_test_batches_agent" json:"agent_id"`
	Name                       string    `gorm:"type:varchar(160);not null" json:"name"`
	Status                     string    `gorm:"type:varchar(32);not null;default:'queued'" json:"status"`
	CaseCount                  int       `gorm:"type:int;not null;default:0" json:"case_count"`
	PassedCount                int       `gorm:"type:int;not null;default:0" json:"passed_count"`
	FailedCount                int       `gorm:"type:int;not null;default:0" json:"failed_count"`
	ReviewCount                int       `gorm:"type:int;not null;default:0" json:"review_count"`
	JudgePromptSnapshot        string    `gorm:"type:text;not null" json:"judge_prompt_snapshot"`
	JudgeModelProviderSnapshot string    `gorm:"type:varchar(100);not null;default:''" json:"judge_model_provider_snapshot"`
	JudgeModelNameSnapshot     string    `gorm:"type:varchar(160);not null;default:''" json:"judge_model_name_snapshot"`
	WorkflowVersionMode        string    `gorm:"type:varchar(40);not null;default:'draft'" json:"workflow_version_mode"`
	WorkflowVersionUUID        *string   `gorm:"type:uuid" json:"workflow_version_uuid,omitempty"`
	WorkflowVersionLabel       string    `gorm:"type:varchar(160);not null;default:'current_draft'" json:"workflow_version_label"`
	Summary                    string    `gorm:"type:text;not null;default:''" json:"summary"`
	CreatedAt                  time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`
	UpdatedAt                  time.Time `gorm:"not null;default:CURRENT_TIMESTAMP" json:"updated_at"`
}

func (Batch) TableName() string {
	return "workflow_test_batches"
}

func newID() string {
	return uuid.NewString()
}
