package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/agentmemoryworker"
	llmclient "github.com/zgiai/zgi/api/internal/modules/llm/client"
	"github.com/zgiai/zgi/api/internal/modules/relationshipmemory"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	agentMemoryConfigRevisionConflictCode = "agent_memory_config_revision_conflict"
	agentMemoryValueRevisionConflictCode  = "agent_memory_revision_conflict"
	agentMemoryRevisionConflictMessage    = "Agent memory has changed. Refresh and try again."
)

func (s *agentsService) ListAgentMemorySlots(ctx context.Context, agentID, accountID string) ([]dto.AgentMemorySlotConfig, error) {
	ag, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	return s.agentMemorySlotsForDraft(ctx, ag.ID), nil
}

func (s *agentsService) ReplaceAgentMemorySlots(ctx context.Context, agentID, accountID string, slots []dto.AgentMemorySlotConfig) ([]dto.AgentMemorySlotConfig, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	if s.db == nil || s.agentMemoryService == nil {
		return nil, fmt.Errorf("database and agent memory service are required")
	}
	actorID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, fmt.Errorf("account id is invalid")
	}
	var result []dto.AgentMemorySlotConfig
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current AgentsConfig
		query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL")
		if cfg != nil && cfg.ID != uuid.Nil {
			query = query.Where("id = ?", cfg.ID)
		} else {
			query = query.Where("agents_id = ?", ag.ID).Order("updated_at DESC")
		}
		if err := query.Take(&current).Error; err != nil {
			return err
		}
		memorySvc := agentmemory.NewService(tx)
		updated, err := memorySvc.ReplaceSlots(ctx, ag.ID, actorID, agentMemoryReplaceRequestFromConfig(slots, true))
		if err != nil {
			return err
		}
		result = agentMemorySlotConfigsFromResponses(updated)
		config := agentConfigResponse(ag.ID.String(), &current)
		revision := agentMemoryConfigRevision(config.AgentMemoryEnabled, config.AgentMemoryAutoExtractionEnabled, result)
		return memorySvc.SetAgentConfigRevision(ctx, ag.TenantID, ag.ID, agentmemory.ConfigScopeDraft, revision)
	})
	return result, err
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
	if s.relationshipMemory != nil && relationshipmemory.RuntimeEnabled() {
		slots, slotErr := s.publishedRelationshipMemorySlots(ctx, ag.ID)
		if slotErr != nil {
			return nil, slotErr
		}
		privacySlots, privacyErr := s.relationshipMemory.PrivacySlots(ctx, ag.TenantID, ag.ID, slots, agentmemory.UserScopeAccount, targetUserID)
		if privacyErr != nil {
			return nil, privacyErr
		}
		exported, readErr := s.relationshipMemory.ExportUserMemory(ctx, ag.TenantID, ag.ID, privacySlots, agentmemory.UserScopeAccount, targetUserID)
		if readErr != nil {
			return nil, readErr
		}
		return &dto.AgentMemoryValuesResponse{UserScope: agentmemory.UserScopeAccount, UserID: targetUserID.String(), Values: agentMemoryValueConfigsFromResponses(exported.Values)}, nil
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
	if s.relationshipMemory != nil && relationshipmemory.RuntimeEnabled() {
		slots, slotErr := s.publishedRelationshipMemorySlots(ctx, ag.ID)
		if slotErr != nil {
			return nil, slotErr
		}
		value, updateErr := s.relationshipMemory.UpdateValue(ctx, ag.TenantID, ag.ID, slots, agentmemory.UserScopeAccount, targetUserID, agentmemory.UpdateValueRequest{Key: req.Key, Content: req.Content, ExpectedRevision: req.ExpectedRevision}, agentmemory.MutationMetadata{ActorType: agentmemory.EventActorOrganizer, Source: agentmemory.EventSourceAPI, SourceKind: agentmemory.SourceKindManager})
		if updateErr != nil {
			return nil, updateErr
		}
		resp := agentMemoryValueConfigFromResponse(*value)
		return &resp, nil
	}
	value, err := s.agentMemoryService.UpdateOrganizerValue(ctx, ag.ID, agentmemory.UserScopeAccount, targetUserID, agentmemory.UpdateValueRequest{
		Key:              req.Key,
		Content:          req.Content,
		ExpectedRevision: req.ExpectedRevision,
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
	if s.relationshipMemory != nil && relationshipmemory.RuntimeEnabled() {
		slots, slotErr := s.publishedRelationshipMemorySlots(ctx, ag.ID)
		if slotErr != nil {
			return nil, slotErr
		}
		value, clearErr := s.relationshipMemory.ClearValue(ctx, ag.TenantID, ag.ID, slots, agentmemory.UserScopeAccount, targetUserID, key, agentmemory.MutationMetadata{ActorType: agentmemory.EventActorOrganizer, Source: agentmemory.EventSourceAPI, SourceKind: agentmemory.SourceKindManager})
		if clearErr != nil {
			return nil, clearErr
		}
		resp := agentMemoryValueConfigFromResponse(*value)
		return &resp, nil
	}
	value, err := s.agentMemoryService.ClearOrganizerValue(ctx, ag.ID, agentmemory.UserScopeAccount, targetUserID, key)
	if err != nil {
		return nil, err
	}
	resp := agentMemoryValueConfigFromResponse(*value)
	return &resp, nil
}

func (s *agentsService) publishedRelationshipMemorySlots(ctx context.Context, agentID uuid.UUID) ([]agentmemory.RuntimeSlot, error) {
	version, err := s.agentsRepo.GetLatestAgentPublishedVersion(ctx, agentID.String())
	if err != nil {
		return nil, err
	}
	config := agentConfigResponseFromSnapshot(agentID.String(), version.ConfigSnapshot)
	if config == nil || !config.AgentMemoryEnabled {
		return nil, fmt.Errorf("Agent memory is disabled in the published version")
	}
	return agentMemoryRuntimeStoreSlots(agentMemoryRuntimeSlots(config.AgentMemorySlots)), nil
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
		Revision:              value.Revision,
		SourceKind:            value.SourceKind,
		SourceConversationID:  value.SourceConversationID,
		SourceMessageID:       value.SourceMessageID,
		SourceCompletedAt:     value.SourceCompletedAt,
		ExtractorVersion:      value.ExtractorVersion,
		LastOperationID:       value.LastOperationID,
		UndoableUntil:         value.UndoableUntil,
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
		ReadOnly:         slot.ReadOnly,
		AutoWrite:        agentMemoryAutoWrite(&slot.AutoWrite),
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
			MaxChars:    slot.MaxChars,
			Enabled:     &enabled,
			SortOrder:   firstNonZeroInt(slot.SortOrder, i),
			ReadOnly:    slot.ReadOnly,
			AutoWrite:   agentMemoryAutoWrite(slot.AutoWrite),
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
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
			ReadOnly:    slot.ReadOnly,
			AutoWrite:   agentMemoryAutoWrite(slot.AutoWrite),
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

// Agent memory block limits (ADR 0013).
const (
	maxAgentMemorySlots            = 10
	maxAgentMemorySlotChars        = 5000
	maxAgentMemoryDescriptionChars = 1000
)

// maxAgentMemoryInstructionsChars bounds the builder's memory instructions.
const maxAgentMemoryInstructionsChars = 2000

func normalizeAgentMemoryInstructions(value string) string {
	return truncateRunes(strings.TrimSpace(value), maxAgentMemoryInstructionsChars)
}

// agentMemoryAutoWrite resolves an optional auto_write: absent means true.
func agentMemoryAutoWrite(value *bool) *bool {
	enabled := value == nil || *value
	return &enabled
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
		if slot.MaxChars > 0 && slot.MaxChars <= maxAgentMemorySlotChars {
			maxChars = slot.MaxChars
		}
		sortOrder := slot.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		out = append(out, dto.AgentMemorySlotConfig{
			ID:          strings.TrimSpace(slot.ID),
			Key:         key,
			Name:        truncateRunes(strings.TrimSpace(slot.Name), 80),
			Description: truncateRunes(strings.TrimSpace(slot.Description), maxAgentMemoryDescriptionChars),
			MaxChars:    maxChars,
			Enabled:     slot.Enabled,
			SortOrder:   sortOrder,
			ReadOnly:    slot.ReadOnly,
			AutoWrite:   agentMemoryAutoWrite(slot.AutoWrite),
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

func (s *agentsService) UpdateAgentMemoryConfig(ctx context.Context, agentID, accountID string, req dto.AgentMemoryConfigRequest) (*dto.AgentMemoryConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	if s.db == nil || s.agentMemoryService == nil {
		return nil, fmt.Errorf("database and agent memory service are required")
	}
	actorID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, fmt.Errorf("account id is invalid")
	}
	var result *dto.AgentMemoryConfigResponse
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Table("agents").Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND deleted_at IS NULL", ag.ID).Select("id").Take(&struct{ ID uuid.UUID }{}).Error; err != nil {
			return err
		}
		var current AgentsConfig
		query := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL")
		if cfg != nil && cfg.ID != uuid.Nil {
			query = query.Where("id = ?", cfg.ID)
		} else {
			query = query.Where("agents_id = ?", ag.ID).Order("updated_at DESC")
		}
		if err := query.Take(&current).Error; err != nil {
			return err
		}
		memorySvc := agentmemory.NewService(tx)
		currentSlots, err := memorySvc.ListSlots(ctx, ag.ID)
		if err != nil {
			return err
		}
		currentConfig := agentConfigResponse(ag.ID.String(), &current)
		currentRevision := agentMemoryConfigRevision(currentConfig.AgentMemoryEnabled, currentConfig.AgentMemoryAutoExtractionEnabled, agentMemorySlotConfigsFromResponses(currentSlots))
		if expected := strings.TrimSpace(req.ConfigRevision); expected != "" && expected != currentRevision {
			return &agentBindingAPIError{Code: agentMemoryConfigRevisionConflictCode, Message: "agent memory configuration has changed", Data: map[string]interface{}{"config_revision": currentRevision}}
		}
		configReq := agentConfigRequestFromResponse(*currentConfig)
		configReq.AgentMemoryEnabled = req.Enabled
		configReq.AgentMemoryAutoExtractionEnabled = req.AutoExtractionEnabled
		if req.Instructions != nil {
			if utf8.RuneCountInString(strings.TrimSpace(*req.Instructions)) > maxAgentMemoryInstructionsChars {
				return fmt.Errorf("%w: memory instructions exceed %d characters", agentmemory.ErrInvalidInput, maxAgentMemoryInstructionsChars)
			}
			configReq.AgentMemoryInstructions = normalizeAgentMemoryInstructions(*req.Instructions)
		}
		if _, err := applyAgentConfigRequestToDraft(&current, configReq, accountID); err != nil {
			return err
		}
		current.UpdatedBy = &actorID
		if err := NewAgentsRepository(tx).UpdateAgentsConfig(ctx, &current); err != nil {
			return err
		}
		updatedSlots, err := memorySvc.ReplaceSlots(ctx, ag.ID, actorID, agentMemoryReplaceRequestFromConfig(req.Slots, true))
		if err != nil {
			return err
		}
		slots := agentMemorySlotConfigsFromResponses(updatedSlots)
		result = &dto.AgentMemoryConfigResponse{Enabled: req.Enabled, AutoExtractionEnabled: req.AutoExtractionEnabled, Slots: slots,
			ConfigRevision: agentMemoryConfigRevision(req.Enabled, req.AutoExtractionEnabled, slots), Instructions: configReq.AgentMemoryInstructions}
		if err := memorySvc.SetAgentConfigRevision(ctx, ag.TenantID, ag.ID, agentmemory.ConfigScopeDraft, result.ConfigRevision); err != nil {
			return err
		}
		return nil
	})
	return result, err
}

// PreviewAgentMemory runs a dry consolidation pass (ADR 0013) over a sample
// conversation with the given or draft memory configuration, using the
// Agent's draft model. Nothing is saved.
func (s *agentsService) PreviewAgentMemory(ctx context.Context, agentID, accountID, organizationID string, req dto.AgentMemoryPreviewRequest) (interface{}, error) {
	if s.llmClient == nil {
		return nil, fmt.Errorf("memory preview needs a model client")
	}
	draft, err := s.GetAgentDraftRuntimeConfig(ctx, agentID, accountID)
	if err != nil {
		return nil, err
	}
	slots := draft.Config.AgentMemorySlots
	if req.Slots != nil {
		if len(*req.Slots) > maxAgentMemorySlots {
			return nil, fmt.Errorf("%w: at most %d memory blocks", agentmemory.ErrInvalidInput, maxAgentMemorySlots)
		}
		slots = normalizeAgentMemorySlotConfigs(*req.Slots)
	}
	instructions := draft.Config.AgentMemoryInstructions
	if req.Instructions != nil {
		if utf8.RuneCountInString(strings.TrimSpace(*req.Instructions)) > maxAgentMemoryInstructionsChars {
			return nil, fmt.Errorf("%w: memory instructions exceed %d characters", agentmemory.ErrInvalidInput, maxAgentMemoryInstructionsChars)
		}
		instructions = normalizeAgentMemoryInstructions(*req.Instructions)
	}
	runtimeSlots := make([]agentmemory.RuntimeSlot, 0, len(slots))
	for _, slot := range enabledAgentMemorySlots(slots) {
		runtimeSlots = append(runtimeSlots, agentmemory.RuntimeSlot{
			Key: slot.Key, Name: slot.Name, Description: slot.Description, MaxChars: slot.MaxChars, Enabled: true,
			SortOrder: slot.SortOrder, ReadOnly: slot.ReadOnly, ManualOnly: !*agentMemoryAutoWrite(slot.AutoWrite),
		})
	}
	if len(runtimeSlots) == 0 {
		return nil, fmt.Errorf("%w: memory preview needs at least one enabled block", agentmemory.ErrInvalidInput)
	}
	turns := make([]agentmemoryworker.PreviewTurn, 0, len(req.Turns))
	for _, turn := range req.Turns {
		turns = append(turns, agentmemoryworker.PreviewTurn{User: turn.User, Assistant: turn.Assistant})
	}
	result, err := agentmemoryworker.Preview(ctx, s.llmClient, agentmemoryworker.PreviewRequest{
		Slots: runtimeSlots, Instructions: instructions, Turns: turns, Current: req.Current,
		Provider: draft.Config.ModelProvider, Model: draft.Config.Model,
		AppContext: &llmclient.AppContext{
			OrganizationID: organizationID, WorkspaceID: draft.WorkspaceID, BillingSubjectType: llmclient.BillingSubjectTypeOrganization,
			AppID: draft.AgentID, AppType: "agent", AccountID: accountID,
		},
	})
	if err != nil {
		if errors.Is(err, agentmemory.ErrInvalidInput) {
			return nil, err
		}
		return nil, fmt.Errorf("memory preview: %w", err)
	}
	return result, nil
}

func agentMemoryConfigRevision(enabled, automatic bool, slots []dto.AgentMemorySlotConfig) string {
	runtimeSlots := make([]agentmemory.RuntimeSlot, 0, len(slots))
	for _, slot := range slots {
		runtimeSlots = append(runtimeSlots, agentmemory.RuntimeSlot{
			Key:         slot.Key,
			Name:        slot.Name,
			Description: slot.Description,
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
			ReadOnly:    slot.ReadOnly,
			ManualOnly:  !*agentMemoryAutoWrite(slot.AutoWrite),
		})
	}
	return agentmemory.ConfigRevision(enabled, automatic, runtimeSlots)
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
