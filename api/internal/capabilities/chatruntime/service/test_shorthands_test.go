package service

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimedto "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/dto"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

// Helpers only tests use: shorthands for calls production makes with explicit
// arguments, passing the AIChat caller's defaults or an empty optional value,
// and builders for fixtures such as legacy transcript metadata.

func streamingMessageMetadata(parts *chatRequestParts) map[string]interface{} {
	return streamingMessageMetadataWithTaskID(parts, "")
}

func BuildStreamErrorPayload(prepared *PreparedChat, err error, requestID string) map[string]interface{} {
	return BuildLocalizedStreamErrorPayload(prepared, err, requestID, "")
}

func hydrateMessageGeneratedFileURLs(message *runtimemodel.Message) {
	hydrateMessageGeneratedFileURLsWithLookup(message, nil, false, time.Now())
}

func applyOperationPlanTerminalCompletionResult(metadata map[string]interface{}, status string, reason string, missingSteps []string, unsupportedClaims []string, nextActionHint string) {
	applyOperationPlanTerminalCompletionResultWithSource(metadata, status, "", reason, missingSteps, unsupportedClaims, nextActionHint)
}

func (s *service) beginClientActionContinuation(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, actionID string) (*ClientActionContinuation, error) {
	return s.beginConfiguredClientActionContinuation(
		ctx,
		scope,
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		conversationID,
		messageID,
		actionID,
	)
}

func (s *service) beginClientActionContinuationForCaller(ctx context.Context, scope Scope, caller Caller, conversationID, messageID uuid.UUID, actionID string) (*ClientActionContinuation, error) {
	return s.beginConfiguredClientActionContinuation(ctx, scope, caller, conversationID, messageID, actionID)
}

func (s *service) prepareClientActionContinuationChat(ctx context.Context, scope Scope, continuation *ClientActionContinuation, req runtimedto.ClientActionResultRequest) (*PreparedChat, error) {
	return s.prepareConfiguredClientActionContinuationChat(
		ctx,
		scope,
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		RunConfig{BillingAppType: runtimemodel.MessageBillingReasonSourceAIChat},
		continuation,
		req,
		configuredClientActionContinuationOptions{},
	)
}

func (s *service) persistClientActionPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	metadata, _ := s.persistClientActionPendingResult(ctx, prepared, payload, usage)
	return metadata
}

func (s *service) persistToolGovernanceApprovalPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	metadata, _ := s.persistToolGovernanceApprovalPendingResult(ctx, prepared, payload, usage)
	return metadata
}

func (s *service) persistUserInputRequestPending(ctx context.Context, prepared *PreparedChat, payload map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	metadata, _ := s.persistUserInputRequestPendingResult(ctx, prepared, payload, usage)
	return metadata
}

func (s *service) beginToolGovernanceContinuation(ctx context.Context, scope Scope, conversationID, messageID uuid.UUID, correlationID string) (*ToolGovernanceContinuation, error) {
	return s.beginToolGovernanceContinuationForCaller(ctx, scope, Caller{Type: runtimemodel.ConversationCallerAIChat}, conversationID, messageID, correlationID)
}

func (s *service) prepareToolGovernanceContinuationChat(ctx context.Context, scope Scope, continuation *ToolGovernanceContinuation) (*PreparedChat, error) {
	return s.prepareConfiguredToolGovernanceContinuationChat(
		ctx,
		scope,
		Caller{Type: runtimemodel.ConversationCallerAIChat},
		RunConfig{BillingAppType: runtimemodel.MessageBillingReasonSourceAIChat},
		continuation,
	)
}

func effectiveAgentSkillIDs(input []string, catalog []skills.SkillDiscoveryMetadata, organizationEnabled []string, runConfig *RunConfig) []string {
	return effectiveBoundSkillIDs(input, catalog, organizationEnabled, runConfig, runtimemodel.ConversationCallerAgent)
}

func skillLoopAdditionalSystemMessages(prepared *PreparedChat) []adapter.Message {
	return skillLoopAdditionalSystemMessagesForResolved(prepared, nil)
}

func contextualConsoleAgentsSkillMessage(prepared *PreparedChat) (adapter.Message, bool) {
	return contextualConsoleAgentsSkillMessageForResolved(prepared, nil)
}

func requestedManagedFileTargetsFromQuery(query string) []requestedManagedFileTarget {
	return explicitRequestedManagedFileTargetsFromQuery(query)
}

func applyProviderPromptUsageCalibration(metadata map[string]interface{}, usage *adapter.Usage) map[string]interface{} {
	return applyProviderPromptUsageCalibrationWithEstimate(metadata, usage, 0, "", "", "")
}

func (s *service) emitAgentMemoryMutationEvents(ctx context.Context, prepared *PreparedChat, response *agentmemory.MutateValuesResponse, onEvent func(StreamEvent) error) {
	s.forEachAgentMemoryMutationEvent(prepared, response, func(eventType string, payload map[string]interface{}) {
		_ = s.emitPreparedEvent(ctx, prepared, eventType, payload, onEvent)
	})
}

func mergeAgentTranscriptMetadata(source map[string]interface{}, transcript []adapter.Message, answer string) map[string]interface{} {
	metadata := copyStringAnyMap(source)
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	normalized := normalizeAgentTranscript(transcript, answer)
	if len(normalized) == 0 {
		delete(metadata, agentTranscriptMetadataKey)
		delete(metadata, agentTranscriptVersionMetadataKey)
		return metadata
	}
	metadata[agentTranscriptVersionMetadataKey] = agentTranscriptSchemaVersion
	metadata[agentTranscriptMetadataKey] = normalized
	return metadata
}

func stringSliceContainsFold(values []string, target string) bool {
	target = strings.TrimSpace(target)
	if target == "" {
		return false
	}
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), target) {
			return true
		}
	}
	return false
}

func cloneChatRequest(source *adapter.ChatRequest) *adapter.ChatRequest {
	if source == nil {
		return &adapter.ChatRequest{}
	}
	cloned := *source
	cloned.Messages = append([]adapter.Message{}, source.Messages...)
	cloned.Stop = append([]string{}, source.Stop...)
	if source.AdditionalParameters != nil {
		cloned.AdditionalParameters = copyStringAnyMap(source.AdditionalParameters)
	}
	if source.LogitBias != nil {
		cloned.LogitBias = make(map[string]float64, len(source.LogitBias))
		for key, value := range source.LogitBias {
			cloned.LogitBias[key] = value
		}
	}
	return &cloned
}

func operationPlanToolStepID(skillID, toolName string) string {
	skillID = strings.TrimSpace(skillID)
	toolName = strings.TrimSpace(toolName)
	if skillID == "" && toolName == "" {
		return ""
	}
	if toolName == "" {
		return "skill:" + skillID
	}
	return "tool:" + skillID + "/" + toolName
}
