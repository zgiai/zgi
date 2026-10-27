package relationshipmemory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// AgentAdapter preserves the existing Agent memory service contract while all
// per-user values are stored in the relationship memory domain.
type AgentAdapter struct {
	service *Service
}

// maxAgentFileOperations bounds the files one memory_edit call or
// consolidation pass may write.
const maxAgentFileOperations = 16

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
	if errors.Is(err, ErrRelationshipNotFound) {
		// A user who has not talked to the Agent yet has the settings a new
		// relationship starts with; reading them creates nothing.
		return &AgentRelationshipSettings{MemoryEnabled: true, AutoCaptureEnabled: true, SettingsRevision: 1}, nil
	}
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	return &AgentRelationshipSettings{MemoryEnabled: relationship.MemoryEnabled, AutoCaptureEnabled: relationship.AutoCaptureEnabled, SettingsRevision: relationship.SettingsRevision, MemoryEpoch: relationship.MemoryEpoch}, nil
}

func (a *AgentAdapter) UpdateSettings(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, memoryEnabled, autoCaptureEnabled *bool, expectedRevision int64) (*AgentRelationshipSettings, error) {
	// Settings may be chosen before the first conversation, so changing them
	// creates the relationship.
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, err
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
	if workspaceID == uuid.Nil || agentID == uuid.Nil || userID == uuid.Nil {
		return nil, agentmemory.ErrUnauthorized
	}
	kind := usersubject.Account
	issuerType := ""
	var issuerID *uuid.UUID
	if userScope == agentmemory.UserScopeEndUser {
		kind = usersubject.APIEndUser
		issuerType = "agent_api"
		value := agentID
		issuerID = &value
		var registered usersubject.Subject
		err := a.service.db.WithContext(ctx).First(&registered, "id = ?", userID).Error
		switch {
		case err == nil:
			kind, issuerType, issuerID, err = agentEndUserIdentity(&registered, agentID)
			if err != nil {
				return nil, err
			}
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return nil, fmt.Errorf("read registered Agent memory subject: %w", err)
		}
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
	relationship, err := a.service.EnsureRelationship(ctx, EnsureRelationshipRequest{OrganizationID: *scope.OrganizationID, WorkspaceID: workspaceID, Subject: usersubject.Ref{Kind: kind, ID: userID}, IssuerType: issuerType, IssuerID: issuerID, ConsumerType: ConsumerAgent, ConsumerID: agentID, ConsumerNameSnapshot: scope.Name})
	return relationship, mapAgentMemoryError(err)
}

// agentEndUserIdentity preserves the canonical issuer established by a
// trusted outer runtime. Runtime Workspace subjects are scoped to the
// exposure group rather than an individual Agent, so rewriting their issuer
// to agent_api would incorrectly reject the same stable subject.
func agentEndUserIdentity(registered *usersubject.Subject, agentID uuid.UUID) (usersubject.Kind, string, *uuid.UUID, error) {
	if registered == nil {
		issuerID := agentID
		return usersubject.APIEndUser, "agent_api", &issuerID, nil
	}
	if registered.Kind != usersubject.APIEndUser || registered.IssuerType == "" || registered.IssuerID == nil || *registered.IssuerID == uuid.Nil {
		return "", "", nil, agentmemory.ErrInvalidInput
	}
	issuerID := *registered.IssuerID
	return registered.Kind, registered.IssuerType, &issuerID, nil
}

// agentContract is the contract of every Agent's memory: files the model
// names itself (ADR 0017).
func agentContract() ConsumerMemoryContract {
	return ConsumerMemoryContract{Enabled: true, DefinitionMode: "files", RenderBudgetChars: defaultRenderBudget}
}

// agentFiles reads a relationship's memory files, decoded, in name order.
func (a *AgentAdapter) agentFiles(ctx context.Context, relationshipID uuid.UUID) ([]UserMemoryItem, error) {
	return a.service.MemoryFiles(ctx, relationshipID)
}

func (a *AgentAdapter) PrivacySlots(ctx context.Context, workspaceID, agentID uuid.UUID, _ []agentmemory.RuntimeSlot, userScope string, userID uuid.UUID) ([]agentmemory.RuntimeSlot, error) {
	// Export and deletion must not create a relationship.
	relationship, err := a.service.FindRelationship(ctx, workspaceID, userID, ConsumerAgent, agentID)
	if errors.Is(err, ErrRelationshipNotFound) {
		return []agentmemory.RuntimeSlot{}, nil
	}
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	items, err := a.agentFiles(ctx, relationship.ID)
	if err != nil {
		return nil, err
	}
	result := make([]agentmemory.RuntimeSlot, 0, len(items))
	for _, item := range items {
		result = append(result, agentmemory.RuntimeSlot{Key: item.MemoryKey, Name: item.MemoryKey, MaxChars: agentmemory.MaxMemoryChars, Enabled: true, SortOrder: len(result)})
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
	items, err := a.agentFiles(ctx, relationship.ID)
	if err != nil {
		return nil, err
	}
	operationIDs := make([]uuid.UUID, 0, len(items))
	for _, item := range items {
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
	values := make([]agentmemory.SlotValueResponse, 0, len(items))
	for _, item := range items {
		response := agentmemory.SlotValueResponse{SlotResponse: slotResponse(slots, item.MemoryKey)}
		if item.LastOperationID != nil {
			if expiresAt, ok := undoExpiries[*item.LastOperationID]; ok {
				item.UndoableUntil = &expiresAt
			}
		}
		applyAgentItem(&response, item)
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

func (a *AgentAdapter) RelationshipRuntimeSettings(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) (bool, bool, error) {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return false, false, err
	}
	contract, err := a.loadPublishedAgentMemoryContract(ctx, a.service.db, workspaceID, agentID)
	if err != nil {
		return false, false, err
	}
	active := relationship.Status == RelationshipActive && relationship.MemoryEnabled && contract.Enabled
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
	if !published.Enabled {
		return nil, mapAgentMemoryError(ErrPolicyRejected)
	}
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
	item, err := a.service.PutItem(ctx, relationship.ID, PutItemRequest{Key: strings.TrimSpace(req.Key), DisplayName: strings.TrimSpace(req.Key), Content: req.Content, ExpectedRevision: expected, Contract: agentContract(), Metadata: metadata})
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
	if !published.Enabled {
		return nil, mapAgentMemoryError(ErrPolicyRejected)
	}
	if meta.ConfigScope == agentmemory.ConfigScopePublished {
		if err := a.validatePublishedConfigRevision(ctx, a.service.db, workspaceID, agentID, meta.ConfigRevision); err != nil {
			return nil, err
		}
	} else if meta.ConfigScope != agentmemory.ConfigScopeDraft || strings.TrimSpace(meta.ConfigRevision) == "" {
		return nil, agentmemory.ErrInvalidInput
	}
	if len(req.Operations) == 0 || len(req.Operations) > maxAgentFileOperations {
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
		if !agentmemory.ValidMemoryKey(operation.Key) {
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
	// One tool call or extraction pass is one commit in the version history.
	ctx = WithCommitScope(ctx)
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
			// An operation's own source wins; without one, the batch's source
			// (the turn that wrote it) is kept, so later passes can tell how
			// recent the file is.
			if operation.SourceMessageID != nil {
				metadata.SourceMessageID = operation.SourceMessageID
			}
			if operation.SourceCompletedAt != nil {
				metadata.SourceCompletedAt = operation.SourceCompletedAt
			}
			metadata.EvidenceFromUserInput = metadata.EvidenceFromUserInput || operation.SourceMessageID != nil
			metadata.Evidence, metadata.Reason = operation.Evidence, operation.Reason
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
			item, err := transactionService.PutItem(ctx, relationship.ID, PutItemRequest{Key: operation.Key, DisplayName: operation.Key, Content: operation.Content, ExpectedRevision: operation.ExpectedRevision, Contract: agentContract(), Metadata: metadata})
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

func (a *AgentAdapter) CleanupExpiredRelationshipMemory(ctx context.Context, now time.Time, auditRetention time.Duration) error {
	return a.service.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at <= ?", now).Delete(&UserMemoryUndoRecord{}).Error; err != nil {
			return err
		}
		// Background consolidation is gone (ADR 0020); jobs it left behind
		// are dropped once they are as old as the audit trail kept elsewhere.
		if err := tx.Where("created_at < ?", now.Add(-extractionJobRetention)).Delete(&UserMemoryExtractionJob{}).Error; err != nil {
			return err
		}
		if err := tx.Where("created_at < ?", now.Add(-auditRetention)).Delete(&UserMemoryEvent{}).Error; err != nil {
			return err
		}
		history := NewService(tx, a.service.codec)
		history.now = func() time.Time { return now }
		return history.CleanupExpiredHistory(ctx)
	})
}

// ArchiveText keeps text in an Agent user's archive (ADR 0014), for
// consolidation moving overflow out of a full file.
func (a *AgentAdapter) ArchiveText(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, content, tags, sourceKey string, meta agentmemory.MutationMetadata) error {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return err
	}
	metadata := relationshipMetadata(meta, uuid.Nil)
	metadata.Reason = "moved out of a full file"
	if _, err := a.service.AddArchiveEntry(ctx, relationship.ID, ArchiveRequest{Content: content, Tags: tags, SourceKey: sourceKey, Metadata: metadata}); err != nil {
		return mapAgentMemoryError(err)
	}
	return nil
}

// ForgottenLines returns what an Agent user removed from their memory. A user
// without relationship memory has forgotten nothing here.
func (a *AgentAdapter) ForgottenLines(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) ([]string, error) {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, err
	}
	return a.service.ForgottenLines(ctx, relationship.ID)
}

// SearchArchive finds an Agent user's archive entries for a query.
func (a *AgentAdapter) SearchArchive(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, query string, limit int) ([]UserMemoryArchiveEntry, error) {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, err
	}
	return a.service.SearchArchive(ctx, relationship.ID, query, limit)
}

// Archive lists an Agent user's archive, newest first.
func (a *AgentAdapter) Archive(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID) ([]UserMemoryArchiveEntry, error) {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, err
	}
	entries, err := a.service.ArchiveEntries(ctx, relationship.ID, 0)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	return entries, nil
}

