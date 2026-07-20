package workflowtest

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	defaultmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/model"
	llmdefaultservice "github.com/zgiai/zgi/api/internal/modules/llm/defaultmodel/service"
	llmmodelmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	llmsharedtypes "github.com/zgiai/zgi/api/internal/modules/llm/shared/types"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"gorm.io/gorm"
)

type stubDefaultModelResolver struct {
	resolved *llmdefaultservice.ResolvedModel
	err      error
	called   bool
	resolve  func(explicitProvider, explicitModel *string) (*llmdefaultservice.ResolvedModel, error)
}

func (s *stubDefaultModelResolver) ResolveModelType(ctx context.Context, organizationID string, explicitProvider, explicitModel *string, modelType sharedmodel.ModelType) (*llmdefaultservice.ResolvedModel, error) {
	return s.ResolveUseCase(ctx, organizationID, llmmodelmodel.UseCaseTextChat, explicitProvider, explicitModel)
}

func (s *stubDefaultModelResolver) ResolveUseCase(ctx context.Context, organizationID string, useCase llmmodelmodel.UseCase, explicitProvider, explicitModel *string) (*llmdefaultservice.ResolvedModel, error) {
	s.called = true
	if s.resolve != nil {
		return s.resolve(explicitProvider, explicitModel)
	}
	return s.resolved, s.err
}

func (s *stubDefaultModelResolver) ListResolved(ctx context.Context, organizationID uuid.UUID) ([]*llmdefaultservice.ResolvedModel, error) {
	return nil, nil
}

func (s *stubDefaultModelResolver) Upsert(ctx context.Context, organizationID uuid.UUID, actorID *uuid.UUID, useCase llmmodelmodel.UseCase, provider string, modelName string, params llmsharedtypes.JSONObject) (*defaultmodelmodel.DefaultModel, error) {
	return nil, nil
}

func (s *stubDefaultModelResolver) Delete(ctx context.Context, organizationID uuid.UUID, useCase llmmodelmodel.UseCase) error {
	return nil
}

func TestResolveBatchJudgeSettingsUsesSavedModel(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	resolver := &stubDefaultModelResolver{resolved: &llmdefaultservice.ResolvedModel{
		UseCase:  string(llmmodelmodel.UseCaseTextChat),
		Provider: "openai",
		Model:    "gpt-4.1",
		Source:   llmdefaultservice.SourceExplicit,
	}}
	service.SetDefaultModelResolver(resolver)
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_settings" WHERE agent_id = $1 ORDER BY "workflow_test_settings"."id" LIMIT $2`)).
		WithArgs("agent-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "agent_id", "judge_prompt_template", "judge_model_provider", "judge_model_name", "created_at", "updated_at",
		}).AddRow("settings-1", "agent-1", "judge", "openai", "gpt-4.1", now, now))
	expectJudgeModelOrganization(mock)

	settings, err := service.resolveBatchJudgeSettings(context.Background(), "agent-1")

	require.NoError(t, err)
	require.Equal(t, "openai", settings.JudgeModelProvider)
	require.Equal(t, "gpt-4.1", settings.JudgeModelName)
	require.True(t, resolver.called)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveBatchJudgeSettingsFallsBackWhenSavedModelIsUnavailable(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	resolver := &stubDefaultModelResolver{
		resolve: func(_ *string, explicitModel *string) (*llmdefaultservice.ResolvedModel, error) {
			if explicitModel != nil {
				require.Equal(t, "qwen3-max", *explicitModel)
				return nil, llmdefaultservice.ErrModelUnavailable
			}
			return &llmdefaultservice.ResolvedModel{
				UseCase:  string(llmmodelmodel.UseCaseTextChat),
				Provider: "qwen",
				Model:    "qwen3.5-omni-plus",
				Source:   llmdefaultservice.SourceExplicit,
			}, nil
		},
	}
	service.SetDefaultModelResolver(resolver)
	now := time.Now()

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_settings" WHERE agent_id = $1 ORDER BY "workflow_test_settings"."id" LIMIT $2`)).
		WithArgs("agent-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "agent_id", "judge_prompt_template", "judge_model_provider", "judge_model_name", "created_at", "updated_at",
		}).AddRow("settings-1", "agent-1", "judge", "qwen", "qwen3-max", now, now))
	expectJudgeModelOrganization(mock)

	settings, err := service.resolveBatchJudgeSettings(context.Background(), "agent-1")

	require.NoError(t, err)
	require.Equal(t, "qwen", settings.JudgeModelProvider)
	require.Equal(t, "qwen3.5-omni-plus", settings.JudgeModelName)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveBatchJudgeSettingsFallsBackToDefaultModel(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	resolver := &stubDefaultModelResolver{
		resolved: &llmdefaultservice.ResolvedModel{
			UseCase:  string(llmmodelmodel.UseCaseTextChat),
			Provider: "qwen",
			Model:    "qwen-flash",
			Source:   llmdefaultservice.SourceAuto,
		},
	}
	service.SetDefaultModelResolver(resolver)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_settings" WHERE agent_id = $1 ORDER BY "workflow_test_settings"."id" LIMIT $2`)).
		WithArgs("agent-1", 1).
		WillReturnError(gorm.ErrRecordNotFound)
	expectJudgeModelOrganization(mock)

	settings, err := service.resolveBatchJudgeSettings(context.Background(), "agent-1")

	require.NoError(t, err)
	require.Equal(t, "qwen", settings.JudgeModelProvider)
	require.Equal(t, "qwen-flash", settings.JudgeModelName)
	require.True(t, resolver.called)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestResolveBatchJudgeSettingsFailsWhenNoDefaultModel(t *testing.T) {
	db, mock, cleanup := newWorkflowTestMockDB(t)
	defer cleanup()
	service := NewService(NewRepository(db))
	resolver := &stubDefaultModelResolver{
		resolved: &llmdefaultservice.ResolvedModel{
			UseCase: string(llmmodelmodel.UseCaseTextChat),
			Source:  llmdefaultservice.SourceNone,
		},
	}
	service.SetDefaultModelResolver(resolver)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT * FROM "workflow_test_settings" WHERE agent_id = $1 ORDER BY "workflow_test_settings"."id" LIMIT $2`)).
		WithArgs("agent-1", 1).
		WillReturnError(gorm.ErrRecordNotFound)
	expectJudgeModelOrganization(mock)

	settings, err := service.resolveBatchJudgeSettings(context.Background(), "agent-1")

	require.Nil(t, settings)
	require.ErrorIs(t, err, ErrJudgeModelNotConfigured)
	require.NoError(t, mock.ExpectationsWereMet())
}

func expectJudgeModelOrganization(mock sqlmock.Sqlmock) {
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT workspaces.organization_id FROM "agents" JOIN workspaces ON workspaces.id = agents.tenant_id WHERE agents.id = $1 AND agents.deleted_at IS NULL LIMIT $2`)).
		WithArgs("agent-1", 1).
		WillReturnRows(sqlmock.NewRows([]string{"organization_id"}).AddRow("org-1"))
}
