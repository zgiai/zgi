package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	actiondto "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/dto"
	actionmodel "github.com/zgiai/zgi/api/internal/capabilities/actionruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/actionruntime/repository"
	"gorm.io/gorm"
)

const (
	ledgerVersion         = "action_runtime.v1"
	adapterPendingMessage = "capability adapter is not connected yet"
)

type service struct {
	repo      repository.Repository
	registry  *Registry
	executors map[string]Executor
	now       func() time.Time
}

func NewService(repo repository.Repository, registry *Registry, opts ...Option) Service {
	if registry == nil {
		registry = NewDefaultRegistry()
	}
	svc := &service{
		repo:      repo,
		registry:  registry,
		executors: map[string]Executor{},
		now:       time.Now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(svc)
		}
	}
	return svc
}

func WithExecutor(capabilityID string, executor Executor) Option {
	return func(s *service) {
		if s == nil || executor == nil {
			return
		}
		id := strings.TrimSpace(capabilityID)
		if id == "" {
			return
		}
		if s.executors == nil {
			s.executors = map[string]Executor{}
		}
		s.executors[id] = executor
	}
}

func (s *service) ListCapabilities(ctx context.Context, scope Scope) ([]CapabilityManifest, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	return s.registry.List(), nil
}

func (s *service) PlanAction(ctx context.Context, scope Scope, req actiondto.ActionPlanRequest) (*ActionRunView, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	capabilityID := strings.TrimSpace(req.CapabilityID)
	if capabilityID == "" {
		return nil, fmt.Errorf("%w: capability_id is required", ErrInvalidInput)
	}
	capability, ok := s.registry.Get(capabilityID)
	if !ok {
		return nil, fmt.Errorf("%w: capability %q is not registered", ErrInvalidInput, capabilityID)
	}
	idempotencyKeyValue := strings.TrimSpace(req.IdempotencyKey)
	if capability.IdempotencyRequired && idempotencyKeyValue == "" {
		return nil, fmt.Errorf("%w: idempotency_key is required for capability %q", ErrInvalidInput, capabilityID)
	}
	if idempotencyKeyValue != "" {
		if existing, steps, err := s.repo.GetRunByIdempotencyKey(ctx, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, capabilityID, idempotencyKeyValue); err == nil {
			existingCapability, _ := s.registry.Get(existing.CapabilityID)
			return &ActionRunView{Run: existing, Steps: steps, Capability: &existingCapability}, nil
		} else if !isRepoNotFound(err) {
			return nil, err
		}
	}

	conversationID, err := parseOptionalUUID(req.ConversationID, "conversation_id")
	if err != nil {
		return nil, err
	}
	messageID, err := parseOptionalUUID(req.MessageID, "message_id")
	if err != nil {
		return nil, err
	}

	policy := evaluatePlanPolicy(capability, req.RiskLevel, req.RequiresConfirmation)
	status := actionmodel.ActionRunStatusPlanned
	if policy.RequiresConfirmation {
		status = actionmodel.ActionRunStatusNeedsConfirmation
	}
	idempotencyKey := optionalString(idempotencyKeyValue)
	title := firstNonEmpty(req.Title, capability.Name)
	summary := strings.TrimSpace(req.Summary)
	resources := resourcesPayload(req.Resources)
	arguments := copyStringAnyMap(req.Arguments)
	metadata := copyStringAnyMap(req.Metadata)
	ledger := map[string]interface{}{
		"version":               ledgerVersion,
		"status":                status,
		"capability_id":         capability.ID,
		"risk_level":            policy.RiskLevel,
		"requires_confirmation": policy.RequiresConfirmation,
		"auth_mode":             capability.AuthMode,
		"created_by":            scope.AccountID.String(),
	}
	if operationContext := copyStringAnyMap(req.OperationContext); len(operationContext) > 0 {
		ledger["operation_context"] = operationContext
	}

	run := &actionmodel.ActionRun{
		OrganizationID:       scope.OrganizationID,
		WorkspaceID:          scope.WorkspaceID,
		AccountID:            scope.AccountID,
		ConversationID:       conversationID,
		MessageID:            messageID,
		IdempotencyKey:       idempotencyKey,
		Intent:               strings.TrimSpace(req.Intent),
		CapabilityID:         capability.ID,
		Title:                title,
		Summary:              summary,
		Status:               status,
		RiskLevel:            policy.RiskLevel,
		RequiresConfirmation: policy.RequiresConfirmation,
		Resources:            resources,
		Arguments:            arguments,
		Ledger:               ledger,
		Metadata:             metadata,
	}
	step := &actionmodel.ActionStep{
		StepKey:              "execute",
		CapabilityID:         capability.ID,
		Title:                title,
		Status:               actionmodel.ActionStepStatusPending,
		RiskLevel:            policy.RiskLevel,
		RequiresConfirmation: policy.RequiresConfirmation,
		Input: map[string]interface{}{
			"arguments": arguments,
			"resources": resources,
		},
		Metadata: map[string]interface{}{
			"auth_mode": capability.AuthMode,
			"runtime":   capability.Runtime,
		},
	}
	if err := s.repo.CreateRunWithSteps(ctx, run, []*actionmodel.ActionStep{step}); err != nil {
		return nil, err
	}
	return s.GetActionRun(ctx, scope, run.ID)
}

