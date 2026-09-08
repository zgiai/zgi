package relationshipmemory

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentAdapter preserves the existing Agent memory service contract while all
// per-user values are stored in the relationship memory domain.
type AgentAdapter struct {
	service *Service
}

func NewAgentAdapter(db *gorm.DB) *AgentAdapter { return &AgentAdapter{service: NewService(db)} }

type AgentRelationshipSettings struct {
	MemoryEnabled      bool  `json:"memory_enabled"`
	AutoCaptureEnabled bool  `json:"auto_capture_enabled"`
	SettingsRevision   int64 `json:"settings_revision"`
	MemoryEpoch        int64 `json:"memory_epoch"`
}

type publishedAgentMemoryContract struct {
	Enabled            bool                      `json:"agent_memory_enabled"`
	AutoCaptureEnabled bool                      `json:"agent_memory_auto_extraction_enabled"`
	ConfigRevision     string                    `json:"agent_memory_config_revision"`
	Slots              []agentmemory.RuntimeSlot `json:"agent_memory_slots"`
}

func (a *AgentAdapter) GetSettings(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (*AgentRelationshipSettings, error) {
	relationship, err := a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	return &AgentRelationshipSettings{MemoryEnabled: relationship.MemoryEnabled, AutoCaptureEnabled: relationship.AutoCaptureEnabled, SettingsRevision: relationship.SettingsRevision, MemoryEpoch: relationship.MemoryEpoch}, nil
}

func (a *AgentAdapter) UpdateSettings(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, memoryEnabled, autoCaptureEnabled *bool, expectedRevision int64) (*AgentRelationshipSettings, error) {
	relationship, err := a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	actorType := "api_key_controller"
	var actorID *uuid.UUID
	if userScope == agentmemory.UserScopeAccount {
		actorType = "user"
		value := userID
		actorID = &value
	}
	updated, err := a.service.UpdateSettings(ctx, relationship.ID, UpdateSettingsRequest{MemoryEnabled: memoryEnabled, AutoCaptureEnabled: autoCaptureEnabled, ExpectedRevision: expectedRevision, ActorType: actorType, ActorID: actorID, Source: "agent_memory_api"})
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	return &AgentRelationshipSettings{MemoryEnabled: updated.MemoryEnabled, AutoCaptureEnabled: updated.AutoCaptureEnabled, SettingsRevision: updated.SettingsRevision, MemoryEpoch: updated.MemoryEpoch}, nil
}

func (a *AgentAdapter) relationship(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (*UserMemoryRelationship, error) {
	if !RuntimeEnabled() {
		return nil, agentmemory.ErrUnauthorized
	}
	if workspaceID == uuid.Nil || agentID == uuid.Nil || userID == uuid.Nil {
		return nil, agentmemory.ErrUnauthorized
	}
	kind := UserSubjectAccount
	issuerType := ""
	var issuerID *uuid.UUID
	if userScope == agentmemory.UserScopeEndUser {
		kind = UserSubjectAPIEndUser
		issuerType = "agent_api"
		value := agentID
		issuerID = &value
	} else if userScope != agentmemory.UserScopeAccount {
		return nil, agentmemory.ErrInvalidInput
	}
	var scope struct {
		OrganizationID *uuid.UUID
		Name           string
	}
	if err := a.service.db.WithContext(ctx).Table("workspaces AS workspace").
		Select("workspace.organization_id, COALESCE(agent.name, '') AS name").
		Joins("JOIN agents AS agent ON agent.id = ? AND agent.tenant_id = workspace.id AND agent.deleted_at IS NULL", agentID).
		Where("workspace.id = ?", workspaceID).Scan(&scope).Error; err != nil {
		return nil, err
	}
	if scope.OrganizationID == nil || *scope.OrganizationID == uuid.Nil {
		if existing, findErr := a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID); findErr == nil && existing.Status == RelationshipActive {
			_ = a.service.ArchiveConsumer(ctx, workspaceID, ConsumerAgent, agentID)
		}
		return nil, agentmemory.ErrUnauthorized
	}
	relationship, err := a.service.EnsureRelationship(ctx, EnsureRelationshipRequest{OrganizationID: *scope.OrganizationID, WorkspaceID: workspaceID, Subject: UserSubjectRef{Kind: kind, ID: userID}, IssuerType: issuerType, IssuerID: issuerID, ConsumerType: ConsumerAgent, ConsumerID: agentID, ConsumerNameSnapshot: scope.Name})
	return relationship, mapAgentMemoryError(err)
}

func agentContract(slots []agentmemory.RuntimeSlot) ConsumerMemoryContract {
	definitions := make([]MemoryDefinition, 0, len(slots))
	for _, slot := range slots {
		definitions = append(definitions, MemoryDefinition{Key: slot.Key, DisplayName: slot.Name, Description: slot.Description, MaxChars: slot.MaxChars, Enabled: slot.Enabled})
	}
	return ConsumerMemoryContract{Enabled: true, DefinitionMode: "slots", Definitions: definitions, RenderBudgetChars: defaultRenderBudget}
}

func (a *AgentAdapter) PrivacySlots(ctx context.Context, workspaceID, agentID uuid.UUID, publishedSlots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID) ([]agentmemory.RuntimeSlot, error) {
	var (
		relationship *UserMemoryRelationship
		err          error
	)
	if len(publishedSlots) == 0 {
		// A disabled/empty published contract may still need to expose migrated
		// values for export and deletion, but must not create a new relationship.
		relationship, err = a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID)
		if errors.Is(err, ErrRelationshipNotFound) {
			return []agentmemory.RuntimeSlot{}, nil
		}
	} else {
		relationship, err = a.relationship(ctx, workspaceID, agentID, userScope, userID)
	}
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	var items []UserMemoryItem
	if err := a.service.db.WithContext(ctx).Where("relationship_id = ?", relationship.ID).Find(&items).Error; err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(publishedSlots))
	result := append([]agentmemory.RuntimeSlot(nil), publishedSlots...)
	for _, slot := range result {
		seen[slot.Key] = struct{}{}
	}
	for _, item := range items {
		if _, exists := seen[item.MemoryKey]; exists {
			continue
		}
		result = append(result, agentmemory.RuntimeSlot{Key: item.MemoryKey, Name: item.DisplayName, MaxChars: maxMemoryContentChars, Enabled: true, SortOrder: len(result)})
		seen[item.MemoryKey] = struct{}{}
	}
	return result, nil
}

