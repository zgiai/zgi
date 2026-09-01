package indexing

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/text2sql"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type RunTable struct {
	Name string `json:"name"`
	Code string `json:"code,omitempty"`
}

type Run struct {
	ID                string         `gorm:"primaryKey;type:uuid" json:"id"`
	DataSourceID      string         `json:"-"`
	OrganizationID    string         `json:"-"`
	WorkspaceID       string         `json:"-"`
	Trigger           string         `json:"trigger"`
	Status            string         `json:"status"`
	EmbeddingProvider string         `json:"provider"`
	EmbeddingModel    string         `json:"model"`
	TargetIDs         datatypes.JSON `json:"-"`
	Tables            datatypes.JSON `json:"-"`
	TableCount        int            `json:"table_count"`
	FailureCount      int            `json:"failure_count"`
	StartedAt         time.Time      `json:"started_at"`
	CompletedAt       *time.Time     `json:"completed_at"`
	CreatedAt         time.Time      `json:"-"`
	UpdatedAt         time.Time      `json:"-"`
	TableDetails      []RunTable     `gorm:"-" json:"tables"`
}

func (Run) TableName() string { return "database_schema_index_runs" }

type RunPage struct {
	Items   []Run `json:"items"`
	Page    int   `json:"page"`
	Limit   int   `json:"limit"`
	Total   int64 `json:"total"`
	HasMore bool  `json:"has_more"`
}

func (s *Service) History(ctx context.Context, org, id, account string, page, limit int) (*RunPage, error) {
	if err := s.authorize(ctx, org, id, account, false); err != nil {
		return nil, err
	}
	ds, err := s.database(ctx, org, id)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 20 {
		limit = 20
	}
	query := s.db.WithContext(ctx).Where("data_source_id = ? AND organization_id = ? AND workspace_id = ?", id, org, *ds.WorkspaceID)
	var total int64
	if err := query.Model(&Run{}).Count(&total).Error; err != nil {
		return nil, err
	}
	var items []Run
	if err := query.Order("started_at DESC, id DESC").Offset((page - 1) * limit).Limit(limit).Find(&items).Error; err != nil {
		return nil, err
	}
	for i := range items {
		if err := json.Unmarshal(items[i].Tables, &items[i].TableDetails); err != nil {
			return nil, err
		}
	}
	return &RunPage{Items: items, Page: page, Limit: limit, Total: total, HasMore: int64(page*limit) < total}, nil
}

func (s *Service) beginRun(ctx context.Context, tx *gorm.DB, setting Settings, trigger, runID string, ids []string, tables []RunTable, provider, model string) error {
	if runID == "" {
		runID = uuid.NewString()
	}
	targetJSON, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	tableJSON, err := json.Marshal(tables)
	if err != nil {
		return err
	}
	run := Run{ID: runID, DataSourceID: setting.DataSourceID, OrganizationID: setting.OrganizationID, WorkspaceID: setting.WorkspaceID, Trigger: trigger, Status: "building", EmbeddingProvider: provider, EmbeddingModel: model, TargetIDs: targetJSON, Tables: tableJSON, TableCount: len(tables), StartedAt: time.Now()}
	return tx.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&run).Error
}

func (s *Service) affectedTables(ctx context.Context, trigger string, previousIDs, ids []string, current []RunTable) ([]RunTable, error) {
	if trigger == "initial" || trigger == "model_changed" {
		return current, nil
	}
	currentNames := make(map[string]string, len(ids))
	for i, id := range ids {
		currentNames[id] = current[i].Name
	}
	seen := map[string]struct{}{}
	result := make([]RunTable, 0)
	add := func(name string) {
		if _, exists := seen[name]; name == "" || exists {
			return
		}
		seen[name] = struct{}{}
		result = append(result, RunTable{Name: name})
	}
	if trigger == "manual_retry" {
		var rows []text2sql.SchemaIndex
		if len(ids) > 0 {
			if err := s.db.WithContext(ctx).Where("id IN ? AND status <> ?", ids, text2sql.IndexReady).Find(&rows).Error; err != nil {
				return nil, err
			}
		}
		for _, row := range rows {
			add(row.PhysicalTableName)
		}
		return result, nil
	}
	previousSet := make(map[string]struct{}, len(previousIDs))
	for _, id := range previousIDs {
		previousSet[id] = struct{}{}
	}
	currentSet := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		currentSet[id] = struct{}{}
		if _, exists := previousSet[id]; !exists {
			add(currentNames[id])
		}
	}
	removed := make([]string, 0)
	for _, id := range previousIDs {
		if _, exists := currentSet[id]; !exists {
			removed = append(removed, id)
		}
	}
	if len(removed) > 0 {
		var rows []text2sql.SchemaIndex
		if err := s.db.WithContext(ctx).Where("id IN ?", removed).Find(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			add(row.PhysicalTableName)
		}
	}
	return result, nil
}

