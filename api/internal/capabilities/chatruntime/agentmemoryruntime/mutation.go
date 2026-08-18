package agentmemoryruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func Execute(ctx context.Context, req MutationRequest, rawArguments string) MutationResult {
	arguments, err := parseToolArguments(rawArguments)
	if err != nil {
		return failedMutation(err, nil)
	}
	operations, summary, err := prepareMutations(req, arguments)
	if err != nil {
		return failedMutation(err, summary)
	}
	response, err := req.MemoryService.MutateValues(
		ctx,
		req.WorkspaceID,
		req.AgentID,
		RuntimeSlots(req.Slots),
		req.UserScope,
		req.UserID,
		agentmemory.MutateValuesRequest{Operations: operations},
		req.MutationMetadata,
	)
	if err != nil {
		return failedMutation(err, summary)
	}
	result := map[string]interface{}{
		"status":     "success",
		"operations": response.Operations,
	}
	return MutationResult{Status: "success", Arguments: summary, Result: result, Response: response}
}

func parseToolArguments(raw string) (ToolArguments, error) {
	var arguments ToolArguments
	decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&arguments); err != nil {
		return ToolArguments{}, fmt.Errorf("%w: malformed memory tool arguments", ErrInvalidInput)
	}
	if decoder.More() {
		return ToolArguments{}, fmt.Errorf("%w: malformed memory tool arguments", ErrInvalidInput)
	}
	return arguments, nil
}

func prepareMutations(req MutationRequest, arguments ToolArguments) ([]agentmemory.ValueMutation, map[string]interface{}, error) {
	summary := summarizeOperations(arguments.Operations)
	if req.MemoryService == nil || req.WorkspaceID == zeroUUID || req.AgentID == zeroUUID || req.UserID == zeroUUID || req.MutationMetadata.SourceMessageID == nil || *req.MutationMetadata.SourceMessageID == zeroUUID {
		return nil, summary, fmt.Errorf("%w: agent memory runtime scope is invalid", ErrInvalidInput)
	}
	if len(arguments.Operations) == 0 || len(arguments.Operations) > 5 {
		return nil, summary, fmt.Errorf("%w: memory batch must contain between one and five operations", ErrInvalidInput)
	}
	latestUserMessage := strings.TrimSpace(req.LatestUserMessage)
	if latestUserMessage == "" {
		return nil, summary, fmt.Errorf("%w: current-user evidence is required", ErrInvalidInput)
	}
	revisions := make(map[string]int64, len(req.CurrentValues))
	for _, value := range req.CurrentValues {
		revisions[strings.TrimSpace(value.Key)] = value.Revision
	}
	seen := make(map[string]struct{}, len(arguments.Operations))
	mutations := make([]agentmemory.ValueMutation, 0, len(arguments.Operations))
	for _, raw := range arguments.Operations {
		action := strings.ToLower(strings.TrimSpace(raw.Action))
		key := strings.TrimSpace(raw.Key)
		mode := strings.ToLower(strings.TrimSpace(raw.Mode))
		evidence := strings.TrimSpace(raw.Evidence)
		content := strings.TrimSpace(raw.Content)
		if action != agentmemory.MutationActionUpsert && action != agentmemory.MutationActionClear {
			return nil, summary, fmt.Errorf("%w: unsupported memory action", ErrInvalidInput)
		}
		if mode != agentMemoryModeExplicit && mode != agentMemoryModeProactive {
			return nil, summary, fmt.Errorf("%w: unsupported memory mode", ErrInvalidInput)
		}
		if evidence == "" || !strings.Contains(latestUserMessage, evidence) {
			return nil, summary, fmt.Errorf("%w: evidence is not an exact quote from the current user message", ErrInvalidInput)
		}
		slot, ok := findEnabledSlot(req.Slots, key)
		if !ok {
			return nil, summary, fmt.Errorf("%w: memory key is not enabled", ErrInvalidInput)
		}
		key = slot.Key
		if _, exists := seen[key]; exists {
			return nil, summary, fmt.Errorf("%w: duplicate memory key", ErrInvalidInput)
		}
		seen[key] = struct{}{}
		if mode == agentMemoryModeProactive {
			if !req.ProactiveAllowed {
				return nil, summary, fmt.Errorf("%w: proactive memory maintenance is disabled", ErrInvalidInput)
			}
			if action != agentmemory.MutationActionUpsert {
				return nil, summary, fmt.Errorf("%w: proactive operation is not allowed for this memory block", ErrInvalidInput)
			}
		}
		if action == agentmemory.MutationActionUpsert {
			if content == "" {
				return nil, summary, fmt.Errorf("%w: memory content is required", ErrInvalidInput)
			}
			if slot.MaxChars > 0 && len([]rune(content)) > slot.MaxChars {
				return nil, summary, fmt.Errorf("%w: memory content exceeds slot limit", ErrInvalidInput)
			}
			if agentmemory.ContainsSensitiveContent(content) || agentmemory.ContainsSensitiveContent(evidence) {
				return nil, summary, fmt.Errorf("%w: sensitive content cannot be saved", ErrInvalidInput)
			}
		} else if content != "" {
			return nil, summary, fmt.Errorf("%w: clear operation must not include content", ErrInvalidInput)
		}
		mutations = append(mutations, agentmemory.ValueMutation{
			Action:           action,
			Key:              key,
			Content:          content,
			Mode:             mode,
			ExpectedRevision: revisions[key],
			OperationID:      deterministicOperationID(*req.MutationMetadata.SourceMessageID, key, action, mode),
		})
	}
	return mutations, summary, nil
}

