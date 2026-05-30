package usermemoryruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/memory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func ApplyDecision(ctx context.Context, req PreflightRequest, decision Decision) (map[string]interface{}, skills.SkillTrace, error) {
	toolName, args, err := ValidateDecision(decision, req.State)
	if err != nil {
		return nil, DecisionTrace(decision, "rejected_validation", nil, err), err
	}
	started := time.Now()
	result, err := ExecuteTool(ctx, req, toolName, args)
	trace := DecisionTrace(decision, "success", result, nil)
	trace.Kind = "tool_call"
	trace.ToolName = toolName
	trace.Arguments = summarizeArguments(toolName, args)
	trace.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		trace.Status = "mutation_error"
		trace.Error = err.Error()
		return map[string]interface{}{"error": err.Error()}, trace, err
	}
	if req.OnMutation != nil {
		req.OnMutation(trace, result)
	}
	return result, trace, nil
}

func ExecuteTool(ctx context.Context, req PreflightRequest, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	if req.MemoryService == nil {
		return nil, fmt.Errorf("%w: user memory runtime service is unavailable", ErrInvalidInput)
	}
	accountID := req.AccountID
	if accountID == uuid.Nil && req.State != nil {
		accountID = req.State.AccountID
	}
	if accountID == uuid.Nil {
		return nil, fmt.Errorf("%w: user memory runtime account scope is invalid", ErrInvalidInput)
	}
	switch toolName {
	case ToolCreate:
		entry, err := req.MemoryService.CreateEntryWithMetadata(ctx, accountID, memory.CreateEntryRequest{
			Content:    stringArg(args, "content"),
			Category:   stringArg(args, "category"),
			MemoryType: stringArg(args, "memory_type"),
			ExpiresAt:  stringArg(args, "expires_at"),
		}, req.MutationMetadata)
		if err != nil {
			return nil, err
		}
		return Result(ActionCreate, entry), nil
	default:
		return executeNonCreateTool(ctx, req, accountID, toolName, args)
	}
}

func executeNonCreateTool(ctx context.Context, req PreflightRequest, accountID uuid.UUID, toolName string, args map[string]interface{}) (map[string]interface{}, error) {
	switch toolName {
	case ToolUpdate:
		entryID, err := uuid.Parse(stringArg(args, "entry_id"))
		if err != nil {
			return nil, fmt.Errorf("%w: entry_id is invalid", ErrInvalidInput)
		}
		content := stringArg(args, "content")
		category := stringArg(args, "category")
		memoryType := stringArg(args, "memory_type")
		expiresAt := stringArg(args, "expires_at")
		updateReq := memory.UpdateEntryRequest{
			Content:    &content,
			Category:   &category,
			MemoryType: &memoryType,
			ExpiresAt:  &expiresAt,
		}
		entry, err := req.MemoryService.UpdateEntryWithMetadata(ctx, accountID, entryID, updateReq, req.MutationMetadata)
		if err != nil {
			return nil, err
		}
		return Result(ActionUpdate, entry), nil
	case ToolDelete:
		entryID, err := uuid.Parse(stringArg(args, "entry_id"))
		if err != nil {
			return nil, fmt.Errorf("%w: entry_id is invalid", ErrInvalidInput)
		}
		if err := req.MemoryService.DeleteEntryWithMetadata(ctx, accountID, entryID, req.MutationMetadata); err != nil {
			return nil, err
		}
		return map[string]interface{}{"status": "success", "action": ActionDelete, "entry_id": entryID.String()}, nil
	default:
		return nil, fmt.Errorf("%w: unsupported user memory tool %s", ErrInvalidInput, toolName)
	}
}

