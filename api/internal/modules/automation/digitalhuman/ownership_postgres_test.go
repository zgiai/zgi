package digitalhumantask

import (
	"errors"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestTaskOwnershipPostgresIsolationAndConstraints(t *testing.T) {
	dsn := os.Getenv("ZGI_MIGRATION_TEST_DSN")
	if dsn == "" {
		t.Skip("set ZGI_MIGRATION_TEST_DSN to run DigitalHuman task PostgreSQL integration")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	t.Cleanup(func() { _ = tx.Rollback().Error })

	organizationID, workspaceID, digitalHumanID := uuid.New(), uuid.New(), uuid.New()
	accountA, accountB := uuid.New(), uuid.New()
	if err := tx.Exec("INSERT INTO organizations (id, name) VALUES (?, ?)", organizationID, "Task ownership integration").Error; err != nil {
		t.Fatal(err)
	}
	if err := tx.Exec("INSERT INTO workspaces (id, organization_id, name) VALUES (?, ?, ?)", workspaceID, organizationID, "Task ownership integration").Error; err != nil {
		t.Fatal(err)
	}
	for _, accountID := range []uuid.UUID{accountA, accountB} {
		if err := tx.Exec("INSERT INTO accounts (id, name, email) VALUES (?, ?, ?)", accountID, "Task owner", "task-owner-"+accountID.String()+"@example.invalid").Error; err != nil {
			t.Fatal(err)
		}
		if err := tx.Exec(`INSERT INTO workspace_members
			(id, workspace_id, account_id, role, permissions, permission_source, current)
			VALUES (?, ?, ?, 'member', '[]'::jsonb, 'role_template', false)`, uuid.NewString(), workspaceID, accountID).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Exec(`INSERT INTO digital_humans
		(id, organization_id, workspace_id, name, created_by, updated_by)
		VALUES (?, ?, ?, ?, ?, ?)`, digitalHumanID, organizationID, workspaceID, "Task ownership integration", accountA, accountA).Error; err != nil {
		t.Fatal(err)
	}

	service := NewService(tx, &recordingDispatcher{}, nil)
	base := digitalhuman.TaskAccessContext{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		DigitalHumanID: digitalHumanID,
		Principal:      invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: accountA.String()},
		Surface:        digitalhuman.SurfaceConsole,
	}
	owners := map[string]digitalhuman.TaskOwnerRef{
		"shared":         digitalhuman.SharedTaskOwner(),
		"account A":      digitalhuman.UserTaskOwner(usersubject.AccountSubject(accountA)),
		"account B":      digitalhuman.UserTaskOwner(usersubject.AccountSubject(accountB)),
		"conversation A": digitalhuman.ConversationTaskOwner(uuid.New()),
		"conversation B": digitalhuman.ConversationTaskOwner(uuid.New()),
	}
	created := make(map[string]*digitalhuman.Task, len(owners))
	for name, owner := range owners {
		access := base
		access.Owner = owner
		if owner.Kind == digitalhuman.TaskOwnerConversation {
			access.Surface = digitalhuman.SurfaceChannel
			access.Principal = invocation.PrincipalRef{Type: invocation.PrincipalExternalUser, ID: name}
		}
		task, createErr := service.CreateTask(t.Context(), access, digitalhuman.CreateTaskRequest{
			Name: name, Goal: "Verify owner isolation", Schedule: digitalhuman.TaskSchedule{Type: digitalhuman.TaskScheduleManual},
		}, digitalhuman.TaskMutationContext{ActorType: "account", ActorRef: accountA.String(), OperationID: uuid.New(), Source: access.Surface})
		if createErr != nil {
			t.Fatalf("create %s task: %v", name, createErr)
		}
		created[name] = task
		result, listErr := service.ListTasks(t.Context(), access, digitalhuman.TaskListQuery{})
		if listErr != nil || result.Total != 1 || result.Items[0].ID != task.ID {
			t.Fatalf("%s namespace list = %#v, error = %v", name, result, listErr)
		}
	}

	accountBAccess := base
	accountBAccess.Principal.ID = accountB.String()
	accountBAccess.Owner = owners["account B"]
	if _, err := service.GetTask(t.Context(), accountBAccess, created["account A"].ID); !errors.Is(err, digitalhuman.ErrTaskNotFound) {
		t.Fatalf("account B GetTask(account A) error = %v, want not found", err)
	}
	base.Owner = digitalhuman.SharedTaskOwner()
	if result, err := service.ListTasks(t.Context(), base, digitalhuman.TaskListQuery{}); err != nil || result.Total != 1 || result.Items[0].OwnerKind != digitalhuman.TaskOwnerDigitalHuman {
		t.Fatalf("shared namespace list = %#v, error = %v", result, err)
	}

	genericTask := &automationmodel.AutomationTask{
		OrganizationID: organizationID.String(), WorkspaceID: workspaceID.String(), Name: "Generic automation",
		TaskKind: automationmodel.AutomationTaskKindAutomation, Status: automationmodel.AutomationTaskStatusActive,
		TriggerType: automationmodel.AutomationTriggerTypeSchedule, ScheduleType: automationmodel.AutomationScheduleTypeManual,
		Timezone: "UTC", ScheduleConfig: map[string]interface{}{}, SourceType: automationmodel.AutomationSourceTypeManual,
		CreatedBy: accountA.String(), UpdatedBy: accountA.String(),
	}
	if err := tx.Create(genericTask).Error; err != nil {
		t.Fatalf("create generic automation: %v", err)
	}

	if err := tx.SavePoint("before_invalid_owner").Error; err != nil {
		t.Fatal(err)
	}
	invalid := tx.Exec("UPDATE automation_tasks SET owner_kind = ? WHERE id = ?", automationmodel.AutomationTaskOwnerDigitalHuman, genericTask.ID).Error
	if invalid == nil {
		t.Fatal("database accepted owner fields on a generic automation task")
	}
	if err := tx.RollbackTo("before_invalid_owner").Error; err != nil {
		t.Fatal(err)
	}
}
