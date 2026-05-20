package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/zgiai/ginext/pkg/logger"
	"gorm.io/gorm"
)

// WorkflowRepository defines the interface for workflow data access operations
type WorkflowRepository interface {
	Create(ctx context.Context, workflow *Workflow) error
	GetByID(ctx context.Context, id string) (*Workflow, error)
	Update(ctx context.Context, workflow *Workflow) error
	Delete(ctx context.Context, id string) error
	GetByAppID(ctx context.Context, appID string) (*Workflow, error)
	GetByAgentID(ctx context.Context, tenantID, agentID string) (*Workflow, error)
	GetByTenantID(ctx context.Context, tenantID string) ([]Workflow, error)
	GetPaginatedWorkflows(ctx context.Context, filter WorkflowFilter, page, limit int) ([]Workflow, int64, error)
	GetDraftWorkflow(ctx context.Context, appID string) (*Workflow, error)
	GetPublishedWorkflows(ctx context.Context, appID string) ([]Workflow, error)
	GetLatestPublishedWorkflow(ctx context.Context, agentID string) (*Workflow, error)
	GetByVersionUUID(ctx context.Context, versionUUID string) (*Workflow, error) // Kept for backward compatibility
	GetLatestPublishedVersion(ctx context.Context, agentID string) (*Workflow, error)
	CreateWorkflow(ctx context.Context, workflow *Workflow) error
	GetPublishedVersions(ctx context.Context, agentID string, limit, offset int) ([]*Workflow, int64, error)
}

// WorkflowFilter defines filter options for workflow queries
type WorkflowFilter struct {
	TenantID string
	AgentID  string
	Status   string
	Type     string
}

// workflowRepository implements WorkflowRepository interface
type workflowRepository struct {
	db *gorm.DB
}

// NewWorkflowRepository creates a new WorkflowRepository instance
func NewWorkflowRepository(db *gorm.DB) WorkflowRepository {
	return &workflowRepository{
		db: db,
	}
}

// Create creates a new workflow
func (r *workflowRepository) Create(ctx context.Context, workflow *Workflow) error {
	return r.db.WithContext(ctx).Create(workflow).Error
}

// GetByID retrieves a workflow by ID
func (r *workflowRepository) GetByID(ctx context.Context, id string) (*Workflow, error) {
	var workflow Workflow
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&workflow).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, err
	}
	return &workflow, nil
}

// Update updates an existing workflow
func (r *workflowRepository) Update(ctx context.Context, workflow *Workflow) error {
	return r.db.WithContext(ctx).Save(workflow).Error
}

// Delete deletes a workflow by ID
func (r *workflowRepository) Delete(ctx context.Context, id string) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&Workflow{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow not found")
	}
	return nil
}

// GetByAppID retrieves a workflow by app ID
func (r *workflowRepository) GetByAppID(ctx context.Context, appID string) (*Workflow, error) {
	var workflow Workflow
	err := r.db.WithContext(ctx).Where("agent_id = ?", appID).First(&workflow).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, err
	}
	return &workflow, nil
}

func (r *workflowRepository) GetByAgentID(ctx context.Context, tenantID, agentID string) (*Workflow, error) {
	if isSystemWorkflowTenantID(tenantID) {
		tenantID = builtInWorkflowTenantID
	}

	var workflow Workflow
	err := r.db.WithContext(ctx).Where("tenant_id = ? AND agent_id = ?", tenantID, agentID).First(&workflow).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, err
	}
	return &workflow, nil
}

// GetByTenantID retrieves all workflows for a tenant
func (r *workflowRepository) GetByTenantID(ctx context.Context, tenantID string) ([]Workflow, error) {
	var workflows []Workflow
	err := r.db.WithContext(ctx).Where("tenant_id = ?", tenantID).Find(&workflows).Error
	return workflows, err
}

