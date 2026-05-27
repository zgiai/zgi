package agentmemory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	defaultSlotMaxChars     = 1000
	maxSlotMaxChars         = 4000
	maxSlotDescriptionChars = 1000
	maxSlotsPerAgent        = 50
	defaultRenderBudget     = 4000
)

var (
	ErrInvalidInput = errors.New("invalid agent memory input")
	ErrNotFound     = errors.New("agent memory not found")
	ErrUnauthorized = errors.New("agent memory requester is unauthorized")

	memoryKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

type Service struct {
	repo store
}

func NewService(db *gorm.DB) *Service {
	return &Service{repo: NewRepository(db)}
}

type store interface {
	WithTransaction(ctx context.Context, fn func(store) error) error
	ResolveAgentWorkspace(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error)
	LockAgent(ctx context.Context, agentID uuid.UUID) error
	ListSlots(ctx context.Context, workspaceID, agentID uuid.UUID, enabledOnly bool) ([]*AgentMemorySlot, error)
	CreateSlot(ctx context.Context, slot *AgentMemorySlot) error
	UpdateSlotScoped(ctx context.Context, workspaceID, agentID, slotID uuid.UUID, values map[string]interface{}) (*AgentMemorySlot, error)
	ListValuesForUser(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) ([]*AgentMemoryValue, error)
	GetValueScoped(ctx context.Context, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID) (*AgentMemoryValue, error)
	UpsertValue(ctx context.Context, value *AgentMemoryValue) error
	CreateEvent(ctx context.Context, event *AgentMemoryEvent) error
}

type RuntimeSlot struct {
	Key         string `json:"key"`
	Description string `json:"description"`
	MaxChars    int    `json:"max_chars"`
	Enabled     bool   `json:"enabled"`
	SortOrder   int    `json:"sort_order"`
}

type MutationMetadata struct {
	ActorType            string
	Source               string
	SourceConversationID *uuid.UUID
	SourceMessageID      *uuid.UUID
}

func (s *Service) ListSlots(ctx context.Context, agentID uuid.UUID) ([]SlotResponse, error) {
	workspaceID, err := s.resolveAgentWorkspace(ctx, agentID)
	if err != nil {
		return nil, err
	}
	slots, err := s.repo.ListSlots(ctx, workspaceID, agentID, false)
	if err != nil {
		return nil, fmt.Errorf("list agent memory slots: %w", err)
	}
	return slotResponses(slots), nil
}

func (s *Service) ReplaceSlots(ctx context.Context, agentID, actorID uuid.UUID, req ReplaceSlotsRequest) ([]SlotResponse, error) {
	if agentID == uuid.Nil {
		return nil, ErrUnauthorized
	}
	if len(req.Slots) > maxSlotsPerAgent {
		return nil, fmt.Errorf("%w: too many memory slots", ErrInvalidInput)
	}

	normalized := make([]normalizedSlotInput, 0, len(req.Slots))
	seen := map[string]struct{}{}
	for i, item := range req.Slots {
		slot, err := normalizeSlotInput(item, i)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[slot.key]; ok {
			return nil, fmt.Errorf("%w: duplicate memory key %s", ErrInvalidInput, slot.key)
		}
		seen[slot.key] = struct{}{}
		normalized = append(normalized, slot)
	}

	var response []SlotResponse
	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		if err := tx.LockAgent(ctx, agentID); err != nil {
			return mapRepoError(err, "lock agent")
		}
		workspaceID, err := tx.ResolveAgentWorkspace(ctx, agentID)
		if err != nil {
			return mapRepoError(err, "resolve agent workspace")
		}
		existing, err := tx.ListSlots(ctx, workspaceID, agentID, false)
		if err != nil {
			return fmt.Errorf("list existing agent memory slots: %w", err)
		}
		existingByKey := map[string]*AgentMemorySlot{}
		for _, slot := range existing {
			if slot != nil {
				existingByKey[slot.Key] = slot
			}
		}
		now := time.Now()
		for _, input := range normalized {
			if current := existingByKey[input.key]; current != nil {
				before := *current
				updated, err := tx.UpdateSlotScoped(ctx, workspaceID, agentID, current.ID, map[string]interface{}{
					"description": input.description,
					"max_chars":   input.maxChars,
					"enabled":     input.enabled,
					"sort_order":  input.sortOrder,
					"updated_by":  actorID,
					"updated_at":  now,
				})
				if err != nil {
					return mapRepoError(err, "update agent memory slot")
				}
				if err := recordSlotEvent(ctx, tx, workspaceID, agentID, updated.Key, slotUpdateAction(&before, updated), organizerMetadata(), &before, updated); err != nil {
					return err
				}
				delete(existingByKey, input.key)
				continue
			}
			slot := &AgentMemorySlot{
				WorkspaceID: workspaceID,
				AgentID:     agentID,
				Key:         input.key,
				Description: input.description,
				MaxChars:    input.maxChars,
				Enabled:     input.enabled,
				SortOrder:   input.sortOrder,
				CreatedBy:   actorID,
				UpdatedBy:   actorID,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			if err := tx.CreateSlot(ctx, slot); err != nil {
				return fmt.Errorf("create agent memory slot: %w", err)
			}
			if err := recordSlotEvent(ctx, tx, workspaceID, agentID, slot.Key, EventActionSlotCreate, organizerMetadata(), nil, slot); err != nil {
				return err
			}
		}
		for _, stale := range existingByKey {
			if stale == nil || !stale.Enabled {
				continue
			}
			before := *stale
			updated, err := tx.UpdateSlotScoped(ctx, workspaceID, agentID, stale.ID, map[string]interface{}{
				"enabled":    false,
				"updated_by": actorID,
				"updated_at": now,
			})
			if err != nil {
				return mapRepoError(err, "disable missing agent memory slot")
			}
			if err := recordSlotEvent(ctx, tx, workspaceID, agentID, updated.Key, EventActionSlotDisable, organizerMetadata(), &before, updated); err != nil {
				return err
			}
		}
		slots, err := tx.ListSlots(ctx, workspaceID, agentID, false)
		if err != nil {
			return fmt.Errorf("list updated agent memory slots: %w", err)
		}
		response = slotResponses(slots)
		return nil
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) ReadUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID) ([]SlotValueResponse, error) {
	slots = normalizeRuntimeSlots(slots)
	if len(slots) == 0 {
		return []SlotValueResponse{}, nil
	}
	userScope, err := s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	values, err := s.repo.ListValuesForUser(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, fmt.Errorf("list agent memory values: %w", err)
	}
	return runtimeSlotValueResponses(slots, values), nil
}

func (s *Service) UpdateValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID, req UpdateValueRequest, meta MutationMetadata) (*SlotValueResponse, error) {
	key, err := normalizeKey(req.Key)
	if err != nil {
		return nil, err
	}
	slot, err := runtimeSlotByKey(slots, key)
	if err != nil {
		return nil, err
	}
	userScope, err = s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		return nil, fmt.Errorf("%w: content is required", ErrInvalidInput)
	}

	var response *SlotValueResponse
	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		if len([]rune(content)) > slot.MaxChars {
			return fmt.Errorf("%w: content exceeds max_chars for %s", ErrInvalidInput, key)
		}
		before, err := tx.GetValueScoped(ctx, workspaceID, agentID, slot.Key, userScope, userID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get agent memory value: %w", err)
		}
		value := &AgentMemoryValue{
			WorkspaceID: workspaceID,
			AgentID:     agentID,
			SlotKey:     slot.Key,
			UserScope:   userScope,
			UserID:      userID,
			Content:     content,
		}
		if err := tx.UpsertValue(ctx, value); err != nil {
			return fmt.Errorf("upsert agent memory value: %w", err)
		}
		after, err := tx.GetValueScoped(ctx, workspaceID, agentID, slot.Key, userScope, userID)
		if err != nil {
			return fmt.Errorf("get updated agent memory value: %w", err)
		}
		if err := recordValueEvent(ctx, tx, workspaceID, agentID, slot.Key, userScope, userID, EventActionValueUpdate, meta, before, after); err != nil {
			return err
		}
		resp := runtimeSlotValueResponse(slot, after)
		response = &resp
		return nil
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) ClearValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID, key string, meta MutationMetadata) (*SlotValueResponse, error) {
	key, err := normalizeKey(key)
	if err != nil {
		return nil, err
	}
	slot, err := runtimeSlotByKey(slots, key)
	if err != nil {
		return nil, err
	}
	userScope, err = s.resolveRuntimeScope(userScope, userID)
	if err != nil {
		return nil, err
	}
	var response *SlotValueResponse
	if err := s.repo.WithTransaction(ctx, func(tx store) error {
		before, err := tx.GetValueScoped(ctx, workspaceID, agentID, slot.Key, userScope, userID)
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("get agent memory value: %w", err)
		}
		value := &AgentMemoryValue{
			WorkspaceID: workspaceID,
			AgentID:     agentID,
			SlotKey:     slot.Key,
			UserScope:   userScope,
			UserID:      userID,
			Content:     "",
		}
		if err := tx.UpsertValue(ctx, value); err != nil {
			return fmt.Errorf("clear agent memory value: %w", err)
		}
		after, err := tx.GetValueScoped(ctx, workspaceID, agentID, slot.Key, userScope, userID)
		if err != nil {
			return fmt.Errorf("get cleared agent memory value: %w", err)
		}
		if err := recordValueEvent(ctx, tx, workspaceID, agentID, slot.Key, userScope, userID, EventActionValueClear, meta, before, after); err != nil {
			return err
		}
		resp := runtimeSlotValueResponse(slot, after)
		response = &resp
		return nil
	}); err != nil {
		return nil, err
	}
	return response, nil
}

