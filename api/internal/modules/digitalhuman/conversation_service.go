package digitalhuman

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
)

const (
	digitalHumanConversationSource        = runtimemodel.ConversationSourceConsole
	digitalHumanChannelConversationSource = runtimemodel.ConversationSourceChannel
	digitalHumanWebAppConversationSource  = runtimemodel.ConversationSourceWebApp
	digitalHumanAPIConversationSource     = runtimemodel.ConversationSourceExternalAPI
)

// ConversationRuntime is the caller-scoped history surface DigitalHuman uses
// from the shared ChatRuntime. Execution remains owned by the durable run path.
type ConversationRuntime interface {
	ListConversationsByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, int, int) ([]*runtimemodel.Conversation, int64, error)
	SearchByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, string, int) ([]*chatruntime.SearchResult, error)
	GetConversationByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID) (*runtimemodel.Conversation, error)
	UpdateConversationByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID, runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error)
	DeleteConversationByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID) error
	ListConversationMessagesByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID, int, int) ([]*runtimemodel.Message, int64, error)
	ListRuntimeInputs(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID) ([]*runtimemodel.RuntimeInput, error)
	SubmitRuntimeInput(context.Context, chatruntime.Scope, chatruntime.Caller, chatruntime.RunConfig, chatruntime.SubmitRuntimeInputRequest) (*chatruntime.SubmitRuntimeInputResult, error)
	StopConversationByCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID) (*chatruntime.StopConversationResult, error)
	StreamConversationEventsForCaller(context.Context, chatruntime.Scope, chatruntime.Caller, uuid.UUID, uuid.UUID, string, func(chatruntime.StreamEvent) error) error
}

func (s *RuntimeService) SubmitRuntimeInput(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, request chatruntime.SubmitRuntimeInputRequest) (*chatruntime.SubmitRuntimeInputResult, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	result, err := runtime.SubmitRuntimeInput(ctx, runtimeScope, caller, chatruntime.RunConfig{
		RuntimeInputPolicy: chatruntime.RuntimeInputPolicyCheckpointAndFollowUp,
	}, request)
	return result, mapConversationRuntimeError(err)
}

func (s *RuntimeService) ListConversations(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, page, limit int) ([]*runtimemodel.Conversation, int64, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, 0, err
	}
	items, total, err := runtime.ListConversationsByCaller(ctx, runtimeScope, caller, page, limit)
	return items, total, mapConversationRuntimeError(err)
}

func (s *RuntimeService) SearchConversations(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, query string, limit int) ([]*chatruntime.SearchResult, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	items, err := runtime.SearchByCaller(ctx, runtimeScope, caller, strings.TrimSpace(query), limit)
	return items, mapConversationRuntimeError(err)
}

func (s *RuntimeService) GetConversation(ctx context.Context, scope Scope, digitalHumanID, conversationID uuid.UUID) (*runtimemodel.Conversation, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	if conversationID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	conversation, err := runtime.GetConversationByCaller(ctx, runtimeScope, caller, conversationID)
	return conversation, mapConversationRuntimeError(err)
}

func (s *RuntimeService) UpdateConversation(ctx context.Context, scope Scope, digitalHumanID, conversationID uuid.UUID, request runtimedto.UpdateConversationRequest) (*runtimemodel.Conversation, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	if conversationID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	conversation, err := runtime.UpdateConversationByCaller(ctx, runtimeScope, caller, conversationID, request)
	return conversation, mapConversationRuntimeError(err)
}

func (s *RuntimeService) DeleteConversation(ctx context.Context, scope Scope, digitalHumanID, conversationID uuid.UUID) error {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return err
	}
	if conversationID == uuid.Nil {
		return ErrInvalidRequest
	}
	return mapConversationRuntimeError(runtime.DeleteConversationByCaller(ctx, runtimeScope, caller, conversationID))
}

func (s *RuntimeService) ListConversationMessages(ctx context.Context, scope Scope, digitalHumanID, conversationID uuid.UUID, page, limit int) ([]*runtimemodel.Message, int64, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, 0, err
	}
	if conversationID == uuid.Nil {
		return nil, 0, ErrInvalidRequest
	}
	items, total, err := runtime.ListConversationMessagesByCaller(ctx, runtimeScope, caller, conversationID, page, limit)
	return items, total, mapConversationRuntimeError(err)
}

