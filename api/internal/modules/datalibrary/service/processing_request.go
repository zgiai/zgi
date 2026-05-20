package service

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/model"
	"github.com/zgiai/zgi/api/internal/modules/datalibrary/repository"
)

var (
	ErrProcessingLevelRequired             = errors.New("processing_level is required")
	ErrProcessingLevelInvalid              = errors.New("processing_level is invalid")
	ErrProcessingRequestIDRequired         = errors.New("processing_request_id is required")
	ErrProcessingRequestNotFound           = errors.New("processing request not found")
	ErrProcessingRequestTransitionInvalid  = errors.New("processing request status transition is invalid")
	ErrProcessingExecutorRequired          = errors.New("processing request executor is required")
	ErrProcessingExecutorKeyRequired       = errors.New("processing request executor key is required")
	ErrProcessingExecutorTargetUnsupported = errors.New("processing request executor does not support target level")
)

type ProcessingRequest struct {
	OrganizationID  string         `json:"organization_id"`
	WorkspaceID     *string        `json:"workspace_id,omitempty"`
	AssetID         uuid.UUID      `json:"asset_id"`
	TargetLevel     string         `json:"target_level"`
	RequestedBy     string         `json:"requested_by,omitempty"`
	Force           bool           `json:"force"`
	RequestMetadata map[string]any `json:"request_metadata,omitempty"`
}

type ProcessingRequestPlan struct {
	AssetID         uuid.UUID `json:"asset_id"`
	TargetLevel     string    `json:"target_level"`
	WillParse       bool      `json:"will_parse"`
	WillSplit       bool      `json:"will_split"`
	WillVectorize   bool      `json:"will_vectorize"`
	WillExtractFull bool      `json:"will_extract_full"`
}

type ProcessingRequestView struct {
	ID                uuid.UUID              `json:"id"`
	OrganizationID    string                 `json:"organization_id"`
	WorkspaceID       *string                `json:"workspace_id,omitempty"`
	AssetID           uuid.UUID              `json:"asset_id"`
	TargetLevel       string                 `json:"target_level"`
	Status            string                 `json:"status"`
	RequestedBy       string                 `json:"requested_by,omitempty"`
	Force             bool                   `json:"force"`
	Plan              *ProcessingRequestPlan `json:"plan"`
	RequestMetadata   map[string]any         `json:"request_metadata,omitempty"`
	ExecutionMetadata map[string]any         `json:"execution_metadata,omitempty"`
	ExecutorKey       string                 `json:"executor_key,omitempty"`
	ErrorCode         string                 `json:"error_code,omitempty"`
	ErrorMessage      string                 `json:"error_message,omitempty"`
	AttemptCount      int                    `json:"attempt_count"`
	QueuedAt          *time.Time             `json:"queued_at,omitempty"`
	StartedAt         *time.Time             `json:"started_at,omitempty"`
	CompletedAt       *time.Time             `json:"completed_at,omitempty"`
	FailedAt          *time.Time             `json:"failed_at,omitempty"`
	CanceledAt        *time.Time             `json:"cancelled_at,omitempty"`
	CreatedAt         time.Time              `json:"created_at"`
	UpdatedAt         time.Time              `json:"updated_at"`
}

type ProcessingRequestQueueSummaryView struct {
	TargetLevel     string     `json:"target_level"`
	Status          string     `json:"status"`
	ExecutorKey     string     `json:"executor_key,omitempty"`
	Count           int64      `json:"count"`
	OldestQueuedAt  *time.Time `json:"oldest_queued_at,omitempty"`
	OldestCreatedAt *time.Time `json:"oldest_created_at,omitempty"`
	NewestCreatedAt *time.Time `json:"newest_created_at,omitempty"`
}

