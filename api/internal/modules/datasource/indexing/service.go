// Package indexing owns database schema index preparation outside workflow runs.
package indexing

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/text2sql"
	dsmodel "github.com/zgiai/zgi/api/internal/modules/datasource/model"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	defaults "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	workspace "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/queue"
	"github.com/zgiai/zgi/api/pkg/sql_base"
	"github.com/zgiai/zgi/api/pkg/vectordb"
	"go.uber.org/zap"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const MaxAttempts = 3

var ErrAccessDenied = errors.New("schema index access denied")

type workspacePermissionChecker interface {
	CheckWorkspaceOrganizationAnyPermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCodes ...workspace.WorkspacePermissionCode) (bool, error)
}

// Settings exists only after explicit consent to build and maintain indexes.
// TargetIDs identifies the catalog snapshot; historical rows do not count as ready.
type Settings struct {
	DataSourceID      string `gorm:"primaryKey;type:uuid"`
	OrganizationID    string
	WorkspaceID       string
	RequestedBy       string
	EmbeddingProvider string
	EmbeddingModel    string
	TargetIDs         datatypes.JSON
	LastErrorCode     *string
	PendingRunID      *string
	PendingTrigger    *string
	ReconciledAt      *time.Time
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

func (Settings) TableName() string { return "database_schema_index_settings" }

type FailedTable struct {
	Name string `json:"name"`
	Code string `json:"code"`
}
type Status struct {
	Enabled   bool          `json:"enabled"`
	State     string        `json:"state"`
	Provider  string        `json:"provider"`
	Model     string        `json:"model"`
	Total     int           `json:"total"`
	Ready     int           `json:"ready"`
	Failed    []FailedTable `json:"failed"`
	ErrorCode string        `json:"error_code,omitempty"`
	UpdatedAt *time.Time    `json:"updated_at"`
}

type Service struct {
	db      *gorm.DB
	sql     sql_base.SQLBase
	models  defaults.DefaultModelService
	llm     llmclient.LLMClient
	vectors vectordb.VectorDB
	queue   *queue.TaskManager
	access  workspacePermissionChecker
	log     *zap.Logger
}

func NewService(db *gorm.DB, sql sql_base.SQLBase, models defaults.DefaultModelService, llm llmclient.LLMClient, vectors vectordb.VectorDB, tasks *queue.TaskManager, access workspacePermissionChecker, log *zap.Logger) *Service {
	return &Service{db: db, sql: sql, models: models, llm: llm, vectors: vectors, queue: tasks, access: access, log: log}
}

func (s *Service) database(ctx context.Context, org, id string) (*dsmodel.DataSource, error) {
	var ds dsmodel.DataSource
	if err := s.db.WithContext(ctx).Where("id = ? AND organization_id = ?", id, org).Take(&ds).Error; err != nil {
		return nil, err
	}
	if ds.WorkspaceID == nil || *ds.WorkspaceID == "" {
		return nil, fmt.Errorf("database has no workspace")
	}
	return &ds, nil
}

func (s *Service) authorizedDatabase(ctx context.Context, org, id, account string, manage bool) (*dsmodel.DataSource, error) {
	ds, err := s.database(ctx, org, id)
	if err != nil {
		return nil, err
	}
	if err := s.authorizeWorkspace(ctx, org, *ds.WorkspaceID, account, manage); err != nil {
		return nil, err
	}
	return ds, nil
}

func (s *Service) authorizeWorkspace(ctx context.Context, org, workspaceID, account string, manage bool) error {
	if s.access == nil {
		return ErrAccessDenied
	}
	permissions := []workspace.WorkspacePermissionCode{
		workspace.WorkspacePermissionDatabaseSchemaView,
		workspace.WorkspacePermissionDatabaseSchemaManage,
	}
	if manage {
		permissions = permissions[1:]
	}
	allowed, err := s.access.CheckWorkspaceOrganizationAnyPermission(ctx, org, workspaceID, account, permissions...)
	if err != nil {
		return fmt.Errorf("check schema index permission: %w", err)
	}
	if !allowed {
		return ErrAccessDenied
	}
	return nil
}

func (s *Service) Status(ctx context.Context, org, id, account string) (*Status, error) {
	ds, err := s.authorizedDatabase(ctx, org, id, account, false)
	if err != nil {
		return nil, err
	}
	result := &Status{State: "disabled", Failed: []FailedTable{}}
	var count int64
	if err := s.db.WithContext(ctx).Table("data_source_tables").Where("organization_id = ? AND data_source_id = ?", org, id).Count(&count).Error; err != nil {
		return nil, err
	}
	result.Total = int(count)
	var settings Settings
	err = s.db.WithContext(ctx).Where("data_source_id = ? AND workspace_id = ?", id, *ds.WorkspaceID).Take(&settings).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		model, resolveErr := s.models.ResolveUseCase(ctx, org, llmmodel.UseCaseEmbedding, nil, nil)
		if resolveErr != nil {
			result.ErrorCode = "model_unavailable"
		} else {
			result.Provider, result.Model = model.Provider, model.Model
		}
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	result.Enabled, result.State = true, "building"
	result.Provider, result.Model, result.UpdatedAt = settings.EmbeddingProvider, settings.EmbeddingModel, settings.ReconciledAt
	if settings.LastErrorCode != nil {
		result.ErrorCode = *settings.LastErrorCode
		result.State = "failed"
	}
	var ids []string
	if err := json.Unmarshal(settings.TargetIDs, &ids); err != nil {
		return nil, err
	}
	var rows []text2sql.SchemaIndex
	if len(ids) > 0 {
		if err := s.db.WithContext(ctx).Where("id IN ?", ids).Find(&rows).Error; err != nil {
			return nil, err
		}
	}
	for _, row := range rows {
		if row.Status == text2sql.IndexReady {
			result.Ready++
		}
		if row.Status == text2sql.IndexFailed {
			code := "build_failed"
			if row.LastErrorCode != nil {
				code = *row.LastErrorCode
			}
			result.Failed = append(result.Failed, FailedTable{row.PhysicalTableName, code})
		}
	}
	if len(result.Failed) > 0 {
		result.State = "failed"
	}
	if result.ErrorCode == "" && result.Total > 0 && result.Ready == result.Total && len(ids) == result.Total {
		result.State = "ready"
	}
	if result.Total == 0 {
		result.State = "empty"
	}
	return result, nil
}

// Start records consent durably. The reconciler submits work even if Redis is down
// at request time. Calling Start again retries failures, without rebuilding ready rows.
func (s *Service) Start(ctx context.Context, org, id, account string) error {
	ds, err := s.authorizedDatabase(ctx, org, id, account, true)
	if err != nil {
		return err
	}
	model, err := s.models.ResolveUseCase(ctx, org, llmmodel.UseCaseEmbedding, nil, nil)
	if err != nil {
		return fmt.Errorf("resolve embedding model: %w", err)
	}
	return s.withLock(ctx, "database:"+id, func(db *gorm.DB) error {
		var existing Settings
		existingErr := db.WithContext(ctx).Where("data_source_id = ?", id).Take(&existing).Error
		if existingErr != nil && !errors.Is(existingErr, gorm.ErrRecordNotFound) {
			return existingErr
		}
		trigger := "initial"
		if existingErr == nil {
			trigger = "manual_retry"
		}
		runID := uuid.NewString()
		settings := Settings{DataSourceID: id, OrganizationID: org, WorkspaceID: *ds.WorkspaceID, RequestedBy: account, EmbeddingProvider: model.Provider, EmbeddingModel: model.Model, TargetIDs: datatypes.JSON("[]")}
		settings.PendingRunID, settings.PendingTrigger = &runID, &trigger
		if err := db.WithContext(ctx).Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "data_source_id"}}, DoUpdates: clause.Assignments(map[string]any{"requested_by": account, "workspace_id": *ds.WorkspaceID, "embedding_provider": model.Provider, "embedding_model": model.Model, "pending_run_id": runID, "pending_trigger": trigger, "last_error_code": nil, "reconciled_at": nil, "updated_at": time.Now()})}).Create(&settings).Error; err != nil {
			return err
		}
		return db.WithContext(ctx).Table("text2sql_schema_indexes").Where("data_source_id = ? AND workspace_id = ? AND status = ?", id, *ds.WorkspaceID, text2sql.IndexFailed).Updates(map[string]any{"build_attempts": 0, "status": text2sql.IndexPending, "last_error_code": nil}).Error
	})
}

