package socialops

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	digitalhumantask "github.com/zgiai/zgi/api/internal/modules/automation/digitalhuman"
	automationmodel "github.com/zgiai/zgi/api/internal/modules/automation/model"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

type provisioningPreflightStub struct {
	input PreflightInput
	err   error
	calls int
}

func (s *provisioningPreflightStub) RestoreSocialOperationsPreflight(_ context.Context, _ Configuration) (PreflightInput, error) {
	s.calls++
	return s.input, s.err
}

type provisioningTaskStub struct {
	tasks    map[uuid.UUID]*digitalhuman.Task
	calls    []digitalhuman.TaskMutationContext
	failOnce int
	pauses   int
}

func (s *provisioningTaskStub) PauseSupersededSocialOperations(context.Context, digitalhuman.TaskAccessContext, uuid.UUID, int64) error {
	s.pauses++
	return nil
}

func (s *provisioningTaskStub) CreateTask(_ context.Context, _ digitalhuman.TaskAccessContext, _ digitalhuman.CreateTaskRequest, mutation digitalhuman.TaskMutationContext) (*digitalhuman.Task, error) {
	s.calls = append(s.calls, mutation)
	if s.failOnce > 0 && len(s.calls) == s.failOnce {
		s.failOnce = 0
		return nil, errors.New("temporary task store failure")
	}
	if task := s.tasks[mutation.OperationID]; task != nil {
		return task, nil
	}
	task := &digitalhuman.Task{ID: uuid.New(), WorkStatus: digitalhuman.TaskWorkStatusPending, LifecycleStatus: string(automationmodel.AutomationTaskStatusActive)}
	s.tasks[mutation.OperationID] = task
	return task, nil
}

type provisioningConfigurationLoaderStub struct {
	configuration Configuration
	record        Record
	calls         int
	changeAt      int
}

func (s *provisioningConfigurationLoaderStub) LoadCurrentConfiguration(_ context.Context, _ Scope) (Configuration, Record, error) {
	s.calls++
	configuration := s.configuration
	record := s.record
	if s.changeAt > 0 && s.calls >= s.changeAt {
		configuration.Revision++
		record.Revision++
	}
	return configuration, record, nil
}

func readyProvisioningInput(t *testing.T, configuration Configuration) PreflightInput {
	t.Helper()
	social := restoredConnection(t, configuration.Scope, configuration.Platforms[0].ConnectorID, configuration.Platforms[0].ConnectionID, configuration.Platforms[0].AccountID)
	team := restoredConnection(t, configuration.Scope, configuration.TeamHandoff.ConnectorID, configuration.TeamHandoff.ConnectionID)
	team.KnownDestinations = []string{configuration.TeamHandoff.Destination}
	return PreflightInput{Scope: configuration.Scope, PublishedEmployeeVersionID: configuration.EmployeeVersionID,
		ConfigurationRevision: configuration.Revision, Connections: []RestoredConnection{social, team}}
}

func provisioningAccess(scope Scope) digitalhuman.TaskAccessContext {
	return digitalhuman.TaskAccessContext{
		OrganizationID: scope.OrganizationID, WorkspaceID: scope.WorkspaceID, DigitalHumanID: scope.EmployeeID,
		Principal: invocation.PrincipalRef{Type: invocation.PrincipalAccount, ID: scope.OwnerID.String()},
		Owner:     digitalhuman.SharedTaskOwner(), Surface: digitalhuman.SurfaceConsole,
	}
}