func (s *service) GetActionRun(ctx context.Context, scope Scope, id uuid.UUID) (*ActionRunView, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	run, steps, err := s.repo.GetRunScoped(ctx, id, scope.OrganizationID, scope.WorkspaceID, scope.AccountID)
	if err != nil {
		if isRepoNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	capability, _ := s.registry.Get(run.CapabilityID)
	return &ActionRunView{Run: run, Steps: steps, Capability: &capability}, nil
}

func (s *service) ConfirmAction(ctx context.Context, scope Scope, id uuid.UUID, req actiondto.ConfirmActionRequest) (*ActionRunView, error) {
	view, err := s.GetActionRun(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	run := view.Run
	now := s.now()
	ledger := copyStringAnyMap(run.Ledger)
	ledger["confirmed_by"] = scope.AccountID.String()
	ledger["confirmation_reason"] = strings.TrimSpace(req.Reason)
	ledger["confirmation_at"] = now.UTC().Format(time.RFC3339)
	metadata := mergeMaps(run.Metadata, req.Metadata)

	if !req.Confirmed {
		ledger["status"] = actionmodel.ActionRunStatusCanceled
		if err := s.repo.UpdateRunFieldsScoped(ctx, id, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, map[string]interface{}{
			"status":      actionmodel.ActionRunStatusCanceled,
			"canceled_at": now,
			"ledger":      ledger,
			"metadata":    metadata,
		}); err != nil {
			return nil, err
		}
		return s.GetActionRun(ctx, scope, id)
	}

	ledger["status"] = actionmodel.ActionRunStatusConfirmed
	if err := s.repo.UpdateRunFieldsScoped(ctx, id, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, map[string]interface{}{
		"status":       actionmodel.ActionRunStatusConfirmed,
		"confirmed_by": scope.AccountID,
		"confirmed_at": now,
		"ledger":       ledger,
		"metadata":     metadata,
	}); err != nil {
		return nil, err
	}
	return s.GetActionRun(ctx, scope, id)
}

func (s *service) ExecuteAction(ctx context.Context, scope Scope, id uuid.UUID, req actiondto.ExecuteActionRequest) (*ActionRunView, error) {
	view, err := s.GetActionRun(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	run := view.Run
	if run.RequiresConfirmation && run.ConfirmedAt == nil {
		return nil, ErrConfirmationRequired
	}
	if run.Status == actionmodel.ActionRunStatusCanceled {
		return nil, fmt.Errorf("%w: action run is canceled", ErrInvalidInput)
	}
	if run.Status == actionmodel.ActionRunStatusCompleted {
		return view, nil
	}

	now := s.now()
	ledger := copyStringAnyMap(run.Ledger)
	ledger["execute_requested_at"] = now.UTC().Format(time.RFC3339)
	if req.DryRun {
		ledger["dry_run"] = true
	}
	metadata := mergeMaps(run.Metadata, req.Metadata)
	executor := s.executors[strings.TrimSpace(run.CapabilityID)]
	if executor == nil {
		ledger["status"] = actionmodel.ActionRunStatusBlocked
		ledger["blocked_reason"] = adapterPendingMessage
		return s.blockActionRun(ctx, scope, view, ledger, metadata, adapterPendingMessage, now)
	}

	if len(view.Steps) > 0 {
		step := view.Steps[0]
		if err := s.repo.UpdateStepFields(ctx, run.ID, step.ID, map[string]interface{}{
			"status":     actionmodel.ActionStepStatusRunning,
			"started_at": now,
			"error":      nil,
		}); err != nil {
			return nil, err
		}
	}
	ledger["status"] = actionmodel.ActionRunStatusRunning
	if err := s.repo.UpdateRunFieldsScoped(ctx, id, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, map[string]interface{}{
		"status":   actionmodel.ActionRunStatusRunning,
		"error":    nil,
		"ledger":   ledger,
		"metadata": metadata,
	}); err != nil {
		return nil, err
	}

	latest, err := s.GetActionRun(ctx, scope, id)
	if err != nil {
		return nil, err
	}
	result, execErr := executor.Execute(ctx, scope, *latest, req)
	finishedAt := s.now()
	if execErr != nil {
		return s.failActionRun(ctx, scope, latest, ledger, metadata, execErr.Error(), finishedAt)
	}
	if result == nil {
		result = &ExecutionResult{}
	}
	finalLedger := mergeMaps(ledger, result.Ledger)
	finalLedger["status"] = actionmodel.ActionRunStatusCompleted
	finalLedger["completed_at"] = finishedAt.UTC().Format(time.RFC3339)
	finalMetadata := mergeMaps(metadata, result.Metadata)
	output := copyStringAnyMap(result.Output)
	if len(output) == 0 {
		output = map[string]interface{}{"status": "completed"}
	}
	if len(latest.Steps) > 0 {
		step := latest.Steps[0]
		if err := s.repo.UpdateStepFields(ctx, run.ID, step.ID, map[string]interface{}{
			"status":       actionmodel.ActionStepStatusDone,
			"error":        nil,
			"completed_at": finishedAt,
			"output":       output,
		}); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateRunFieldsScoped(ctx, id, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, map[string]interface{}{
		"status":   actionmodel.ActionRunStatusCompleted,
		"error":    nil,
		"ledger":   finalLedger,
		"metadata": finalMetadata,
	}); err != nil {
		return nil, err
	}
	return s.GetActionRun(ctx, scope, id)
}

func (s *service) blockActionRun(ctx context.Context, scope Scope, view *ActionRunView, ledger map[string]interface{}, metadata map[string]interface{}, errText string, now time.Time) (*ActionRunView, error) {
	if view == nil || view.Run == nil {
		return nil, ErrNotFound
	}
	if len(view.Steps) > 0 {
		step := view.Steps[0]
		if err := s.repo.UpdateStepFields(ctx, view.Run.ID, step.ID, map[string]interface{}{
			"status":       actionmodel.ActionStepStatusBlocked,
			"error":        errText,
			"completed_at": now,
			"output": map[string]interface{}{
				"status": "adapter_unavailable",
				"reason": adapterPendingMessage,
			},
		}); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateRunFieldsScoped(ctx, view.Run.ID, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, map[string]interface{}{
		"status":   actionmodel.ActionRunStatusBlocked,
		"error":    errText,
		"ledger":   ledger,
		"metadata": metadata,
	}); err != nil {
		return nil, err
	}
	return s.GetActionRun(ctx, scope, view.Run.ID)
}

func (s *service) failActionRun(ctx context.Context, scope Scope, view *ActionRunView, ledger map[string]interface{}, metadata map[string]interface{}, errText string, now time.Time) (*ActionRunView, error) {
	if view == nil || view.Run == nil {
		return nil, ErrNotFound
	}
	ledger = copyStringAnyMap(ledger)
	ledger["status"] = actionmodel.ActionRunStatusFailed
	ledger["failed_at"] = now.UTC().Format(time.RFC3339)
	ledger["error"] = errText
	if len(view.Steps) > 0 {
		step := view.Steps[0]
		if err := s.repo.UpdateStepFields(ctx, view.Run.ID, step.ID, map[string]interface{}{
			"status":       actionmodel.ActionStepStatusFailed,
			"error":        errText,
			"completed_at": now,
			"output": map[string]interface{}{
				"status": "failed",
				"error":  errText,
			},
		}); err != nil {
			return nil, err
		}
	}
	if err := s.repo.UpdateRunFieldsScoped(ctx, view.Run.ID, scope.OrganizationID, scope.WorkspaceID, scope.AccountID, map[string]interface{}{
		"status":   actionmodel.ActionRunStatusFailed,
		"error":    errText,
		"ledger":   ledger,
		"metadata": metadata,
	}); err != nil {
		return nil, err
	}
	return s.GetActionRun(ctx, scope, view.Run.ID)
}

func (s *service) ensureMember(ctx context.Context, scope Scope) error {
	if scope.OrganizationID == uuid.Nil || scope.AccountID == uuid.Nil {
		return ErrPermissionDenied
	}
	if scope.SkipAccessCheck {
		return nil
	}
	ok, err := s.repo.IsOrganizationMember(ctx, scope.OrganizationID, scope.AccountID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPermissionDenied
	}
	return nil
}

func parseOptionalUUID(raw string, field string) (*uuid.UUID, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil, nil
	}
	parsed, err := uuid.Parse(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %s must be a uuid", ErrInvalidInput, field)
	}
	return &parsed, nil
}

func optionalString(raw string) *string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	return &value
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text != "" {
			return text
		}
	}
	return ""
}

func resourcesPayload(resources []actiondto.ResourceRef) map[string]interface{} {
	items := make([]map[string]interface{}, 0, len(resources))
	for _, resource := range resources {
		item := map[string]interface{}{}
		if value := strings.TrimSpace(resource.Type); value != "" {
			item["type"] = value
		}
		if value := strings.TrimSpace(resource.ID); value != "" {
			item["id"] = value
		}
		if value := strings.TrimSpace(resource.Name); value != "" {
			item["name"] = value
		}
		if value := strings.TrimSpace(resource.Source); value != "" {
			item["source"] = value
		}
		if len(resource.Metadata) > 0 {
			item["metadata"] = copyStringAnyMap(resource.Metadata)
		}
		if len(item) > 0 {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return map[string]interface{}{}
	}
	return map[string]interface{}{"items": items}
}

func copyStringAnyMap(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func mergeMaps(left map[string]interface{}, right map[string]interface{}) map[string]interface{} {
	out := copyStringAnyMap(left)
	for key, value := range right {
		out[key] = value
	}
	return out
}

func isRepoNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
