package workflowtest

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type Repository struct {
	db *gorm.DB
}

func NewRepository(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) GetSettings(ctx context.Context, agentID string) (*Setting, error) {
	var settings Setting
	err := r.db.WithContext(ctx).Where("agent_id = ?", agentID).First(&settings).Error
	if err != nil {
		return nil, err
	}
	return &settings, nil
}

func (r *Repository) GetAgentOrganizationID(ctx context.Context, agentID string) (string, error) {
	var row struct {
		OrganizationID string `gorm:"column:organization_id"`
	}
	err := r.db.WithContext(ctx).
		Table("agents").
		Select("workspaces.organization_id").
		Joins("JOIN workspaces ON workspaces.id = agents.tenant_id").
		Where("agents.id = ? AND agents.deleted_at IS NULL", agentID).
		Take(&row).Error
	if err != nil {
		return "", err
	}
	return row.OrganizationID, nil
}

func (r *Repository) UpsertSettings(ctx context.Context, settings *Setting) error {
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"judge_prompt_template", "judge_model_provider", "judge_model_name", "updated_at"}),
	}).Create(settings).Error
}

func (r *Repository) CreateScenario(ctx context.Context, scenario *Scenario) error {
	return r.db.WithContext(ctx).Create(scenario).Error
}

func (r *Repository) UpdateScenario(ctx context.Context, scenario *Scenario) error {
	return r.db.WithContext(ctx).Model(&Scenario{}).
		Where("agent_id = ? AND id = ?", scenario.AgentID, scenario.ID).
		Updates(map[string]interface{}{
			"name":        scenario.Name,
			"description": scenario.Description,
			"updated_at":  scenario.UpdatedAt,
		}).Error
}

func (r *Repository) DeleteScenario(ctx context.Context, agentID string, scenarioID string) error {
	return r.db.WithContext(ctx).Where("agent_id = ? AND id = ?", agentID, scenarioID).Delete(&Scenario{}).Error
}

func (r *Repository) CountCasesByScenario(ctx context.Context, agentID string, scenarioID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&Case{}).
		Where("agent_id = ? AND scenario_id = ?", agentID, scenarioID).
		Count(&count).Error
	return count, err
}

func (r *Repository) GetScenarioByName(ctx context.Context, agentID string, name string) (*Scenario, error) {
	var scenario Scenario
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND name = ?", agentID, name).
		First(&scenario).Error
	if err != nil {
		return nil, err
	}
	return &scenario, nil
}

func (r *Repository) ListScenarios(ctx context.Context, agentID string) ([]Scenario, error) {
	var scenarios []Scenario
	err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("created_at DESC").
		Find(&scenarios).Error
	return scenarios, err
}

func (r *Repository) CreateCase(ctx context.Context, testCase *Case) error {
	return r.db.WithContext(ctx).Create(testCase).Error
}

func (r *Repository) GetCase(ctx context.Context, agentID string, caseID string) (*Case, error) {
	var testCase Case
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND id = ?", agentID, caseID).
		First(&testCase).Error
	if err != nil {
		return nil, err
	}
	return &testCase, nil
}

func (r *Repository) ListCases(ctx context.Context, agentID string, status string) ([]Case, error) {
	var cases []Case
	query := r.db.WithContext(ctx).Where("agent_id = ?", agentID)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	err := query.Order("created_at DESC").Find(&cases).Error
	return cases, err
}

func (r *Repository) UpdateCase(ctx context.Context, testCase *Case) error {
	var scenarioID interface{}
	if testCase.ScenarioID != nil {
		scenarioID = *testCase.ScenarioID
	}
	return r.db.WithContext(ctx).Model(&Case{}).
		Where("agent_id = ? AND id = ?", testCase.AgentID, testCase.ID).
		Updates(map[string]interface{}{
			"scenario_id":     scenarioID,
			"content":         testCase.Content,
			"expected_result": testCase.ExpectedResult,
			"question_type":   testCase.QuestionType,
			"status":          testCase.Status,
			"turns":           testCase.Turns,
			"updated_at":      testCase.UpdatedAt,
		}).Error
}

