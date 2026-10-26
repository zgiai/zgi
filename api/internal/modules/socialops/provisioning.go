package socialops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
)

var (
	ErrProvisioningUnauthorized = errors.New("social operations provisioning requires explicit owner access")
	ErrProvisioningStale        = errors.New("social operations configuration changed during provisioning")
	ErrProvisioningNotReady     = errors.New("social operations preflight is not ready")
)

var provisioningOperationNamespace = uuid.MustParse("172aa504-138a-5278-8dc6-986849dd6f09")

type CurrentConfigurationLoader interface {
	LoadCurrentConfiguration(context.Context, Scope) (Configuration, Record, error)
}

// ProvisioningPreflightRestorer obtains live server-side connection,
// connector-contract and grant state. It must not consume grant capacity.
type ProvisioningPreflightRestorer interface {
	RestoreSocialOperationsPreflight(context.Context, Configuration) (PreflightInput, error)
}

type ProvisioningTaskCreator interface {
	CreateTask(context.Context, digitalhuman.TaskAccessContext, digitalhuman.CreateTaskRequest, digitalhuman.TaskMutationContext) (*digitalhuman.Task, error)
	PauseSupersededSocialOperations(context.Context, digitalhuman.TaskAccessContext, uuid.UUID, int64) error
}

type ProvisioningRequest struct {
	Scope                         Scope
	ExpectedEmployeeVersionID     uuid.UUID
	ExpectedConfigurationRevision int64
	Access                        digitalhuman.TaskAccessContext
}

type ProvisionedTask struct {
	Key         string             `json:"key"`
	OperationID uuid.UUID          `json:"operation_id"`
	Task        *digitalhuman.Task `json:"task"`
}

type ProvisioningResult struct {
	ConfigurationRecord Record            `json:"configuration_record"`
	Preflight           PreflightReport   `json:"preflight"`
	Plan                SetupPlan         `json:"plan"`
	Tasks               []ProvisionedTask `json:"tasks"`
}

// ProvisioningService is the trusted bridge from a validated current SOP
// configuration into the existing Automation task creation boundary.
type ProvisioningService struct {
	configurations CurrentConfigurationLoader
	preflight      ProvisioningPreflightRestorer
	tasks          ProvisioningTaskCreator
}

func NewProvisioningService(configurations CurrentConfigurationLoader, preflight ProvisioningPreflightRestorer, tasks ProvisioningTaskCreator) (*ProvisioningService, error) {
	if configurations == nil || preflight == nil || tasks == nil {
		return nil, invalid()
	}
	return &ProvisioningService{configurations: configurations, preflight: preflight, tasks: tasks}, nil
}

func (s *ProvisioningService) Provision(ctx context.Context, request ProvisioningRequest) (ProvisioningResult, error) {
	var result ProvisioningResult
	if s == nil || !validProvisioningAccess(request) {
		return result, ErrProvisioningUnauthorized
	}
	configuration, record, err := s.configurations.LoadCurrentConfiguration(ctx, request.Scope)
	if err != nil {
		return result, err
	}
	if err = validateExpectedConfiguration(request, configuration, record); err != nil {
		return result, err
	}
	result.ConfigurationRecord = record
	result.Preflight, err = s.restoreReadyPreflight(ctx, configuration)
	if err != nil {
		return result, err
	}
	result.Plan, err = BuildSetupPlan(configuration)
	if err != nil {
		return result, err
	}
	if err = s.tasks.PauseSupersededSocialOperations(ctx, request.Access, configuration.EmployeeVersionID, configuration.Revision); err != nil {
		return result, err
	}
	result.Tasks = make([]ProvisionedTask, 0, len(result.Plan.Tasks))
	for _, planned := range result.Plan.Tasks {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		current, currentRecord, loadErr := s.configurations.LoadCurrentConfiguration(ctx, request.Scope)
		if loadErr != nil {
			return result, loadErr
		}
		if !sameProvisioningConfiguration(configuration, record, current, currentRecord) {
			return result, ErrProvisioningStale
		}
		if _, err = s.restoreReadyPreflight(ctx, current); err != nil {
			return result, err
		}

		operationID := provisioningOperationID(configuration, planned.Key)
		mutation := digitalhuman.TaskMutationContext{
			ActorType: "account", ActorRef: request.Scope.OwnerID.String(), OperationID: operationID,
			Source: "social_operations", Proposed: false,
			SocialOperations: socialOperationsTaskMetadata(configuration, record, planned),
		}
		task, createErr := s.tasks.CreateTask(ctx, request.Access, planned.Request, mutation)
		if createErr != nil {
			return result, createErr
		}
		result.Tasks = append(result.Tasks, ProvisionedTask{Key: planned.Key, OperationID: operationID, Task: task})
	}
	return result, nil
}