// Session advisory locks serialize a table's build and deletion across API
// instances. A process crash releases the lock; no in-memory owner survives it.
func (s *Service) withLock(ctx context.Context, key string, run func(*gorm.DB) error) error {
	return s.db.WithContext(ctx).Connection(func(db *gorm.DB) error {
		discard := func() {
			if conn, ok := db.Statement.ConnPool.(*sql.Conn); ok {
				_ = conn.Raw(func(any) error { return driver.ErrBadConn })
			}
		}
		if err := db.Exec("SELECT pg_advisory_lock(hashtextextended(?, 0))", "schema-index:"+key).Error; err != nil {
			discard()
			return err
		}
		defer func() {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			if err := db.WithContext(cleanup).Exec("SELECT pg_advisory_unlock(hashtextextended(?, 0))", "schema-index:"+key).Error; err != nil {
				discard()
				s.log.Error("release schema index lock", zap.Error(err))
			}
		}()
		return run(db)
	})
}

type embedder struct {
	service  *Service
	settings Settings
}

func (e embedder) Embed(ctx context.Context, provider, model, input string) ([]float64, error) {
	if provider != e.settings.EmbeddingProvider || model != e.settings.EmbeddingModel {
		return nil, text2sql.ErrIndexNotReady
	}
	result, err := e.service.llm.AppEmbed(ctx, &llmclient.AppContext{
		OrganizationID: e.settings.OrganizationID, WorkspaceID: e.settings.WorkspaceID, AccountID: e.settings.RequestedBy,
		BillingSubjectType: llmclient.BillingSubjectTypeWorkspace, AppID: e.settings.DataSourceID, AppType: "database_schema_index", ModelUseCase: string(llmmodel.UseCaseEmbedding), SuppressInvocationContent: true,
	}, &adapter.EmbeddingsRequest{Model: model, Input: input, EncodingFormat: "float"})
	if err != nil {
		return nil, err
	}
	if result == nil || len(result.Data) != 1 || len(result.Data[0].Embedding) == 0 {
		return nil, fmt.Errorf("empty embedding response")
	}
	vector := make([]float64, len(result.Data[0].Embedding))
	for i, v := range result.Data[0].Embedding {
		vector[i] = float64(v)
	}
	return vector, nil
}