func (r *Repository) DeleteCases(ctx context.Context, agentID string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).
		Where("agent_id = ? AND id IN ?", agentID, ids).
		Delete(&Case{}).Error
}

func (r *Repository) ListCasesByIDs(ctx context.Context, agentID string, ids []string) ([]Case, error) {
	var cases []Case
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND id IN ?", agentID, ids).
		Order("created_at ASC").
		Find(&cases).Error
	return cases, err
}

func (r *Repository) UpdateCaseScenario(ctx context.Context, agentID string, caseID string, scenarioID *string) error {
	var value interface{}
	if scenarioID != nil {
		value = *scenarioID
	}
	return r.db.WithContext(ctx).Model(&Case{}).
		Where("agent_id = ? AND id = ?", agentID, caseID).
		Updates(map[string]interface{}{"scenario_id": value, "updated_at": time.Now()}).Error
}

func (r *Repository) ResetScenarioCaseCounts(ctx context.Context, agentID string) error {
	return r.db.WithContext(ctx).Model(&Scenario{}).
		Where("agent_id = ?", agentID).
		Update("case_count", 0).Error
}

func (r *Repository) UpdateScenarioCaseCount(ctx context.Context, agentID string, scenarioID string, count int) error {
	return r.db.WithContext(ctx).Model(&Scenario{}).
		Where("agent_id = ? AND id = ?", agentID, scenarioID).
		Updates(map[string]interface{}{"case_count": count, "updated_at": time.Now()}).Error
}

func (r *Repository) IncrementScenarioCaseCount(ctx context.Context, agentID string, scenarioID string, delta int) error {
	return r.db.WithContext(ctx).Model(&Scenario{}).
		Where("agent_id = ? AND id = ?", agentID, scenarioID).
		Updates(map[string]interface{}{"case_count": gorm.Expr("case_count + ?", delta), "updated_at": time.Now()}).Error
}

func (r *Repository) CreateBatch(ctx context.Context, batch *Batch) error {
	return r.db.WithContext(ctx).Create(batch).Error
}

func (r *Repository) CreateBatchWithItems(ctx context.Context, batch *Batch, items []BatchItem) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(batch).Error; err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		return tx.Create(&items).Error
	})
}

func (r *Repository) GetBatch(ctx context.Context, agentID string, batchID string) (*Batch, error) {
	var batch Batch
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND id = ?", agentID, batchID).
		First(&batch).Error
	if err != nil {
		return nil, err
	}
	return &batch, nil
}

func (r *Repository) UpdateBatchStatus(ctx context.Context, agentID string, batchID string, status string) error {
	return r.db.WithContext(ctx).Model(&Batch{}).
		Where("agent_id = ? AND id = ?", agentID, batchID).
		Updates(map[string]interface{}{"status": status, "updated_at": time.Now()}).Error
}

func (r *Repository) UpdateBatchStatusIfCurrent(ctx context.Context, agentID string, batchID string, currentStatus string, nextStatus string) (bool, error) {
	result := r.db.WithContext(ctx).Model(&Batch{}).
		Where("agent_id = ? AND id = ? AND status = ?", agentID, batchID, currentStatus).
		Updates(map[string]interface{}{"status": nextStatus, "updated_at": time.Now()})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) TouchBatch(ctx context.Context, agentID string, batchID string) error {
	return r.db.WithContext(ctx).Model(&Batch{}).
		Where("agent_id = ? AND id = ?", agentID, batchID).
		Update("updated_at", time.Now()).Error
}