func deterministicOperationID(messageID uuid.UUID, key, action, mode string) uuid.UUID {
	token := action
	if mode == agentMemoryModeExplicit {
		if action == agentmemory.MutationActionUpsert {
			token = legacyToolUpdate
		} else {
			token = legacyToolClear
		}
		return uuid.NewSHA1(uuid.NameSpaceOID, []byte(messageID.String()+":"+key+":"+token))
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(messageID.String()+":"+key+":"+action+":"+mode))
}

func summarizeOperations(operations []ToolOperation) map[string]interface{} {
	keys := make([]string, 0, len(operations))
	actions := make([]string, 0, len(operations))
	modes := make([]string, 0, len(operations))
	for _, operation := range operations {
		keys = append(keys, strings.TrimSpace(operation.Key))
		actions = append(actions, strings.ToLower(strings.TrimSpace(operation.Action)))
		modes = append(modes, strings.ToLower(strings.TrimSpace(operation.Mode)))
	}
	sort.Strings(keys)
	return map[string]interface{}{
		"operation_count": len(operations),
		"keys":            keys,
		"actions":         actions,
		"modes":           modes,
	}
}

func failedMutation(err error, arguments map[string]interface{}) MutationResult {
	return MutationResult{
		Status:    "failed",
		Arguments: arguments,
		Result: map[string]interface{}{
			"status":     "failed",
			"error_code": ErrorCode(err),
		},
		Error: err,
	}
}

func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, ErrInvalidInput), errors.Is(err, agentmemory.ErrInvalidInput), errors.Is(err, agentmemory.ErrUnauthorized):
		return "invalid_arguments"
	case errors.Is(err, agentmemory.ErrConflict):
		return "conflict"
	default:
		return "storage_unavailable"
	}
}

func ValidationError(err error) bool {
	return ErrorCode(err) == "invalid_arguments"
}

func findEnabledSlot(slots []Slot, key string) (Slot, bool) {
	key = strings.TrimSpace(key)
	for _, slot := range slots {
		if slot.Enabled && strings.TrimSpace(slot.Key) == key {
			return slot, true
		}
	}
	return Slot{}, false
}
