package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	connectorbindings "github.com/zgiai/zgi/api/internal/modules/connectors/bindings"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	connectorpreferences "github.com/zgiai/zgi/api/internal/modules/connectors/preferences"
	connectorruntimeauth "github.com/zgiai/zgi/api/internal/modules/connectors/runtimeauth"
	"gorm.io/gorm"
)

func initialChatConnections(caller Caller, req runtimedto.ChatRequest) ([]connectorbindings.InitialChatConnection, error) {
	if !hasInitialChatConnections(req) {
		return nil, nil
	}
	if req.InitialConnectorDefaultsRevision != nil && *req.InitialConnectorDefaultsRevision < 0 {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if len(req.InitialConnectorBindings) > connectorbindings.MaxInitialChatConnections {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	if normalizeCallerType(caller.Type) != runtimemodel.ConversationCallerAIChat || strings.TrimSpace(req.ConversationID) != "" || strings.TrimSpace(req.ParentID) != "" {
		return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	selections := make([]connectorbindings.InitialChatConnection, 0, len(req.InitialConnectorBindings))
	for _, input := range req.InitialConnectorBindings {
		connectionID, err := uuid.Parse(input.ConnectionID)
		if err != nil || connectionID.String() != input.ConnectionID {
			return nil, core.NewError(core.ErrorCodeInvalidInput, false, nil)
		}
		selections = append(selections, connectorbindings.InitialChatConnection{ConnectorID: core.Identifier(input.ConnectorID), ConnectionID: connectionID})
	}
	if err := connectorbindings.ValidateInitialChatConnections(selections); err != nil {
		return nil, err
	}
	return selections, nil
}

func hasInitialChatConnections(req runtimedto.ChatRequest) bool {
	// A present empty list is still first-turn input. In particular, an empty
	// final selection must not bypass its reviewed defaults snapshot revision.
	return req.InitialConnectorBindings != nil || req.InitialConnectorDefaultsRevision != nil
}

func (s *service) createChatWithInitialConnections(ctx context.Context, scope Scope, caller Caller, parts *chatRequestParts, selections []connectorbindings.InitialChatConnection, defaultsRevision *int64) (*runtimemodel.Conversation, error) {
	var conversation *runtimemodel.Conversation
	initialTitle := conversationTitleFallback(parts.Query, initialConversationTitle())
	err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if defaultsRevision != nil {
			// Preference updates/deletion use this same owner lock first. Check
			// before conversation creation and lifecycle connection locks so a
			// changed snapshot cannot leave a chat or a binding behind.
			if err := connectorpreferences.ValidateChatRevision(ctx, tx, scope.OrganizationID, scope.AccountID, *defaultsRevision); err != nil {
				return err
			}
		}
		transactionService := *s
		transactionService.repos = repository.NewRepositories(tx)
		var err error
		conversation, err = transactionService.createConversationForCaller(ctx, scope, caller, initialTitle, parts.Surface)
		if err != nil {
			return err
		}
		if err := connectorbindings.InitializeChatConnections(ctx, tx, scope.OrganizationID, scope.AccountID, conversation.ID, selections); err != nil {
			return err
		}
		if len(selections) == 0 {
			return nil
		}
		validator, ok := s.connectorChatRuntime.(connectorruntimeauth.InitialChatConnectionValidator)
		if !ok {
			return core.NewError(core.ErrorCodeInternal, false, nil)
		}
		return validator.ValidateInitialChatConnections(ctx, tx, connectorruntimeauth.ChatCapabilityRequest{
			OrganizationID: scope.OrganizationID, OwnerAccountID: scope.AccountID, ConversationID: conversation.ID.String(),
		}, selections)
	})
	if err != nil {
		return nil, err
	}
	if s.titleGen != nil {
		s.markConversationTitleGenerationPending(ctx, scope, conversation)
		s.generateConversationTitleAsync(ctx, scope, caller, conversation, parts, initialTitle)
	}
	return conversation, nil
}