func (a *AgentAdapter) ReadUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID) ([]agentmemory.SlotValueResponse, error) {
	return a.readUserMemory(ctx, workspaceID, agentID, slots, userScope, userID, true)
}

func (a *AgentAdapter) readUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, enforcePublishedContract bool) ([]agentmemory.SlotValueResponse, error) {
	if enforcePublishedContract {
		published, err := a.loadPublishedAgentMemoryContract(ctx, a.service.db, workspaceID, agentID)
		if err != nil {
			return nil, mapAgentMemoryError(err)
		}
		if !published.Enabled {
			return []agentmemory.SlotValueResponse{}, nil
		}
		allowed := make(map[string]struct{}, len(published.Slots))
		for _, slot := range published.Slots {
			allowed[slot.Key] = struct{}{}
		}
		filtered := make([]agentmemory.RuntimeSlot, 0, len(slots))
		for _, slot := range slots {
			if _, ok := allowed[slot.Key]; ok {
				filtered = append(filtered, slot)
			}
		}
		slots = filtered
	}
	var (
		relationship *UserMemoryRelationship
		err          error
	)
	if enforcePublishedContract {
		relationship, err = a.relationship(ctx, workspaceID, agentID, userScope, userID)
	} else {
		relationship, err = a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID)
		if errors.Is(err, ErrRelationshipNotFound) {
			return []agentmemory.SlotValueResponse{}, nil
		}
	}
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	if relationship.Status != RelationshipActive || (enforcePublishedContract && !relationship.MemoryEnabled) {
		return []agentmemory.SlotValueResponse{}, nil
	}
	var items []UserMemoryItem
	if err := a.service.db.WithContext(ctx).Where("relationship_id = ?", relationship.ID).Find(&items).Error; err != nil {
		return nil, err
	}
	byKey := make(map[string]UserMemoryItem, len(items))
	operationIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
		decoded, decodeErr := a.service.codec.Decode(ctx, EncodedContent{Payload: item.ContentPayload, Encoding: item.ContentEncoding, KeyID: item.ContentKeyID})
		if decodeErr != nil {
			return nil, decodeErr
		}
		item.ContentPayload = decoded
		byKey[item.MemoryKey] = item
		if item.LastOperationID != nil {
			operationIDs = append(operationIDs, *item.LastOperationID)
		}
	}
	undoExpiries := make(map[uuid.UUID]time.Time, len(operationIDs))
	if len(operationIDs) > 0 {
		var records []UserMemoryUndoRecord
		if err := a.service.db.WithContext(ctx).Select("operation_id", "expires_at").Where("operation_id IN ? AND expires_at > ?", operationIDs, time.Now()).Find(&records).Error; err != nil {
			return nil, err
		}
		for _, record := range records {
			undoExpiries[record.OperationID] = record.ExpiresAt
		}
	}
	values := make([]agentmemory.SlotValueResponse, 0, len(slots))
	for _, slot := range slots {
		response := agentmemory.SlotValueResponse{SlotResponse: agentmemory.SlotResponse{Key: slot.Key, Name: slot.Name, Description: slot.Description, MaxChars: slot.MaxChars, Enabled: slot.Enabled, SortOrder: slot.SortOrder}}
		if item, exists := byKey[slot.Key]; exists {
			if item.LastOperationID != nil {
				if expiresAt, ok := undoExpiries[*item.LastOperationID]; ok {
					item.UndoableUntil = &expiresAt
				}
			}
			applyAgentItem(&response, item)
		}
		values = append(values, response)
	}
	return values, nil
}

func (a *AgentAdapter) ReadRuntimeFence(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, configScope, configRevision string) (int64, error) {
	if strings.TrimSpace(configRevision) == "" {
		return 0, agentmemory.ErrInvalidInput
	}
	if configScope == agentmemory.ConfigScopePublished {
		if err := a.validatePublishedConfigRevision(ctx, a.service.db, workspaceID, agentID, configRevision); err != nil {
			return 0, err
		}
	} else if configScope != agentmemory.ConfigScopeDraft {
		return 0, agentmemory.ErrInvalidInput
	}
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return 0, err
	}
	return relationship.MemoryEpoch, nil
}