func (s *Service) RenderContext(ctx context.Context, workspaceID, agentID uuid.UUID, slots []RuntimeSlot, userScope string, userID uuid.UUID, budget int) (string, error) {
	if budget <= 0 {
		budget = defaultRenderBudget
	}
	entries, err := s.ReadUserMemory(ctx, workspaceID, agentID, slots, userScope, userID)
	if err != nil {
		return "", err
	}
	return renderMemoryContext(entries, budget), nil
}

func (s *Service) resolveAgentWorkspace(ctx context.Context, agentID uuid.UUID) (uuid.UUID, error) {
	if agentID == uuid.Nil {
		return uuid.Nil, ErrUnauthorized
	}
	workspaceID, err := s.repo.ResolveAgentWorkspace(ctx, agentID)
	if err != nil {
		return uuid.Nil, mapRepoError(err, "resolve agent workspace")
	}
	return workspaceID, nil
}

func (s *Service) resolveRuntimeScope(userScope string, userID uuid.UUID) (string, error) {
	if userID == uuid.Nil {
		return "", ErrUnauthorized
	}
	return normalizeUserScope(userScope), nil
}

type normalizedSlotInput struct {
	key         string
	description string
	maxChars    int
	enabled     bool
	sortOrder   int
}

func normalizeSlotInput(req SlotUpsertRequest, index int) (normalizedSlotInput, error) {
	key, err := normalizeKey(req.Key)
	if err != nil {
		return normalizedSlotInput{}, err
	}
	description := strings.TrimSpace(req.Description)
	if len([]rune(description)) > maxSlotDescriptionChars {
		return normalizedSlotInput{}, fmt.Errorf("%w: description is too long for %s", ErrInvalidInput, key)
	}
	maxChars := req.MaxChars
	if maxChars <= 0 {
		maxChars = defaultSlotMaxChars
	}
	if maxChars > maxSlotMaxChars {
		return normalizedSlotInput{}, fmt.Errorf("%w: max_chars exceeds limit for %s", ErrInvalidInput, key)
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	sortOrder := req.SortOrder
	if sortOrder == 0 {
		sortOrder = index
	}
	return normalizedSlotInput{
		key:         key,
		description: description,
		maxChars:    maxChars,
		enabled:     enabled,
		sortOrder:   sortOrder,
	}, nil
}

func normalizeKey(raw string) (string, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	if key == "" {
		return "", fmt.Errorf("%w: key is required", ErrInvalidInput)
	}
	if !memoryKeyPattern.MatchString(key) {
		return "", fmt.Errorf("%w: key must start with a lowercase letter and contain only lowercase letters, numbers, and underscores", ErrInvalidInput)
	}
	return key, nil
}

func normalizeUserScope(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case UserScopeEndUser:
		return UserScopeEndUser
	default:
		return UserScopeAccount
	}
}