type ProcessingExecutionRequest struct {
	RequestID      uuid.UUID              `json:"request_id"`
	OrganizationID string                 `json:"organization_id"`
	WorkspaceID    *string                `json:"workspace_id,omitempty"`
	AssetID        uuid.UUID              `json:"asset_id"`
	TargetLevel    string                 `json:"target_level"`
	Plan           *ProcessingRequestPlan `json:"plan"`
	Force          bool                   `json:"force"`
}

type ProcessingRequestExecutor interface {
	Key() string
	Enqueue(ctx context.Context, req ProcessingExecutionRequest) error
}

type ProcessingRequestService interface {
	CreatePlannedRequest(ctx context.Context, req ProcessingRequest) (*ProcessingRequestView, error)
	ListRequests(ctx context.Context, filter repository.ProcessingRequestListFilter) ([]*ProcessingRequestView, int64, error)
	QueueSummary(ctx context.Context, filter repository.ProcessingRequestQueueSummaryFilter) ([]ProcessingRequestQueueSummaryView, error)
	EnqueueRequest(ctx context.Context, organizationID string, id uuid.UUID, executor ProcessingRequestExecutor) (*ProcessingRequestView, error)
	ClaimNextQueuedRequest(ctx context.Context, organizationID string, executorKey string) (*ProcessingRequestView, error)
	ClaimNextQueuedRequestForExecutor(ctx context.Context, organizationID string, executor RegisteredProcessingRequestExecutor) (*ProcessingRequestView, error)
	QueueRequest(ctx context.Context, organizationID string, id uuid.UUID) (*ProcessingRequestView, error)
	RetryRequest(ctx context.Context, organizationID string, id uuid.UUID, requestedBy string, force *bool, metadata map[string]any) (*ProcessingRequestView, error)
	StartRequest(ctx context.Context, organizationID string, id uuid.UUID, executorKey string) (*ProcessingRequestView, error)
	CompleteRequest(ctx context.Context, organizationID string, id uuid.UUID, metadata map[string]any) (*ProcessingRequestView, error)
	FailRequest(ctx context.Context, organizationID string, id uuid.UUID, errorCode string, errorMessage string, metadata map[string]any) (*ProcessingRequestView, error)
	CancelRequest(ctx context.Context, organizationID string, id uuid.UUID, reason string) (*ProcessingRequestView, error)
}

type processingRequestService struct {
	repo repository.ProcessingRequestRepository
}

func NewProcessingRequestService(repo repository.ProcessingRequestRepository) ProcessingRequestService {
	return &processingRequestService{repo: repo}
}

func (s *processingRequestService) CreatePlannedRequest(ctx context.Context, req ProcessingRequest) (*ProcessingRequestView, error) {
	plan, err := PlanProcessingRequest(req)
	if err != nil {
		return nil, err
	}
	item := &model.ProcessingRequest{
		OrganizationID:  req.OrganizationID,
		WorkspaceID:     req.WorkspaceID,
		AssetID:         req.AssetID,
		TargetLevel:     req.TargetLevel,
		Status:          model.ProcessingRequestStatusPlanned,
		RequestedBy:     req.RequestedBy,
		Force:           req.Force,
		PlanJSON:        processingPlanToJSON(plan),
		RequestMetadata: req.RequestMetadata,
	}
	if err := s.repo.Create(ctx, item); err != nil {
		return nil, err
	}
	return newProcessingRequestView(item), nil
}

