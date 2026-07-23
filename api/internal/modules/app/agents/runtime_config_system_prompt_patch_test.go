package agents

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestUpdateAgentConfigWithSystemPromptPatchCASRejectsChangedBaseline(t *testing.T) {
	db, mock := newAgentSystemPromptPatchTestDB(t)
	agentID := uuid.New()
	workspaceID := uuid.New()
	configID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-agent:" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "tenant_id" FROM "agents" WHERE id = $1 AND deleted_at IS NULL LIMIT $2`)).
		WithArgs(agentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(workspaceID))
	mock.ExpectQuery(`SELECT \* FROM "agents_configs" WHERE deleted_at IS NULL AND id = \$1.*LIMIT \$2 FOR UPDATE`).
		WithArgs(configID, 1).
		WillReturnRows(agentSystemPromptPatchConfigRows(configID, agentID, "changed while approval waited", now))
	mock.ExpectRollback()

	service := &agentsService{db: db, agentBindings: agentbindings.NewRepositoryWithTokenSecret(db, "prompt-patch-test")}
	_, err := service.updateAgentConfigWithSystemPromptPatchCAS(
		context.Background(),
		&Agent{ID: agentID, TenantID: workspaceID},
		&AgentsConfig{ID: configID, AgentsID: agentID, PrePrompt: stringPtr("approved baseline"), UpdatedAt: now},
		uuid.NewString(),
		dto.AgentSystemPromptPatchRequest{
			Operation:          agentSystemPromptPatchOperationAppend,
			AppendContent:      "new section",
			Separator:          "\n---\n",
			ExpectedBaseSHA256: agentSystemPromptSHA256("approved baseline"),
			RequestedFields:    []string{"system_prompt"},
			Config: dto.AgentConfigRequest{
				BindingRevision: agentBindingRevision(nil),
			},
		},
	)
	var conflict *agentBindingAPIError
	if !errors.As(err, &conflict) || conflict.Code != agentSystemPromptBaseChangedCode {
		t.Fatalf("error = %#v, want %s", err, agentSystemPromptBaseChangedCode)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("prompt patch must reject the changed locked baseline before writing: %v", err)
	}
}

func TestUpdateAgentConfigWithSystemPromptPatchCASAppendsLockedPrompt(t *testing.T) {
	db, mock := newAgentSystemPromptPatchTestDB(t)
	agentID := uuid.New()
	workspaceID := uuid.New()
	configID := uuid.New()
	now := time.Now()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`SELECT pg_advisory_xact_lock(hashtextextended($1, 0))`)).
		WithArgs("agent-binding-agent:" + agentID.String()).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT "tenant_id" FROM "agents" WHERE id = $1 AND deleted_at IS NULL LIMIT $2`)).
		WithArgs(agentID, 1).
		WillReturnRows(sqlmock.NewRows([]string{"tenant_id"}).AddRow(workspaceID))
	mock.ExpectQuery(`SELECT \* FROM "agents_configs" WHERE deleted_at IS NULL AND id = \$1.*LIMIT \$2 FOR UPDATE`).
		WithArgs(configID, 1).
		WillReturnRows(agentSystemPromptPatchConfigRows(configID, agentID, "locked baseline", now))
	mock.ExpectQuery(`SELECT \* FROM "agent_resource_bindings" WHERE \(agent_id = \$1 AND binding_scope = \$2\).*ORDER BY`).
		WithArgs(agentID, agentbindings.ScopeDraft).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec(`UPDATE "agents_configs" SET`).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`DELETE FROM "agent_resource_bindings" WHERE`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectCommit()

	service := &agentsService{db: db, agentBindings: agentbindings.NewRepositoryWithTokenSecret(db, "prompt-patch-test")}
	result, err := service.updateAgentConfigWithSystemPromptPatchCAS(
		context.Background(),
		&Agent{ID: agentID, TenantID: workspaceID},
		&AgentsConfig{ID: configID, AgentsID: agentID, PrePrompt: stringPtr("locked baseline"), UpdatedAt: now},
		uuid.NewString(),
		dto.AgentSystemPromptPatchRequest{
			Operation:          agentSystemPromptPatchOperationAppend,
			AppendContent:      "new section",
			Separator:          "\n---\n",
			ExpectedBaseSHA256: agentSystemPromptSHA256("locked baseline"),
			RequestedFields:    []string{"system_prompt", "home_title"},
			Config: dto.AgentConfigRequest{
				BindingRevision: agentBindingRevision(nil),
				HomeTitle:       "Updated title",
			},
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := result.SystemPrompt, "locked baseline\n---\nnew section"; got != want {
		t.Fatalf("SystemPrompt = %q, want %q", got, want)
	}
	if got := result.HomeTitle; got != "Updated title" {
		t.Fatalf("HomeTitle = %q, want Updated title", got)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("prompt patch transaction expectations: %v", err)
	}
}

func TestAppendAgentSystemPromptPreservesExactContent(t *testing.T) {
	current := "  current\n"
	addition := "\n  managed file body  \n"
	separator := "\n---\n"
	result, err := appendAgentSystemPrompt(current, addition, separator)
	if err != nil {
		t.Fatal(err)
	}
	if want := current + separator + addition; result != want {
		t.Fatalf("appendAgentSystemPrompt() = %q, want exact %q", result, want)
	}
}

func TestUpsertAgentSystemPromptSectionReplacesOnlyManagedSection(t *testing.T) {
	patch := dto.AgentSystemPromptPatchRequest{
		Operation:     agentSystemPromptPatchOperationUpsertSection,
		AppendContent: "first version",
		Separator:     "\n\n",
		SectionID:     "story.context",
		SectionTitle:  "Story context",
	}
	first, err := applyAgentSystemPromptPatch("base instructions", patch)
	if err != nil {
		t.Fatal(err)
	}
	patch.AppendContent = "second version"
	second, err := applyAgentSystemPromptPatch(first, patch)
	if err != nil {
		t.Fatal(err)
	}
	start, end := agentSystemPromptSectionMarkers(patch.SectionID)
	if strings.Count(second, start) != 1 || strings.Count(second, end) != 1 {
		t.Fatalf("managed section markers duplicated: %s", second)
	}
	if strings.Contains(second, "first version") || !strings.Contains(second, "second version") || !strings.Contains(second, "base instructions") {
		t.Fatalf("upsert result lost unrelated content or retained stale section: %s", second)
	}
}

func newAgentSystemPromptPatchTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open() error = %v", err)
	}
	return db, mock
}

func agentSystemPromptPatchConfigRows(configID, agentID uuid.UUID, prompt string, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "agents_id", "pre_prompt", "agent_mode", "configs", "prompt_type", "created_at", "updated_at"}).
		AddRow(configID, agentID, prompt, `{}`, `{}`, "simple", now, now)
}