func (s *RuntimeService) ListConversationRuntimeInputs(ctx context.Context, scope Scope, digitalHumanID, conversationID uuid.UUID) ([]*runtimemodel.RuntimeInput, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	if conversationID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	items, err := runtime.ListRuntimeInputs(ctx, runtimeScope, caller, conversationID)
	return items, mapConversationRuntimeError(err)
}

func (s *RuntimeService) StopConversation(ctx context.Context, scope Scope, digitalHumanID, conversationID uuid.UUID) (*chatruntime.StopConversationResult, error) {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return nil, err
	}
	if conversationID == uuid.Nil {
		return nil, ErrInvalidRequest
	}
	runs, listErr := s.ListRuns(ctx, scope, digitalHumanID, ListQuery{Page: 1, PageSize: 100})
	if listErr != nil {
		return nil, listErr
	}
	for _, run := range runs.Items {
		if run.ConversationID == nil || *run.ConversationID != conversationID || !isActiveRunStatus(run.Status) {
			continue
		}
		if _, cancelErr := s.CancelRun(ctx, scope, digitalHumanID, run.ID); cancelErr != nil && !errors.Is(cancelErr, ErrRunConflict) {
			return nil, cancelErr
		}
		break
	}
	result, err := runtime.StopConversationByCaller(ctx, runtimeScope, caller, conversationID)
	return result, mapConversationRuntimeError(err)
}

func (s *RuntimeService) StreamConversationEvents(ctx context.Context, scope Scope, digitalHumanID, conversationID, messageID uuid.UUID, afterID string, onEvent func(chatruntime.StreamEvent) error) error {
	runtime, runtimeScope, caller, err := s.conversationContext(scope, digitalHumanID)
	if err != nil {
		return err
	}
	if conversationID == uuid.Nil || messageID == uuid.Nil || onEvent == nil {
		return ErrInvalidRequest
	}
	return mapConversationRuntimeError(runtime.StreamConversationEventsForCaller(ctx, runtimeScope, caller, conversationID, messageID, strings.TrimSpace(afterID), onEvent))
}

func (s *RuntimeService) conversationContext(scope Scope, digitalHumanID uuid.UUID) (ConversationRuntime, chatruntime.Scope, chatruntime.Caller, error) {
	if !validScope(scope) || digitalHumanID == uuid.Nil {
		return nil, chatruntime.Scope{}, chatruntime.Caller{}, ErrInvalidRequest
	}
	runtime, ok := s.chat.(ConversationRuntime)
	if !ok {
		return nil, chatruntime.Scope{}, chatruntime.Caller{}, fmt.Errorf("digital human conversation runtime is unavailable")
	}
	workspaceID := scope.WorkspaceID
	caller := chatruntime.Caller{
		Type:   runtimemodel.ConversationCallerDigitalHuman,
		ID:     &digitalHumanID,
		Source: digitalHumanConversationSourceForSurface(scope.Surface),
	}
	if scope.Surface == SurfaceWebApp && scope.WebAppID != nil && *scope.WebAppID != uuid.Nil {
		caller.SourceWebAppID = cloneUUIDPointer(scope.WebAppID)
	}
	return runtime,
		chatruntime.Scope{
			OrganizationID:  scope.OrganizationID,
			WorkspaceID:     &workspaceID,
			AccountID:       scope.ActorID,
			SkipAccessCheck: scope.Surface == SurfaceAPI || scope.Surface == SurfaceWebApp,
		},
		caller,
		nil
}

func digitalHumanConversationSourceForSurface(surface string) string {
	switch strings.TrimSpace(surface) {
	case SurfaceWebApp:
		return digitalHumanWebAppConversationSource
	case SurfaceAPI:
		return digitalHumanAPIConversationSource
	case SurfaceChannel:
		return digitalHumanChannelConversationSource
	case SurfacePreview:
		return runtimemodel.ConversationSourcePreview
	default:
		return digitalHumanConversationSource
	}
}

func mapConversationRuntimeError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, chatruntime.ErrNotFound):
		return fmt.Errorf("%w: %v", ErrConversationNotFound, err)
	case errors.Is(err, chatruntime.ErrInvalidInput):
		return fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	case errors.Is(err, chatruntime.ErrConversationIdle), errors.Is(err, chatruntime.ErrRuntimeInputDisabled):
		return fmt.Errorf("%w: %v", ErrRunConflict, err)
	default:
		return err
	}
}

func isActiveRunStatus(status RunStatus) bool {
	switch status {
	case RunStatusQueued, RunStatusRunning, RunStatusCanceling:
		return true
	default:
		return false
	}
}