func TestProvisioningRequiresReadyCurrentConfigurationAndExplicitOwnerAccess(t *testing.T) {
	configuration := validConfiguration("x-social", RequirementOptional)
	loader := &provisioningConfigurationLoaderStub{configuration: configuration, record: Record{ID: uuid.New(), Scope: configuration.Scope, Kind: CurrentConfigurationRecordKind, Key: CurrentConfigurationRecordKey, Revision: configuration.Revision, ArtifactID: uuid.New()}}
	preflight := &provisioningPreflightStub{input: readyProvisioningInput(t, configuration)}
	tasks := &provisioningTaskStub{tasks: map[uuid.UUID]*digitalhuman.Task{}}
	service, err := NewProvisioningService(loader, preflight, tasks)
	if err != nil {
		t.Fatal(err)
	}
	request := ProvisioningRequest{Scope: configuration.Scope, ExpectedEmployeeVersionID: configuration.EmployeeVersionID,
		ExpectedConfigurationRevision: configuration.Revision, Access: provisioningAccess(configuration.Scope)}

	result, err := service.Provision(t.Context(), request)
	if err != nil || len(result.Tasks) != 4 || len(tasks.tasks) != 4 || tasks.pauses != 1 {
		t.Fatalf("Provision() tasks = %#v, calls = %d, error = %v", result.Tasks, len(tasks.tasks), err)
	}
	wantKeys := []string{"daily_content", "frequent_inbox", "daily_seven_day_samples", "weekly_review"}
	seenOperations := map[uuid.UUID]bool{}
	for index, item := range result.Tasks {
		if item.Key != wantKeys[index] || item.OperationID == uuid.Nil || seenOperations[item.OperationID] {
			t.Fatalf("provisioned task[%d] = %#v", index, item)
		}
		seenOperations[item.OperationID] = true
		metadata := tasks.calls[index].SocialOperations
		if metadata == nil || metadata.Scope.OrganizationID != configuration.Scope.OrganizationID || metadata.Scope.OwnerID != configuration.Scope.OwnerID ||
			metadata.EmployeeVersionID != configuration.EmployeeVersionID || metadata.ConfigurationRevision != configuration.Revision || metadata.TaskKey != item.Key || tasks.calls[index].Proposed {
			t.Fatalf("mutation[%d] = %#v", index, tasks.calls[index])
		}
	}

	for name, mutate := range map[string]func(*ProvisioningRequest){
		"owner":    func(r *ProvisioningRequest) { r.Access.Principal.ID = uuid.NewString() },
		"revision": func(r *ProvisioningRequest) { r.ExpectedConfigurationRevision++ },
		"version":  func(r *ProvisioningRequest) { r.ExpectedEmployeeVersionID = uuid.New() },
	} {
		t.Run(name, func(t *testing.T) {
			before := len(tasks.calls)
			changed := request
			mutate(&changed)
			if _, err := service.Provision(t.Context(), changed); err == nil || len(tasks.calls) != before {
				t.Fatalf("stale or unauthorized provisioning = %v, calls %d -> %d", err, before, len(tasks.calls))
			}
		})
	}

	preflight.input.Connections[0].GrantedOperations = nil
	before := len(tasks.calls)
	if _, err := service.Provision(t.Context(), request); !errors.Is(err, ErrProvisioningNotReady) || len(tasks.calls) != before {
		t.Fatalf("unready provisioning error = %v, calls %d -> %d", err, before, len(tasks.calls))
	}
}

func TestProvisioningRetriesPartialFailureWithStableOperationIDs(t *testing.T) {
	configuration := validConfiguration("x-social", RequirementOptional)
	loader := &provisioningConfigurationLoaderStub{configuration: configuration, record: Record{ID: uuid.New(), Scope: configuration.Scope, Kind: CurrentConfigurationRecordKind, Key: CurrentConfigurationRecordKey, Revision: configuration.Revision, ArtifactID: uuid.New()}}
	preflight := &provisioningPreflightStub{input: readyProvisioningInput(t, configuration)}
	tasks := &provisioningTaskStub{tasks: map[uuid.UUID]*digitalhuman.Task{}, failOnce: 3}
	service, _ := NewProvisioningService(loader, preflight, tasks)
	request := ProvisioningRequest{Scope: configuration.Scope, ExpectedEmployeeVersionID: configuration.EmployeeVersionID,
		ExpectedConfigurationRevision: configuration.Revision, Access: provisioningAccess(configuration.Scope)}
	partial, err := service.Provision(t.Context(), request)
	if err == nil || len(partial.Tasks) != 2 || len(tasks.tasks) != 2 {
		t.Fatalf("partial Provision() = %#v, stored = %d, error = %v", partial, len(tasks.tasks), err)
	}
	firstIDs := []uuid.UUID{partial.Tasks[0].OperationID, partial.Tasks[1].OperationID}
	completed, err := service.Provision(t.Context(), request)
	if err != nil || len(completed.Tasks) != 4 || len(tasks.tasks) != 4 || completed.Tasks[0].OperationID != firstIDs[0] || completed.Tasks[1].OperationID != firstIDs[1] {
		t.Fatalf("retry Provision() = %#v, stored = %d, error = %v", completed, len(tasks.tasks), err)
	}
}