func (s *ProvisioningService) restoreReadyPreflight(ctx context.Context, configuration Configuration) (PreflightReport, error) {
	input, err := s.preflight.RestoreSocialOperationsPreflight(ctx, configuration)
	if err != nil {
		return PreflightReport{}, err
	}
	report, err := Preflight(configuration, input)
	if err != nil {
		return PreflightReport{}, err
	}
	if !report.Ready {
		return report, ErrProvisioningNotReady
	}
	return report, nil
}

func validProvisioningAccess(request ProvisioningRequest) bool {
	if !validScope(request.Scope) || request.ExpectedEmployeeVersionID == uuid.Nil || request.ExpectedConfigurationRevision < 1 {
		return false
	}
	access := request.Access
	if access.OrganizationID != request.Scope.OrganizationID || access.WorkspaceID != request.Scope.WorkspaceID ||
		access.DigitalHumanID != request.Scope.EmployeeID || access.Principal.Type != invocation.PrincipalAccount ||
		access.Owner != digitalhuman.SharedTaskOwner() || access.Surface != digitalhuman.SurfaceConsole || access.CurrentTaskID != nil {
		return false
	}
	principalID, err := uuid.Parse(strings.TrimSpace(access.Principal.ID))
	return err == nil && principalID == request.Scope.OwnerID
}

func validateExpectedConfiguration(request ProvisioningRequest, configuration Configuration, record Record) error {
	if configuration.Scope != request.Scope || record.Scope != request.Scope ||
		record.Kind != CurrentConfigurationRecordKind || record.Key != CurrentConfigurationRecordKey ||
		record.Revision != configuration.Revision || record.ArtifactID == uuid.Nil || record.ID == uuid.Nil {
		return ErrProvisioningStale
	}
	if configuration.EmployeeVersionID != request.ExpectedEmployeeVersionID || configuration.Revision != request.ExpectedConfigurationRevision {
		return ErrProvisioningStale
	}
	return nil
}

func sameProvisioningConfiguration(configuration Configuration, record Record, current Configuration, currentRecord Record) bool {
	return current.Scope == configuration.Scope && current.EmployeeVersionID == configuration.EmployeeVersionID &&
		current.Revision == configuration.Revision && currentRecord.Scope == record.Scope && currentRecord.ID == record.ID &&
		currentRecord.Kind == record.Kind && currentRecord.Key == record.Key && currentRecord.Revision == record.Revision &&
		currentRecord.ArtifactID == record.ArtifactID
}

func provisioningOperationID(configuration Configuration, taskKey string) uuid.UUID {
	identity := fmt.Sprintf("%s/%s/%s/%s/%s/%d/%s", configuration.Scope.OrganizationID, configuration.Scope.WorkspaceID,
		configuration.Scope.OwnerID, configuration.Scope.EmployeeID, configuration.EmployeeVersionID, configuration.Revision, taskKey)
	return uuid.NewSHA1(provisioningOperationNamespace, []byte(identity))
}

func socialOperationsTaskMetadata(configuration Configuration, record Record, planned PlannedTask) *digitalhuman.SocialOperationsTaskMetadata {
	stages := make([]string, len(planned.Stages))
	for index, stage := range planned.Stages {
		stages[index] = string(stage)
	}
	updates := make([]digitalhuman.SocialOperationsRecordTarget, len(planned.UpdatesRecords))
	for index, target := range planned.UpdatesRecords {
		updates[index] = digitalhuman.SocialOperationsRecordTarget{Kind: target.Kind, Key: target.Key}
	}
	return &digitalhuman.SocialOperationsTaskMetadata{
		ResearchConnectorID: configuration.Research.ConnectorID, ResearchConnectionID: configuration.Research.ConnectionID,
		Scope: digitalhuman.SocialOperationsTaskScope{
			OrganizationID: configuration.Scope.OrganizationID, WorkspaceID: configuration.Scope.WorkspaceID,
			OwnerID: configuration.Scope.OwnerID, EmployeeID: configuration.Scope.EmployeeID,
		},
		EmployeeVersionID: configuration.EmployeeVersionID, ConfigurationRevision: configuration.Revision,
		ConfigurationRecordID: record.ID, ConfigurationArtifactID: record.ArtifactID, TaskKey: planned.Key,
		Stages: stages,
		Budget: digitalhuman.SocialOperationsTaskBudget{
			MaxPages: planned.Budget.MaxPages, MaxRecords: planned.Budget.MaxRecords,
			MaxConnectorCalls: planned.Budget.MaxConnectorCalls, MaxTokens: planned.Budget.MaxTokens,
			MaxSafeReadAttempts: planned.Budget.MaxSafeReadAttempts,
		},
		ConsumesRecordKinds: append([]string(nil), planned.ConsumesRecordKinds...),
		ProducesRecordKinds: append([]string(nil), planned.ProducesRecordKinds...),
		UpdatesRecords:      updates,
	}
}