// GetPaginatedWorkflows retrieves paginated workflows with filters
func (r *workflowRepository) GetPaginatedWorkflows(ctx context.Context, filter WorkflowFilter, page, limit int) ([]Workflow, int64, error) {
	var workflows []Workflow
	var total int64

	query := r.db.WithContext(ctx).Model(&Workflow{})

	// Apply filters
	query = r.applyFilters(query, filter)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&workflows).Error

	return workflows, total, err
}

// GetDraftWorkflow retrieves the draft workflow for an app
func (r *workflowRepository) GetDraftWorkflow(ctx context.Context, appID string) (*Workflow, error) {
	var workflow Workflow
	err := r.db.WithContext(ctx).Where("agent_id = ? AND version = ?", appID, "draft").First(&workflow).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("draft workflow not found")
		}
		return nil, err
	}
	return &workflow, nil
}

// GetPublishedWorkflows retrieves all published workflows for an app
func (r *workflowRepository) GetPublishedWorkflows(ctx context.Context, appID string) ([]Workflow, error) {
	var workflows []Workflow
	err := r.db.WithContext(ctx).Where("agent_id = ? AND status = ?", appID, "published").Find(&workflows).Error
	return workflows, err
}

// GetLatestPublishedWorkflow gets the latest published workflow for an agent
func (r *workflowRepository) GetLatestPublishedWorkflow(ctx context.Context, agentID string) (*Workflow, error) {
	var workflow Workflow
	// Order by created_at DESC to get the latest published version (version is now timestamp)
	query := r.db.WithContext(ctx).
		Where("agent_id = ? AND version != ?", agentID, "draft")
	err := query.
		Order("created_at DESC").
		First(&workflow).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("latest published workflow not found")
		}
		return nil, err
	}
	return &workflow, nil
}

// GetByVersionUUID retrieves a workflow by version UUID (kept for backward compatibility)
func (r *workflowRepository) GetByVersionUUID(ctx context.Context, versionUUID string) (*Workflow, error) {
	var workflow Workflow
	err := r.db.WithContext(ctx).
		Where("(version_uuid = ? OR id = ?) AND version != ?", versionUUID, versionUUID, "draft").
		First(&workflow).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow not found")
		}
		return nil, err
	}
	return &workflow, nil
}

