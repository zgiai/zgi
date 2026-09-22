package indexing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/zgiai/zgi/api/internal/capabilities/text2sql"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const taskType = "database:schema_index"
const tableTimeout = 2 * time.Minute
const valueSampleTimeout = 15 * time.Second
const maxValueSampleBackfills = 20

type taskPayload struct {
	IndexID string `json:"index_id"`
	AssetID string `json:"asset_id"`
}
type taskRegistry interface {
	Register(string, func(context.Context, *asynq.Task) error) bool
}

func (s *Service) Register(registry taskRegistry) {
	registry.Register(s.queue.GetTaskTypeWithPrefix(taskType), s.handleTask)
}

// Run reconciles only explicitly enabled databases. It also repairs missed
// deliveries after Redis outages, and notices schema/model changes outside ZGI.
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if err := s.Reconcile(ctx); err != nil && ctx.Err() == nil {
			s.log.Error("reconcile database schema indexes", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) Reconcile(ctx context.Context) error {
	var settings []Settings
	if err := s.db.WithContext(ctx).Order("data_source_id").Find(&settings).Error; err != nil {
		return err
	}
	for _, setting := range settings {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		scanCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		err := s.withLock(scanCtx, "database:"+setting.DataSourceID, func(db *gorm.DB) error {
			// Another instance or a manual retry may have updated the settings.
			if err := db.WithContext(scanCtx).Where("data_source_id = ?", setting.DataSourceID).Take(&setting).Error; err != nil {
				return err
			}
			return s.reconcileDatabase(scanCtx, setting)
		})
		cancel()
		if err != nil {
			s.log.Error("database schema index reconciliation failed", zap.String("data_source_id", setting.DataSourceID), zap.Error(err))
			writeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			_ = s.failPendingRun(writeCtx, setting, "preparation_failed")
			writeErr := s.db.WithContext(writeCtx).Model(&Settings{}).Where("data_source_id = ?", setting.DataSourceID).Update("last_error_code", "preparation_failed").Error
			stop()
			if writeErr != nil {
				return errors.Join(err, writeErr)
			}
		}
	}
	return nil
}

func (s *Service) reconcileDatabase(ctx context.Context, setting Settings) error {
	ds, err := s.database(ctx, setting.OrganizationID, setting.DataSourceID)
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && *ds.WorkspaceID != setting.WorkspaceID) {
		if err := s.retire(ctx, setting, nil); err != nil {
			return err
		}
		if err := s.db.WithContext(ctx).Where("data_source_id = ?", setting.DataSourceID).Delete(&Run{}).Error; err != nil {
			return err
		}
		return s.db.WithContext(ctx).Delete(&setting).Error
	}
	if err != nil {
		return err
	}
	if err := s.authorize(ctx, setting.OrganizationID, setting.DataSourceID, setting.RequestedBy, true); err != nil {
		return err
	}
	model, err := s.models.ResolveUseCase(ctx, setting.OrganizationID, llmmodel.UseCaseEmbedding, nil, nil)
	if err != nil {
		return err
	}
	previousProvider, previousModel := setting.EmbeddingProvider, setting.EmbeddingModel
	var previousIDs []string
	if err := json.Unmarshal(setting.TargetIDs, &previousIDs); err != nil {
		return err
	}
	repo := text2sql.NewIndexRepository(s.db)
	tables, err := repo.Snapshot(ctx, setting.OrganizationID, setting.DataSourceID, s.sql)
	if err != nil {
		return err
	}
	ids := make([]string, 0, len(tables))
	tableDetails := make([]RunTable, 0, len(tables))
	backfills := 0
	for _, table := range tables {
		row := text2sql.NewSchemaIndex(setting.OrganizationID, setting.WorkspaceID, setting.DataSourceID, table.AssetID, table.Table, model.Provider, model.Model)
		if err := repo.Ensure(ctx, row); err != nil {
			return err
		}
		if row.Status == text2sql.IndexReady && !text2sql.ValueSamplesInitialized(row.ValueSamples) && backfills < maxValueSampleBackfills {
			sampleCtx, sampleCancel := context.WithTimeout(ctx, valueSampleTimeout)
			encoded := text2sql.EncodeValueSamples(text2sql.SampleColumnValues(sampleCtx, s.sql, table.Table))
			sampleCancel()
			if err := s.db.WithContext(ctx).Model(&text2sql.SchemaIndex{}).Where("id = ?", row.ID).Update("value_samples", encoded).Error; err != nil {
				return err
			}
			backfills++
		}
		ids = append(ids, row.ID)
		tableDetails = append(tableDetails, RunTable{Name: table.Table.Name})
	}
	targets, err := json.Marshal(ids)
	if err != nil {
		return err
	}
	setting.TargetIDs = datatypes.JSON(targets)
	trigger, runID := "", ""
	if setting.PendingTrigger != nil {
		trigger = *setting.PendingTrigger
		if setting.PendingRunID != nil {
			runID = *setting.PendingRunID
		}
	} else if previousProvider != model.Provider || previousModel != model.Model {
		trigger = "model_changed"
	} else if !slices.Equal(previousIDs, ids) {
		trigger = "schema_changed"
	} else if setting.LastErrorCode != nil {
		trigger, err = s.lastFailedTrigger(ctx, setting.DataSourceID)
		if err != nil {
			return err
		}
	}
	if trigger != "" && runID == "" {
		runID = uuid.NewString()
	}
	if trigger != "" {
		tableDetails, err = s.affectedTables(ctx, trigger, previousIDs, ids, tableDetails)
		if err != nil {
			return err
		}
	}
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if trigger != "" {
			if err := s.beginRun(ctx, tx, setting, trigger, runID, ids, tableDetails, model.Provider, model.Model); err != nil {
				return err
			}
		}
		return tx.WithContext(ctx).Model(&Settings{}).Where("data_source_id = ? AND workspace_id = ?", setting.DataSourceID, setting.WorkspaceID).Updates(map[string]any{
			"target_ids": setting.TargetIDs, "embedding_provider": model.Provider, "embedding_model": model.Model, "pending_run_id": nil, "pending_trigger": nil, "last_error_code": nil, "reconciled_at": time.Now(),
		}).Error
	}); err != nil {
		return err
	}
	setting.EmbeddingProvider, setting.EmbeddingModel = model.Provider, model.Model
	if err := s.retire(ctx, setting, ids); err != nil {
		return err
	}
	var pending []struct {
		text2sql.SchemaIndex
		BuildAttempts int
	}
	if len(ids) == 0 {
		_ = s.finalizeRuns(ctx, setting.DataSourceID)
		return nil
	}
	if err := s.db.WithContext(ctx).Table("text2sql_schema_indexes").Where("id IN ? AND status <> ?", ids, text2sql.IndexReady).Find(&pending).Error; err != nil {
		return err
	}
	for _, row := range pending {
		if row.BuildAttempts >= MaxAttempts {
			// A last-attempt crash can leave indexing behind. Once its lock is
			// released, turn that durable record into an actionable failure.
			if err := s.withLock(ctx, "table:"+row.AssetTableID, func(db *gorm.DB) error {
				return db.WithContext(ctx).Table("text2sql_schema_indexes").Where("id = ? AND status = ?", row.ID, text2sql.IndexIndexing).Updates(map[string]any{"status": text2sql.IndexFailed, "last_error_code": "interrupted"}).Error
			}); err != nil {
				return err
			}
			continue
		}
		payload, err := json.Marshal(taskPayload{row.ID, row.AssetTableID})
		if err != nil {
			return err
		}
		task := asynq.NewTask(s.queue.GetTaskTypeWithPrefix(taskType), payload)
		_, err = s.queue.EnqueueTask(task, asynq.Queue("low"), asynq.Unique(5*time.Minute), asynq.MaxRetry(0), asynq.Timeout(3*time.Minute))
		if err != nil && !errors.Is(err, asynq.ErrDuplicateTask) {
			return err
		}
	}
	return s.finalizeRuns(ctx, setting.DataSourceID)
}

