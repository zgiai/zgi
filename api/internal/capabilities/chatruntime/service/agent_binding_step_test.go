package service

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestCurrentAgentBindingVerifierUsesDraftIndexForConsolePreview(t *testing.T) {
	db, mock := newAgentBindingStepTestDB(t)
	agentID := uuid.New()
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "agent_resource_bindings" WHERE (agent_id = $1 AND binding_scope = $2 AND binding_type = $3 AND resource_id = $4 AND parent_resource_id = $5) AND published_version_uuid IS NULL AND access_mode IN ($6,$7)`)).
		WithArgs(agentID, "draft", "database_table", "table-1", "database-1", "read", "write").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	svc := &service{repos: &repository.Repositories{DB: db}}
	verifier := svc.currentAgentBindingVerifier(&PreparedChat{Caller: Caller{
		Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceConsole,
	}})
	matched, err := verifier(context.Background(), skills.AgentBindingCheck{
		BindingType: "database_table", ResourceID: "table-1", ParentResourceID: "database-1", AccessMode: "read",
	})
	if err != nil || !matched {
		t.Fatalf("verifier() = %v, %v", matched, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentAgentBindingVerifierUsesCurrentPublishedHead(t *testing.T) {
	db, mock := newAgentBindingStepTestDB(t)
	agentID := uuid.New()
	versionID := uuid.New()
	mock.ExpectQuery(`SELECT "published_version_uuid" FROM "agent_resource_bindings"`).
		WithArgs(agentID, "published", 1).
		WillReturnRows(sqlmock.NewRows([]string{"published_version_uuid"}).AddRow(versionID))
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT count(*) FROM "agent_resource_bindings" WHERE (agent_id = $1 AND binding_scope = $2 AND binding_type = $3 AND resource_id = $4 AND parent_resource_id = $5) AND published_version_uuid = $6 AND access_mode = $7`)).
		WithArgs(agentID, "published", "workflow", "binding-1", "workflow-agent-1", versionID, "execute").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	svc := &service{repos: &repository.Repositories{DB: db}}
	verifier := svc.currentAgentBindingVerifier(&PreparedChat{Caller: Caller{
		Type: runtimemodel.ConversationCallerAgent, ID: &agentID, Source: runtimemodel.ConversationSourceWebApp,
	}})
	matched, err := verifier(context.Background(), skills.AgentBindingCheck{
		BindingType: "workflow", ResourceID: "binding-1", ParentResourceID: "workflow-agent-1", AccessMode: "execute",
	})
	if err != nil || matched {
		t.Fatalf("verifier() = %v, %v", matched, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func newAgentBindingStepTestDB(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherRegexp))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	db, err := gorm.Open(postgres.New(postgres.Config{Conn: sqlDB}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, mock
}