func normalizeRuntimeSlots(slots []RuntimeSlot) []RuntimeSlot {
	normalized := make([]RuntimeSlot, 0, len(slots))
	seen := map[string]struct{}{}
	for i, slot := range slots {
		key, err := normalizeKey(slot.Key)
		if err != nil {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		maxChars := slot.MaxChars
		if maxChars <= 0 {
			maxChars = defaultSlotMaxChars
		}
		if maxChars > maxSlotMaxChars {
			maxChars = maxSlotMaxChars
		}
		sortOrder := slot.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		normalized = append(normalized, RuntimeSlot{
			Key:         key,
			Description: strings.TrimSpace(slot.Description),
			MaxChars:    maxChars,
			Enabled:     slot.Enabled,
			SortOrder:   sortOrder,
		})
	}
	sort.SliceStable(normalized, func(i, j int) bool {
		if normalized[i].SortOrder != normalized[j].SortOrder {
			return normalized[i].SortOrder < normalized[j].SortOrder
		}
		return normalized[i].Key < normalized[j].Key
	})
	return normalized
}

func runtimeSlotByKey(slots []RuntimeSlot, key string) (RuntimeSlot, error) {
	for _, slot := range normalizeRuntimeSlots(slots) {
		if slot.Key == key && slot.Enabled {
			return slot, nil
		}
	}
	return RuntimeSlot{}, fmt.Errorf("%w: memory key %s is not configured for this agent version", ErrInvalidInput, key)
}

func slotResponses(slots []*AgentMemorySlot) []SlotResponse {
	out := make([]SlotResponse, 0, len(slots))
	for _, slot := range slots {
		if slot == nil {
			continue
		}
		out = append(out, slotResponse(slot))
	}
	return out
}

func slotResponse(slot *AgentMemorySlot) SlotResponse {
	return SlotResponse{
		ID:          slot.ID.String(),
		Key:         slot.Key,
		Description: slot.Description,
		MaxChars:    slot.MaxChars,
		Enabled:     slot.Enabled,
		SortOrder:   slot.SortOrder,
		CreatedAt:   slot.CreatedAt.Unix(),
		UpdatedAt:   slot.UpdatedAt.Unix(),
	}
}

func slotValueResponses(slots []*AgentMemorySlot, values []*AgentMemoryValue) []SlotValueResponse {
	valuesBySlot := map[string]*AgentMemoryValue{}
	for _, value := range values {
		if value != nil {
			valuesBySlot[value.SlotKey] = value
		}
	}
	out := make([]SlotValueResponse, 0, len(slots))
	for _, slot := range slots {
		if slot == nil {
			continue
		}
		out = append(out, slotValueResponse(slot, valuesBySlot[slot.Key]))
	}
	return out
}

func runtimeSlotValueResponses(slots []RuntimeSlot, values []*AgentMemoryValue) []SlotValueResponse {
	valuesBySlot := map[string]*AgentMemoryValue{}
	for _, value := range values {
		if value != nil {
			valuesBySlot[value.SlotKey] = value
		}
	}
	out := make([]SlotValueResponse, 0, len(slots))
	for _, slot := range slots {
		out = append(out, runtimeSlotValueResponse(slot, valuesBySlot[slot.Key]))
	}
	return out
}

func slotValueResponse(slot *AgentMemorySlot, value *AgentMemoryValue) SlotValueResponse {
	resp := SlotValueResponse{SlotResponse: slotResponse(slot)}
	if value != nil {
		resp.Content = value.Content
	}
	return resp
}

func runtimeSlotValueResponse(slot RuntimeSlot, value *AgentMemoryValue) SlotValueResponse {
	resp := SlotValueResponse{
		SlotResponse: SlotResponse{
			Key:         slot.Key,
			Description: slot.Description,
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
		},
	}
	if value != nil {
		resp.Content = value.Content
		resp.CreatedAt = value.CreatedAt.Unix()
		resp.UpdatedAt = value.UpdatedAt.Unix()
	}
	return resp
}

func renderMemoryContext(entries []SlotValueResponse, budget int) string {
	if len(entries) == 0 {
		return ""
	}
	var builder strings.Builder
	header := "Agent memory is enabled for this agent and current user.\nOnly use the listed memory keys. Do not invent new keys or temporary memories.\n\nAvailable memory slots:\n"
	if len(header) > budget {
		return ""
	}
	builder.WriteString(header)
	for _, entry := range entries {
		line := fmt.Sprintf("- key: %s\n  description: %s\n  max_chars: %d\n  content: %s\n",
			entry.Key,
			strings.TrimSpace(entry.Description),
			entry.MaxChars,
			strings.TrimSpace(entry.Content),
		)
		if builder.Len()+len(line) > budget {
			break
		}
		builder.WriteString(line)
	}
	return strings.TrimSpace(builder.String())
}

func organizerMetadata() MutationMetadata {
	return MutationMetadata{ActorType: EventActorOrganizer, Source: EventSourceAPI}
}

func modelMetadata(conversationID *string, messageID *string) MutationMetadata {
	meta := MutationMetadata{ActorType: EventActorModel, Source: EventSourceAgent}
	if conversationID != nil {
		if id, err := uuid.Parse(*conversationID); err == nil {
			meta.SourceConversationID = &id
		}
	}
	if messageID != nil {
		if id, err := uuid.Parse(*messageID); err == nil {
			meta.SourceMessageID = &id
		}
	}
	return meta
}

func normalizeMutationMetadata(meta MutationMetadata) MutationMetadata {
	if meta.ActorType == "" {
		meta.ActorType = EventActorSystem
	}
	if meta.Source == "" {
		meta.Source = EventSourceAPI
	}
	return meta
}

func slotUpdateAction(before *AgentMemorySlot, after *AgentMemorySlot) string {
	if before != nil && after != nil && before.Enabled && !after.Enabled {
		return EventActionSlotDisable
	}
	return EventActionSlotUpdate
}

func recordSlotEvent(ctx context.Context, repo store, workspaceID, agentID uuid.UUID, slotKey string, action string, meta MutationMetadata, before *AgentMemorySlot, after *AgentMemorySlot) error {
	return recordEvent(ctx, repo, workspaceID, agentID, slotKey, "", nil, action, meta, slotSnapshot(before), slotSnapshot(after))
}

func recordValueEvent(ctx context.Context, repo store, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID uuid.UUID, action string, meta MutationMetadata, before *AgentMemoryValue, after *AgentMemoryValue) error {
	redactContent := action == EventActionValueClear
	return recordEvent(ctx, repo, workspaceID, agentID, slotKey, userScope, &userID, action, meta, valueSnapshot(before, redactContent), valueSnapshot(after, redactContent))
}

func recordEvent(ctx context.Context, repo store, workspaceID, agentID uuid.UUID, slotKey string, userScope string, userID *uuid.UUID, action string, meta MutationMetadata, before datatypes.JSON, after datatypes.JSON) error {
	meta = normalizeMutationMetadata(meta)
	event := &AgentMemoryEvent{
		WorkspaceID:          workspaceID,
		AgentID:              agentID,
		SlotKey:              slotKey,
		UserScope:            userScope,
		UserID:               userID,
		Action:               action,
		ActorType:            meta.ActorType,
		Source:               meta.Source,
		SourceConversationID: meta.SourceConversationID,
		SourceMessageID:      meta.SourceMessageID,
		BeforeSnapshot:       before,
		AfterSnapshot:        after,
	}
	if err := repo.CreateEvent(ctx, event); err != nil {
		return fmt.Errorf("record agent memory event: %w", err)
	}
	return nil
}

func slotSnapshot(slot *AgentMemorySlot) datatypes.JSON {
	if slot == nil {
		return datatypes.JSON([]byte("null"))
	}
	return mustJSON(map[string]interface{}{
		"id":           slot.ID.String(),
		"workspace_id": slot.WorkspaceID.String(),
		"agent_id":     slot.AgentID.String(),
		"key":          slot.Key,
		"description":  slot.Description,
		"max_chars":    slot.MaxChars,
		"enabled":      slot.Enabled,
		"sort_order":   slot.SortOrder,
		"created_at":   slot.CreatedAt.Unix(),
		"updated_at":   slot.UpdatedAt.Unix(),
	})
}

func valueSnapshot(value *AgentMemoryValue, redactContent bool) datatypes.JSON {
	if value == nil {
		return datatypes.JSON([]byte("null"))
	}
	snapshot := map[string]interface{}{
		"id":           value.ID.String(),
		"workspace_id": value.WorkspaceID.String(),
		"agent_id":     value.AgentID.String(),
		"slot_key":     value.SlotKey,
		"user_scope":   value.UserScope,
		"user_id":      value.UserID.String(),
		"created_at":   value.CreatedAt.Unix(),
		"updated_at":   value.UpdatedAt.Unix(),
	}
	if redactContent {
		snapshot["content_redacted"] = true
		snapshot["content_length"] = len([]rune(value.Content))
	} else {
		snapshot["content"] = value.Content
	}
	return mustJSON(snapshot)
}

func mustJSON(value interface{}) datatypes.JSON {
	data, err := json.Marshal(value)
	if err != nil {
		return datatypes.JSON([]byte("null"))
	}
	return data
}

func mapRepoError(err error, message string) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", message, err)
}

func sortSlotResponses(slots []SlotResponse) {
	sort.SliceStable(slots, func(i, j int) bool {
		if slots[i].SortOrder != slots[j].SortOrder {
			return slots[i].SortOrder < slots[j].SortOrder
		}
		return slots[i].Key < slots[j].Key
	})
}