func TestProvisioningStopsWhenCurrentConfigurationChangesDuringSetup(t *testing.T) {
	configuration := validConfiguration("x-social", RequirementOptional)
	loader := &provisioningConfigurationLoaderStub{configuration: configuration, changeAt: 3, record: Record{ID: uuid.New(), Scope: configuration.Scope, Kind: CurrentConfigurationRecordKind, Key: CurrentConfigurationRecordKey, Revision: configuration.Revision, ArtifactID: uuid.New()}}
	preflight := &provisioningPreflightStub{input: readyProvisioningInput(t, configuration)}
	tasks := &provisioningTaskStub{tasks: map[uuid.UUID]*digitalhuman.Task{}}
	service, _ := NewProvisioningService(loader, preflight, tasks)
	request := ProvisioningRequest{Scope: configuration.Scope, ExpectedEmployeeVersionID: configuration.EmployeeVersionID,
		ExpectedConfigurationRevision: configuration.Revision, Access: provisioningAccess(configuration.Scope)}
	result, err := service.Provision(t.Context(), request)
	if !errors.Is(err, ErrProvisioningStale) || len(result.Tasks) != 1 || len(tasks.tasks) != 1 {
		t.Fatalf("configuration changed Provision() = %#v, stored = %d, error = %v", result, len(tasks.tasks), err)
	}
}

type provisioningDispatcher struct{}

func (provisioningDispatcher) DispatchTaskRun(context.Context, string) error { return nil }

func TestProvisioningUsesRealAutomationTaskAdapterWithoutDuplicates(t *testing.T) {
	journal, scope, _, _ := fixture(t)
	if err := journal.db.AutoMigrate(&usersubject.Subject{}, &automationmodel.AutomationTask{}, &automationmodel.AutomationTaskAction{},
		&automationmodel.AutomationTaskRun{}, &automationmodel.AutomationActionRun{}, &automationmodel.AutomationTaskActivity{}); err != nil {
		t.Fatal(err)
	}
	configuration := validConfiguration("x-social", RequirementOptional)
	configuration.Scope = scope
	saved, record, err := journal.SaveCurrentConfiguration(t.Context(), scope, 0, configuration)
	if err != nil {
		t.Fatal(err)
	}
	preflight := &provisioningPreflightStub{input: readyProvisioningInput(t, saved)}
	taskService := digitalhumantask.NewService(journal.db, provisioningDispatcher{}, nil)
	service, err := NewProvisioningService(journal, preflight, taskService)
	if err != nil {
		t.Fatal(err)
	}
	request := ProvisioningRequest{Scope: scope, ExpectedEmployeeVersionID: saved.EmployeeVersionID,
		ExpectedConfigurationRevision: record.Revision, Access: provisioningAccess(scope)}
	first, err := service.Provision(t.Context(), request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.Provision(t.Context(), request)
	if err != nil || len(first.Tasks) != 4 || len(second.Tasks) != 4 {
		t.Fatalf("second Provision() = %#v, %v", second, err)
	}
	var stored []automationmodel.AutomationTask
	if err := journal.db.Order("name").Find(&stored).Error; err != nil || len(stored) != 4 {
		t.Fatalf("stored tasks = %d, %v", len(stored), err)
	}
	seen := map[string]bool{}
	for _, task := range stored {
		operationID := ""
		if task.SourceOperationID != nil {
			operationID = *task.SourceOperationID
		}
		metadata, ok := task.SourceSnapshot["social_operations"].(map[string]any)
		if task.Status != automationmodel.AutomationTaskStatusActive || task.CreatedBy != scope.OwnerID.String() || operationID == "" || seen[operationID] || !ok || metadata["task_key"] == "" {
			t.Fatalf("stored task = %#v, metadata = %#v", task, metadata)
		}
		seen[operationID] = true
	}
	if got := fmt.Sprint(first.Tasks[0].OperationID); got != fmt.Sprint(second.Tasks[0].OperationID) {
		t.Fatalf("operation ID changed across retry: %s != %s", got, second.Tasks[0].OperationID)
	}
}