// applyFilters applies filter conditions to the query
func (r *workflowRepository) applyFilters(query *gorm.DB, filter WorkflowFilter) *gorm.DB {
	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if filter.AgentID != "" {
		query = query.Where("agent_id = ?", filter.AgentID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.Type != "" {
		query = query.Where("type = ?", filter.Type)
	}
	return query
}

// WorkflowNodeRuntimeLogRepository defines the interface for workflow node runtime log data access operations
type WorkflowNodeRuntimeLogRepository interface {
	Create(ctx context.Context, log *WorkflowNodeRuntimeLog) error
	GetByID(ctx context.Context, id string) (*WorkflowNodeRuntimeLog, error)
	Update(ctx context.Context, log *WorkflowNodeRuntimeLog) error
	Delete(ctx context.Context, id string) error
	GetByWorkflowRunID(ctx context.Context, workflowRunID string) ([]WorkflowNodeRuntimeLog, error)
	GetByNodeExecutionID(ctx context.Context, nodeExecutionID string) (*WorkflowNodeRuntimeLog, error)
	GetPaginatedLogs(ctx context.Context, filter WorkflowNodeRuntimeLogFilter, page, limit int) ([]WorkflowNodeRuntimeLog, int64, error)
	UpdateStatus(ctx context.Context, id string, status string, finishedAt *time.Time) error
	UpdateOutputsAndMetadata(ctx context.Context, id string, outputs, processData, executionMetadata *string, elapsedTime float64) error
	UpdateDiagnosisResult(ctx context.Context, id string, result, model string, tokens, latencyMs int, isLLMDiagnosed bool) error
	UpdateDiagnosisYaml(ctx context.Context, id string, errorType, errorStack, result, model string, tokens, latencyMs int, isLLMDiagnosed bool, nodeYAML, upstreamYAML, inputSnapshot, upstreamOutputs string) error
	GetByAgentAndWorkflow(ctx context.Context, agentID, workflowID string, page, limit int) ([]WorkflowNodeRuntimeLog, int64, error)
	GetExecutionPath(ctx context.Context, workflowRunID string) ([]WorkflowNodeRuntimeLog, error)
	GetNextIndex(ctx context.Context, tenantID, agentID, workflowID, triggeredFrom string, workflowRunID *string) (int, error)
	MigrateNodeRuntimeLogsByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error)
}

// WorkflowNodeRuntimeLogFilter defines filter options for workflow node runtime log queries
type WorkflowNodeRuntimeLogFilter struct {
	TenantID        string
	AgentID         string
	WorkflowID      string
	WorkflowRunID   string
	TriggeredFrom   string
	NodeID          string
	NodeType        string
	Status          string
	CreatedByRole   string
	NodeExecutionID string
}

// workflowNodeRuntimeLogRepository implements WorkflowNodeRuntimeLogRepository interface
type workflowNodeRuntimeLogRepository struct {
	db *gorm.DB
}

// NewWorkflowNodeRuntimeLogRepository creates a new WorkflowNodeRuntimeLogRepository instance
func NewWorkflowNodeRuntimeLogRepository(db *gorm.DB) WorkflowNodeRuntimeLogRepository {
	return &workflowNodeRuntimeLogRepository{
		db: db,
	}
}

// Create creates a new workflow node runtime log
func (r *workflowNodeRuntimeLogRepository) Create(ctx context.Context, log *WorkflowNodeRuntimeLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// GetByID retrieves a workflow node runtime log by ID
func (r *workflowNodeRuntimeLogRepository) GetByID(ctx context.Context, id string) (*WorkflowNodeRuntimeLog, error) {
	var log WorkflowNodeRuntimeLog
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&log).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow node runtime log not found")
		}
		return nil, err
	}
	return &log, nil
}

// Update updates an existing workflow node runtime log
func (r *workflowNodeRuntimeLogRepository) Update(ctx context.Context, log *WorkflowNodeRuntimeLog) error {
	return r.db.WithContext(ctx).Save(log).Error
}

// Delete soft deletes a workflow node runtime log by ID
func (r *workflowNodeRuntimeLogRepository) Delete(ctx context.Context, id string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&WorkflowNodeRuntimeLog{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow node runtime log not found")
	}
	return nil
}

// GetByWorkflowRunID retrieves all workflow node runtime logs for a workflow run
func (r *workflowNodeRuntimeLogRepository) GetByWorkflowRunID(ctx context.Context, workflowRunID string) ([]WorkflowNodeRuntimeLog, error) {
	var logs []WorkflowNodeRuntimeLog
	err := r.db.WithContext(ctx).
		Where("workflow_run_id = ? AND deleted_at IS NULL", workflowRunID).
		Order("index ASC, created_at ASC").
		Find(&logs).Error
	return logs, err
}

// GetByNodeExecutionID retrieves a workflow node runtime log by node execution ID
func (r *workflowNodeRuntimeLogRepository) GetByNodeExecutionID(ctx context.Context, nodeExecutionID string) (*WorkflowNodeRuntimeLog, error) {
	var log WorkflowNodeRuntimeLog
	err := r.db.WithContext(ctx).Where("node_execution_id = ? AND deleted_at IS NULL", nodeExecutionID).First(&log).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow node runtime log not found")
		}
		return nil, err
	}
	return &log, nil
}

// GetPaginatedLogs retrieves paginated workflow node runtime logs with filters
func (r *workflowNodeRuntimeLogRepository) GetPaginatedLogs(ctx context.Context, filter WorkflowNodeRuntimeLogFilter, page, limit int) ([]WorkflowNodeRuntimeLog, int64, error) {
	var logs []WorkflowNodeRuntimeLog
	var total int64

	query := r.db.WithContext(ctx).Model(&WorkflowNodeRuntimeLog{}).Where("deleted_at IS NULL")

	// Apply filters
	query = r.applyNodeRuntimeLogFilters(query, filter)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error

	return logs, total, err
}

