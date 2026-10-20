package container

import (
	"context"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/workjournal"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	connectorcore "github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorexecution "github.com/zgiai/zgi/api/internal/modules/connectors/execution"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	journaltools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/workjournal"
	"gorm.io/datatypes"
	"testing"
)

func TestGenericJournalRestoresOrdinaryTaskWithoutSocialSetup(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	org, workspace, owner, employee, version := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	conv, msg, runID, taskID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	event := backgroundTaskEvent(t, org, workspace, employee, taskID, uuid.New(), uuid.New())
	raw, _ := json.Marshal(event)
	run := digitalhuman.Run{ID: runID, OrganizationID: org, WorkspaceID: workspace, DigitalHumanID: employee, DigitalHumanVersionID: version, Surface: digitalhuman.SurfaceTask, Status: digitalhuman.RunStatusRunning, ConversationID: &conv, MessageID: &msg, InvocationEvent: raw, RunSpec: datatypes.JSON(`{}`), ExecutionSnapshot: datatypes.JSON(`{}`), ExecutionTrace: datatypes.JSON(`{}`)}
	task := automationmodel.AutomationTask{ID: taskID.String(), OrganizationID: org.String(), WorkspaceID: workspace.String(), SubjectID: stringPointer(employee.String()), TaskKind: automationmodel.AutomationTaskKindDigitalHuman, CreatedBy: owner.String(), UpdatedBy: owner.String(), Status: automationmodel.AutomationTaskStatusActive}
	if e := db.Create(&run).Error; e != nil {
		t.Fatal(e)
	}
	if e := db.Create(&task).Error; e != nil {
		t.Fatal(e)
	}
	c := &ServiceContainer{db: db, backgroundTaskRunResolver: NewBackgroundTaskRunResolver(db, &backgroundAccessStub{})}
	identity := journaltools.InvocationIdentity{Runtime: &tools.ToolRuntime{TenantID: org.String(), InvokeFrom: tools.ToolInvokeFromDigitalHuman}, ActorID: owner.String(), ConversationID: conv.String(), EmployeeID: employee.String(), MessageID: msg.String()}
	scope, restored, e := (&workJournalRuntime{container: c}).restore(t.Context(), identity)
	if e != nil || scope != (workjournal.Scope{OrganizationID: org, WorkspaceID: workspace, OwnerID: owner, EmployeeID: employee}) || restored.SocialOperations {
		t.Fatal(scope, restored, e)
	}
	identity.ActorID = uuid.NewString()
	if _, _, e = (&workJournalRuntime{container: c}).restore(t.Context(), identity); e == nil {
		t.Fatal("accepted foreign actor")
	}
}

func TestGenericJournalClaimIsBoundToOperationInputAndScope(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	if err := db.AutoMigrate(&workjournal.Operation{}); err != nil {
		t.Fatal(err)
	}
	org, ws, owner, employee, version := uuid.New(), uuid.New(), uuid.New(), uuid.New(), uuid.New()
	scope := workjournal.Scope{OrganizationID: org, WorkspaceID: ws, OwnerID: owner, EmployeeID: employee}
	service, _ := workjournal.New(db, nil)
	op, err := service.Prepare(t.Context(), scope, workjournal.PrepareRequest{WorkKey: "ticket-42/reply-1", EmployeeVersionID: version, ConnectorID: "gmail", ConnectionID: uuid.New(), OperationID: "gmail.message.send", Input: datatypes.JSON(`{"to":["customer@example.com"],"text":"reviewed answer"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err = db.Model(&workjournal.Operation{}).Where("id = ?", op.ID).Updates(map[string]any{"state": workjournal.StateExecuting, "attempts": 1}).Error; err != nil {
		t.Fatal(err)
	}
	input := map[string]any{"to": []string{"customer@example.com"}, "text": "reviewed answer"}
	digest, _ := backgroundInputDigest(input)
	restored := ActiveBackgroundTaskRun{WorkspaceID: ws, OwnerID: owner, Run: digitalhuman.Run{OrganizationID: org, WorkspaceID: ws, DigitalHumanID: employee, DigitalHumanVersionID: version}}
	authorizer := &ConnectorBackgroundAuthorization{resolver: &BackgroundTaskRunResolver{db: db}}
	request := connectorexecution.BackgroundAuthorizationRequest{Request: connectorexecution.ExecutionRequest{OrganizationID: org, OwnerAccountID: owner, ConnectorID: "gmail", OperationID: "gmail.message.send", ConnectionID: op.ConnectionID, LogicalOperationID: op.LogicalID()}, Connection: connectorcore.PersonalConnection{ID: op.ConnectionID}}
	ctx := context.WithValue(t.Context(), workJournalClaimContextKey{}, workJournalClaim{OperationID: op.ID, InputDigest: digest})
	if err = authorizer.authorizeWorkJournalClaim(ctx, restored, request, input); err != nil {
		t.Fatal(err)
	}
	for name, change := range map[string]func(){"arguments": func() { input["text"] = "changed" }, "logical key": func() { request.Request.LogicalOperationID = "other" }, "owner": func() { restored.OwnerID = uuid.New() }, "version": func() { restored.Run.DigitalHumanVersionID = uuid.New() }} {
		t.Run(name, func(t *testing.T) {
			originalInput := input["text"]
			oldRequest := request
			oldRestored := restored
			change()
			if err := authorizer.authorizeWorkJournalClaim(ctx, restored, request, input); err == nil {
				t.Fatal("accepted forged claim")
			}
			input["text"] = originalInput
			request = oldRequest
			restored = oldRestored
		})
	}
	if err = db.Model(&workjournal.Operation{}).Where("id = ?", op.ID).Updates(map[string]any{"state": workjournal.StateSucceeded, "unresolved_fingerprint": nil}).Error; err != nil {
		t.Fatal(err)
	}
	if err = authorizer.authorizeWorkJournalClaim(ctx, restored, request, input); err == nil {
		t.Fatal("reused completed operation claim")
	}
}

func TestWorkJournalManagementKeepsOwnerScopeWithoutSocialOrPublishedConfiguration(t *testing.T) {
	db := backgroundAuthorizationTestDB(t)
	store, err := workjournal.New(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	org, ws, owner, employee := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	definition := &digitalhuman.Definition{ID: employee, OrganizationID: org, WorkspaceID: ws}
	manager := &WorkJournalManagement{employees: backgroundEmployeeReaderStub{detail: &digitalhuman.Detail{Definition: definition}}, service: func() (*workjournal.Service, error) { return store, nil }}
	scope := digitalhuman.Scope{OrganizationID: org, WorkspaceID: ws, ActorID: owner}
	got, journalScope, err := manager.ForEmployee(t.Context(), scope, employee)
	if err != nil || got != store || journalScope.OwnerID != owner || journalScope.EmployeeID != employee {
		t.Fatal(journalScope, err)
	}
	for _, field := range []string{"workspace", "organization", "employee"} {
		t.Run(field, func(t *testing.T) {
			copy := *definition
			switch field {
			case "workspace":
				definition.WorkspaceID = uuid.New()
			case "organization":
				definition.OrganizationID = uuid.New()
			case "employee":
				definition.ID = uuid.New()
			}
			if _, _, err := manager.ForEmployee(t.Context(), scope, employee); err == nil {
				t.Fatal("accepted other scope")
			}
			*definition = copy
		})
	}
}