func (s *processingRequestService) ListRequests(ctx context.Context, filter repository.ProcessingRequestListFilter) ([]*ProcessingRequestView, int64, error) {
	if filter.OrganizationID == "" {
		return nil, 0, ErrOrganizationIDRequired
	}
	if filter.TargetLevel != "" && !isSupportedProcessingTarget(filter.TargetLevel) {
		return nil, 0, ErrProcessingLevelInvalid
	}
	items, total, err := s.repo.List(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views := make([]*ProcessingRequestView, 0, len(items))
	for _, item := range items {
		views = append(views, newProcessingRequestView(item))
	}
	return views, total, nil
}

func (s *processingRequestService) QueueSummary(ctx context.Context, filter repository.ProcessingRequestQueueSummaryFilter) ([]ProcessingRequestQueueSummaryView, error) {
	if filter.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if filter.TargetLevel != "" && !isSupportedProcessingTarget(filter.TargetLevel) {
		return nil, ErrProcessingLevelInvalid
	}
	summaries, err := s.repo.QueueSummary(ctx, filter)
	if err != nil {
		return nil, err
	}
	views := make([]ProcessingRequestQueueSummaryView, 0, len(summaries))
	for _, summary := range summaries {
		views = append(views, ProcessingRequestQueueSummaryView{
			TargetLevel:     summary.TargetLevel,
			Status:          summary.Status,
			ExecutorKey:     summary.ExecutorKey,
			Count:           summary.Count,
			OldestQueuedAt:  summary.OldestQueuedAt,
			OldestCreatedAt: summary.OldestCreatedAt,
			NewestCreatedAt: summary.NewestCreatedAt,
		})
	}
	return views, nil
}

func (s *processingRequestService) EnqueueRequest(ctx context.Context, organizationID string, id uuid.UUID, executor ProcessingRequestExecutor) (*ProcessingRequestView, error) {
	if executor == nil {
		return nil, ErrProcessingExecutorRequired
	}
	if executor.Key() == "" {
		return nil, ErrProcessingExecutorKeyRequired
	}
	if registered, ok := executor.(RegisteredProcessingRequestExecutor); ok {
		info := registered.Info()
		if !info.Enabled {
			return nil, ErrProcessingExecutorDisabled
		}
		item, err := s.repo.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if item == nil || item.OrganizationID != organizationID {
			return nil, ErrProcessingRequestNotFound
		}
		if !processingExecutorSupportsTarget(info, item.TargetLevel) {
			return nil, ErrProcessingExecutorTargetUnsupported
		}
	}
	view, err := s.QueueRequest(ctx, organizationID, id)
	if err != nil {
		return nil, err
	}
	executionRequest, err := NewProcessingExecutionRequest(view)
	if err != nil {
		return nil, err
	}
	if err := executor.Enqueue(ctx, executionRequest); err != nil {
		failed, failErr := s.FailRequest(ctx, organizationID, id, "enqueue_failed", err.Error(), map[string]any{
			"executor_key": executor.Key(),
		})
		if failErr != nil {
			return failed, failErr
		}
		return failed, err
	}
	return view, nil
}

func (s *processingRequestService) ClaimNextQueuedRequest(ctx context.Context, organizationID string, executorKey string) (*ProcessingRequestView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if executorKey == "" {
		return nil, ErrProcessingExecutorKeyRequired
	}
	item, err := s.repo.ClaimNextQueued(ctx, repository.ProcessingRequestClaimFilter{
		OrganizationID: organizationID,
		ExecutorKey:    executorKey,
	})
	if err != nil {
		return nil, err
	}
	return newProcessingRequestView(item), nil
}

func (s *processingRequestService) ClaimNextQueuedRequestForExecutor(ctx context.Context, organizationID string, executor RegisteredProcessingRequestExecutor) (*ProcessingRequestView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if executor == nil {
		return nil, ErrProcessingExecutorRequired
	}
	info := executor.Info()
	if executor.Key() == "" || info.Key == "" || executor.Key() != info.Key {
		return nil, ErrProcessingExecutorKeyRequired
	}
	if !info.Enabled {
		return nil, ErrProcessingExecutorDisabled
	}
	if len(info.TargetLevels) == 0 {
		return nil, ErrProcessingExecutorTargetUnsupported
	}
	item, err := s.repo.ClaimNextQueued(ctx, repository.ProcessingRequestClaimFilter{
		OrganizationID: organizationID,
		ExecutorKey:    executor.Key(),
		TargetLevels:   info.TargetLevels,
	})
	if err != nil {
		return nil, err
	}
	return newProcessingRequestView(item), nil
}

func (s *processingRequestService) QueueRequest(ctx context.Context, organizationID string, id uuid.UUID) (*ProcessingRequestView, error) {
	now := time.Now()
	return s.transitionRequest(ctx, id, repository.ProcessingRequestStatusPatch{
		OrganizationID: organizationID,
		Status:         model.ProcessingRequestStatusQueued,
		AllowedFrom:    AllowedProcessingRequestPreviousStatuses(model.ProcessingRequestStatusQueued),
		QueuedAt:       &now,
	})
}

func (s *processingRequestService) RetryRequest(ctx context.Context, organizationID string, id uuid.UUID, requestedBy string, force *bool, metadata map[string]any) (*ProcessingRequestView, error) {
	if organizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if id == uuid.Nil {
		return nil, ErrProcessingRequestIDRequired
	}
	source, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if source == nil || source.OrganizationID != organizationID {
		return nil, ErrProcessingRequestNotFound
	}
	if source.Status != model.ProcessingRequestStatusFailed && source.Status != model.ProcessingRequestStatusCancelled {
		return nil, ErrProcessingRequestTransitionInvalid
	}
	retryMetadata := cloneMap(source.RequestMetadata)
	for key, value := range metadata {
		retryMetadata[key] = value
	}
	retryMetadata["retry_of_request_id"] = source.ID.String()
	retryMetadata["retry_of_status"] = source.Status
	retryForce := source.Force
	if force != nil {
		retryForce = *force
	}
	return s.CreatePlannedRequest(ctx, ProcessingRequest{
		OrganizationID:  source.OrganizationID,
		WorkspaceID:     source.WorkspaceID,
		AssetID:         source.AssetID,
		TargetLevel:     source.TargetLevel,
		RequestedBy:     requestedBy,
		Force:           retryForce,
		RequestMetadata: retryMetadata,
	})
}

func (s *processingRequestService) StartRequest(ctx context.Context, organizationID string, id uuid.UUID, executorKey string) (*ProcessingRequestView, error) {
	if executorKey == "" {
		return nil, ErrProcessingExecutorKeyRequired
	}
	now := time.Now()
	return s.transitionRequest(ctx, id, repository.ProcessingRequestStatusPatch{
		OrganizationID:    organizationID,
		Status:            model.ProcessingRequestStatusRunning,
		AllowedFrom:       AllowedProcessingRequestPreviousStatuses(model.ProcessingRequestStatusRunning),
		ExecutorKey:       &executorKey,
		AttemptCountDelta: 1,
		StartedAt:         &now,
	})
}

func (s *processingRequestService) CompleteRequest(ctx context.Context, organizationID string, id uuid.UUID, metadata map[string]any) (*ProcessingRequestView, error) {
	now := time.Now()
	return s.transitionRequest(ctx, id, repository.ProcessingRequestStatusPatch{
		OrganizationID:    organizationID,
		Status:            model.ProcessingRequestStatusCompleted,
		AllowedFrom:       AllowedProcessingRequestPreviousStatuses(model.ProcessingRequestStatusCompleted),
		CompletedAt:       &now,
		ExecutionMetadata: metadata,
	})
}

func (s *processingRequestService) FailRequest(ctx context.Context, organizationID string, id uuid.UUID, errorCode string, errorMessage string, metadata map[string]any) (*ProcessingRequestView, error) {
	now := time.Now()
	return s.transitionRequest(ctx, id, repository.ProcessingRequestStatusPatch{
		OrganizationID:    organizationID,
		Status:            model.ProcessingRequestStatusFailed,
		AllowedFrom:       AllowedProcessingRequestPreviousStatuses(model.ProcessingRequestStatusFailed),
		ErrorCode:         &errorCode,
		ErrorMessage:      &errorMessage,
		FailedAt:          &now,
		ExecutionMetadata: metadata,
	})
}

func (s *processingRequestService) CancelRequest(ctx context.Context, organizationID string, id uuid.UUID, reason string) (*ProcessingRequestView, error) {
	now := time.Now()
	metadata := map[string]any{}
	if reason != "" {
		metadata["reason"] = reason
	}
	return s.transitionRequest(ctx, id, repository.ProcessingRequestStatusPatch{
		OrganizationID:    organizationID,
		Status:            model.ProcessingRequestStatusCancelled,
		AllowedFrom:       AllowedProcessingRequestPreviousStatuses(model.ProcessingRequestStatusCancelled),
		CanceledAt:        &now,
		ExecutionMetadata: metadata,
	})
}

func (s *processingRequestService) transitionRequest(ctx context.Context, id uuid.UUID, patch repository.ProcessingRequestStatusPatch) (*ProcessingRequestView, error) {
	if patch.OrganizationID == "" {
		return nil, ErrOrganizationIDRequired
	}
	if id == uuid.Nil {
		return nil, ErrProcessingRequestIDRequired
	}
	item, err := s.repo.TransitionStatus(ctx, id, patch)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, ErrProcessingRequestNotFound
	}
	if item.Status != patch.Status {
		return nil, ErrProcessingRequestTransitionInvalid
	}
	return newProcessingRequestView(item), nil
}

