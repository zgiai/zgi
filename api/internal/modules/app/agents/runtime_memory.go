package agents

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"sort"
	"strings"
)

func (s *agentsService) ListAgentMemorySlots(ctx context.Context, agentID, accountID string) ([]dto.AgentMemorySlotConfig, error) {
	ag, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	return s.agentMemorySlotsForDraft(ctx, ag.ID), nil
}

func (s *agentsService) ReplaceAgentMemorySlots(ctx context.Context, agentID, accountID string, slots []dto.AgentMemorySlotConfig) ([]dto.AgentMemorySlotConfig, error) {
	ag, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	if s.agentMemoryService == nil {
		return nil, fmt.Errorf("agent memory service is not configured")
	}
	actorID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, fmt.Errorf("account id is invalid")
	}
	updated, err := s.agentMemoryService.ReplaceSlots(ctx, ag.ID, actorID, agentMemoryReplaceRequestFromConfig(slots, true))
	if err != nil {
		return nil, err
	}
	return agentMemorySlotConfigsFromResponses(updated), nil
}

func (s *agentsService) ListAgentMemoryValues(ctx context.Context, agentID, accountID string) (*dto.AgentMemoryValuesResponse, error) {
	ag, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	if s.agentMemoryService == nil {
		return nil, fmt.Errorf("agent memory service is not configured")
	}
	targetUserID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, fmt.Errorf("account id is invalid")
	}
	values, err := s.agentMemoryService.ListOrganizerValues(ctx, ag.ID, agentmemory.UserScopeAccount, targetUserID)
	if err != nil {
		return nil, err
	}
	return &dto.AgentMemoryValuesResponse{
		UserScope: agentmemory.UserScopeAccount,
		UserID:    targetUserID.String(),
		Values:    agentMemoryValueConfigsFromResponses(values),
	}, nil
}

func (s *agentsService) UpdateAgentMemoryValue(ctx context.Context, agentID, accountID string, req dto.UpdateAgentMemoryValueRequest) (*dto.AgentMemoryValueResponse, error) {
	ag, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	if s.agentMemoryService == nil {
		return nil, fmt.Errorf("agent memory service is not configured")
	}
	targetUserID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, fmt.Errorf("account id is invalid")
	}
	value, err := s.agentMemoryService.UpdateOrganizerValue(ctx, ag.ID, agentmemory.UserScopeAccount, targetUserID, agentmemory.UpdateValueRequest{
		Key:     req.Key,
		Content: req.Content,
	})
	if err != nil {
		return nil, err
	}
	resp := agentMemoryValueConfigFromResponse(*value)
	return &resp, nil
}

func (s *agentsService) ClearAgentMemoryValue(ctx context.Context, agentID, accountID, key string) (*dto.AgentMemoryValueResponse, error) {
	ag, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	if s.agentMemoryService == nil {
		return nil, fmt.Errorf("agent memory service is not configured")
	}
	targetUserID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, fmt.Errorf("account id is invalid")
	}
	value, err := s.agentMemoryService.ClearOrganizerValue(ctx, ag.ID, agentmemory.UserScopeAccount, targetUserID, key)
	if err != nil {
		return nil, err
	}
	resp := agentMemoryValueConfigFromResponse(*value)
	return &resp, nil
}

func (s *agentsService) agentMemorySlotsForDraft(ctx context.Context, agentID uuid.UUID) []dto.AgentMemorySlotConfig {
	slots, err := s.loadAgentMemorySlotsForDraft(ctx, agentID)
	if err != nil {
		return []dto.AgentMemorySlotConfig{}
	}
	return slots
}

func (s *agentsService) loadAgentMemorySlotsForDraft(ctx context.Context, agentID uuid.UUID) ([]dto.AgentMemorySlotConfig, error) {
	if s.agentMemoryService == nil || agentID == uuid.Nil {
		return []dto.AgentMemorySlotConfig{}, nil
	}
	slots, err := s.agentMemoryService.ListSlots(ctx, agentID)
	if err != nil {
		return nil, fmt.Errorf("list agent memory slots: %w", err)
	}
	return agentMemorySlotConfigsFromResponses(slots), nil
}

func enabledAgentMemorySlots(slots []dto.AgentMemorySlotConfig) []dto.AgentMemorySlotConfig {
	out := make([]dto.AgentMemorySlotConfig, 0, len(slots))
	for _, slot := range slots {
		if slot.Enabled {
			out = append(out, slot)
		}
	}
	return out
}

func agentMemorySlotConfigsFromResponses(slots []agentmemory.SlotResponse) []dto.AgentMemorySlotConfig {
	out := make([]dto.AgentMemorySlotConfig, 0, len(slots))
	for _, slot := range slots {
		out = append(out, agentMemorySlotConfigFromResponse(slot))
	}
	return normalizeAgentMemorySlotConfigs(out)
}