func (s *Service) retire(ctx context.Context, setting Settings, keep []string) error {
	var rows []text2sql.SchemaIndex
	query := s.db.WithContext(ctx).Where("data_source_id = ?", setting.DataSourceID)
	if len(keep) > 0 {
		query = query.Where("id NOT IN ?", keep)
	}
	if err := query.Find(&rows).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if err := s.withLock(ctx, "table:"+row.AssetTableID, func(db *gorm.DB) error {
			if err := db.WithContext(ctx).Model(&text2sql.SchemaIndex{}).Where("id = ?", row.ID).Update("status", text2sql.IndexDeleting).Error; err != nil {
				return err
			}
			if err := s.vectors.DeleteVector(ctx, row.VectorObjectID, row.CollectionName); err != nil {
				return err
			}
			return db.WithContext(ctx).Delete(&row).Error
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) handleTask(ctx context.Context, task *asynq.Task) error {
	var payload taskPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return err
	}
	if payload.IndexID == "" || payload.AssetID == "" {
		return fmt.Errorf("schema index task identifiers required")
	}
	var sourceID string
	err := s.withLock(ctx, "table:"+payload.AssetID, func(db *gorm.DB) error {
		var row struct {
			text2sql.SchemaIndex
			BuildAttempts int
		}
		err := db.WithContext(ctx).Table("text2sql_schema_indexes").Where("id = ? AND asset_table_id = ?", payload.IndexID, payload.AssetID).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		sourceID = row.DataSourceID
		if row.Status == text2sql.IndexReady || row.Status == text2sql.IndexDeleting || row.BuildAttempts >= MaxAttempts {
			return nil
		}
		var setting Settings
		if err := db.WithContext(ctx).Where("data_source_id = ? AND workspace_id = ?", row.DataSourceID, row.WorkspaceID).Take(&setting).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return err
		}
		var ids []string
		if err := json.Unmarshal(setting.TargetIDs, &ids); err != nil {
			return err
		}
		if !slices.Contains(ids, row.ID) {
			return nil
		}
		buildCtx, cancel := context.WithTimeout(ctx, tableTimeout)
		defer cancel()
		if err := db.WithContext(buildCtx).Table("text2sql_schema_indexes").Where("id = ?", row.ID).Update("build_attempts", gorm.Expr("build_attempts + 1")).Error; err != nil {
			return err
		}
		buildErr := s.validateTask(buildCtx, setting, &row.SchemaIndex)
		if buildErr == nil {
			table, tableErr := s.sql.GetTable(buildCtx, row.PhysicalTableID)
			if tableErr != nil {
				s.log.Warn("sample column values skipped", zap.String("index_id", row.ID), zap.Error(tableErr))
				row.ValueSamples = text2sql.EncodeValueSamples(text2sql.ColumnValues{})
			} else {
				sampleCtx, sampleCancel := context.WithTimeout(buildCtx, valueSampleTimeout)
				row.ValueSamples = text2sql.EncodeValueSamples(text2sql.SampleColumnValues(sampleCtx, s.sql, table))
				sampleCancel()
			}
			indexer := text2sql.NewIndexer(text2sql.NewIndexRepository(db), s.sql, embedder{s, setting}, s.vectors)
			buildErr = indexer.Process(buildCtx, &row.SchemaIndex)
		}
		if buildErr == nil {
			buildErr = s.validateTask(buildCtx, setting, &row.SchemaIndex)
		}
		if buildErr != nil {
			s.log.Error("database schema index build failed", zap.String("index_id", row.ID), zap.Error(buildErr))
			code := "build_failed"
			if row.LastErrorCode != nil {
				code = *row.LastErrorCode
			}
			if errors.Is(buildErr, context.DeadlineExceeded) {
				code = "timeout"
			}
			writeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer stop()
			// A revoked permission or invalid model is terminal until manual retry;
			// transient embedding/storage errors retain the bounded retry budget.
			updates := map[string]any{"status": text2sql.IndexFailed, "last_error_code": code, "updated_at": time.Now()}
			if errors.Is(buildErr, errTaskScope) {
				updates["build_attempts"] = MaxAttempts
				updates["last_error_code"] = "scope_changed"
			}
			if err := db.WithContext(writeCtx).Table("text2sql_schema_indexes").Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				return errors.Join(buildErr, err)
			}
		}
		return buildErr
	})
	if sourceID != "" {
		writeCtx, stop := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		finalizeErr := s.finalizeRuns(writeCtx, sourceID)
		stop()
		if finalizeErr != nil {
			return errors.Join(err, finalizeErr)
		}
	}
	return err
}