func AllowedProcessingRequestPreviousStatuses(targetStatus string) []string {
	switch targetStatus {
	case model.ProcessingRequestStatusQueued:
		return []string{model.ProcessingRequestStatusPlanned}
	case model.ProcessingRequestStatusRunning:
		return []string{model.ProcessingRequestStatusQueued}
	case model.ProcessingRequestStatusCompleted:
		return []string{model.ProcessingRequestStatusRunning}
	case model.ProcessingRequestStatusFailed:
		return []string{model.ProcessingRequestStatusQueued, model.ProcessingRequestStatusRunning}
	case model.ProcessingRequestStatusCancelled:
		return []string{
			model.ProcessingRequestStatusPlanned,
			model.ProcessingRequestStatusQueued,
			model.ProcessingRequestStatusRunning,
		}
	default:
		return nil
	}
}

func IsProcessingRequestTerminalStatus(status string) bool {
	switch status {
	case model.ProcessingRequestStatusCompleted,
		model.ProcessingRequestStatusFailed,
		model.ProcessingRequestStatusCancelled:
		return true
	default:
		return false
	}
}

func NewProcessingExecutionRequest(view *ProcessingRequestView) (ProcessingExecutionRequest, error) {
	if view == nil {
		return ProcessingExecutionRequest{}, ErrProcessingRequestNotFound
	}
	if view.ID == uuid.Nil {
		return ProcessingExecutionRequest{}, ErrProcessingRequestIDRequired
	}
	if view.OrganizationID == "" {
		return ProcessingExecutionRequest{}, ErrOrganizationIDRequired
	}
	if view.AssetID == uuid.Nil {
		return ProcessingExecutionRequest{}, ErrAssetIDRequired
	}
	if view.Plan == nil {
		return ProcessingExecutionRequest{}, ErrProcessingLevelRequired
	}
	return ProcessingExecutionRequest{
		RequestID:      view.ID,
		OrganizationID: view.OrganizationID,
		WorkspaceID:    view.WorkspaceID,
		AssetID:        view.AssetID,
		TargetLevel:    view.TargetLevel,
		Plan:           view.Plan,
		Force:          view.Force,
	}, nil
}