// RelationshipRuntimeSettings lets the Agent runtime honor the per-user
// relationship switches before it renders context, exposes mutation tools, or
// schedules background extraction.
func (a *AgentAdapter) RelationshipRuntimeSettings(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (bool, bool, error) {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return false, false, err
	}
	contract, err := a.loadPublishedAgentMemoryContract(ctx, a.service.db, workspaceID, agentID)
	if err != nil {
		return false, false, err
	}
	active := relationship.Status == RelationshipActive && relationship.MemoryEnabled && contract.Enabled && len(contract.Slots) > 0
	return active, active && relationship.AutoCaptureEnabled && contract.AutoCaptureEnabled, nil
}

func (a *AgentAdapter) UpdateValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, req agentmemory.UpdateValueRequest, meta agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error) {
	published, err := a.loadPublishedAgentMemoryContract(ctx, a.service.db, workspaceID, agentID)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	if scope := strings.TrimSpace(meta.ConfigScope); scope != "" {
		if scope != agentmemory.ConfigScopePublished || strings.TrimSpace(meta.ConfigRevision) == "" || strings.TrimSpace(meta.ConfigRevision) != published.ConfigRevision {
			return nil, agentmemory.ErrConflict
		}
	}
	if !published.Enabled || len(published.Slots) == 0 {
		return nil, mapAgentMemoryError(ErrPolicyRejected)
	}
	slots = published.Slots
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, err
	}
	expected := int64(0)
	if req.ExpectedRevision != nil {
		expected = *req.ExpectedRevision
	} else {
		var item UserMemoryItem
		if findErr := a.service.db.WithContext(ctx).Where("relationship_id = ? AND memory_key = ?", relationship.ID, strings.TrimSpace(req.Key)).First(&item).Error; findErr == nil {
			expected = item.Revision
		} else if !errors.Is(findErr, gorm.ErrRecordNotFound) {
			return nil, findErr
		}
	}
	operationID := uuid.New()
	if meta.OperationID != nil {
		operationID = *meta.OperationID
	}
	metadata := relationshipMetadata(meta, operationID)
	if userScope == agentmemory.UserScopeAccount {
		value := userID
		metadata.ActorID = &value
	}
	item, err := a.service.PutItem(ctx, relationship.ID, PutItemRequest{Key: req.Key, DisplayName: slotName(slots, req.Key), Content: req.Content, ExpectedRevision: expected, Contract: agentContract(slots), Metadata: metadata})
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	response := agentmemory.SlotValueResponse{SlotResponse: slotResponse(slots, req.Key)}
	applyAgentItem(&response, *item)
	return &response, nil
}

func (a *AgentAdapter) ClearValue(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, key string, meta agentmemory.MutationMetadata) (*agentmemory.SlotValueResponse, error) {
	relationship, err := a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID)
	if errors.Is(err, ErrRelationshipNotFound) {
		response := agentmemory.SlotValueResponse{SlotResponse: slotResponse(slots, key)}
		return &response, nil
	}
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	var item UserMemoryItem
	findErr := a.service.db.WithContext(ctx).Where("relationship_id = ? AND memory_key = ?", relationship.ID, strings.TrimSpace(key)).First(&item).Error
	if errors.Is(findErr, gorm.ErrRecordNotFound) {
		response := agentmemory.SlotValueResponse{SlotResponse: slotResponse(slots, key)}
		return &response, nil
	}
	if findErr != nil {
		return nil, findErr
	}
	expected := item.Revision
	if meta.ExpectedRevision != nil {
		expected = *meta.ExpectedRevision
	}
	metadata := relationshipMetadata(meta, uuid.New())
	if userScope == agentmemory.UserScopeAccount {
		value := userID
		metadata.ActorID = &value
	}
	deleted, err := a.service.DeleteItem(ctx, relationship.ID, key, expected, metadata)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	response := agentmemory.SlotValueResponse{SlotResponse: slotResponse(slots, key)}
	response.LastOperationID = deleted.OperationID.String()
	if deleted.UndoableUntil != nil {
		value := deleted.UndoableUntil.Unix()
		response.UndoableUntil = &value
	}
	return &response, nil
}

func (a *AgentAdapter) ClearAllValues(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, _ agentmemory.MutationMetadata) error {
	relationship, err := a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID)
	if errors.Is(err, ErrRelationshipNotFound) {
		return nil
	}
	if err != nil {
		return mapAgentMemoryError(err)
	}
	return mapAgentMemoryError(a.service.ClearItems(ctx, relationship.ID, agentManagementMetadata(userScope, userID)))
}

func (a *AgentAdapter) ExportUserMemory(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID) (*agentmemory.MemoryExportResponse, error) {
	values, err := a.readUserMemory(ctx, workspaceID, agentID, slots, userScope, userID, false)
	if err != nil {
		return nil, err
	}
	return &agentmemory.MemoryExportResponse{AgentID: agentID.String(), UserScope: userScope, UserID: userID.String(), ExportedAt: time.Now().Unix(), Values: values}, nil
}

