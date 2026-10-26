package agentmemoryruntime

import (
	"errors"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

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