// UpdateStatus updates the status and finished time of a workflow node runtime log
func (r *workflowNodeRuntimeLogRepository) UpdateStatus(ctx context.Context, id string, status string, finishedAt *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if finishedAt != nil {
		updates["finished_at"] = finishedAt
	}

	result := r.db.WithContext(ctx).Model(&WorkflowNodeRuntimeLog{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow node runtime log not found")
	}
	return nil
}

// UpdateOutputsAndMetadata updates outputs, process data, metadata and elapsed time of a workflow node runtime log
func (r *workflowNodeRuntimeLogRepository) UpdateOutputsAndMetadata(ctx context.Context, id string, outputs, processData, executionMetadata *string, elapsedTime float64) error {
	updates := map[string]interface{}{
		"elapsed_time": elapsedTime,
	}

	if outputs != nil {
		updates["outputs"] = *outputs
	}
	if processData != nil {
		updates["process_data"] = *processData
	}
	if executionMetadata != nil {
		updates["execution_metadata"] = *executionMetadata
	}

	result := r.db.WithContext(ctx).Model(&WorkflowNodeRuntimeLog{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow node runtime log not found")
	}
	return nil
}

// UpdateDiagnosisResult updates the intelligent diagnosis fields of a workflow node runtime log
func (r *workflowNodeRuntimeLogRepository) UpdateDiagnosisResult(ctx context.Context, id string, resultStr, model string, tokens, latencyMs int, isLLMDiagnosed bool) error {
	updates := map[string]interface{}{
		"diagnosis_result":     resultStr,
		"diagnosis_model":      model,
		"diagnosis_tokens":     tokens,
		"diagnosis_latency_ms": latencyMs,
		"is_llm_diagnosed":     isLLMDiagnosed,
	}

	result := r.db.WithContext(ctx).Model(&WorkflowNodeRuntimeLog{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow node runtime log not found")
	}
	return nil
}

// UpdateDiagnosisResult updates the intelligent diagnosis fields of a workflow node runtime log
func (r *workflowNodeRuntimeLogRepository) UpdateDiagnosisYaml(ctx context.Context, id string, errorType, errorStack, resultStr, model string, tokens, latencyMs int, isLLMDiagnosed bool, nodeYAML, upstreamYAML, inputSnapshot, upstreamOutputs string) error {
	// Helper to ensure valid JSON
	validJSON := func(s string) json.RawMessage {
		if s == "" || s == "null" {
			return json.RawMessage("{}")
		}
		return json.RawMessage(s)
	}

	updates := map[string]interface{}{
		"error_type":                 errorType,
		"error_stack":                errorStack,
		"diagnosis_result":           resultStr,
		"diagnosis_model":            model,
		"diagnosis_tokens":           tokens,
		"diagnosis_latency_ms":       latencyMs,
		"is_llm_diagnosed":           isLLMDiagnosed,
		"diagnosis_node_config":      validJSON(nodeYAML),
		"diagnosis_upstream_config":  validJSON(upstreamYAML),
		"diagnosis_input_snapshot":   validJSON(inputSnapshot),
		"diagnosis_upstream_outputs": validJSON(upstreamOutputs),
	}

	logger.Info("Executing GORM Updates for diagnosis yaml", "nodeLogID", id, "keys", len(updates))
	result := r.db.WithContext(ctx).Model(&WorkflowNodeRuntimeLog{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)

	logger.Info("GORM Updates result for diagnosis yaml", "nodeLogID", id, "rowsAffected", result.RowsAffected, "error", result.Error)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow node runtime log not found")
	}
	return nil
}

// GetByAgentAndWorkflow retrieves workflow node runtime logs for an agent and workflow with pagination
func (r *workflowNodeRuntimeLogRepository) GetByAgentAndWorkflow(ctx context.Context, agentID, workflowID string, page, limit int) ([]WorkflowNodeRuntimeLog, int64, error) {
	var logs []WorkflowNodeRuntimeLog
	var total int64

	query := r.db.WithContext(ctx).Model(&WorkflowNodeRuntimeLog{}).
		Where("agent_id = ? AND workflow_id = ? AND deleted_at IS NULL", agentID, workflowID)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error

	return logs, total, err
}

// GetExecutionPath retrieves the execution path for a workflow run ordered by execution sequence
func (r *workflowNodeRuntimeLogRepository) GetExecutionPath(ctx context.Context, workflowRunID string) ([]WorkflowNodeRuntimeLog, error) {
	var logs []WorkflowNodeRuntimeLog
	err := r.db.WithContext(ctx).
		Where("workflow_run_id = ? AND deleted_at IS NULL", workflowRunID).
		Order("index ASC, created_at ASC").
		Find(&logs).Error
	return logs, err
}

// GetNextIndex gets the next index number for a workflow execution
func (r *workflowNodeRuntimeLogRepository) GetNextIndex(ctx context.Context, tenantID, agentID, workflowID, triggeredFrom string, workflowRunID *string) (int, error) {
	var maxIndex int
	query := r.db.WithContext(ctx).Model(&WorkflowNodeRuntimeLog{}).
		Where("tenant_id = ? AND agent_id = ? AND workflow_id = ? AND triggered_from = ? AND deleted_at IS NULL",
			tenantID, agentID, workflowID, triggeredFrom)

	if workflowRunID != nil {
		query = query.Where("workflow_run_id = ?", *workflowRunID)
	}

	err := query.Select("COALESCE(MAX(index), 0)").Scan(&maxIndex).Error
	if err != nil {
		return 0, err
	}

	return maxIndex + 1, nil
}

// applyNodeRuntimeLogFilters applies filters to the workflow node runtime log query
func (r *workflowNodeRuntimeLogRepository) applyNodeRuntimeLogFilters(query *gorm.DB, filter WorkflowNodeRuntimeLogFilter) *gorm.DB {
	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if filter.AgentID != "" {
		query = query.Where("agent_id = ?", filter.AgentID)
	}
	if filter.WorkflowID != "" {
		query = query.Where("workflow_id = ?", filter.WorkflowID)
	}
	if filter.WorkflowRunID != "" {
		query = query.Where("workflow_run_id = ?", filter.WorkflowRunID)
	}
	if filter.TriggeredFrom != "" {
		query = query.Where("triggered_from = ?", filter.TriggeredFrom)
	}
	if filter.NodeID != "" {
		query = query.Where("node_id = ?", filter.NodeID)
	}
	if filter.NodeType != "" {
		query = query.Where("node_type = ?", filter.NodeType)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.CreatedByRole != "" {
		query = query.Where("created_by_role = ?", filter.CreatedByRole)
	}
	if filter.NodeExecutionID != "" {
		query = query.Where("node_execution_id = ?", filter.NodeExecutionID)
	}
	return query
}

// WorkflowRunLogRepository defines the interface for workflow run log data access operations
type WorkflowRunLogRepository interface {
	Create(ctx context.Context, log *WorkflowRunLog) error
	GetByID(ctx context.Context, id string) (*WorkflowRunLog, error)
	Update(ctx context.Context, log *WorkflowRunLog) error
	Delete(ctx context.Context, id string) error
	GetByAgentID(ctx context.Context, agentID string, page, limit int, triggeredFrom string, appWorkspaceID string, accountID string) ([]WorkflowRunLog, int64, error)
	GetByWorkflowRunID(ctx context.Context, workflowRunID string) (*WorkflowRunLog, error)
	GetPaginatedLogs(ctx context.Context, filter WorkflowRunLogFilter, page, limit int) ([]WorkflowRunLog, int64, error)
	UpdateStatus(ctx context.Context, id string, status string, finishedAt *time.Time) error
	UpdateOutputsAndTokens(ctx context.Context, id string, outputs string, totalTokens int64, elapsedTime float64) error
	GetNextSequenceNumber(ctx context.Context, tenantID, agentID string) (int, error)
	GetByAgentAndWorkflowID(ctx context.Context, agentID, workflowID string, page, limit int) ([]WorkflowRunLog, int64, error)
	GetRuntimeLogs(ctx context.Context, filter WorkflowRunLogFilter, page, limit int) ([]WorkflowRunLog, int64, error)
	MigrateWorkflowRunLogsByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error)
}

// WorkflowRunLogFilter defines filter options for workflow run log queries
type WorkflowRunLogFilter struct {
	TenantID      string
	AgentID       string
	WorkflowID    string
	Status        string
	TriggeredFrom string
	CreatedByRole string
	StartDate     *time.Time
	EndDate       *time.Time
	ExcludeDebug  bool
}

// workflowRunLogRepository implements WorkflowRunLogRepository interface
type workflowRunLogRepository struct {
	db *gorm.DB
}

// NewWorkflowRunLogRepository creates a new WorkflowRunLogRepository instance
func NewWorkflowRunLogRepository(db *gorm.DB) WorkflowRunLogRepository {
	return &workflowRunLogRepository{
		db: db,
	}
}

// Create creates a new workflow run log
func (r *workflowRunLogRepository) Create(ctx context.Context, log *WorkflowRunLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}

// GetByID retrieves a workflow run log by ID
func (r *workflowRunLogRepository) GetByID(ctx context.Context, id string) (*WorkflowRunLog, error) {
	var log WorkflowRunLog
	err := r.db.WithContext(ctx).Where("id = ? AND deleted_at IS NULL", id).First(&log).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow run log not found")
		}
		return nil, err
	}
	return &log, nil
}

// Update updates an existing workflow run log
func (r *workflowRunLogRepository) Update(ctx context.Context, log *WorkflowRunLog) error {
	return r.db.WithContext(ctx).Save(log).Error
}

// Delete soft deletes a workflow run log by ID
func (r *workflowRunLogRepository) Delete(ctx context.Context, id string) error {
	now := time.Now()
	result := r.db.WithContext(ctx).Model(&WorkflowRunLog{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Update("deleted_at", now)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow run log not found")
	}
	return nil
}

// GetByAgentID retrieves workflow run logs for an agent with pagination.
func (r *workflowRunLogRepository) GetByAgentID(ctx context.Context, agentID string, page, limit int, triggeredFrom string, appWorkspaceID string, accountID string) ([]WorkflowRunLog, int64, error) {
	var logs []WorkflowRunLog
	var total int64

	query := r.db.WithContext(ctx).Model(&WorkflowRunLog{}).
		Where("agent_id = ? AND deleted_at IS NULL", agentID)

	// Workflow run logs historically persisted either caller workspace or app workspace
	// in tenant_id, so listing relies on globally unique agent_id instead.

	// System workflows store run logs under caller scope, so users can only view their own runs.
	if isSystemWorkflowTenantID(appWorkspaceID) {
		if accountID != "" {
			query = query.Where("created_by = ?", accountID)
		}
	}

	if triggeredFrom != "" {
		query = query.Where("triggered_from = ?", triggeredFrom)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error

	return logs, total, err
}

// GetByWorkflowRunID retrieves a workflow run log by workflow run ID
func (r *workflowRunLogRepository) GetByWorkflowRunID(ctx context.Context, workflowRunID string) (*WorkflowRunLog, error) {
	var log WorkflowRunLog
	err := r.db.WithContext(ctx).Where("workflow_run_id = ? AND deleted_at IS NULL", workflowRunID).First(&log).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("workflow run log not found")
		}
		return nil, err
	}
	return &log, nil
}

// GetPaginatedLogs retrieves paginated workflow run logs with filters
func (r *workflowRunLogRepository) GetPaginatedLogs(ctx context.Context, filter WorkflowRunLogFilter, page, limit int) ([]WorkflowRunLog, int64, error) {
	var logs []WorkflowRunLog
	var total int64

	query := r.db.WithContext(ctx).Model(&WorkflowRunLog{}).Where("deleted_at IS NULL")

	// Apply filters
	query = r.applyRunLogFilters(query, filter)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error

	return logs, total, err
}

// UpdateStatus updates the status and finished time of a workflow run log
func (r *workflowRunLogRepository) UpdateStatus(ctx context.Context, id string, status string, finishedAt *time.Time) error {
	updates := map[string]interface{}{
		"status": status,
	}
	if finishedAt != nil {
		updates["finished_at"] = finishedAt
	}

	result := r.db.WithContext(ctx).Model(&WorkflowRunLog{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow run log not found")
	}
	return nil
}

// UpdateOutputsAndTokens updates outputs, tokens and elapsed time of a workflow run log
func (r *workflowRunLogRepository) UpdateOutputsAndTokens(ctx context.Context, id string, outputs string, totalTokens int64, elapsedTime float64) error {
	updates := map[string]interface{}{
		"outputs":      outputs,
		"total_tokens": totalTokens,
		"elapsed_time": elapsedTime,
	}

	result := r.db.WithContext(ctx).Model(&WorkflowRunLog{}).
		Where("id = ? AND deleted_at IS NULL", id).
		Updates(updates)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("workflow run log not found")
	}
	return nil
}

// GetNextSequenceNumber gets the next sequence number for a tenant and agent
func (r *workflowRunLogRepository) GetNextSequenceNumber(ctx context.Context, tenantID, agentID string) (int, error) {
	var maxSeq int
	err := r.db.WithContext(ctx).Model(&WorkflowRunLog{}).
		Where("tenant_id = ? AND agent_id = ? AND deleted_at IS NULL", tenantID, agentID).
		Select("COALESCE(MAX(sequence_number), 0)").
		Scan(&maxSeq).Error

	if err != nil {
		return 0, err
	}

	return maxSeq + 1, nil
}

// GetByAgentAndWorkflowID retrieves workflow run logs for an agent and workflow ID
func (r *workflowRunLogRepository) GetByAgentAndWorkflowID(ctx context.Context, agentID, workflowID string, page, limit int) ([]WorkflowRunLog, int64, error) {
	var logs []WorkflowRunLog
	var total int64

	query := r.db.WithContext(ctx).Model(&WorkflowRunLog{}).
		Where("agent_id = ? AND workflow_id = ? AND deleted_at IS NULL", agentID, workflowID)

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error

	return logs, total, err
}

// GetRuntimeLogs retrieves runtime logs with filters (excluding debugging logs)
func (r *workflowRunLogRepository) GetRuntimeLogs(ctx context.Context, filter WorkflowRunLogFilter, page, limit int) ([]WorkflowRunLog, int64, error) {
	var logs []WorkflowRunLog
	var total int64

	query := r.db.WithContext(ctx).Model(&WorkflowRunLog{}).
		Where("deleted_at IS NULL")

	// Apply filters
	query = r.applyRunLogFilters(query, filter)

	// Exclude debugging logs if requested
	if filter.ExcludeDebug {
		query = query.Where("triggered_from IN (?)", []string{"web-app", "external-api"})
	}

	// Apply date range filters
	if filter.StartDate != nil {
		query = query.Where("created_at >= ?", filter.StartDate)
	}
	if filter.EndDate != nil {
		query = query.Where("created_at <= ?", filter.EndDate)
	}

	// Get total count
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	offset := (page - 1) * limit
	err := query.Offset(offset).Limit(limit).Order("created_at DESC").Find(&logs).Error

	return logs, total, err
}

// applyRunLogFilters applies filters to the workflow run log query
func (r *workflowRunLogRepository) applyRunLogFilters(query *gorm.DB, filter WorkflowRunLogFilter) *gorm.DB {
	if filter.TenantID != "" {
		query = query.Where("tenant_id = ?", filter.TenantID)
	}
	if filter.AgentID != "" {
		query = query.Where("agent_id = ?", filter.AgentID)
	}
	if filter.WorkflowID != "" {
		query = query.Where("workflow_id = ?", filter.WorkflowID)
	}
	if filter.Status != "" {
		query = query.Where("status = ?", filter.Status)
	}
	if filter.TriggeredFrom != "" {
		query = query.Where("triggered_from = ?", filter.TriggeredFrom)
	}
	if filter.CreatedByRole != "" {
		query = query.Where("created_by_role = ?", filter.CreatedByRole)
	}
	return query
}

// GetLatestPublishedVersion retrieves the latest published version for an agent
func (r *workflowRepository) GetLatestPublishedVersion(ctx context.Context, agentID string) (*Workflow, error) {
	var workflow Workflow
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND version != ?", agentID, "draft").
		Order("created_at DESC").
		First(&workflow).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}
	return &workflow, nil
}