func (a *AgentAdapter) UndoAutomaticOperation(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID, operationID uuid.UUID, slots []agentmemory.RuntimeSlot) (*agentmemory.UndoResponse, error) {
	relationship, err := a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	item, err := a.service.Undo(ctx, relationship.ID, operationID, agentManagementMetadata(userScope, userID))
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	response := &agentmemory.UndoResponse{OperationID: operationID.String()}
	if item != nil {
		value := agentmemory.SlotValueResponse{SlotResponse: slotResponse(slots, item.MemoryKey)}
		applyAgentItem(&value, *item)
		response.Value = &value
	}
	return response, nil
}

func (a *AgentAdapter) MutateValues(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID, req agentmemory.MutateValuesRequest, meta agentmemory.MutationMetadata) (*agentmemory.MutateValuesResponse, error) {
	published, err := a.loadPublishedAgentMemoryContract(ctx, a.service.db, workspaceID, agentID)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	if !published.Enabled || len(published.Slots) == 0 {
		return nil, mapAgentMemoryError(ErrPolicyRejected)
	}
	if meta.ConfigScope == agentmemory.ConfigScopePublished {
		if err := a.validatePublishedConfigRevision(ctx, a.service.db, workspaceID, agentID, meta.ConfigRevision); err != nil {
			return nil, err
		}
	} else if meta.ConfigScope != agentmemory.ConfigScopeDraft || strings.TrimSpace(meta.ConfigRevision) == "" {
		return nil, agentmemory.ErrInvalidInput
	}
	slots = published.Slots
	allowedSlots := make(map[string]struct{}, len(slots))
	for _, slot := range slots {
		allowedSlots[slot.Key] = struct{}{}
	}
	if len(req.Operations) == 0 || len(req.Operations) > 5 {
		return nil, agentmemory.ErrInvalidInput
	}
	seenKeys := make(map[string]struct{}, len(req.Operations))
	seenOperations := make(map[uuid.UUID]struct{}, len(req.Operations))
	for index := range req.Operations {
		operation := &req.Operations[index]
		operation.Key = strings.TrimSpace(operation.Key)
		operation.Action = strings.ToLower(strings.TrimSpace(operation.Action))
		operation.Mode = strings.ToLower(strings.TrimSpace(operation.Mode))
		if operation.OperationID == uuid.Nil || (operation.Action != agentmemory.MutationActionUpsert && operation.Action != agentmemory.MutationActionClear) || (operation.Mode != agentmemory.MutationModeExplicit && operation.Mode != agentmemory.MutationModeProactive) {
			return nil, agentmemory.ErrInvalidInput
		}
		if operation.Mode == agentmemory.MutationModeProactive && operation.Action != agentmemory.MutationActionUpsert {
			return nil, agentmemory.ErrUnauthorized
		}
		if operation.Action == agentmemory.MutationActionClear && strings.TrimSpace(operation.Content) != "" {
			return nil, agentmemory.ErrInvalidInput
		}
		if _, ok := allowedSlots[operation.Key]; !ok {
			return nil, agentmemory.ErrInvalidInput
		}
		if _, duplicate := seenKeys[operation.Key]; duplicate {
			return nil, agentmemory.ErrInvalidInput
		}
		if _, duplicate := seenOperations[operation.OperationID]; duplicate {
			return nil, agentmemory.ErrInvalidInput
		}
		seenKeys[operation.Key] = struct{}{}
		seenOperations[operation.OperationID] = struct{}{}
	}
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, err
	}
	results := make([]agentmemory.ValueMutationResult, 0, len(req.Operations))
	err = a.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked UserMemoryRelationship
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&locked, "id = ?", relationship.ID).Error; err != nil {
			return err
		}
		if locked.Status != RelationshipActive || !locked.MemoryEnabled {
			return ErrAccessDenied
		}
		if meta.MemoryEpoch != nil && locked.MemoryEpoch != *meta.MemoryEpoch {
			return ErrRevisionConflict
		}
		transactionService := NewService(tx, a.service.codec)
		for _, operation := range req.Operations {
			metadata := relationshipMetadata(meta, operation.OperationID)
			if userScope == agentmemory.UserScopeAccount {
				value := userID
				metadata.ActorID = &value
			}
			metadata.SourceKind = SourceExplicit
			if operation.Mode == agentmemory.MutationModeProactive {
				metadata.SourceKind = SourceAutomatic
			}
			// The batch has one relationship-level epoch fence. A clear operation
			// intentionally advances the epoch to invalidate other in-flight jobs,
			// but must not invalidate later operations in this same atomic batch.
			metadata.ExpectedMemoryEpoch = nil
			metadata.SourceMessageID = operation.SourceMessageID
			metadata.SourceCompletedAt = operation.SourceCompletedAt
			metadata.EvidenceFromUserInput = metadata.EvidenceFromUserInput || operation.SourceMessageID != nil
			if operation.Action == agentmemory.MutationActionClear {
				deleted, err := transactionService.DeleteItem(ctx, relationship.ID, operation.Key, operation.ExpectedRevision, metadata)
				if err != nil {
					return err
				}
				status := agentmemory.MutationStatusUnchanged
				if deleted.Deleted {
					status = agentmemory.MutationStatusCleared
				}
				result := agentmemory.ValueMutationResult{Action: operation.Action, Status: status, Key: operation.Key, OperationID: operation.OperationID.String()}
				if deleted.UndoableUntil != nil {
					value := deleted.UndoableUntil.Unix()
					result.UndoableUntil = &value
				}
				results = append(results, result)
				continue
			}
			item, err := transactionService.PutItem(ctx, relationship.ID, PutItemRequest{Key: operation.Key, DisplayName: slotName(slots, operation.Key), Content: operation.Content, ExpectedRevision: operation.ExpectedRevision, Contract: agentContract(slots), Metadata: metadata})
			if err != nil {
				return err
			}
			result := agentmemory.ValueMutationResult{Action: operation.Action, Status: agentmemory.MutationStatusUpdated, Key: operation.Key, Revision: item.Revision, SourceKind: item.SourceKind, OperationID: operation.OperationID.String()}
			if operation.Mode == agentmemory.MutationModeProactive {
				until := time.Now().Add(undoTTL).Unix()
				result.UndoableUntil = &until
			}
			results = append(results, result)
		}
		return nil
	})
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	return &agentmemory.MutateValuesResponse{Status: "success", Operations: results}, nil
}