func ValidateProcessingRequest(req ProcessingRequest) error {
	if req.OrganizationID == "" {
		return ErrOrganizationIDRequired
	}
	if req.AssetID == uuid.Nil {
		return ErrAssetIDRequired
	}
	if req.TargetLevel == "" {
		return ErrProcessingLevelRequired
	}
	if !isSupportedProcessingTarget(req.TargetLevel) {
		return ErrProcessingLevelInvalid
	}
	return nil
}

func PlanProcessingRequest(req ProcessingRequest) (*ProcessingRequestPlan, error) {
	if err := ValidateProcessingRequest(req); err != nil {
		return nil, err
	}
	plan := &ProcessingRequestPlan{
		AssetID:     req.AssetID,
		TargetLevel: req.TargetLevel,
	}
	switch req.TargetLevel {
	case model.DocumentProcessingLevelParse:
		plan.WillParse = true
	case model.DocumentProcessingLevelSplit:
		plan.WillParse = true
		plan.WillSplit = true
	case model.DocumentProcessingLevelVectorize:
		plan.WillParse = true
		plan.WillSplit = true
		plan.WillVectorize = true
	case model.DocumentProcessingLevelFull:
		plan.WillParse = true
		plan.WillSplit = true
		plan.WillVectorize = true
		plan.WillExtractFull = true
	}
	return plan, nil
}

