package digitalhuman

import (
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"gorm.io/datatypes"
)

func backgroundAuthorizationFixture(t *testing.T, surface string) (*AccessService, *Run, invocation.Event) {
	t.Helper()
	run, _, event, _ := backgroundLinuxFixture(t, surface)
	_, repo := newTestService(t)
	db := repo.db
	if err := db.AutoMigrate(&channelruntime.Installation{}, &channelruntime.ConversationMapping{}, &channelruntime.Binding{}, &automationmodel.AutomationTask{}, &automationmodel.AutomationTaskRun{}, &automationmodel.AutomationActionRun{}, &usersubject.Subject{}); err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{
		`CREATE TABLE accounts(id TEXT PRIMARY KEY,status TEXT,deleted_at DATETIME)`,
		`CREATE TABLE workspace_members(account_id TEXT,workspace_id TEXT)`,
		`CREATE TABLE workspaces(id TEXT PRIMARY KEY,organization_id TEXT,status TEXT)`,
		`CREATE TABLE published_runtime_surfaces(id TEXT PRIMARY KEY,resource_type TEXT,resource_id TEXT,organization_id TEXT,workspace_id TEXT,surface TEXT,enabled BOOLEAN,compatibility_source TEXT,created_at DATETIME,updated_at DATETIME,deleted_at DATETIME)`,
		`CREATE TABLE published_runtime_surface_grants(id TEXT PRIMARY KEY,surface_id TEXT,subject_type TEXT,subject_id TEXT,enabled BOOLEAN,created_at DATETIME,updated_at DATETIME,deleted_at DATETIME)`,
	} {
		if err := db.Exec(sql).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, record := range []any{
		&Definition{ID: run.DigitalHumanID, OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, Name: "background", Status: StatusActive, PublishedVersionID: &run.DigitalHumanVersionID},
		&Version{ID: run.DigitalHumanVersionID, DigitalHumanID: run.DigitalHumanID, OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, State: VersionStatePublished,
			ExecutionRuntime: &ExecutionRuntimeConfig{Enabled: true}, Goals: datatypes.JSON(`[]`), Rules: datatypes.JSON(`[]`), MemoryPolicy: datatypes.JSON(`{}`), TriggerPolicy: datatypes.JSON(`{}`), PresentationPolicy: datatypes.JSON(`{}`)},
	} {
		if err := db.Create(record).Error; err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO accounts(id,status) VALUES (?,?)`, []any{*run.ActorID, "active"}},
		{`INSERT INTO workspace_members(account_id,workspace_id) VALUES (?,?)`, []any{*run.ActorID, run.WorkspaceID}},
		{`INSERT INTO workspaces(id,organization_id,status) VALUES (?,?,?)`, []any{run.WorkspaceID, run.OrganizationID, "normal"}},
	} {
		if err := db.Exec(query.sql, query.args...).Error; err != nil {
			t.Fatal(err)
		}
	}
	if surface == SurfaceChannel {
		installation := uuid.MustParse(event.Origin.InstallationID)
		binding := uuid.MustParse(event.Context["channel_binding_id"].(string))
		mapping := uuid.New()
		for _, record := range []any{
			&channelruntime.Installation{ID: installation, OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, Provider: event.Origin.Provider, Status: channelruntime.InstallationStatusActive, ConnectionID: installation, CreatedBy: *run.ActorID},
			&channelruntime.ConversationMapping{ID: mapping, OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, InstallationID: installation, ExternalConversationID: event.ReplyTo.ConversationID, LocalConversationID: run.ConversationID},
			&channelruntime.Binding{ID: binding, OrganizationID: run.OrganizationID, WorkspaceID: run.WorkspaceID, InstallationID: installation, ConversationMappingID: mapping, SubjectType: event.Target.Type, SubjectID: run.DigitalHumanID, Enabled: true},
		} {
			if err := db.Create(record).Error; err != nil {
				t.Fatal(err)
			}
		}
	} else {
		app, kind, runID := run.DigitalHumanID.String(), automationmodel.AutomationTaskOwnerDigitalHuman, run.ID.String()
		for _, record := range []any{
			&automationmodel.AutomationTask{ID: event.Context["task_id"].(string), OrganizationID: run.OrganizationID.String(), WorkspaceID: run.WorkspaceID.String(), SubjectID: &app,
				TaskKind: automationmodel.AutomationTaskKindDigitalHuman, OwnerKind: &kind, CreatedBy: run.ActorID.String(), Status: automationmodel.AutomationTaskStatusActive},
			&automationmodel.AutomationTaskRun{ID: event.Context["task_run_id"].(string), TaskID: event.Context["task_id"].(string), Status: automationmodel.AutomationTaskRunStatusRunning},
			&automationmodel.AutomationActionRun{ID: event.Context["action_run_id"].(string), TaskRunID: event.Context["task_run_id"].(string), TaskActionID: uuid.NewString(), ActionType: automationmodel.AutomationActionTypeRunDigitalHuman, Status: automationmodel.AutomationActionRunStatusRunning, ExternalRunID: &runID},
		} {
			if err := db.Create(record).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	return NewAccessService(db), run, event
}

func TestBackgroundAuthorizationRechecksLivePublicationRouteAndTask(t *testing.T) {
	for _, surface := range []string{SurfaceChannel, SurfaceTask} {
		t.Run(surface, func(t *testing.T) {
			s, run, event := backgroundAuthorizationFixture(t, surface)
			if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err != nil {
				t.Fatal(err)
			}
			// The billing account is an authorization principal, not the owner of
			// the sandbox; revocation still stops newly requested operations.
			if err := s.db.Exec(`UPDATE accounts SET status='frozen'`).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
				t.Fatal("revoked account continued")
			}
			// The administrator created during setup stays pending yet signs in.
			if err := s.db.Exec(`UPDATE accounts SET status='pending'`).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err != nil {
				t.Fatalf("pending account: %v", err)
			}
			if err := s.db.Exec(`UPDATE accounts SET status='active'`).Error; err != nil {
				t.Fatal(err)
			}
			if surface == SurfaceChannel {
				if err := s.db.Model(&channelruntime.Binding{}).Where("id = ?", event.Context["channel_binding_id"]).Update("enabled", false).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
					t.Fatal("disabled binding continued")
				}
				if err := s.db.Model(&channelruntime.Binding{}).Where("id = ?", event.Context["channel_binding_id"]).Update("enabled", true).Error; err != nil {
					t.Fatal(err)
				}
				foreign := *run
				foreign.ConversationID = uuidPointer(uuid.New())
				if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), &foreign, event); err == nil {
					t.Fatal("foreign mapping continued")
				}
			} else {
				if err := s.db.Model(&automationmodel.AutomationActionRun{}).Where("id = ?", event.ID).Update("external_run_id", uuid.NewString()).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
					t.Fatal("another action run continued")
				}
				if err := s.db.Model(&automationmodel.AutomationActionRun{}).Where("id = ?", event.ID).Update("external_run_id", run.ID.String()).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", event.Context["task_id"]).Update("work_status", automationmodel.AutomationWorkStatusCanceled).Error; err != nil {
					t.Fatal(err)
				}
				if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
					t.Fatal("cancelled task continued")
				}
				if err := s.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", event.Context["task_id"]).Update("work_status", nil).Error; err != nil {
					t.Fatal(err)
				}
			}
			if err := s.db.Model(&Version{}).Where("id = ?", run.DigitalHumanVersionID).Update("execution_runtime", nil).Error; err != nil {
				t.Fatal(err)
			}
			if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
				t.Fatal("disabled publication continued")
			}
		})
	}
}

func TestBackgroundConnectorAuthorizationDoesNotRequireLinuxRuntime(t *testing.T) {
	s, run, event := backgroundAuthorizationFixture(t, SurfaceTask)
	if err := s.db.Model(&Version{}).Where("id = ?", run.DigitalHumanVersionID).Update("execution_runtime", nil).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBackgroundConnectorExecution(t.Context(), run, event); err != nil {
		t.Fatalf("connector authorization inherited Linux requirement: %v", err)
	}
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
		t.Fatal("Linux execution accepted a publication without Linux enabled")
	}
}

func TestBackgroundConnectorAuthorizationRejectsSupersededPublication(t *testing.T) {
	s, run, event := backgroundAuthorizationFixture(t, SurfaceTask)
	var current Version
	if err := s.db.Where("id = ?", run.DigitalHumanVersionID).Take(&current).Error; err != nil {
		t.Fatal(err)
	}
	current.ID = uuid.New()
	current.VersionNumber++
	if err := s.db.Create(&current).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Model(&Definition{}).Where("id = ?", run.DigitalHumanID).Update("published_version_id", current.ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBackgroundConnectorExecution(t.Context(), run, event); err == nil {
		t.Fatal("superseded published version retained background authority")
	}
}

func TestPrivateScheduledAPIUserRequiresApplicationIssuer(t *testing.T) {
	s, run, event := backgroundAuthorizationFixture(t, SurfaceTask)
	user := uuid.New()
	run.UserSubjectID = &user
	run.UserSubjectKind = string(usersubject.APIEndUser)
	event.Context["task_owner_kind"] = string(TaskOwnerUserSubject)
	if err := s.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", event.Context["task_id"]).Updates(map[string]any{"owner_kind": automationmodel.AutomationTaskOwnerUserSubject, "owner_subject_id": user.String(), "owner_subject_kind": run.UserSubjectKind}).Error; err != nil {
		t.Fatal(err)
	}
	subject := usersubject.Subject{ID: user, Kind: usersubject.APIEndUser, IssuerType: "digital_human_api", IssuerID: &run.DigitalHumanID}
	if err := s.db.Create(&subject).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err != nil {
		t.Fatal(err)
	}
	if err := s.db.Model(&usersubject.Subject{}).Where("id = ?", user).Update("issuer_id", uuid.New()).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
		t.Fatal("another application's API user accepted")
	}
}

func TestScheduledTaskAuthorizesPersistedSystemPrincipalWithoutActor(t *testing.T) {
	s, run, event := backgroundAuthorizationFixture(t, SurfaceTask)
	creator := *run.ActorID
	run.ActorID = nil // Production SubmitInvocation omits the system actor.
	run.PrincipalID = &creator
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err != nil {
		t.Fatalf("confirmed scheduler principal denied: %v", err)
	}
	foreign := uuid.New()
	run.PrincipalID = &foreign
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
		t.Fatal("unrelated billing principal authorized task execution")
	}
	run.PrincipalID = nil
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
		t.Fatal("missing scheduler principal fell back to task creator")
	}
}

func TestOnceTaskCompletedScheduleStillRequiresActiveCorrelatedAction(t *testing.T) {
	s, run, event := backgroundAuthorizationFixture(t, SurfaceTask)
	taskID := event.Context["task_id"]
	if err := s.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status": automationmodel.AutomationTaskStatusCompleted, "schedule_type": automationmodel.AutomationScheduleTypeOnce,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err != nil {
		t.Fatal(err)
	}
	for _, status := range []automationmodel.AutomationTaskStatus{automationmodel.AutomationTaskStatusPaused, automationmodel.AutomationTaskStatusArchived} {
		if err := s.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", taskID).Update("status", status).Error; err != nil {
			t.Fatal(err)
		}
		if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
			t.Fatalf("%s task executed", status)
		}
	}
	if err := s.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", taskID).Updates(map[string]any{
		"status": automationmodel.AutomationTaskStatusCompleted, "schedule_type": automationmodel.AutomationScheduleTypeManual,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
		t.Fatal("completed manual task executed")
	}
	if err := s.db.Model(&automationmodel.AutomationTask{}).Where("id = ?", taskID).Update("schedule_type", automationmodel.AutomationScheduleTypeOnce).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Model(&automationmodel.AutomationActionRun{}).Where("id = ?", event.ID).Update("status", automationmodel.AutomationActionRunStatusSucceeded).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.AuthorizeBackgroundLinuxExecution(t.Context(), run, event); err == nil {
		t.Fatal("finished action executed again")
	}
}