var errTaskScope = errors.New("schema index task scope changed")

func (s *Service) validateTask(ctx context.Context, setting Settings, row *text2sql.SchemaIndex) error {
	ds, err := s.database(ctx, setting.OrganizationID, setting.DataSourceID)
	if err != nil || *ds.WorkspaceID != setting.WorkspaceID {
		return errors.Join(errTaskScope, err)
	}
	if err := s.authorize(ctx, setting.OrganizationID, setting.DataSourceID, setting.RequestedBy, true); err != nil {
		return errors.Join(errTaskScope, err)
	}
	model, err := s.models.ResolveUseCase(ctx, setting.OrganizationID, llmmodel.UseCaseEmbedding, nil, nil)
	if err != nil || model.Provider != row.EmbeddingProvider || model.Model != row.EmbeddingModel {
		return errors.Join(errTaskScope, err)
	}
	var asset struct{ TableID int }
	if err := s.db.WithContext(ctx).Table("data_source_tables").Where("id = ? AND data_source_id = ?", row.AssetTableID, row.DataSourceID).Take(&asset).Error; err != nil {
		return errors.Join(errTaskScope, err)
	}
	table, err := s.sql.GetTable(ctx, asset.TableID)
	if err != nil {
		return err
	}
	if !text2sql.MatchesSchema(*row, table) {
		return errTaskScope
	}
	return nil
}