func isSupportedProcessingTarget(targetLevel string) bool {
	switch targetLevel {
	case model.DocumentProcessingLevelParse,
		model.DocumentProcessingLevelSplit,
		model.DocumentProcessingLevelVectorize,
		model.DocumentProcessingLevelFull:
		return true
	default:
		return false
	}
}

func processingExecutorSupportsTarget(info ProcessingRequestExecutorInfo, targetLevel string) bool {
	for _, supportedLevel := range info.TargetLevels {
		if supportedLevel == targetLevel {
			return true
		}
	}
	return false
}

func newProcessingRequestView(item *model.ProcessingRequest) *ProcessingRequestView {
	if item == nil {
		return nil
	}
	return &ProcessingRequestView{
		ID:                item.ID,
		OrganizationID:    item.OrganizationID,
		WorkspaceID:       item.WorkspaceID,
		AssetID:           item.AssetID,
		TargetLevel:       item.TargetLevel,
		Status:            item.Status,
		RequestedBy:       item.RequestedBy,
		Force:             item.Force,
		Plan:              processingPlanFromJSON(item.AssetID, item.TargetLevel, item.PlanJSON),
		RequestMetadata:   item.RequestMetadata,
		ExecutionMetadata: item.ExecutionMetadata,
		ExecutorKey:       item.ExecutorKey,
		ErrorCode:         item.ErrorCode,
		ErrorMessage:      item.ErrorMessage,
		AttemptCount:      item.AttemptCount,
		QueuedAt:          item.QueuedAt,
		StartedAt:         item.StartedAt,
		CompletedAt:       item.CompletedAt,
		FailedAt:          item.FailedAt,
		CanceledAt:        item.CanceledAt,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
	}
}

func processingPlanToJSON(plan *ProcessingRequestPlan) map[string]any {
	if plan == nil {
		return map[string]any{}
	}
	return map[string]any{
		"asset_id":           plan.AssetID.String(),
		"target_level":       plan.TargetLevel,
		"will_parse":         plan.WillParse,
		"will_split":         plan.WillSplit,
		"will_vectorize":     plan.WillVectorize,
		"will_extract_full":  plan.WillExtractFull,
		"execution_attached": false,
	}
}

func processingPlanFromJSON(assetID uuid.UUID, targetLevel string, data map[string]any) *ProcessingRequestPlan {
	plan := &ProcessingRequestPlan{
		AssetID:     assetID,
		TargetLevel: targetLevel,
	}
	if data == nil {
		return plan
	}
	plan.WillParse = boolFromMap(data, "will_parse")
	plan.WillSplit = boolFromMap(data, "will_split")
	plan.WillVectorize = boolFromMap(data, "will_vectorize")
	plan.WillExtractFull = boolFromMap(data, "will_extract_full")
	return plan
}

func boolFromMap(data map[string]any, key string) bool {
	value, ok := data[key]
	if !ok {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func cloneMap(data map[string]any) map[string]any {
	clone := make(map[string]any, len(data)+2)
	for key, value := range data {
		clone[key] = value
	}
	return clone
}