func agentMemoryValueConfigsFromResponses(values []agentmemory.SlotValueResponse) []dto.AgentMemoryValueResponse {
	out := make([]dto.AgentMemoryValueResponse, 0, len(values))
	for _, value := range values {
		out = append(out, agentMemoryValueConfigFromResponse(value))
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func agentMemoryValueConfigFromResponse(value agentmemory.SlotValueResponse) dto.AgentMemoryValueResponse {
	return dto.AgentMemoryValueResponse{
		AgentMemorySlotConfig: agentMemorySlotConfigFromResponse(value.SlotResponse),
		Content:               value.Content,
	}
}

func agentMemorySlotConfigFromResponse(slot agentmemory.SlotResponse) dto.AgentMemorySlotConfig {
	return dto.AgentMemorySlotConfig{
		ID:               slot.ID,
		Key:              strings.TrimSpace(slot.Key),
		Name:             strings.TrimSpace(slot.Name),
		Description:      strings.TrimSpace(slot.Description),
		MaxChars:         slot.MaxChars,
		Enabled:          slot.Enabled,
		SortOrder:        slot.SortOrder,
		CreatedAt:        slot.CreatedAt,
		UpdatedAt:        slot.UpdatedAt,
		CreatedAtUnix:    slot.CreatedAtUnix,
		UpdatedAtUnix:    slot.UpdatedAtUnix,
		CreatedAtISO:     slot.CreatedAtISO,
		UpdatedAtISO:     slot.UpdatedAtISO,
		CreatedAtDisplay: slot.CreatedAtDisplay,
		UpdatedAtDisplay: slot.UpdatedAtDisplay,
	}
}

func agentMemoryReplaceRequestFromConfig(slots []dto.AgentMemorySlotConfig, preserveIDs bool) agentmemory.ReplaceSlotsRequest {
	req := agentmemory.ReplaceSlotsRequest{Slots: make([]agentmemory.SlotUpsertRequest, 0, len(slots))}
	for i, slot := range slots {
		enabled := slot.Enabled
		id := ""
		if preserveIDs {
			id = strings.TrimSpace(slot.ID)
		}
		req.Slots = append(req.Slots, agentmemory.SlotUpsertRequest{
			ID:          id,
			Key:         strings.TrimSpace(slot.Key),
			Name:        strings.TrimSpace(slot.Name),
			Description: strings.TrimSpace(slot.Description),
			MaxChars:    2000,
			Enabled:     &enabled,
			SortOrder:   firstNonZeroInt(slot.SortOrder, i),
		})
	}
	return req
}

func agentMemorySnapshotSlots(slots []dto.AgentMemorySlotConfig) []dto.AgentMemorySlotConfig {
	out := make([]dto.AgentMemorySlotConfig, 0, len(slots))
	for _, slot := range slots {
		key := strings.TrimSpace(slot.Key)
		if key == "" {
			continue
		}
		out = append(out, dto.AgentMemorySlotConfig{
			Key:         key,
			Name:        truncateRunes(strings.TrimSpace(slot.Name), 80),
			Description: strings.TrimSpace(slot.Description),
			MaxChars:    2000,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
		})
	}
	return normalizeAgentMemorySlotConfigs(out)
}

func agentMemoryKeys(slots []dto.AgentMemorySlotConfig) []string {
	keys := make([]string, 0, len(slots))
	for _, slot := range slots {
		key := strings.TrimSpace(slot.Key)
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func firstNonZeroInt(value, fallback int) int {
	if value != 0 {
		return value
	}
	return fallback
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit])
}

func normalizeAgentMemorySlotConfigs(slots []dto.AgentMemorySlotConfig) []dto.AgentMemorySlotConfig {
	if len(slots) == 0 {
		return []dto.AgentMemorySlotConfig{}
	}
	out := make([]dto.AgentMemorySlotConfig, 0, len(slots))
	seen := map[string]struct{}{}
	for i, slot := range slots {
		key := strings.ToLower(strings.TrimSpace(slot.Key))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		maxChars := 2000
		sortOrder := slot.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		out = append(out, dto.AgentMemorySlotConfig{
			ID:          strings.TrimSpace(slot.ID),
			Key:         key,
			Name:        truncateRunes(strings.TrimSpace(slot.Name), 80),
			Description: truncateRunes(strings.TrimSpace(slot.Description), 200),
			MaxChars:    maxChars,
			Enabled:     slot.Enabled,
			SortOrder:   sortOrder,
			CreatedAt:   slot.CreatedAt,
			UpdatedAt:   slot.UpdatedAt,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].SortOrder != out[j].SortOrder {
			return out[i].SortOrder < out[j].SortOrder
		}
		return out[i].Key < out[j].Key
	})
	return out
}

func agentMemorySlotConfigsFromSnapshot(raw interface{}) []dto.AgentMemorySlotConfig {
	if raw == nil {
		return []dto.AgentMemorySlotConfig{}
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return []dto.AgentMemorySlotConfig{}
	}
	var slots []dto.AgentMemorySlotConfig
	if err := json.Unmarshal(data, &slots); err != nil {
		return []dto.AgentMemorySlotConfig{}
	}
	return normalizeAgentMemorySlotConfigs(slots)
}