func (r *Repository) UpdateBatchSummary(ctx context.Context, agentID string, batchID string, status string, passed, failed, review int, summary string) error {
	return r.db.WithContext(ctx).Model(&Batch{}).
		Where("agent_id = ? AND id = ?", agentID, batchID).
		Updates(map[string]interface{}{
			"status":       status,
			"passed_count": passed,
			"failed_count": failed,
			"review_count": review,
			"summary":      summary,
			"updated_at":   time.Now(),
		}).Error
}

func (r *Repository) UpdateBatchCaseCount(ctx context.Context, batchID string, count int) error {
	return r.db.WithContext(ctx).Model(&Batch{}).
		Where("id = ?", batchID).
		Updates(map[string]interface{}{"case_count": count, "updated_at": time.Now()}).Error
}

func (r *Repository) ListBatches(ctx context.Context, agentID string) ([]Batch, error) {
	var batches []Batch
	err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("created_at DESC").
		Find(&batches).Error
	if err != nil || len(batches) == 0 {
		return batches, err
	}

	batchIDs := make([]string, 0, len(batches))
	for _, batch := range batches {
		batchIDs = append(batchIDs, batch.ID)
	}
	counts, err := r.CountBatchItemResults(ctx, agentID, batchIDs)
	if err != nil {
		return nil, err
	}
	for i := range batches {
		count := counts[batches[i].ID]
		batches[i].PassedCount = count.Passed
		batches[i].FailedCount = count.Failed
		batches[i].ReviewCount = count.Review
	}
	return batches, nil
}

func (r *Repository) CreateBatchItems(ctx context.Context, items []BatchItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&items).Error
}

type BatchItemResultCount struct {
	Passed int
	Failed int
	Review int
}

func (r *Repository) CountBatchItemResults(ctx context.Context, agentID string, batchIDs []string) (map[string]BatchItemResultCount, error) {
	counts := make(map[string]BatchItemResultCount, len(batchIDs))
	if len(batchIDs) == 0 {
		return counts, nil
	}

	var rows []struct {
		BatchID string `gorm:"column:batch_id"`
		Status  string `gorm:"column:status"`
		Count   int    `gorm:"column:count"`
	}
	if err := r.db.WithContext(ctx).
		Model(&BatchItem{}).
		Select("batch_id, status, COUNT(*) AS count").
		Where("agent_id = ? AND batch_id IN ? AND status IN ?", agentID, batchIDs, []string{
			string(BatchItemStatusPassed),
			string(BatchItemStatusFailed),
			string(BatchItemStatusReview),
		}).
		Group("batch_id, status").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	for _, row := range rows {
		count := counts[row.BatchID]
		switch row.Status {
		case string(BatchItemStatusPassed):
			count.Passed = row.Count
		case string(BatchItemStatusFailed):
			count.Failed = row.Count
		case string(BatchItemStatusReview):
			count.Review = row.Count
		}
		counts[row.BatchID] = count
	}
	return counts, nil
}

func (r *Repository) ListBatchItems(ctx context.Context, agentID string, batchID string) ([]BatchItem, error) {
	var items []BatchItem
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND batch_id = ?", agentID, batchID).
		Order("created_at ASC").
		Find(&items).Error
	return items, err
}

func (r *Repository) UpdateBatchItemsStatus(ctx context.Context, agentID string, batchID string, fromStatuses []string, toStatus string) error {
	query := r.db.WithContext(ctx).Model(&BatchItem{}).
		Where("agent_id = ? AND batch_id = ?", agentID, batchID)
	if len(fromStatuses) > 0 {
		query = query.Where("status IN ?", fromStatuses)
	}
	return query.Updates(map[string]interface{}{"status": toStatus, "updated_at": time.Now()}).Error
}