func ValidateDecision(decision Decision, state *State) (string, map[string]interface{}, error) {
	action := strings.ToLower(strings.TrimSpace(decision.Action))
	if action == "" || action == ActionNone || action == ActionAskConfirmation {
		return "", nil, fmt.Errorf("%w: memory decision has no mutation", ErrInvalidInput)
	}
	if containsSensitiveContent(decision.Content) {
		return "", nil, fmt.Errorf("%w: sensitive content cannot be saved to user memory", ErrInvalidInput)
	}
	args := map[string]interface{}{}
	switch action {
	case ActionCreate:
		if strings.TrimSpace(decision.Content) == "" {
			return "", nil, fmt.Errorf("%w: content is required", ErrInvalidInput)
		}
		expiresAt := normalizedDecisionExpiresAtForMutation(decision.MemoryType, decision.ExpiresAt)
		args["content"] = decision.Content
		args["category"] = normalizedCategory(decision.Category)
		args["memory_type"] = normalizedMemoryType(decision.MemoryType, expiresAt)
		args["expires_at"] = expiresAt
		return ToolCreate, args, nil
	case ActionUpdate:
		if !entryExists(state, decision.EntryID) {
			return "", nil, fmt.Errorf("%w: entry_id is not available", ErrInvalidInput)
		}
		if strings.TrimSpace(decision.Content) == "" {
			return "", nil, fmt.Errorf("%w: content is required", ErrInvalidInput)
		}
		args["entry_id"] = decision.EntryID
		args["content"] = decision.Content
		args["category"] = normalizedCategory(decision.Category)
		expiresAt := normalizedDecisionExpiresAtForMutation(decision.MemoryType, decision.ExpiresAt)
		args["memory_type"] = normalizedMemoryType(decision.MemoryType, expiresAt)
		args["expires_at"] = expiresAt
		return ToolUpdate, args, nil
	case ActionDelete:
		if !entryExists(state, decision.EntryID) {
			return "", nil, fmt.Errorf("%w: entry_id is not available", ErrInvalidInput)
		}
		args["entry_id"] = decision.EntryID
		return ToolDelete, args, nil
	default:
		return "", nil, fmt.Errorf("%w: unsupported memory decision action", ErrInvalidInput)
	}
}

func normalizedDecisionExpiresAtForMutation(memoryType string, expiresAt string) string {
	value := normalizeDecisionExpiresAt(expiresAt)
	if strings.ToLower(strings.TrimSpace(memoryType)) == memory.MemoryTypeLongTerm {
		return ""
	}
	return value
}

func Result(action string, entry *memory.MemoryEntryResponse) map[string]interface{} {
	result := map[string]interface{}{"status": "success", "action": action}
	if entry == nil {
		return result
	}
	result["entry_id"] = entry.ID
	result["content"] = entry.Content
	result["category"] = entry.Category
	result["memory_type"] = entry.MemoryType
	result["updated_at"] = entry.UpdatedAt
	return result
}

func entryExists(state *State, entryID string) bool {
	entryID = strings.TrimSpace(entryID)
	if state == nil || entryID == "" {
		return false
	}
	for _, entry := range state.Entries {
		if strings.TrimSpace(entry.ID) == entryID {
			return true
		}
	}
	return false
}

func normalizedCategory(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case memory.CategoryProfile, memory.CategoryPreference, memory.CategoryInstruction, memory.CategoryFact:
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return memory.CategoryOther
	}
}

func normalizedMemoryType(raw string, expiresAt string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case memory.MemoryTypeTemporary:
		return memory.MemoryTypeTemporary
	default:
		if strings.TrimSpace(expiresAt) != "" {
			return memory.MemoryTypeTemporary
		}
		return memory.MemoryTypeLongTerm
	}
}

func summarizeArguments(toolName string, args map[string]interface{}) map[string]interface{} {
	summary := map[string]interface{}{}
	if entryID := stringArg(args, "entry_id"); entryID != "" {
		summary["entry_id"] = entryID
	}
	if category := stringArg(args, "category"); category != "" {
		summary["category"] = category
	}
	if memoryType := stringArg(args, "memory_type"); memoryType != "" {
		summary["memory_type"] = memoryType
	}
	if toolName == ToolCreate || toolName == ToolUpdate {
		content := stringArg(args, "content")
		if content != "" {
			summary["content_preview"] = truncateRunes(content, 160)
			summary["content_chars"] = len([]rune(content))
		}
	}
	return summary
}

func stringArg(args map[string]interface{}, key string) string {
	if args == nil {
		return ""
	}
	value, ok := args[key]
	if !ok || value == nil {
		return ""
	}
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func containsSensitiveContent(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}
	if containsLongDigitRun(normalized, 12) {
		return true
	}
	return containsAny(normalized, []string{
		"password", "passwd", "passcode", "credential", "credentials", "secret", "api key", "apikey", "access token", "refresh token", "private key", "credit card", "bank card", "card number", "ssn",
		"\u5bc6\u7801", "\u53e3\u4ee4", "\u51ed\u636e", "\u4ee4\u724c", "\u79d8\u94a5", "\u94f6\u884c\u5361", "\u4fe1\u7528\u5361", "\u8eab\u4efd\u8bc1", "\u8bc1\u4ef6\u53f7", "\u9a8c\u8bc1\u7801", "\u652f\u4ed8",
	})
}

func containsAny(value string, needles []string) bool {
	for _, needle := range needles {
		if strings.Contains(value, strings.ToLower(needle)) {
			return true
		}
	}
	return false
}

func containsLongDigitRun(value string, limit int) bool {
	run := 0
	for _, r := range value {
		if r >= '0' && r <= '9' {
			run++
			if run >= limit {
				return true
			}
			continue
		}
		run = 0
	}
	return false
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}