func (s *Service) failPendingRun(ctx context.Context, setting Settings, code string) error {
	now := time.Now()
	var active []Run
	if err := s.db.WithContext(ctx).Where("data_source_id = ? AND status = ?", setting.DataSourceID, "building").Find(&active).Error; err != nil {
		return err
	}
	for _, run := range active {
		var details []RunTable
		if err := json.Unmarshal(run.Tables, &details); err != nil {
			return err
		}
		if len(details) == 0 {
			details = []RunTable{{Code: code}}
		}
		for i := range details {
			details[i].Code = code
		}
		tables, err := json.Marshal(details)
		if err != nil {
			return err
		}
		if err := s.db.WithContext(ctx).Model(&Run{}).Where("id = ? AND status = ?", run.ID, "building").Updates(map[string]any{"status": "failed", "failure_count": max(1, len(details)), "tables": tables, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	if len(active) > 0 || setting.PendingRunID == nil {
		return nil
	}
	tables, _ := json.Marshal([]RunTable{{Code: code}})
	run := Run{ID: *setting.PendingRunID, DataSourceID: setting.DataSourceID, OrganizationID: setting.OrganizationID, WorkspaceID: setting.WorkspaceID, Trigger: valueOr(setting.PendingTrigger, "initial"), Status: "failed", EmbeddingProvider: setting.EmbeddingProvider, EmbeddingModel: setting.EmbeddingModel, Tables: tables, TargetIDs: datatypes.JSON("[]"), FailureCount: 1, StartedAt: now, CompletedAt: &now}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "id"}}, DoUpdates: clause.Assignments(map[string]any{"status": "failed", "failure_count": 1, "tables": tables, "completed_at": now, "updated_at": now})}).Create(&run).Error
}

func (s *Service) lastFailedTrigger(ctx context.Context, sourceID string) (string, error) {
	var run Run
	err := s.db.WithContext(ctx).Select("trigger").Where("data_source_id = ? AND status = ?", sourceID, "failed").Order("started_at DESC, id DESC").Take(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "schema_changed", nil
	}
	return run.Trigger, err
}

func (s *Service) finalizeRuns(ctx context.Context, sourceID string) error {
	var runs []Run
	if err := s.db.WithContext(ctx).Where("data_source_id = ? AND status = ?", sourceID, "building").Find(&runs).Error; err != nil {
		return err
	}
	for _, run := range runs {
		var ids []string
		if err := json.Unmarshal(run.TargetIDs, &ids); err != nil {
			return err
		}
		if len(ids) == 0 {
			now := time.Now()
			if err := s.db.WithContext(ctx).Model(&Run{}).Where("id = ? AND status = ?", run.ID, "building").Updates(map[string]any{"status": "succeeded", "completed_at": now, "updated_at": now}).Error; err != nil {
				return err
			}
			continue
		}
		var rows []struct {
			text2sql.SchemaIndex
			BuildAttempts int
		}
		if err := s.db.WithContext(ctx).Table("text2sql_schema_indexes").Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return err
		}
		if len(rows) != len(ids) {
			continue
		}
		ready, terminal := 0, 0
		codes := make(map[string]string, len(rows))
		for _, row := range rows {
			if row.Status == text2sql.IndexReady {
				ready++
			}
			if row.Status == text2sql.IndexFailed && row.BuildAttempts >= MaxAttempts {
				terminal++
				if row.LastErrorCode != nil {
					codes[row.PhysicalTableName] = *row.LastErrorCode
				} else {
					codes[row.PhysicalTableName] = "build_failed"
				}
			}
		}
		if !allTargetsTerminal(ready, terminal, len(ids)) {
			continue
		}
		var details []RunTable
		if err := json.Unmarshal(run.Tables, &details); err != nil {
			return err
		}
		for i := range details {
			details[i].Code = codes[details[i].Name]
		}
		tablesJSON, err := json.Marshal(details)
		if err != nil {
			return err
		}
		status := "succeeded"
		if terminal > 0 {
			status = "failed"
		}
		now := time.Now()
		if err := s.db.WithContext(ctx).Model(&Run{}).Where("id = ? AND status = ?", run.ID, "building").Updates(map[string]any{"status": status, "failure_count": terminal, "tables": tablesJSON, "completed_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	return nil
}

func allTargetsTerminal(ready, terminal, total int) bool {
	return ready+terminal == total
}

func valueOr(value *string, fallback string) string {
	if value != nil && *value != "" {
		return *value
	}
	return fallback
}