func (a *AgentAdapter) ScheduleExtraction(ctx context.Context, req agentmemory.ScheduleExtractionRequest) (*agentmemory.AgentMemoryExtractionJob, error) {
	workspaceID, wErr := uuid.Parse(req.WorkspaceID)
	agentID, aErr := uuid.Parse(req.AgentID)
	userID, uErr := uuid.Parse(req.UserID)
	conversationID, cErr := uuid.Parse(req.ConversationID)
	watermarkID, mErr := uuid.Parse(req.MessageWatermarkID)
	if wErr != nil || aErr != nil || uErr != nil || cErr != nil || mErr != nil {
		return nil, agentmemory.ErrInvalidInput
	}
	// P0 extraction is tied to immutable published Agent definitions. Draft
	// console conversations may still use explicit inline memory operations.
	if req.ConfigScope != agentmemory.ConfigScopePublished {
		return nil, agentmemory.ErrInvalidInput
	}
	if err := a.validatePublishedConfigRevision(ctx, a.service.db, workspaceID, agentID, req.ConfigRevision); err != nil {
		return nil, err
	}
	published, err := a.loadPublishedAgentMemoryContract(ctx, a.service.db, workspaceID, agentID)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	if !published.Enabled || !published.AutoCaptureEnabled || len(published.Slots) == 0 {
		return nil, mapAgentMemoryError(ErrPolicyRejected)
	}
	req.Slots = published.Slots
	relationship, err := a.relationship(ctx, workspaceID, agentID, req.UserScope, userID)
	if err != nil {
		return nil, err
	}
	keyMaterial := strings.Join([]string{workspaceID.String(), agentID.String(), req.UserScope, userID.String(), conversationID.String(), watermarkID.String(), req.ExtractorVersion, req.ConfigScope, req.ConfigRevision}, ":")
	digest := sha256.Sum256([]byte(keyMaterial))
	now := time.Now()
	job, err := a.service.ScheduleExtraction(ctx, ScheduleRequest{RelationshipID: relationship.ID, ConversationID: conversationID, MessageWatermarkID: watermarkID, MemoryEpoch: relationship.MemoryEpoch, ConfigRevision: req.ConfigRevision, Definitions: req.Slots, ExtractorVersion: fallback(req.ExtractorVersion, "agent-memory-v2"), IdempotencyKey: hex.EncodeToString(digest[:]), ScheduledAt: now.Add(time.Minute), ForceAt: now.Add(10 * time.Minute)})
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	return &agentmemory.AgentMemoryExtractionJob{ID: job.ID, WorkspaceID: workspaceID, AgentID: agentID, UserScope: req.UserScope, UserID: userID, ConversationID: conversationID, MessageWatermarkID: watermarkID, MemoryEpoch: job.MemoryEpoch, ConfigScope: req.ConfigScope, ConfigRevision: req.ConfigRevision, RuntimeSlots: job.DefinitionSnapshot, ExtractorVersion: job.ExtractorVersion, IdempotencyKey: job.IdempotencyKey, Status: job.Status, ScheduledAt: job.ScheduledAt, ForceAt: job.ForceAt, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt}, nil
}