// DeleteArchiveEntry removes one entry from an Agent user's archive.
func (a *AgentAdapter) DeleteArchiveEntry(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID, entryID uuid.UUID) error {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return err
	}
	return mapAgentMemoryError(a.service.DeleteArchiveEntry(ctx, relationship.ID, entryID, agentManagementMetadata(userScope, userID)))
}

// History returns the memory history of one Agent user.
func (a *AgentAdapter) History(ctx context.Context, workspaceID, agentID uuid.UUID, userScope string, userID uuid.UUID, req HistoryRequest) (*HistoryPage, error) {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, err
	}
	page, err := a.service.History(ctx, relationship.ID, req)
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	return page, nil
}

// RevertVersion undoes the latest change to one of an Agent user's memory
// files, within the published slots.
func (a *AgentAdapter) RevertVersion(ctx context.Context, workspaceID, agentID uuid.UUID, slots []agentmemory.RuntimeSlot, userScope string, userID, versionID uuid.UUID) (*UserMemoryItem, error) {
	relationship, err := a.relationship(ctx, workspaceID, agentID, userScope, userID)
	if err != nil {
		return nil, err
	}
	item, err := a.service.RevertVersion(ctx, relationship.ID, versionID, agentContract(), agentManagementMetadata(userScope, userID))
	if err != nil {
		return nil, mapAgentMemoryError(err)
	}
	return item, nil
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
			return agentmemory.SlotResponse{Key: slot.Key, Name: slot.Name, Description: slot.Description, MaxChars: slot.MaxChars,
				Enabled: slot.Enabled, SortOrder: slot.SortOrder, ReadOnly: slot.ReadOnly, AutoWrite: !slot.ManualOnly}
		}
	}
	return agentmemory.SlotResponse{Key: strings.TrimSpace(key), Name: strings.TrimSpace(key), MaxChars: maxMemoryContentChars, Enabled: true, AutoWrite: true}
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
	case errors.Is(err, ErrPolicyRejected), errors.Is(err, usersubject.ErrInvalidUser):
		return fmt.Errorf("%w: %v", agentmemory.ErrInvalidInput, err)
	case errors.Is(err, ErrRelationshipNotFound):
		return fmt.Errorf("%w: %v", agentmemory.ErrNotFound, err)
	case errors.Is(err, ErrAccessDenied):
		return fmt.Errorf("%w: %v", agentmemory.ErrUnauthorized, err)
	default:
		return err
	}
}