func (r *Repository) UpdateBatchItemStatusIfCurrent(ctx context.Context, agentID string, itemID string, currentStatus string, nextStatus string) (bool, error) {
	result := r.db.WithContext(ctx).Model(&BatchItem{}).
		Where("agent_id = ? AND id = ? AND status = ?", agentID, itemID, currentStatus).
		Updates(map[string]interface{}{
			"status":     nextStatus,
			"updated_at": time.Now(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) UpdateBatchItemResult(ctx context.Context, item *BatchItem) error {
	result := r.db.WithContext(ctx).Model(&BatchItem{}).
		Where("agent_id = ? AND id = ? AND status = ?", item.AgentID, item.ID, string(BatchItemStatusRunning)).
		Updates(map[string]interface{}{
			"status":           item.Status,
			"workflow_run_id":  item.WorkflowRunID,
			"outputs":          item.Outputs,
			"error":            item.Error,
			"judge_reason":     item.JudgeReason,
			"judge_suggestion": item.JudgeSuggestion,
			"judge_confidence": item.JudgeConfidence,
			"updated_at":       time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *Repository) RecoverStaleRunningBatches(ctx context.Context, agentID string, staleBefore time.Time, summary string, itemError string, completedAt time.Time) (int64, error) {
	var batchIDs []string
	if err := r.db.WithContext(ctx).Model(&Batch{}).
		Where("agent_id = ? AND status = ? AND updated_at < ?", agentID, BatchStatusRunning, staleBefore).
		Pluck("id", &batchIDs).Error; err != nil {
		return 0, err
	}
	if len(batchIDs) == 0 {
		return 0, nil
	}
	return int64(len(batchIDs)), r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Batch{}).
			Where("agent_id = ? AND id IN ?", agentID, batchIDs).
			Updates(map[string]interface{}{
				"status":     BatchStatusStopped,
				"summary":    summary,
				"updated_at": completedAt,
			}).Error; err != nil {
			return err
		}
		return tx.Model(&BatchItem{}).
			Where("agent_id = ? AND batch_id IN ? AND status IN ?", agentID, batchIDs, []string{
				string(BatchItemStatusPending),
				string(BatchItemStatusRunning),
			}).
			Updates(map[string]interface{}{
				"status":     string(BatchItemStatusFailed),
				"error":      itemError,
				"updated_at": completedAt,
			}).Error
	})
}

func activeGenerationStatuses() []string {
	return []string{
		GenerationTaskStatusQueued,
		GenerationTaskStatusRunning,
		GenerationTaskStatusCanceling,
	}
}

func (r *Repository) CreateGenerationTask(ctx context.Context, task *GenerationTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *Repository) GetGenerationTask(ctx context.Context, agentID, taskID string) (*GenerationTask, error) {
	var task GenerationTask
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND id = ?", agentID, taskID).
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) GetGenerationTaskByID(ctx context.Context, taskID string) (*GenerationTask, error) {
	var task GenerationTask
	err := r.db.WithContext(ctx).
		Where("id = ?", taskID).
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) GetActiveGenerationTask(ctx context.Context, agentID string) (*GenerationTask, error) {
	var task GenerationTask
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND status IN ?", agentID, activeGenerationStatuses()).
		Order("created_at DESC").
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) GetLatestGenerationTask(ctx context.Context, agentID string) (*GenerationTask, error) {
	var task GenerationTask
	err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("created_at DESC").
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) UpdateGenerationTaskStatus(ctx context.Context, taskID, status string, updates map[string]interface{}) error {
	values := map[string]interface{}{}
	for key, value := range updates {
		values[key] = value
	}
	values["status"] = status
	values["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&GenerationTask{}).
		Where("id = ?", taskID).
		Updates(values).Error
}

func (r *Repository) RecoverStaleRunningGenerationTasks(ctx context.Context, staleBefore time.Time, reason string, completedAt time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&GenerationTask{}).
		Where("status IN ? AND updated_at < ?", []string{GenerationTaskStatusRunning, GenerationTaskStatusCanceling}, staleBefore).
		Updates(map[string]interface{}{
			"status":       GenerationTaskStatusFailed,
			"error":        reason,
			"completed_at": completedAt,
			"updated_at":   completedAt,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (r *Repository) MarkGenerationTaskRunning(ctx context.Context, taskID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&GenerationTask{}).
		Where("id = ? AND status = ?", taskID, GenerationTaskStatusQueued).
		Updates(map[string]interface{}{
			"status":     GenerationTaskStatusRunning,
			"started_at": now,
			"updated_at": now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *Repository) IncrementGenerationTaskCreatedCount(ctx context.Context, taskID string, delta int) error {
	return r.db.WithContext(ctx).Model(&GenerationTask{}).
		Where("id = ?", taskID).
		Update("created_count", gorm.Expr("created_count + ?", delta)).Error
}

func (r *Repository) CancelGenerationTask(ctx context.Context, agentID, taskID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&GenerationTask{}).
		Where("agent_id = ? AND id = ? AND status IN ?", agentID, taskID, []string{
			GenerationTaskStatusQueued,
			GenerationTaskStatusRunning,
		}).
		Updates(map[string]interface{}{
			"status":              GenerationTaskStatusCanceling,
			"cancel_requested_at": now,
			"updated_at":          now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func activeScenarioRecognitionStatuses() []string {
	return []string{
		GenerationTaskStatusQueued,
		GenerationTaskStatusRunning,
		GenerationTaskStatusCanceling,
	}
}

func (r *Repository) CreateScenarioRecognitionTask(ctx context.Context, task *ScenarioRecognitionTask) error {
	return r.db.WithContext(ctx).Create(task).Error
}

func (r *Repository) GetScenarioRecognitionTask(ctx context.Context, agentID, taskID string) (*ScenarioRecognitionTask, error) {
	var task ScenarioRecognitionTask
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND id = ?", agentID, taskID).
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) GetScenarioRecognitionTaskByID(ctx context.Context, taskID string) (*ScenarioRecognitionTask, error) {
	var task ScenarioRecognitionTask
	err := r.db.WithContext(ctx).
		Where("id = ?", taskID).
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) GetActiveScenarioRecognitionTask(ctx context.Context, agentID string) (*ScenarioRecognitionTask, error) {
	var task ScenarioRecognitionTask
	err := r.db.WithContext(ctx).
		Where("agent_id = ? AND status IN ?", agentID, activeScenarioRecognitionStatuses()).
		Order("created_at DESC").
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) GetLatestScenarioRecognitionTask(ctx context.Context, agentID string) (*ScenarioRecognitionTask, error) {
	var task ScenarioRecognitionTask
	err := r.db.WithContext(ctx).
		Where("agent_id = ?", agentID).
		Order("created_at DESC").
		First(&task).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

func (r *Repository) UpdateScenarioRecognitionTaskStatus(ctx context.Context, taskID, status string, updates map[string]interface{}) error {
	values := map[string]interface{}{}
	for key, value := range updates {
		values[key] = value
	}
	values["status"] = status
	values["updated_at"] = time.Now()
	return r.db.WithContext(ctx).Model(&ScenarioRecognitionTask{}).
		Where("id = ?", taskID).
		Updates(values).Error
}

func (r *Repository) RecoverStaleRunningScenarioRecognitionTasks(ctx context.Context, staleBefore time.Time, reason string, completedAt time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&ScenarioRecognitionTask{}).
		Where("status IN ? AND updated_at < ?", []string{GenerationTaskStatusRunning, GenerationTaskStatusCanceling}, staleBefore).
		Updates(map[string]interface{}{
			"status":       GenerationTaskStatusFailed,
			"error":        reason,
			"completed_at": completedAt,
			"updated_at":   completedAt,
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

func (r *Repository) MarkScenarioRecognitionTaskRunning(ctx context.Context, taskID string, now time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&ScenarioRecognitionTask{}).
		Where("id = ? AND status = ?", taskID, GenerationTaskStatusQueued).
		Updates(map[string]interface{}{
			"status":     GenerationTaskStatusRunning,
			"started_at": now,
			"updated_at": now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