// ListDueExtractionJobs and the following lifecycle methods intentionally use
// the legacy Agent job DTO. This keeps the mature extraction worker reusable
// while the durable job and user values live exclusively in the relationship
// memory tables.
func (a *AgentAdapter) ListDueExtractionJobs(ctx context.Context, limit int) ([]*agentmemory.AgentMemoryExtractionJob, error) {
	if limit <= 0 {
		limit = 100
	}
	var jobs []UserMemoryExtractionJob
	err := a.service.db.WithContext(ctx).Table("user_memory_extraction_jobs AS jobs").
		Select("jobs.*").
		Joins("JOIN user_memory_relationships AS relationships ON relationships.id = jobs.relationship_id").
		Where("relationships.consumer_type = ? AND relationships.status = ? AND relationships.memory_enabled = ? AND relationships.auto_capture_enabled = ?", ConsumerAgent, RelationshipActive, true, true).
		Where("jobs.status IN ? AND jobs.scheduled_at <= ?", []string{agentmemory.ExtractionJobPending, agentmemory.ExtractionJobFailed}, time.Now()).
		Order("jobs.scheduled_at ASC").Limit(limit).Scan(&jobs).Error
	if err != nil {
		return nil, err
	}
	result := make([]*agentmemory.AgentMemoryExtractionJob, 0, len(jobs))
	for index := range jobs {
		converted, convertErr := a.agentExtractionJob(ctx, &jobs[index])
		if convertErr != nil {
			return nil, convertErr
		}
		result = append(result, converted)
	}
	return result, nil
}

func (a *AgentAdapter) ClaimExtractionJob(ctx context.Context, id uuid.UUID) (*agentmemory.AgentMemoryExtractionJob, error) {
	var claimed UserMemoryExtractionJob
	cancelled := false
	err := a.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var job UserMemoryExtractionJob
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&job, "id = ?", id).Error; err != nil {
			return err
		}
		var relationship UserMemoryRelationship
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&relationship, "id = ?", job.RelationshipID).Error; err != nil {
			return err
		}
		if err := a.validatePublishedConfigRevision(ctx, tx, relationship.WorkspaceID, relationship.ConsumerID, job.ConfigRevision); err != nil {
			if !errors.Is(err, agentmemory.ErrConflict) && !errors.Is(err, agentmemory.ErrInvalidInput) {
				return err
			}
			now := time.Now()
			if updateErr := tx.Model(&job).Updates(map[string]any{"status": agentmemory.ExtractionJobCancelled, "error_code": "config_revision_changed", "finished_at": now, "updated_at": now}).Error; updateErr != nil {
				return updateErr
			}
			cancelled = true
			return nil
		}
		if relationship.ConsumerType != ConsumerAgent || relationship.Status != RelationshipActive || !relationship.MemoryEnabled || !relationship.AutoCaptureEnabled || relationship.MemoryEpoch != job.MemoryEpoch {
			now := time.Now()
			if updateErr := tx.Model(&job).Updates(map[string]any{"status": agentmemory.ExtractionJobCancelled, "error_code": "memory_epoch_changed", "finished_at": now, "updated_at": now}).Error; updateErr != nil {
				return updateErr
			}
			cancelled = true
			return nil
		}
		result := tx.Model(&UserMemoryExtractionJob{}).
			Where("id = ? AND memory_epoch = ? AND status IN ?", id, job.MemoryEpoch, []string{agentmemory.ExtractionJobPending, agentmemory.ExtractionJobQueued, agentmemory.ExtractionJobFailed}).
			Updates(map[string]any{"status": agentmemory.ExtractionJobRunning, "attempt_count": gorm.Expr("attempt_count + 1"), "error_code": "", "started_at": time.Now(), "finished_at": nil, "updated_at": time.Now()})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return agentmemory.ErrConflict
		}
		return tx.First(&claimed, "id = ?", id).Error
	})
	if err != nil {
		return nil, err
	}
	if cancelled {
		recordExtractionJob(ctx, ConsumerAgent, agentmemory.ExtractionJobCancelled)
		return nil, agentmemory.ErrConflict
	}
	return a.agentExtractionJob(ctx, &claimed)
}

func (a *AgentAdapter) validatePublishedConfigRevision(ctx context.Context, db *gorm.DB, workspaceID, agentID uuid.UUID, expected string) error {
	expected = strings.TrimSpace(expected)
	if expected == "" {
		return agentmemory.ErrInvalidInput
	}
	published, err := a.loadPublishedAgentMemoryContract(ctx, db, workspaceID, agentID)
	if err != nil {
		if errors.Is(err, ErrPolicyRejected) {
			return agentmemory.ErrConflict
		}
		return err
	}
	if strings.TrimSpace(published.ConfigRevision) != expected {
		return agentmemory.ErrConflict
	}
	return nil
}

func (a *AgentAdapter) loadPublishedAgentMemoryContract(ctx context.Context, db *gorm.DB, workspaceID, agentID uuid.UUID) (*publishedAgentMemoryContract, error) {
	var row struct {
		ConfigSnapshot []byte
	}
	err := db.WithContext(ctx).Table("agent_published_versions").Select("config_snapshot").
		Where("workspace_id = ? AND agent_id = ? AND deleted_at IS NULL", workspaceID, agentID).
		Order("created_at DESC").Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrPolicyRejected
	}
	if err != nil {
		return nil, err
	}
	var contract publishedAgentMemoryContract
	if err := json.Unmarshal(row.ConfigSnapshot, &contract); err != nil {
		return nil, fmt.Errorf("%w: invalid published Agent memory contract", ErrPolicyRejected)
	}
	normalizePublishedAgentMemoryContract(&contract)
	return &contract, nil
}