// CreateWorkflow creates a new workflow (used for publishing)
func (r *workflowRepository) CreateWorkflow(ctx context.Context, workflow *Workflow) error {
	return r.db.WithContext(ctx).Create(workflow).Error
}

// GetPublishedVersions retrieves all published versions for an agent with pagination
func (r *workflowRepository) GetPublishedVersions(ctx context.Context, agentID string, limit, offset int) ([]*Workflow, int64, error) {
	var workflows []*Workflow
	var total int64

	// Count total
	if err := r.db.WithContext(ctx).
		Model(&Workflow{}).
		Where("agent_id = ? AND version != ?", agentID, "draft").
		Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Get paginated results
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND version != ?", agentID, "draft").
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&workflows).Error

	if err != nil {
		return nil, 0, err
	}

	return workflows, total, nil
}

// MigrateWorkflowRunLogsByAccountID migrates workflow run logs from virtual user to authenticated user
func (r *workflowRunLogRepository) MigrateWorkflowRunLogsByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error) {
	logger.Debug("Migrating workflow run logs", map[string]interface{}{
		"virtual_account_id":       virtualAccountID,
		"authenticated_account_id": authenticatedAccountID,
	})

	result := tx.WithContext(ctx).
		Model(&WorkflowRunLog{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualAccountID).
		Updates(map[string]interface{}{
			"created_by": authenticatedAccountID,
		})

	if result.Error != nil {
		logger.Error("Failed to migrate workflow run logs", result.Error)
		return 0, fmt.Errorf("failed to migrate workflow run logs: %w", result.Error)
	}

	logger.Info("Workflow run logs migrated", map[string]interface{}{
		"rows_affected":            result.RowsAffected,
		"virtual_account_id":       virtualAccountID,
		"authenticated_account_id": authenticatedAccountID,
	})

	return result.RowsAffected, nil
}

// MigrateNodeRuntimeLogsByAccountID migrates workflow node runtime logs from virtual user to authenticated user
func (r *workflowNodeRuntimeLogRepository) MigrateNodeRuntimeLogsByAccountID(ctx context.Context, tx *gorm.DB, virtualAccountID, authenticatedAccountID string) (int64, error) {
	logger.Debug("Migrating workflow node runtime logs", map[string]interface{}{
		"virtual_account_id":       virtualAccountID,
		"authenticated_account_id": authenticatedAccountID,
	})

	result := tx.WithContext(ctx).
		Model(&WorkflowNodeRuntimeLog{}).
		Where("created_by = ? AND deleted_at IS NULL", virtualAccountID).
		Updates(map[string]interface{}{
			"created_by": authenticatedAccountID,
		})

	if result.Error != nil {
		logger.Error("Failed to migrate node runtime logs", result.Error)
		return 0, fmt.Errorf("failed to migrate node runtime logs: %w", result.Error)
	}

	logger.Info("Workflow node runtime logs migrated", map[string]interface{}{
		"rows_affected":            result.RowsAffected,
		"virtual_account_id":       virtualAccountID,
		"authenticated_account_id": authenticatedAccountID,
	})

	return result.RowsAffected, nil
}
