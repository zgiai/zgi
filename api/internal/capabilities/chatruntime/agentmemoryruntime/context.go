package agentmemoryruntime

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
)

func BuildContext(ctx context.Context, req ContextRequest) (ContextResult, error) {
	if !req.Enabled || len(enabledSlots(req.Slots)) == 0 {
		return ContextResult{SystemPrompt: req.SystemPrompt}, nil
	}
	slots := enabledSlots(req.Slots)
	metadata := map[string]interface{}{
		"agent_memory": map[string]interface{}{
			"enabled":        true,
			"available":      false,
			"injected":       false,
			"context_status": "skipped_scope",
		},
	}
	state := &State{
		Enabled:            true,
		AutoCaptureEnabled: true,
		AgentID:            req.AgentID,
		UserScope:          strings.TrimSpace(req.UserScope),
		ConfigScope:        strings.TrimSpace(req.ConfigScope),
		ConfigRevision:     strings.TrimSpace(req.ConfigRevision),
		EnabledSlots:       slots,
		ContextStatus:      "skipped_scope",
	}
	if req.MemoryService == nil || req.WorkspaceID == zeroUUID || req.AgentID == zeroUUID {
		return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
	}
	if settings, ok := req.MemoryService.(RelationshipSettingsService); ok {
		memoryEnabled, autoCaptureEnabled, err := settings.RelationshipRuntimeSettings(ctx, req.WorkspaceID, req.AgentID, req.UserScope, req.UserID)
		if err != nil {
			state.Enabled = false
			state.ContextStatus = "error"
			metadata["agent_memory"] = map[string]interface{}{"enabled": false, "available": false, "injected": false, "context_status": "error"}
			return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
		}
		state.RelationshipSettingsResolved = true
		state.Enabled = memoryEnabled
		state.AutoCaptureEnabled = autoCaptureEnabled
		if !memoryEnabled {
			state.ContextStatus = "disabled_by_user"
			metadata["agent_memory"] = map[string]interface{}{"enabled": false, "available": false, "injected": false, "context_status": "disabled_by_user"}
			return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
		}
	}
	epoch, err := req.MemoryService.ReadRuntimeFence(ctx, req.WorkspaceID, req.AgentID, req.UserScope, req.UserID, req.ConfigScope, req.ConfigRevision)
	if err != nil {
		state.ContextStatus = "error"
		metadata["agent_memory"] = map[string]interface{}{
			"enabled":        true,
			"available":      false,
			"injected":       false,
			"context_status": "error",
		}
		return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
	}
	state.MemoryEpoch = &epoch
	values, err := req.MemoryService.ReadUserMemory(ctx, req.WorkspaceID, req.AgentID, RuntimeSlots(slots), req.UserScope, req.UserID)
	if err != nil {
		state.ContextStatus = "error"
		metadata["agent_memory"] = map[string]interface{}{
			"enabled":        true,
			"available":      false,
			"injected":       false,
			"context_status": "error",
		}
		return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
	}
	state.SavedValues = append([]agentmemory.SlotValueResponse(nil), values...)
	state.ContextStatus = "success"
	rendered, injected := Render(values, req.Budget)
	metadata["agent_memory"] = map[string]interface{}{
		"enabled":        true,
		"available":      len(injected) > 0,
		"injected":       strings.TrimSpace(rendered) != "",
		"value_count":    len(injected),
		"values":         injected,
		"context_status": "success",
	}
	if strings.TrimSpace(rendered) == "" {
		return ContextResult{SystemPrompt: req.SystemPrompt, Metadata: metadata, State: state}, nil
	}
	return ContextResult{SystemPrompt: req.SystemPrompt, Context: rendered, Metadata: metadata, State: state}, nil
}

func enabledSlots(input []Slot) []Slot {
	out := make([]Slot, 0, len(input))
	for _, slot := range input {
		if slot.Enabled {
			out = append(out, slot)
		}
	}
	return out
}