func normalizePublishedAgentMemoryContract(contract *publishedAgentMemoryContract) {
	contract.ConfigRevision = strings.TrimSpace(contract.ConfigRevision)
	if contract.ConfigRevision == "" {
		contract.ConfigRevision = agentmemory.ConfigRevision(contract.Enabled, contract.AutoCaptureEnabled, contract.Slots)
	}
	enabledSlots := make([]agentmemory.RuntimeSlot, 0, len(contract.Slots))
	for _, slot := range contract.Slots {
		if slot.Enabled {
			enabledSlots = append(enabledSlots, slot)
		}
	}
	contract.Slots = enabledSlots
}

func (a *AgentAdapter) FinishExtractionJob(ctx context.Context, id uuid.UUID, status, errorCode string) error {
	now := time.Now()
	err := a.service.db.WithContext(ctx).Model(&UserMemoryExtractionJob{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "error_code": errorCode, "finished_at": now, "updated_at": now}).Error
	result := status
	if err != nil {
		result = "storage_error"
	}
	recordExtractionJob(ctx, ConsumerAgent, result)
	return err
}

func (a *AgentAdapter) RescheduleExtractionJob(ctx context.Context, id uuid.UUID, errorCode string, scheduledAt time.Time) error {
	now := time.Now()
	err := a.service.db.WithContext(ctx).Model(&UserMemoryExtractionJob{}).Where("id = ?", id).
		Updates(map[string]any{"status": agentmemory.ExtractionJobFailed, "error_code": errorCode, "scheduled_at": scheduledAt, "finished_at": now, "updated_at": now}).Error
	result := agentmemory.ExtractionJobFailed
	if err != nil {
		result = "storage_error"
	}
	recordExtractionJob(ctx, ConsumerAgent, result)
	return err
}

func (a *AgentAdapter) DeleteTerminalExtractionJobs(ctx context.Context, finishedBefore time.Time, limit int) (int64, error) {
	if limit <= 0 {
		limit = 500
	}
	var ids []uuid.UUID
	if err := a.service.db.WithContext(ctx).Model(&UserMemoryExtractionJob{}).
		Where("status IN ? AND finished_at < ?", []string{agentmemory.ExtractionJobCompleted, agentmemory.ExtractionJobCancelled, agentmemory.ExtractionJobExhausted}, finishedBefore).
		Order("finished_at ASC").Limit(limit).Pluck("id", &ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	result := a.service.db.WithContext(ctx).Where("id IN ?", ids).Delete(&UserMemoryExtractionJob{})
	return result.RowsAffected, result.Error
}

// ExtractionBounds provides the worker with the relationship epoch/cutoff and
// the last completed watermark without exposing relationship tables to it.
func (a *AgentAdapter) ExtractionBounds(ctx context.Context, job *agentmemory.AgentMemoryExtractionJob, watermarkCreatedAt time.Time) (*time.Time, *time.Time, error) {
	if job == nil {
		return nil, nil, agentmemory.ErrInvalidInput
	}
	relationship, err := a.relationship(ctx, job.WorkspaceID, job.AgentID, job.UserScope, job.UserID)
	if err != nil {
		return nil, nil, err
	}
	if relationship.MemoryEpoch != job.MemoryEpoch || relationship.Status != RelationshipActive || !relationship.MemoryEnabled || !relationship.AutoCaptureEnabled {
		return nil, nil, agentmemory.ErrConflict
	}
	cutoff := relationship.ExtractionCutoffAt
	if cutoff == nil && relationship.MemoryEpoch > 0 && !relationship.UpdatedAt.IsZero() {
		value := relationship.UpdatedAt
		cutoff = &value
	}
	var previous struct{ CreatedAt *time.Time }
	if err := a.service.db.WithContext(ctx).Table("user_memory_extraction_jobs AS jobs").
		Select("MAX(messages.created_at) AS created_at").
		Joins("JOIN chat_runtime_messages AS messages ON messages.id = jobs.message_watermark_id").
		Where("jobs.id <> ? AND jobs.relationship_id = ? AND jobs.conversation_id = ? AND jobs.memory_epoch = ? AND jobs.status = ? AND messages.created_at < ?", job.ID, relationship.ID, job.ConversationID, job.MemoryEpoch, agentmemory.ExtractionJobCompleted, watermarkCreatedAt).
		Scan(&previous).Error; err != nil {
		return nil, nil, err
	}
	return cutoff, previous.CreatedAt, nil
}

func (a *AgentAdapter) CleanupExpiredRelationshipMemory(ctx context.Context, now time.Time, auditRetention time.Duration) error {
	return a.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at <= ?", now).Delete(&UserMemoryUndoRecord{}).Error; err != nil {
			return err
		}
		return tx.Where("created_at < ?", now.Add(-auditRetention)).Delete(&UserMemoryEvent{}).Error
	})
}

func (a *AgentAdapter) agentExtractionJob(ctx context.Context, job *UserMemoryExtractionJob) (*agentmemory.AgentMemoryExtractionJob, error) {
	if job == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var relationship UserMemoryRelationship
	if err := a.service.db.WithContext(ctx).First(&relationship, "id = ?", job.RelationshipID).Error; err != nil {
		return nil, err
	}
	var subject UserMemorySubject
	if err := a.service.db.WithContext(ctx).First(&subject, "id = ?", relationship.SubjectID).Error; err != nil {
		return nil, err
	}
	userScope := agentmemory.UserScopeAccount
	if subject.Kind == UserSubjectAPIEndUser {
		userScope = agentmemory.UserScopeEndUser
	}
	return &agentmemory.AgentMemoryExtractionJob{
		ID: job.ID, WorkspaceID: relationship.WorkspaceID, AgentID: relationship.ConsumerID,
		UserScope: userScope, UserID: subject.ID, ConversationID: job.ConversationID,
		MessageWatermarkID: job.MessageWatermarkID, MemoryEpoch: job.MemoryEpoch,
		ConfigScope: agentmemory.ConfigScopePublished, ConfigRevision: job.ConfigRevision,
		RuntimeSlots: job.DefinitionSnapshot, ExtractorVersion: job.ExtractorVersion,
		IdempotencyKey: job.IdempotencyKey, Status: job.Status, AttemptCount: job.AttemptCount,
		ErrorCode: job.ErrorCode, ScheduledAt: job.ScheduledAt, ForceAt: job.ForceAt,
		StartedAt: job.StartedAt, FinishedAt: job.FinishedAt, CreatedAt: job.CreatedAt, UpdatedAt: job.UpdatedAt,
	}, nil
}

func relationshipMetadata(meta agentmemory.MutationMetadata, operationID uuid.UUID) MutationMetadata {
	return MutationMetadata{ActorType: meta.ActorType, Source: meta.Source, SourceKind: meta.SourceKind, SourceConversationID: meta.SourceConversationID, SourceMessageID: meta.SourceMessageID, SourceCompletedAt: meta.SourceCompletedAt, ExtractorVersion: meta.ExtractorVersion, OperationID: operationID, ExpectedMemoryEpoch: meta.MemoryEpoch, EvidenceFromUserInput: meta.SourceMessageID != nil}
}

func agentManagementMetadata(userScope string, userID uuid.UUID) MutationMetadata {
	if userScope == agentmemory.UserScopeAccount {
		value := userID
		return MutationMetadata{ActorType: "user", ActorID: &value, Source: "agent_memory_api", SourceKind: SourceManager}
	}
	return MutationMetadata{ActorType: "api_key_controller", Source: "agent_memory_api", SourceKind: SourceManager}
}

func slotResponse(slots []agentmemory.RuntimeSlot, key string) agentmemory.SlotResponse {
	for _, slot := range slots {
		if slot.Key == strings.TrimSpace(key) {
			return agentmemory.SlotResponse{Key: slot.Key, Name: slot.Name, Description: slot.Description, MaxChars: slot.MaxChars, Enabled: slot.Enabled, SortOrder: slot.SortOrder}
		}
	}
	return agentmemory.SlotResponse{Key: strings.TrimSpace(key), Name: strings.TrimSpace(key), MaxChars: maxMemoryContentChars, Enabled: true}
}

func slotName(slots []agentmemory.RuntimeSlot, key string) string {
	return slotResponse(slots, key).Name
}

func applyAgentItem(response *agentmemory.SlotValueResponse, item UserMemoryItem) {
	createdAt, updatedAt := item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	response.ID = item.ID.String()
	response.Content = item.ContentPayload
	response.Revision = item.Revision
	response.SourceKind = item.SourceKind
	response.CreatedAt, response.CreatedAtUnix, response.CreatedAtISO, response.CreatedAtDisplay = createdAt.Unix(), createdAt.Unix(), createdAt.Format(time.RFC3339), createdAt.Format("2006-01-02 15:04:05 UTC")
	response.UpdatedAt, response.UpdatedAtUnix, response.UpdatedAtISO, response.UpdatedAtDisplay = updatedAt.Unix(), updatedAt.Unix(), updatedAt.Format(time.RFC3339), updatedAt.Format("2006-01-02 15:04:05 UTC")
	if item.SourceConversationID != nil {
		response.SourceConversationID = item.SourceConversationID.String()
	}
	if item.SourceMessageID != nil {
		response.SourceMessageID = item.SourceMessageID.String()
	}
	if item.SourceCompletedAt != nil {
		response.SourceCompletedAt = item.SourceCompletedAt.Unix()
	}
	response.ExtractorVersion = item.ExtractorVersion
	if item.LastOperationID != nil {
		response.LastOperationID = item.LastOperationID.String()
	}
	if item.UndoableUntil != nil {
		until := item.UndoableUntil.Unix()
		response.UndoableUntil = &until
	}
}

func mapAgentMemoryError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, ErrRevisionConflict), errors.Is(err, ErrUndoUnavailable), errors.Is(err, ErrCapacityExceeded):
		return fmt.Errorf("%w: %v", agentmemory.ErrConflict, err)
	case errors.Is(err, ErrPolicyRejected), errors.Is(err, ErrInvalidUser):
		return fmt.Errorf("%w: %v", agentmemory.ErrInvalidInput, err)
	case errors.Is(err, ErrRelationshipNotFound):
		return fmt.Errorf("%w: %v", agentmemory.ErrNotFound, err)
	case errors.Is(err, ErrAccessDenied):
		return fmt.Errorf("%w: %v", agentmemory.ErrUnauthorized, err)
	default:
		return err
	}
}
