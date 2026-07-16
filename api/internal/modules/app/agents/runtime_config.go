package agents

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	llmmodelservice "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/service"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errAgentWebAppOffline         = errors.New("web app is offline")
	errAgentWebAppNotPublished    = errors.New("agent web app has no published version")
	errAgentWebAppNotAgentRuntime = errors.New("web app is not an AGENT runtime")
	errAgentPromptTooLong         = errors.New("agent system prompt is too long")
)

const agentModelSelectionUseCase = llmmodelservice.AgentRuntimeUseCase

const (
	agentSystemPromptPatchOperationAppend        = "append"
	agentSystemPromptPatchOperationUpsertSection = "upsert_section"
	agentSystemPromptBaseChangedCode             = "agent_system_prompt_base_changed"
	agentSystemPromptPatchInvalidCode            = "agent_system_prompt_patch_invalid"
	maxAgentSystemPromptSeparatorChars           = 64
)

func (s *agentsService) GetAgentConfig(ctx context.Context, agentID, accountID string) (*dto.AgentConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true, agentRuntimeConfigReadPermissionCodes("AGENT")...)
	if err != nil {
		return nil, err
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
	resp.WorkflowBindings = s.hydrateAgentWorkflowBindingRuntimeInputs(ctx, ag.TenantID.String(), resp.WorkflowBindings)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	rows, bindingRevision, bindingHealth, err := s.draftBindingState(ctx, ag, cfg, accountID)
	if err != nil {
		return nil, err
	}
	applyAgentBindingAuthorizationsFromRows(resp, rows)
	resp.BindingRevision = bindingRevision
	resp.BindingHealth = bindingHealth
	return resp, nil
}

func (s *agentsService) GetAgentDraftRuntimeConfig(ctx context.Context, agentID, accountID string) (*dto.AgentDraftRuntimeConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
	resp.WorkflowBindings = s.hydrateAgentWorkflowBindingRuntimeInputs(ctx, ag.TenantID.String(), resp.WorkflowBindings)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	rows, bindingRevision, bindingHealth, err := s.draftBindingState(ctx, ag, cfg, accountID)
	if err != nil {
		return nil, err
	}
	applyAgentBindingAuthorizationsFromRows(resp, rows)
	resp.BindingRevision = bindingRevision
	resp.BindingHealth = bindingHealth
	return &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     ag.ID.String(),
		WorkspaceID: ag.TenantID.String(),
		Config:      *resp,
	}, nil
}

// UpdateAgentConfigWithSystemPromptPatch applies an incremental prompt mutation
// against a frozen baseline. The baseline check and draft mutation happen while
// the current config row is locked, so an approval continuation cannot overwrite
// a prompt that changed while it was waiting.
func (s *agentsService) UpdateAgentConfigWithSystemPromptPatch(ctx context.Context, agentID, accountID string, req dto.AgentSystemPromptPatchRequest) (*dto.AgentConfigResponse, error) {
	operation := strings.ToLower(strings.TrimSpace(req.Operation))
	if operation != agentSystemPromptPatchOperationAppend && operation != agentSystemPromptPatchOperationUpsertSection {
		return nil, agentSystemPromptPatchInvalidAPIError("system prompt patch operation must be append or upsert_section")
	}
	if strings.TrimSpace(req.AppendContent) == "" {
		return nil, agentSystemPromptPatchInvalidAPIError("system prompt patch content is required")
	}
	if !utf8.ValidString(req.AppendContent) {
		return nil, agentSystemPromptPatchInvalidAPIError("system prompt patch content must contain valid UTF-8 text")
	}
	if !utf8.ValidString(req.Separator) {
		return nil, agentSystemPromptPatchInvalidAPIError("system prompt patch separator must contain valid UTF-8 text")
	}
	if characters := utf8.RuneCountInString(req.Separator); characters > maxAgentSystemPromptSeparatorChars {
		return nil, agentSystemPromptPatchInvalidAPIError("system prompt patch separator exceeds %d characters", maxAgentSystemPromptSeparatorChars)
	}
	if strings.TrimSpace(req.ExpectedBaseSHA256) == "" {
		return nil, agentSystemPromptPatchInvalidAPIError("system prompt patch baseline digest is required")
	}
	if operation == agentSystemPromptPatchOperationUpsertSection {
		if err := validateAgentSystemPromptSection(req.SectionID, req.SectionTitle); err != nil {
			return nil, err
		}
	}
	req.Operation = operation
	if s.db == nil || s.agentBindings == nil {
		return nil, fmt.Errorf("database and agent binding repository are required for atomic system prompt patch")
	}
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	runtimeReq := normalizeAgentConfigRequest(req.Config)
	organizationID, err := s.organizationUUIDForAgentWorkspace(ctx, ag.TenantID.String())
	if err != nil {
		return nil, err
	}
	if err := s.validateAgentModelEligibility(ctx, organizationID, runtimeReq.ModelProvider, runtimeReq.Model); err != nil {
		return nil, err
	}
	runtimeReq.WorkflowBindings = s.hydrateAgentWorkflowBindingTypes(ctx, ag.TenantID.String(), runtimeReq.WorkflowBindings)
	req.Config = runtimeReq
	return s.updateAgentConfigWithSystemPromptPatchCAS(ctx, ag, cfg, accountID, req)
}

func (s *agentsService) UpdateAgentConfig(ctx context.Context, agentID, accountID string, req dto.AgentConfigRequest) (*dto.AgentConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	runtimeReq := normalizeAgentConfigRequest(req)
	organizationID, err := s.organizationUUIDForAgentWorkspace(ctx, ag.TenantID.String())
	if err != nil {
		return nil, err
	}
	if err := s.validateAgentModelEligibility(ctx, organizationID, runtimeReq.ModelProvider, runtimeReq.Model); err != nil {
		return nil, err
	}
	runtimeReq.WorkflowBindings = s.hydrateAgentWorkflowBindingTypes(ctx, ag.TenantID.String(), runtimeReq.WorkflowBindings)
	previous := agentConfigResponse(ag.ID.String(), cfg)
	previousRows, currentRevision, currentHealth, err := s.draftBindingState(ctx, ag, cfg, accountID)
	if err != nil {
		return nil, err
	}
	applyAgentBindingAuthorizationsFromRows(previous, previousRows)
	if revision := strings.TrimSpace(req.BindingRevision); revision != "" && revision != currentRevision {
		previous.BindingRevision = currentRevision
		previous.BindingHealth = currentHealth
		return nil, &agentBindingAPIError{
			Code:    agentBindingRevisionConflictCode,
			Message: "agent binding revision has changed",
			Data: map[string]interface{}{
				"current_config":   previous,
				"binding_revision": currentRevision,
				"binding_health":   currentHealth,
			},
		}
	}
	if err := s.validateIncrementalAgentBindingChanges(ctx, ag, accountID, previous, runtimeReq); err != nil {
		return nil, err
	}
	var rows []agentbindings.Binding
	if s.db != nil && s.agentBindings != nil {
		cfg, rows, err = s.updateAgentConfigCAS(ctx, ag, cfg, currentRevision, runtimeReq, accountID)
		if err != nil {
			return nil, err
		}
	} else {
		if _, err := applyAgentConfigRequestToDraft(cfg, runtimeReq, accountID); err != nil {
			return nil, err
		}
		if uid, parseErr := uuid.Parse(accountID); parseErr == nil {
			cfg.UpdatedBy = &uid
		}
		resp := agentConfigResponse(ag.ID.String(), cfg)
		rows, err = s.bindingRowsForConfig(ctx, ag, resp, agentbindings.ScopeDraft, nil, accountID, time.Now())
		if err != nil {
			return nil, err
		}
		if err := s.persistAgentDraftConfigAndBindings(ctx, cfg, ag.ID, rows); err != nil {
			return nil, err
		}
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
	resp.BindingRevision = agentBindingRevision(rows)
	resp.BindingHealth = s.resolveAgentBindingHealth(ctx, ag, accountID, resp, rows)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	if removedBindings := removedAgentBindingAuditItems(previousRows, rows); len(removedBindings) > 0 {
		logger.InfoContext(ctx, "agent draft resource bindings removed",
			"log_type", "audit",
			"actor_account_id", accountID,
			"organization_id", s.organizationIDForAgentWorkspace(ctx, ag.TenantID.String()),
			"workspace_id", ag.TenantID,
			"agent_id", ag.ID,
			"removed_bindings", removedBindings,
			"binding_state_before", "bound",
			"binding_state_after", "unbound",
		)
	}
	return resp, nil
}

func removedAgentBindingAuditItems(before, after []agentbindings.Binding) []map[string]string {
	remaining := make(map[string]struct{}, len(after))
	for _, row := range after {
		remaining[agentBindingItemKey(string(row.BindingType), row.ParentResourceID, row.ResourceID, row.AccessMode)] = struct{}{}
	}
	removed := make([]map[string]string, 0)
	for _, row := range before {
		if _, ok := remaining[agentBindingItemKey(string(row.BindingType), row.ParentResourceID, row.ResourceID, row.AccessMode)]; ok {
			continue
		}
		removed = append(removed, map[string]string{
			"binding_type":       string(row.BindingType),
			"resource_id":        row.ResourceID,
			"parent_resource_id": row.ParentResourceID,
			"access_mode":        row.AccessMode,
		})
	}
	return removed
}

func (s *agentsService) updateAgentConfigCAS(ctx context.Context, ag *Agent, stale *AgentsConfig, expectedRevision string, req dto.AgentConfigRequest, accountID string) (*AgentsConfig, []agentbindings.Binding, error) {
	if stale == nil {
		return nil, nil, fmt.Errorf("agent draft config is required")
	}
	candidate := *stale
	if _, err := applyAgentConfigRequestToDraft(&candidate, req, accountID); err != nil {
		return nil, nil, err
	}
	candidateRows, err := s.bindingRowsForConfig(ctx, ag, agentConfigResponse(ag.ID.String(), &candidate), agentbindings.ScopeDraft, nil, accountID, time.Now())
	if err != nil {
		return nil, nil, err
	}
	var saved *AgentsConfig
	var savedRows []agentbindings.Binding
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bindingRepo := s.agentBindings.WithTx(tx)
		if err := bindingRepo.LockResources(ctx, tx, agentBindingResourceRefs(candidateRows)); err != nil {
			return err
		}
		if err := bindingRepo.LockAgents(ctx, tx, []uuid.UUID{ag.ID}); err != nil {
			return err
		}
		if err := ensureAgentWorkspaceUnchanged(ctx, tx, ag); err != nil {
			return err
		}
		var current AgentsConfig
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL")
		if stale != nil && stale.ID != uuid.Nil {
			query = query.Where("id = ?", stale.ID)
		} else {
			query = query.Where("agents_id = ?", ag.ID).Order("updated_at DESC, created_at DESC")
		}
		if err := query.First(&current).Error; err != nil {
			return err
		}
		currentConfig := agentConfigResponse(ag.ID.String(), &current)
		currentRows, err := s.bindingRowsForConfig(ctx, ag, currentConfig, agentbindings.ScopeDraft, nil, accountID, time.Now())
		if err != nil {
			return err
		}
		existing, err := bindingRepo.ListScope(ctx, agentbindings.ScopeRef{AgentID: ag.ID, Scope: agentbindings.ScopeDraft})
		if err != nil {
			return err
		}
		currentRows = preserveAgentBindingEvidence(currentRows, existing)
		applyAgentBindingAuthorizationsFromRows(currentConfig, currentRows)
		currentRevision := agentBindingRevision(currentRows)
		if currentRevision != expectedRevision {
			currentHealth := s.resolveAgentBindingHealth(ctx, ag, accountID, currentConfig, currentRows)
			currentConfig.BindingRevision = currentRevision
			currentConfig.BindingHealth = currentHealth
			return &agentBindingAPIError{Code: agentBindingRevisionConflictCode, Message: "agent binding revision has changed", Data: map[string]interface{}{
				"current_config": currentConfig, "binding_revision": currentRevision, "binding_health": currentHealth,
			}}
		}
		txService := *s
		txService.db = tx
		txService.agentBindings = bindingRepo
		if err := txService.validateIncrementalAgentBindingChanges(ctx, ag, accountID, currentConfig, req); err != nil {
			return err
		}
		if len(req.BindingAuthorizations) == 0 {
			req.BindingAuthorizations = append([]dto.AgentBindingAuthorization(nil), currentConfig.BindingAuthorizations...)
		}
		if _, err := applyAgentConfigRequestToDraft(&current, req, accountID); err != nil {
			return err
		}
		if actorID, parseErr := uuid.Parse(accountID); parseErr == nil {
			current.UpdatedBy = &actorID
		}
		nextConfig := agentConfigResponse(ag.ID.String(), &current)
		nextRows, err := s.bindingRowsForConfig(ctx, ag, nextConfig, agentbindings.ScopeDraft, nil, accountID, time.Now())
		if err != nil {
			return err
		}
		nextRows = preserveAgentBindingEvidence(nextRows, existing)
		if err := NewAgentsRepository(tx).UpdateAgentsConfig(ctx, &current); err != nil {
			return err
		}
		if err := bindingRepo.ReplaceScope(ctx, tx, agentbindings.ScopeRef{AgentID: ag.ID, Scope: agentbindings.ScopeDraft}, nextRows); err != nil {
			return err
		}
		saved = &current
		savedRows = nextRows
		return nil
	})
	return saved, savedRows, err
}

func (s *agentsService) updateAgentConfigWithSystemPromptPatchCAS(ctx context.Context, ag *Agent, stale *AgentsConfig, accountID string, patch dto.AgentSystemPromptPatchRequest) (*dto.AgentConfigResponse, error) {
	if ag == nil || stale == nil {
		return nil, fmt.Errorf("agent and draft config are required")
	}
	if s.db == nil || s.agentBindings == nil {
		return nil, fmt.Errorf("database and agent binding repository are required for atomic system prompt patch")
	}
	staleConfig := agentConfigResponse(ag.ID.String(), stale)
	candidateReq, err := mergeAgentConfigRequestedFields(agentConfigRequestFromResponse(*staleConfig), patch.Config, patch.RequestedFields)
	if err != nil {
		return nil, err
	}
	candidateReq.SystemPrompt, err = applyAgentSystemPromptPatch(staleConfig.SystemPrompt, patch)
	if err != nil {
		return nil, err
	}
	candidate := *stale
	if _, err := applyAgentConfigRequestToDraft(&candidate, candidateReq, accountID); err != nil {
		return nil, err
	}
	candidate.PrePrompt = stringPtr(candidateReq.SystemPrompt)
	candidateRows, err := s.bindingRowsForConfig(ctx, ag, agentConfigResponse(ag.ID.String(), &candidate), agentbindings.ScopeDraft, nil, accountID, time.Now())
	if err != nil {
		return nil, err
	}
	var saved *AgentsConfig
	var savedRows []agentbindings.Binding
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bindingRepo := s.agentBindings.WithTx(tx)
		if err := bindingRepo.LockResources(ctx, tx, agentBindingResourceRefs(candidateRows)); err != nil {
			return err
		}
		if err := bindingRepo.LockAgents(ctx, tx, []uuid.UUID{ag.ID}); err != nil {
			return err
		}
		if err := ensureAgentWorkspaceUnchanged(ctx, tx, ag); err != nil {
			return err
		}
		var current AgentsConfig
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL")
		if stale.ID != uuid.Nil {
			query = query.Where("id = ?", stale.ID)
		} else {
			query = query.Where("agents_id = ?", ag.ID).Order("updated_at DESC, created_at DESC")
		}
		if err := query.First(&current).Error; err != nil {
			return fmt.Errorf("lock agent draft config for system prompt patch: %w", err)
		}
		currentConfig := agentConfigResponse(ag.ID.String(), &current)
		actualBaseDigest := agentSystemPromptSHA256(currentConfig.SystemPrompt)
		if expected := strings.TrimSpace(patch.ExpectedBaseSHA256); expected != actualBaseDigest {
			return &agentBindingAPIError{
				Code:    agentSystemPromptBaseChangedCode,
				Message: "agent system prompt has changed",
				Data: map[string]interface{}{
					"agent_id":                    ag.ID.String(),
					"expected_base_sha256":        expected,
					"current_base_sha256":         actualBaseDigest,
					"current_system_prompt_chars": utf8.RuneCountInString(currentConfig.SystemPrompt),
				},
			}
		}
		currentRows, err := s.bindingRowsForConfig(ctx, ag, currentConfig, agentbindings.ScopeDraft, nil, accountID, time.Now())
		if err != nil {
			return err
		}
		existing, err := bindingRepo.ListScope(ctx, agentbindings.ScopeRef{AgentID: ag.ID, Scope: agentbindings.ScopeDraft})
		if err != nil {
			return err
		}
		currentRows = preserveAgentBindingEvidence(currentRows, existing)
		applyAgentBindingAuthorizationsFromRows(currentConfig, currentRows)
		currentRevision := agentBindingRevision(currentRows)
		if expectedRevision := strings.TrimSpace(patch.Config.BindingRevision); expectedRevision != "" && expectedRevision != currentRevision {
			currentHealth := s.resolveAgentBindingHealth(ctx, ag, accountID, currentConfig, currentRows)
			currentConfig.BindingRevision = currentRevision
			currentConfig.BindingHealth = currentHealth
			return &agentBindingAPIError{Code: agentBindingRevisionConflictCode, Message: "agent binding revision has changed", Data: map[string]interface{}{
				"current_config": currentConfig, "binding_revision": currentRevision, "binding_health": currentHealth,
			}}
		}
		nextReq, err := mergeAgentConfigRequestedFields(agentConfigRequestFromResponse(*currentConfig), patch.Config, patch.RequestedFields)
		if err != nil {
			return err
		}
		nextReq.SystemPrompt, err = applyAgentSystemPromptPatch(currentConfig.SystemPrompt, patch)
		if err != nil {
			return err
		}
		txService := *s
		txService.db = tx
		txService.agentBindings = bindingRepo
		if err := txService.validateIncrementalAgentBindingChanges(ctx, ag, accountID, currentConfig, nextReq); err != nil {
			return err
		}
		if len(nextReq.BindingAuthorizations) == 0 {
			nextReq.BindingAuthorizations = append([]dto.AgentBindingAuthorization(nil), currentConfig.BindingAuthorizations...)
		}
		if _, err := applyAgentConfigRequestToDraft(&current, nextReq, accountID); err != nil {
			return err
		}
		current.PrePrompt = stringPtr(nextReq.SystemPrompt)
		if actorID, parseErr := uuid.Parse(accountID); parseErr == nil {
			current.UpdatedBy = &actorID
		}
		nextConfig := agentConfigResponse(ag.ID.String(), &current)
		nextRows, err := txService.bindingRowsForConfig(ctx, ag, nextConfig, agentbindings.ScopeDraft, nil, accountID, time.Now())
		if err != nil {
			return err
		}
		nextRows = preserveAgentBindingEvidence(nextRows, existing)
		if err := NewAgentsRepository(tx).UpdateAgentsConfig(ctx, &current); err != nil {
			return err
		}
		if err := bindingRepo.ReplaceScope(ctx, tx, agentbindings.ScopeRef{AgentID: ag.ID, Scope: agentbindings.ScopeDraft}, nextRows); err != nil {
			return err
		}
		saved = &current
		savedRows = nextRows
		return nil
	})
	if err != nil {
		return nil, err
	}
	resp := agentConfigResponse(ag.ID.String(), saved)
	resp.BindingRevision = agentBindingRevision(savedRows)
	resp.BindingHealth = s.resolveAgentBindingHealth(ctx, ag, accountID, resp, savedRows)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	return resp, nil
}

func mergeAgentConfigRequestedFields(current dto.AgentConfigRequest, requested dto.AgentConfigRequest, fields []string) (dto.AgentConfigRequest, error) {
	for _, field := range fields {
		switch strings.TrimSpace(field) {
		case "system_prompt":
			// The locked current prompt is patched separately after the field merge.
		case "model_provider":
			current.ModelProvider = requested.ModelProvider
		case "model":
			current.Model = requested.Model
		case "model_parameters":
			current.ModelParameters = requested.ModelParameters
		case "enabled_skill_ids":
			current.EnabledSkillIDs = requested.EnabledSkillIDs
		case "agent_memory_enabled":
			current.AgentMemoryEnabled = requested.AgentMemoryEnabled
		case "file_upload_enabled":
			current.FileUpload = requested.FileUpload
		case "home_title":
			current.HomeTitle = requested.HomeTitle
		case "opening_statement":
			current.OpeningStatement = requested.OpeningStatement
		case "input_placeholder":
			current.InputPlaceholder = requested.InputPlaceholder
		case "theme_color":
			current.ThemeColor = requested.ThemeColor
		case "suggested_questions":
			current.SuggestedQuestions = requested.SuggestedQuestions
		case "knowledge_dataset_ids":
			current.KnowledgeDatasetIDs = requested.KnowledgeDatasetIDs
		case "knowledge_retrieval_config":
			current.KnowledgeRetrievalConfig = requested.KnowledgeRetrievalConfig
		case "database_bindings":
			current.DatabaseBindings = requested.DatabaseBindings
		case "workflow_bindings":
			current.WorkflowBindings = requested.WorkflowBindings
		case "":
			continue
		default:
			return dto.AgentConfigRequest{}, fmt.Errorf("unsupported agent config patch field %q", field)
		}
	}
	current.BindingRevision = requested.BindingRevision
	return current, nil
}

func appendAgentSystemPrompt(current string, addition string, separator string) (string, error) {
	if strings.TrimSpace(addition) == "" {
		return "", agentSystemPromptPatchInvalidAPIError("system prompt patch content is required")
	}
	result := addition
	if current != "" {
		result = current + separator + addition
	}
	if err := validateAgentSystemPromptSource(result); err != nil {
		return "", agentSystemPromptPatchInvalidAPIError("%v", err)
	}
	return result, nil
}

func applyAgentSystemPromptPatch(current string, patch dto.AgentSystemPromptPatchRequest) (string, error) {
	switch strings.ToLower(strings.TrimSpace(patch.Operation)) {
	case agentSystemPromptPatchOperationAppend:
		return appendAgentSystemPrompt(current, patch.AppendContent, patch.Separator)
	case agentSystemPromptPatchOperationUpsertSection:
		return upsertAgentSystemPromptSection(current, patch.AppendContent, patch.Separator, patch.SectionID, patch.SectionTitle)
	default:
		return "", agentSystemPromptPatchInvalidAPIError("system prompt patch operation must be append or upsert_section")
	}
}

func validateAgentSystemPromptSection(sectionID string, sectionTitle string) error {
	sectionID = strings.TrimSpace(sectionID)
	if sectionID == "" || len(sectionID) > 64 {
		return agentSystemPromptPatchInvalidAPIError("system prompt section id is invalid")
	}
	for index := 0; index < len(sectionID); index++ {
		char := sectionID[index]
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '.' || char == '_' || char == '-' {
			continue
		}
		return agentSystemPromptPatchInvalidAPIError("system prompt section id is invalid")
	}
	if !utf8.ValidString(sectionTitle) || utf8.RuneCountInString(sectionTitle) > 128 || strings.ContainsAny(sectionTitle, "\r\n") {
		return agentSystemPromptPatchInvalidAPIError("system prompt section title is invalid")
	}
	return nil
}

func upsertAgentSystemPromptSection(current string, content string, separator string, sectionID string, sectionTitle string) (string, error) {
	if err := validateAgentSystemPromptSection(sectionID, sectionTitle); err != nil {
		return "", err
	}
	if strings.TrimSpace(content) == "" {
		return "", agentSystemPromptPatchInvalidAPIError("system prompt patch content is required")
	}
	startMarker, endMarker := agentSystemPromptSectionMarkers(strings.TrimSpace(sectionID))
	if strings.Contains(content, startMarker) || strings.Contains(content, endMarker) {
		return "", agentSystemPromptPatchInvalidAPIError("system prompt section content contains reserved markers")
	}
	body := content
	if title := strings.TrimSpace(sectionTitle); title != "" {
		body = "## " + title + "\n\n" + content
	}
	block := startMarker + "\n" + body + "\n" + endMarker
	startCount := strings.Count(current, startMarker)
	endCount := strings.Count(current, endMarker)
	var result string
	switch {
	case startCount == 0 && endCount == 0:
		result = block
		if current != "" {
			result = current + separator + block
		}
	case startCount == 1 && endCount == 1:
		start := strings.Index(current, startMarker)
		endRelative := strings.Index(current[start+len(startMarker):], endMarker)
		if endRelative < 0 {
			return "", agentSystemPromptPatchInvalidAPIError("system prompt section markers are malformed")
		}
		end := start + len(startMarker) + endRelative + len(endMarker)
		result = current[:start] + block + current[end:]
	default:
		return "", agentSystemPromptPatchInvalidAPIError("system prompt section markers are duplicated or malformed")
	}
	if err := validateAgentSystemPromptSource(result); err != nil {
		return "", agentSystemPromptPatchInvalidAPIError("%v", err)
	}
	return result, nil
}

func agentSystemPromptSectionMarkers(sectionID string) (string, string) {
	prefix := "<!-- zgi:system-prompt-section:" + sectionID
	return prefix + ":start -->", prefix + ":end -->"
}

func agentSystemPromptPatchInvalidAPIError(format string, args ...interface{}) error {
	return &agentBindingAPIError{
		Code:    agentSystemPromptPatchInvalidCode,
		Message: fmt.Sprintf(format, args...),
	}
}

func agentSystemPromptSHA256(content string) string {
	digest := sha256.Sum256([]byte(content))
	return fmt.Sprintf("sha256:%x", digest[:])
}

func (s *agentsService) PublishAgent(ctx context.Context, agentID, accountID string, req dto.PublishAgentRequest) (*dto.PublishAgentResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true, agentPublishPermissionCodes("AGENT")...)
	if err != nil {
		return nil, err
	}
	snapshot := agentConfigSnapshot(ag.ID.String(), cfg)
	organizationID, err := s.organizationUUIDForAgentWorkspace(ctx, ag.TenantID.String())
	if err != nil {
		return nil, err
	}
	if err := s.validateAgentModelEligibility(
		ctx,
		organizationID,
		stringFromSnapshot(snapshot, "model_provider"),
		stringFromSnapshot(snapshot, "model"),
	); err != nil {
		return nil, err
	}
	if err := validateAgentSystemPromptSource(stringFromSnapshot(snapshot, "system_prompt")); err != nil {
		return nil, err
	}
	_, bindingRevision, bindingHealth, err := s.draftBindingState(ctx, ag, cfg, accountID)
	if err != nil {
		return nil, err
	}
	if revision := strings.TrimSpace(req.BindingRevision); revision != "" && revision != bindingRevision {
		current := agentConfigResponse(ag.ID.String(), cfg)
		current.BindingRevision = bindingRevision
		current.BindingHealth = bindingHealth
		return nil, &agentBindingAPIError{
			Code:    agentBindingRevisionConflictCode,
			Message: "agent binding revision has changed",
			Data: map[string]interface{}{
				"current_config":   current,
				"binding_revision": bindingRevision,
				"binding_health":   bindingHealth,
			},
		}
	}
	if bindingHealth.UnavailableCount > 0 {
		return nil, &agentBindingAPIError{Code: agentBindingsInvalidCode, Message: "agent has unavailable bindings", Data: map[string]interface{}{"binding_health": bindingHealth}}
	}
	if bindingHealth.SuspendedCount > 0 && !req.AcknowledgeSuspendedBindings {
		return nil, &agentBindingAPIError{Code: agentBindingsSuspendedCode, Message: "agent has suspended bindings", Data: map[string]interface{}{"binding_health": bindingHealth}}
	}
	currentMemorySlots, err := s.loadAgentMemorySlotsForDraft(ctx, ag.ID)
	if err != nil {
		return nil, fmt.Errorf("load agent memory slots for publish: %w", err)
	}
	now := time.Now()
	versionUUID := uuid.New()
	version := &AgentPublishedVersion{
		AgentID:     ag.ID,
		WorkspaceID: ag.TenantID,
		Version:     now.Format("20060102150405"),
		VersionUUID: versionUUID,
		Name:        strings.TrimSpace(req.Name),
		Description: strings.TrimSpace(req.Description),
		CreatedAt:   now,
	}
	if uid, err := uuid.Parse(accountID); err == nil {
		version.CreatedBy = &uid
	}
	if err := s.createAgentPublishedVersion(ctx, version, ag, cfg, accountID, bindingRevision, req.AcknowledgeSuspendedBindings, currentMemorySlots); err != nil {
		return nil, err
	}
	return &dto.PublishAgentResponse{
		AgentID:     ag.ID.String(),
		VersionUUID: versionUUID.String(),
		Version:     version.Version,
		Name:        version.Name,
		Description: version.Description,
		WebAppID:    ag.WebAppID.String(),
		PublishedAt: now.Unix(),
	}, nil
}

func (s *agentsService) persistAgentDraftConfigAndBindings(ctx context.Context, cfg *AgentsConfig, agentID uuid.UUID, bindings []agentbindings.Binding) error {
	if cfg == nil {
		return fmt.Errorf("agent config is required")
	}
	if s.db == nil || s.agentBindings == nil {
		return s.agentsRepo.UpdateAgentsConfig(ctx, cfg)
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := NewAgentsRepository(tx).UpdateAgentsConfig(ctx, cfg); err != nil {
			return err
		}
		return s.agentBindings.ReplaceScope(ctx, tx, agentbindings.ScopeRef{
			AgentID: agentID,
			Scope:   agentbindings.ScopeDraft,
		}, bindings)
	})
}

func ensureAgentWorkspaceUnchanged(ctx context.Context, tx *gorm.DB, ag *Agent) error {
	if tx == nil || ag == nil || ag.ID == uuid.Nil {
		return fmt.Errorf("agent transaction and id are required")
	}
	var current struct {
		TenantID uuid.UUID
	}
	if err := tx.WithContext(ctx).Table("agents").Select("tenant_id").Where("id = ? AND deleted_at IS NULL", ag.ID).Take(&current).Error; err != nil {
		return fmt.Errorf("reload agent workspace: %w", err)
	}
	if current.TenantID != ag.TenantID {
		return &agentBindingAPIError{
			Code:    agentBindingRevisionConflictCode,
			Message: "agent workspace has changed",
			Data: map[string]interface{}{
				"agent_id":             ag.ID.String(),
				"current_workspace_id": current.TenantID.String(),
			},
		}
	}
	return nil
}

func (s *agentsService) validateAgentModelEligibility(ctx context.Context, organizationID uuid.UUID, provider, modelName string) error {
	provider = strings.TrimSpace(provider)
	modelName = strings.TrimSpace(modelName)
	if provider == "" || modelName == "" {
		return fmt.Errorf("agent model provider and model are required")
	}
	if s.agentModels == nil {
		return fmt.Errorf("agent model eligibility service is unavailable")
	}
	models, err := s.agentModels.ListAvailable(ctx, organizationID, provider, agentModelSelectionUseCase)
	if err != nil {
		return fmt.Errorf("list available agent models: %w", err)
	}
	for _, candidate := range models {
		if candidate != nil && candidate.Provider == provider && candidate.Name == modelName {
			if !candidate.Features.FunctionCalling {
				return fmt.Errorf("agent model %s/%s does not support function calling", provider, modelName)
			}
			return nil
		}
	}
	return fmt.Errorf("agent model %s/%s is unavailable", provider, modelName)
}

func (s *agentsService) createAgentPublishedVersion(
	ctx context.Context,
	version *AgentPublishedVersion,
	ag *Agent,
	staleConfig *AgentsConfig,
	accountID string,
	expectedBindingRevision string,
	acknowledgeSuspended bool,
	currentMemorySlots []dto.AgentMemorySlotConfig,
) error {
	if version == nil {
		return fmt.Errorf("agent published version is required")
	}
	if ag == nil || staleConfig == nil {
		return fmt.Errorf("agent and draft config are required")
	}
	if s.db == nil {
		return fmt.Errorf("database is required")
	}
	candidateRows, err := s.bindingRowsForConfig(ctx, ag, agentConfigResponse(ag.ID.String(), staleConfig), agentbindings.ScopeDraft, nil, accountID, time.Now())
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bindingRepo := s.agentBindings
		if bindingRepo == nil {
			bindingRepo = agentbindings.NewRepository(tx)
		} else {
			bindingRepo = bindingRepo.WithTx(tx)
		}
		if err := bindingRepo.LockResources(ctx, tx, agentBindingResourceRefs(candidateRows)); err != nil {
			return err
		}
		if err := bindingRepo.LockAgents(ctx, tx, []uuid.UUID{ag.ID}); err != nil {
			return err
		}
		if err := ensureAgentWorkspaceUnchanged(ctx, tx, ag); err != nil {
			return err
		}
		var current AgentsConfig
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL")
		if staleConfig.ID != uuid.Nil {
			query = query.Where("id = ?", staleConfig.ID)
		} else {
			query = query.Where("agents_id = ?", ag.ID).Order("updated_at DESC, created_at DESC")
		}
		if err := query.First(&current).Error; err != nil {
			return fmt.Errorf("lock agent draft config for publish: %w", err)
		}

		txService := *s
		txService.db = tx
		txService.agentBindings = bindingRepo
		currentConfig := agentConfigResponse(ag.ID.String(), &current)
		rows, err := txService.bindingRowsForConfig(ctx, ag, currentConfig, agentbindings.ScopeDraft, nil, accountID, time.Now())
		if err != nil {
			return err
		}
		existing, err := bindingRepo.ListScope(ctx, agentbindings.ScopeRef{AgentID: ag.ID, Scope: agentbindings.ScopeDraft})
		if err != nil {
			return err
		}
		rows = preserveAgentBindingEvidence(rows, existing)
		applyAgentBindingAuthorizationsFromRows(currentConfig, rows)
		bindingRevision := agentBindingRevision(rows)
		bindingHealth := txService.resolveAgentBindingHealth(ctx, ag, accountID, currentConfig, rows)
		if bindingRevision != expectedBindingRevision {
			currentConfig.BindingRevision = bindingRevision
			currentConfig.BindingHealth = bindingHealth
			return &agentBindingAPIError{
				Code:    agentBindingRevisionConflictCode,
				Message: "agent binding revision has changed",
				Data: map[string]interface{}{
					"current_config":   currentConfig,
					"binding_revision": bindingRevision,
					"binding_health":   bindingHealth,
				},
			}
		}
		if bindingHealth.UnavailableCount > 0 {
			return &agentBindingAPIError{Code: agentBindingsInvalidCode, Message: "agent has unavailable bindings", Data: map[string]interface{}{"binding_health": bindingHealth}}
		}
		if bindingHealth.SuspendedCount > 0 && !acknowledgeSuspended {
			return &agentBindingAPIError{Code: agentBindingsSuspendedCode, Message: "agent has suspended bindings", Data: map[string]interface{}{"binding_health": bindingHealth}}
		}

		snapshot := agentConfigSnapshot(ag.ID.String(), &current)
		snapshot["binding_authorizations"] = normalizeAgentBindingAuthorizations(currentConfig.BindingAuthorizations)
		snapshot["knowledge_bound_by_account_id"] = currentConfig.KnowledgeBoundByAccountID
		snapshot["knowledge_bound_at_unix"] = currentConfig.KnowledgeBoundAtUnix
		snapshot["database_bound_by_account_id"] = currentConfig.DatabaseBoundByAccountID
		snapshot["database_bound_at_unix"] = currentConfig.DatabaseBoundAtUnix
		snapshot["workflow_bound_by_account_id"] = currentConfig.WorkflowBoundByAccountID
		snapshot["workflow_bound_at_unix"] = currentConfig.WorkflowBoundAtUnix
		snapshot["binding_indexed"] = true
		snapshot["binding_revision"] = bindingRevision
		snapshot["supports_vision"] = txService.resolveAgentModelSupportsVision(
			ctx,
			txService.organizationIDForAgentWorkspace(ctx, ag.TenantID.String()),
			stringFromSnapshot(snapshot, "model_provider"),
			stringFromSnapshot(snapshot, "model"),
		)
		if err := validateAgentSystemPromptSource(stringFromSnapshot(snapshot, "system_prompt")); err != nil {
			return err
		}
		snapshotMemorySlots := []dto.AgentMemorySlotConfig{}
		if enabled, _ := snapshot["agent_memory_enabled"].(bool); enabled {
			snapshotMemorySlots = agentMemorySnapshotSlots(enabledAgentMemorySlots(currentMemorySlots))
		}
		snapshot["agent_memory_slots"] = snapshotMemorySlots
		version.ConfigSnapshot = snapshot
		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("failed to create agent published version: %w", err)
		}
		// Suspended bindings remain indexed so a later organization-policy re-enable
		// restores the published capability without requiring a republish.
		publishedBindings := publishedAgentBindingRows(rows)
		for idx := range publishedBindings {
			publishedBindings[idx].BindingScope = agentbindings.ScopePublished
			publishedBindings[idx].PublishedVersionUUID = &version.VersionUUID
		}
		if err := bindingRepo.ReplacePublishedHead(ctx, tx, agentbindings.ScopeRef{
			AgentID:              ag.ID,
			Scope:                agentbindings.ScopePublished,
			PublishedVersionUUID: &version.VersionUUID,
		}, publishedBindings); err != nil {
			return err
		}
		if s.agentMemoryService == nil {
			return nil
		}
		memoryService := agentmemory.NewService(tx)
		if err := memoryService.ClearValuesNotInKeys(ctx, ag.ID, agentMemoryKeys(currentMemorySlots)); err != nil {
			return fmt.Errorf("clear removed agent memory values: %w", err)
		}
		return nil
	})
}

func (s *agentsService) ListAgentPublishedVersions(ctx context.Context, agentID, accountID string, page, limit int) (*dto.AgentPublishedVersionsResponse, error) {
	_, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false, agentPublishPermissionCodes("AGENT")...)
	if err != nil {
		return nil, err
	}
	if page <= 0 {
		page = 1
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	versions, total, err := s.agentsRepo.ListAgentPublishedVersions(ctx, agentID, limit, (page-1)*limit)
	if err != nil {
		return nil, err
	}
	latest, err := s.agentsRepo.GetLatestAgentPublishedVersion(ctx, agentID)
	if err != nil {
		return nil, err
	}
	items := make([]dto.AgentPublishedVersionResponse, 0, len(versions))
	for _, version := range versions {
		if version == nil {
			continue
		}
		snapshot := agentConfigResponseFromSnapshot(version.AgentID.String(), version.ConfigSnapshot)
		items = append(items, dto.AgentPublishedVersionResponse{
			ID:             version.ID.String(),
			AgentID:        version.AgentID.String(),
			VersionUUID:    version.VersionUUID.String(),
			Version:        version.Version,
			Name:           version.Name,
			Description:    version.Description,
			ConfigSnapshot: *snapshot,
			IsCurrent:      latest != nil && latest.ID == version.ID,
			CreatedAt:      version.CreatedAt.Unix(),
		})
	}
	return &dto.AgentPublishedVersionsResponse{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: int64(page*limit) < total,
	}, nil
}

func (s *agentsService) PreviewAgentPublishedVersionRollback(ctx context.Context, agentID, accountID, versionID string) (*dto.AgentRollbackPreviewResponse, error) {
	ag, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false, agentPublishPermissionCodes("AGENT")...)
	if err != nil {
		return nil, err
	}
	version, snapshot, rows, health, err := s.agentRollbackImpact(ctx, ag, accountID, versionID)
	if err != nil {
		return nil, err
	}
	actorID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, fmt.Errorf("invalid account id")
	}
	if s.agentBindings == nil {
		return nil, fmt.Errorf("agent binding index is not configured")
	}
	impactToken, err := s.agentBindings.CreateRollbackImpactToken(actorID, ag.ID, version.ID.String(), agentBindingHealthRevision(health), time.Now())
	if err != nil {
		return nil, err
	}
	return agentRollbackPreview(version, snapshot, rows, health, impactToken), nil
}

func (s *agentsService) RollbackAgentPublishedVersion(ctx context.Context, agentID, accountID string, req dto.RollbackAgentPublishedVersionRequest) (*dto.AgentConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true, agentPublishPermissionCodes("AGENT")...)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.BindingAction) != "remove_all_abnormal" {
		return nil, fmt.Errorf("binding_action must be remove_all_abnormal")
	}
	_, _, candidateRows, _, err := s.agentRollbackImpact(ctx, ag, accountID, req.VersionID)
	if err != nil {
		return nil, err
	}
	actorID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, fmt.Errorf("invalid account id")
	}
	if s.agentBindings == nil {
		return nil, fmt.Errorf("agent binding index is not configured")
	}
	resp, err := s.rollbackAgentPublishedVersionCAS(ctx, ag, cfg, actorID, accountID, req, candidateRows)
	if err != nil {
		return nil, err
	}
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	return resp, nil
}

func agentRollbackPreview(version *AgentPublishedVersion, snapshot *dto.AgentConfigResponse, rows []agentbindings.Binding, health dto.AgentBindingHealth, impactToken string) *dto.AgentRollbackPreviewResponse {
	snapshotCopy := *snapshot
	snapshotCopy.BindingRevision = agentBindingRevision(rows)
	snapshotCopy.BindingHealth = health
	removed := make([]dto.AgentBindingHealthItem, 0, health.SuspendedCount+health.UnavailableCount)
	for _, item := range health.Items {
		if item.Status != agentBindingStatusActive {
			removed = append(removed, item)
		}
	}
	return &dto.AgentRollbackPreviewResponse{
		VersionID:       version.ID.String(),
		ConfigSnapshot:  snapshotCopy,
		BindingHealth:   health,
		RemovedBindings: removed,
		ImpactToken:     impactToken,
	}
}

func (s *agentsService) rollbackAgentPublishedVersionCAS(
	ctx context.Context,
	ag *Agent,
	staleConfig *AgentsConfig,
	actorID uuid.UUID,
	accountID string,
	req dto.RollbackAgentPublishedVersionRequest,
	candidateRows []agentbindings.Binding,
) (*dto.AgentConfigResponse, error) {
	if s.db == nil || s.agentBindings == nil || staleConfig == nil {
		return nil, fmt.Errorf("database, binding index, and draft config are required")
	}
	var result *dto.AgentConfigResponse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		bindingRepo := s.agentBindings.WithTx(tx)
		if err := bindingRepo.LockResources(ctx, tx, agentBindingResourceRefs(candidateRows)); err != nil {
			return err
		}
		if err := bindingRepo.LockAgents(ctx, tx, []uuid.UUID{ag.ID}); err != nil {
			return err
		}
		if err := ensureAgentWorkspaceUnchanged(ctx, tx, ag); err != nil {
			return err
		}
		var current AgentsConfig
		query := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("deleted_at IS NULL")
		if staleConfig.ID != uuid.Nil {
			query = query.Where("id = ?", staleConfig.ID)
		} else {
			query = query.Where("agents_id = ?", ag.ID).Order("updated_at DESC, created_at DESC")
		}
		if err := query.First(&current).Error; err != nil {
			return fmt.Errorf("lock agent draft config for rollback: %w", err)
		}

		txService := *s
		txService.db = tx
		txService.agentBindings = bindingRepo
		versionID := strings.TrimSpace(req.VersionID)
		if versionID == "" {
			return fmt.Errorf("version id is required")
		}
		version, err := NewAgentsRepository(tx).GetAgentPublishedVersionByID(ctx, ag.ID.String(), versionID)
		if err != nil {
			return err
		}
		if version == nil {
			return fmt.Errorf("published version not found")
		}
		snapshot := agentConfigResponseFromSnapshot(ag.ID.String(), version.ConfigSnapshot)
		impactRows, err := txService.bindingRowsForConfig(ctx, ag, snapshot, agentbindings.ScopeDraft, nil, accountID, version.CreatedAt)
		if err != nil {
			return err
		}
		health := txService.resolveAgentBindingHealth(ctx, ag, accountID, snapshot, impactRows)
		if err := bindingRepo.VerifyRollbackImpactToken(actorID, ag.ID, version.ID.String(), agentBindingHealthRevision(health), req.ImpactToken, time.Now()); err != nil {
			freshToken, tokenErr := bindingRepo.CreateRollbackImpactToken(actorID, ag.ID, version.ID.String(), agentBindingHealthRevision(health), time.Now())
			if tokenErr != nil {
				return tokenErr
			}
			return &agentBindingAPIError{
				Code:    agentRollbackImpactChangedCode,
				Message: "agent rollback impact has changed",
				Data:    agentRollbackPreview(version, snapshot, impactRows, health, freshToken),
			}
		}
		if !staleConfig.UpdatedAt.IsZero() && !current.UpdatedAt.Equal(staleConfig.UpdatedAt) {
			currentConfig := agentConfigResponse(ag.ID.String(), &current)
			currentRows, rowErr := txService.bindingRowsForConfig(ctx, ag, currentConfig, agentbindings.ScopeDraft, nil, accountID, time.Now())
			if rowErr != nil {
				return rowErr
			}
			currentConfig.BindingRevision = agentBindingRevision(currentRows)
			currentConfig.BindingHealth = txService.resolveAgentBindingHealth(ctx, ag, accountID, currentConfig, currentRows)
			return &agentBindingAPIError{Code: agentBindingRevisionConflictCode, Message: "agent draft config has changed", Data: map[string]interface{}{
				"current_config": currentConfig, "binding_revision": currentConfig.BindingRevision, "binding_health": currentConfig.BindingHealth,
			}}
		}

		snapshot.BindingHealth = health
		filtered := filterAgentConfigByBindingHealth(*snapshot)
		applied, err := applyAgentConfigRequestToDraft(&current, agentConfigRequestFromResponse(filtered), accountID)
		if err != nil {
			return err
		}
		current.UpdatedBy = &actorID
		currentConfig := agentConfigResponse(ag.ID.String(), &current)
		rows, err := txService.bindingRowsForConfig(ctx, ag, currentConfig, agentbindings.ScopeDraft, nil, accountID, time.Now())
		if err != nil {
			return err
		}
		if err := NewAgentsRepository(tx).UpdateAgentsConfig(ctx, &current); err != nil {
			return err
		}
		if err := bindingRepo.ReplaceScope(ctx, tx, agentbindings.ScopeRef{AgentID: ag.ID, Scope: agentbindings.ScopeDraft}, rows); err != nil {
			return err
		}
		if s.agentMemoryService != nil {
			if _, err := agentmemory.NewService(tx).ReplaceSlots(ctx, ag.ID, actorID, agentMemoryReplaceRequestFromConfig(snapshot.AgentMemorySlots, false)); err != nil {
				return fmt.Errorf("replace agent memory slots during rollback: %w", err)
			}
		}
		result = agentConfigResponse(ag.ID.String(), &current)
		result.EnabledSkillIDs = applied.EnabledSkillIDs
		result.BindingRevision = agentBindingRevision(rows)
		result.BindingHealth = txService.resolveAgentBindingHealth(ctx, ag, accountID, result, rows)
		return nil
	})
	return result, err
}

func (s *agentsService) agentRollbackImpact(ctx context.Context, ag *Agent, accountID, versionID string) (*AgentPublishedVersion, *dto.AgentConfigResponse, []agentbindings.Binding, dto.AgentBindingHealth, error) {
	versionID = strings.TrimSpace(versionID)
	if versionID == "" {
		return nil, nil, nil, dto.AgentBindingHealth{}, fmt.Errorf("version id is required")
	}
	version, err := s.agentsRepo.GetAgentPublishedVersionByID(ctx, ag.ID.String(), versionID)
	if err != nil {
		return nil, nil, nil, dto.AgentBindingHealth{}, err
	}
	if version == nil {
		return nil, nil, nil, dto.AgentBindingHealth{}, fmt.Errorf("published version not found")
	}
	snapshot := agentConfigResponseFromSnapshot(ag.ID.String(), version.ConfigSnapshot)
	rows, err := s.bindingRowsForConfig(ctx, ag, snapshot, agentbindings.ScopeDraft, nil, accountID, version.CreatedAt)
	if err != nil {
		return nil, nil, nil, dto.AgentBindingHealth{}, err
	}
	health := s.resolveAgentBindingHealth(ctx, ag, accountID, snapshot, rows)
	return version, snapshot, rows, health, nil
}

func agentConfigRequestFromResponse(config dto.AgentConfigResponse) dto.AgentConfigRequest {
	return dto.AgentConfigRequest{
		SystemPrompt:              config.SystemPrompt,
		ModelProvider:             config.ModelProvider,
		Model:                     config.Model,
		ModelParameters:           config.ModelParameters,
		EnabledSkillIDs:           config.EnabledSkillIDs,
		UseMemory:                 false,
		AgentMemoryEnabled:        config.AgentMemoryEnabled,
		FileUpload:                config.FileUpload,
		HomeTitle:                 config.HomeTitle,
		OpeningStatement:          config.OpeningStatement,
		InputPlaceholder:          config.InputPlaceholder,
		ThemeColor:                config.ThemeColor,
		SuggestedQuestions:        config.SuggestedQuestions,
		KnowledgeDatasetIDs:       config.KnowledgeDatasetIDs,
		KnowledgeBoundByAccountID: config.KnowledgeBoundByAccountID,
		KnowledgeBoundAtUnix:      config.KnowledgeBoundAtUnix,
		KnowledgeRetrievalConfig:  config.KnowledgeRetrievalConfig,
		DatabaseBindings:          config.DatabaseBindings,
		DatabaseBoundByAccountID:  config.DatabaseBoundByAccountID,
		DatabaseBoundAtUnix:       config.DatabaseBoundAtUnix,
		WorkflowBindings:          config.WorkflowBindings,
		WorkflowBoundByAccountID:  config.WorkflowBoundByAccountID,
		WorkflowBoundAtUnix:       config.WorkflowBoundAtUnix,
		BindingAuthorizations:     config.BindingAuthorizations,
	}
}

func (s *agentsService) loadAuthorizedAgentRuntimeDraft(ctx context.Context, agentID, accountID string, ensureConfig bool, permissionCodes ...model.WorkspacePermissionCode) (*Agent, *AgentsConfig, error) {
	ag, cfg, err := s.loadAgentRuntimeDraft(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	if ag.AgentsType != "AGENT" {
		return nil, nil, fmt.Errorf("agent runtime config is only available for AGENT type")
	}
	if len(permissionCodes) == 0 {
		permissionCodes = agentRuntimeConfigManagePermissionCodes(ag.AgentsType)
	}
	if err := s.ensureCanManageAgent(ctx, ag, accountID, permissionCodes...); err != nil {
		return nil, nil, err
	}
	if ensureConfig {
		cfg, err = s.ensureAgentRuntimeDraftConfig(ctx, ag, cfg)
		if err != nil {
			return nil, nil, err
		}
	}
	return ag, cfg, nil
}

func (s *agentsService) loadAgentRuntimeDraft(ctx context.Context, agentID string) (*Agent, *AgentsConfig, error) {
	ag, err := s.agentsRepo.GetByID(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	var cfg *AgentsConfig
	if ag.AgentsModelConfigID != nil {
		cfg, err = s.agentsRepo.GetAgentsConfigByID(ctx, ag.AgentsModelConfigID.String())
		if err != nil {
			return nil, nil, err
		}
	}
	if cfg == nil {
		cfg, err = s.agentsRepo.GetAgentsConfigByAgentID(ctx, agentID)
		if err != nil {
			return nil, nil, err
		}
	}
	return ag, cfg, nil
}

func (s *agentsService) ensureAgentRuntimeDraftConfig(ctx context.Context, ag *Agent, cfg *AgentsConfig) (*AgentsConfig, error) {
	if ag == nil {
		return nil, fmt.Errorf("agent is required")
	}
	if cfg == nil {
		cfg = &AgentsConfig{AgentsID: ag.ID, PromptType: "simple"}
		if err := s.agentsRepo.CreateAgentsConfig(ctx, cfg); err != nil {
			return nil, err
		}
		ag.AgentsModelConfigID = &cfg.ID
		_ = s.agentsRepo.Update(ctx, ag)
	}
	return cfg, nil
}

func normalizeAgentConfigRequest(req dto.AgentConfigRequest) dto.AgentConfigRequest {
	req.SystemPrompt = strings.TrimSpace(req.SystemPrompt)
	req.ModelProvider = strings.TrimSpace(req.ModelProvider)
	req.Model = strings.TrimSpace(req.Model)
	req.HomeTitle = normalizeAgentHomeTitle(req.HomeTitle)
	req.OpeningStatement = normalizeAgentOpeningStatement(req.OpeningStatement)
	req.InputPlaceholder = normalizeAgentInputPlaceholder(req.InputPlaceholder)
	req.ThemeColor = normalizeAgentThemeColor(req.ThemeColor)
	if req.ModelParameters == nil {
		req.ModelParameters = map[string]interface{}{}
	}
	req.EnabledSkillIDs = normalizeAgentEnabledSkillIDs(req.EnabledSkillIDs)
	req.SuggestedQuestions = normalizeSuggestedQuestions(req.SuggestedQuestions)
	req.KnowledgeDatasetIDs = normalizeStringIDs(req.KnowledgeDatasetIDs)
	if req.KnowledgeRetrievalConfig == nil {
		req.KnowledgeRetrievalConfig = map[string]interface{}{}
	}
	req.DatabaseBindings = normalizeAgentDatabaseBindings(req.DatabaseBindings)
	req.WorkflowBindings = normalizeAgentWorkflowBindings(req.WorkflowBindings)
	return req
}

func applyAgentConfigRequestToDraft(cfg *AgentsConfig, req dto.AgentConfigRequest, actorAccountIDs ...string) (dto.AgentConfigRequest, error) {
	if cfg == nil {
		return dto.AgentConfigRequest{}, fmt.Errorf("agent config is required")
	}
	previousMode := agentRuntimeModeFromConfig(cfg)
	runtimeCfg := normalizeAgentConfigRequest(req)
	if err := validateAgentSystemPromptSource(runtimeCfg.SystemPrompt); err != nil {
		return dto.AgentConfigRequest{}, err
	}
	actorAccountID := ""
	if len(actorAccountIDs) > 0 {
		actorAccountID = strings.TrimSpace(actorAccountIDs[0])
	}
	nowUnix := time.Now().Unix()
	bindingAuthorizations := resolveAgentBindingAuthorizations(previousMode, runtimeCfg, actorAccountID, nowUnix)
	knowledgeBoundByAccountID, knowledgeBoundAtUnix := aggregateAgentBindingAuthorization(
		bindingAuthorizations,
		agentbindings.BindingTypeKnowledgeDataset,
	)
	databaseBoundByAccountID, databaseBoundAtUnix := aggregateAgentBindingAuthorization(
		bindingAuthorizations,
		agentbindings.BindingTypeDatabase,
		agentbindings.BindingTypeDatabaseTable,
	)
	workflowBoundByAccountID, workflowBoundAtUnix := aggregateAgentBindingAuthorization(
		bindingAuthorizations,
		agentbindings.BindingTypeWorkflow,
	)
	cfg.PrePrompt = stringPtr(runtimeCfg.SystemPrompt)
	cfg.ModelProvider = nullableStringPtr(runtimeCfg.ModelProvider)
	cfg.ModelVersionID = nullableStringPtr(runtimeCfg.Model)
	paramsJSON, err := json.Marshal(runtimeCfg.ModelParameters)
	if err != nil {
		return dto.AgentConfigRequest{}, fmt.Errorf("failed to marshal model parameters: %w", err)
	}
	params := string(paramsJSON)
	cfg.Configs = &params
	modeJSON, err := json.Marshal(dto.AgentRuntimeModeConfig{
		EnabledSkillIDs:           runtimeCfg.EnabledSkillIDs,
		UseMemory:                 false,
		AgentMemoryEnabled:        runtimeCfg.AgentMemoryEnabled,
		FileUploadEnabled:         runtimeCfg.FileUpload,
		HomeTitle:                 runtimeCfg.HomeTitle,
		OpeningStatement:          runtimeCfg.OpeningStatement,
		InputPlaceholder:          runtimeCfg.InputPlaceholder,
		ThemeColor:                runtimeCfg.ThemeColor,
		SuggestedQuestions:        runtimeCfg.SuggestedQuestions,
		KnowledgeDatasetIDs:       runtimeCfg.KnowledgeDatasetIDs,
		KnowledgeBoundByAccountID: knowledgeBoundByAccountID,
		KnowledgeBoundAtUnix:      knowledgeBoundAtUnix,
		KnowledgeRetrievalConfig:  runtimeCfg.KnowledgeRetrievalConfig,
		DatabaseBindings:          runtimeCfg.DatabaseBindings,
		DatabaseBoundByAccountID:  databaseBoundByAccountID,
		DatabaseBoundAtUnix:       databaseBoundAtUnix,
		WorkflowBindings:          runtimeCfg.WorkflowBindings,
		WorkflowBoundByAccountID:  workflowBoundByAccountID,
		WorkflowBoundAtUnix:       workflowBoundAtUnix,
		BindingAuthorizations:     bindingAuthorizations,
	})
	if err != nil {
		return dto.AgentConfigRequest{}, fmt.Errorf("failed to marshal agent mode: %w", err)
	}
	mode := string(modeJSON)
	cfg.AgentMode = &mode
	return runtimeCfg, nil
}

func agentConfigResponse(agentID string, cfg *AgentsConfig) *dto.AgentConfigResponse {
	params := map[string]interface{}{}
	if cfg != nil && cfg.Configs != nil && strings.TrimSpace(*cfg.Configs) != "" {
		_ = json.Unmarshal([]byte(*cfg.Configs), &params)
	}
	mode := agentRuntimeModeFromConfig(cfg)
	resp := &dto.AgentConfigResponse{
		AgentID:                   agentID,
		ModelParameters:           params,
		EnabledSkillIDs:           normalizeAgentEnabledSkillIDs(mode.EnabledSkillIDs),
		UseMemory:                 false,
		AgentMemoryEnabled:        mode.AgentMemoryEnabled,
		AgentMemorySlots:          normalizeAgentMemorySlotConfigs(mode.AgentMemorySlots),
		FileUpload:                mode.FileUploadEnabled,
		HomeTitle:                 normalizeAgentHomeTitle(mode.HomeTitle),
		OpeningStatement:          normalizeAgentOpeningStatement(mode.OpeningStatement),
		InputPlaceholder:          normalizeAgentInputPlaceholder(mode.InputPlaceholder),
		ThemeColor:                normalizeAgentThemeColor(mode.ThemeColor),
		SuggestedQuestions:        normalizeSuggestedQuestions(mode.SuggestedQuestions),
		KnowledgeDatasetIDs:       normalizeStringIDs(mode.KnowledgeDatasetIDs),
		KnowledgeBoundByAccountID: strings.TrimSpace(mode.KnowledgeBoundByAccountID),
		KnowledgeBoundAtUnix:      mode.KnowledgeBoundAtUnix,
		KnowledgeRetrievalConfig:  copyStringAnyMap(mode.KnowledgeRetrievalConfig),
		DatabaseBindings:          normalizeAgentDatabaseBindings(mode.DatabaseBindings),
		DatabaseBoundByAccountID:  strings.TrimSpace(mode.DatabaseBoundByAccountID),
		DatabaseBoundAtUnix:       mode.DatabaseBoundAtUnix,
		WorkflowBindings:          normalizeAgentWorkflowBindings(mode.WorkflowBindings),
		WorkflowBoundByAccountID:  strings.TrimSpace(mode.WorkflowBoundByAccountID),
		WorkflowBoundAtUnix:       mode.WorkflowBoundAtUnix,
		BindingAuthorizations:     bindingAuthorizationsForRuntimeMode(mode),
	}
	if cfg != nil {
		resp.SystemPrompt = stringPtrValue(cfg.PrePrompt)
		resp.ModelProvider = stringPtrValue(cfg.ModelProvider)
		resp.Model = stringPtrValue(cfg.ModelVersionID)
		resp.SupportsVision = inferAgentModelSupportsVision(resp.Model)
		resp.UpdatedAt = cfg.UpdatedAt.Unix()
	}
	return resp
}

func agentConfigSnapshot(agentID string, cfg *AgentsConfig) map[string]interface{} {
	resp := agentConfigResponse(agentID, cfg)
	return map[string]interface{}{
		"agent_id":                      resp.AgentID,
		"system_prompt":                 resp.SystemPrompt,
		"model_provider":                resp.ModelProvider,
		"model":                         resp.Model,
		"supports_vision":               resp.SupportsVision,
		"model_parameters":              resp.ModelParameters,
		"enabled_skill_ids":             resp.EnabledSkillIDs,
		"use_memory":                    false,
		"agent_memory_enabled":          resp.AgentMemoryEnabled,
		"agent_memory_slots":            normalizeAgentMemorySlotConfigs(resp.AgentMemorySlots),
		"file_upload_enabled":           resp.FileUpload,
		"home_title":                    resp.HomeTitle,
		"opening_statement":             resp.OpeningStatement,
		"input_placeholder":             resp.InputPlaceholder,
		"theme_color":                   resp.ThemeColor,
		"suggested_questions":           resp.SuggestedQuestions,
		"knowledge_dataset_ids":         resp.KnowledgeDatasetIDs,
		"knowledge_bound_by_account_id": resp.KnowledgeBoundByAccountID,
		"knowledge_bound_at_unix":       resp.KnowledgeBoundAtUnix,
		"knowledge_retrieval_config":    resp.KnowledgeRetrievalConfig,
		"database_bindings":             normalizeAgentDatabaseBindings(resp.DatabaseBindings),
		"database_bound_by_account_id":  resp.DatabaseBoundByAccountID,
		"database_bound_at_unix":        resp.DatabaseBoundAtUnix,
		"workflow_bindings":             normalizeAgentWorkflowBindings(resp.WorkflowBindings),
		"workflow_bound_by_account_id":  resp.WorkflowBoundByAccountID,
		"workflow_bound_at_unix":        resp.WorkflowBoundAtUnix,
		"binding_authorizations":        normalizeAgentBindingAuthorizations(resp.BindingAuthorizations),
	}
}

func agentConfigResponseFromSnapshot(agentID string, snapshot map[string]interface{}) *dto.AgentConfigResponse {
	resp := &dto.AgentConfigResponse{
		AgentID:         agentID,
		ModelParameters: map[string]interface{}{},
		EnabledSkillIDs: []string{},
	}
	if snapshot == nil {
		return resp
	}
	resp.BindingRevision = strings.TrimSpace(stringFromSnapshot(snapshot, "binding_revision"))
	resp.SystemPrompt = stringFromSnapshot(snapshot, "system_prompt")
	resp.ModelProvider = stringFromSnapshot(snapshot, "model_provider")
	resp.Model = stringFromSnapshot(snapshot, "model")
	if supportsVision, ok := snapshot["supports_vision"].(bool); ok {
		resp.SupportsVision = supportsVision
	} else {
		resp.SupportsVision = inferAgentModelSupportsVision(resp.Model)
	}
	if params, ok := snapshot["model_parameters"].(map[string]interface{}); ok {
		resp.ModelParameters = params
	}
	resp.EnabledSkillIDs = normalizeAgentEnabledSkillIDs(stringSliceFromSnapshot(snapshot["enabled_skill_ids"]))
	resp.UseMemory = false
	if enabled, ok := snapshot["agent_memory_enabled"].(bool); ok {
		resp.AgentMemoryEnabled = enabled
	}
	resp.AgentMemorySlots = agentMemorySlotConfigsFromSnapshot(snapshot["agent_memory_slots"])
	if fileUpload, ok := snapshot["file_upload_enabled"].(bool); ok {
		resp.FileUpload = fileUpload
	}
	resp.HomeTitle = normalizeAgentHomeTitle(stringFromSnapshot(snapshot, "home_title"))
	resp.OpeningStatement = normalizeAgentOpeningStatement(stringFromSnapshot(snapshot, "opening_statement"))
	resp.InputPlaceholder = normalizeAgentInputPlaceholder(stringFromSnapshot(snapshot, "input_placeholder"))
	resp.ThemeColor = normalizeAgentThemeColor(stringFromSnapshot(snapshot, "theme_color"))
	resp.SuggestedQuestions = normalizeSuggestedQuestions(stringSliceFromSnapshot(snapshot["suggested_questions"]))
	resp.KnowledgeDatasetIDs = normalizeStringIDs(stringSliceFromSnapshot(snapshot["knowledge_dataset_ids"]))
	resp.KnowledgeBoundByAccountID = strings.TrimSpace(stringFromSnapshot(snapshot, "knowledge_bound_by_account_id"))
	resp.KnowledgeBoundAtUnix = int64FromSnapshot(snapshot["knowledge_bound_at_unix"])
	if cfg, ok := snapshot["knowledge_retrieval_config"].(map[string]interface{}); ok {
		resp.KnowledgeRetrievalConfig = copyStringAnyMap(cfg)
	}
	resp.DatabaseBindings = agentDatabaseBindingsFromSnapshot(snapshot["database_bindings"])
	resp.DatabaseBoundByAccountID = strings.TrimSpace(stringFromSnapshot(snapshot, "database_bound_by_account_id"))
	resp.DatabaseBoundAtUnix = int64FromSnapshot(snapshot["database_bound_at_unix"])
	resp.WorkflowBindings = agentWorkflowBindingsFromSnapshot(snapshot["workflow_bindings"])
	resp.WorkflowBoundByAccountID = strings.TrimSpace(stringFromSnapshot(snapshot, "workflow_bound_by_account_id"))
	resp.WorkflowBoundAtUnix = int64FromSnapshot(snapshot["workflow_bound_at_unix"])
	resp.BindingAuthorizations = agentBindingAuthorizationsFromSnapshot(snapshot["binding_authorizations"])
	if len(resp.BindingAuthorizations) == 0 {
		resp.BindingAuthorizations = bindingAuthorizationsForRuntimeMode(dto.AgentRuntimeModeConfig{
			KnowledgeDatasetIDs:       resp.KnowledgeDatasetIDs,
			KnowledgeBoundByAccountID: resp.KnowledgeBoundByAccountID,
			KnowledgeBoundAtUnix:      resp.KnowledgeBoundAtUnix,
			DatabaseBindings:          resp.DatabaseBindings,
			DatabaseBoundByAccountID:  resp.DatabaseBoundByAccountID,
			DatabaseBoundAtUnix:       resp.DatabaseBoundAtUnix,
			WorkflowBindings:          resp.WorkflowBindings,
			WorkflowBoundByAccountID:  resp.WorkflowBoundByAccountID,
			WorkflowBoundAtUnix:       resp.WorkflowBoundAtUnix,
		})
	}
	return resp
}

func agentBindingAuthorizationsFromSnapshot(value interface{}) []dto.AgentBindingAuthorization {
	payload, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var authorizations []dto.AgentBindingAuthorization
	if err := json.Unmarshal(payload, &authorizations); err != nil {
		return nil
	}
	return normalizeAgentBindingAuthorizations(authorizations)
}

func stringFromSnapshot(snapshot map[string]interface{}, key string) string {
	if value, ok := snapshot[key].(string); ok {
		return value
	}
	return ""
}

func int64FromSnapshot(value interface{}) int64 {
	switch typed := value.(type) {
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case json.Number:
		if parsed, err := typed.Int64(); err == nil {
			return parsed
		}
	case string:
		if parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 64); err == nil {
			return parsed
		}
	}
	return 0
}

func stringSliceFromSnapshot(value interface{}) []string {
	switch items := value.(type) {
	case []string:
		return append([]string(nil), items...)
	case []interface{}:
		out := make([]string, 0, len(items))
		for _, item := range items {
			if value, ok := item.(string); ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return []string{}
	}
}

func normalizeStringIDs(input []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, raw := range input {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func agentRuntimeModeFromConfig(cfg *AgentsConfig) dto.AgentRuntimeModeConfig {
	mode := dto.AgentRuntimeModeConfig{}
	if cfg == nil || cfg.AgentMode == nil || strings.TrimSpace(*cfg.AgentMode) == "" {
		return mode
	}
	_ = json.Unmarshal([]byte(*cfg.AgentMode), &mode)
	return mode
}

func normalizeAgentEnabledSkillIDs(input []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, raw := range input {
		id := strings.ToLower(strings.TrimSpace(raw))
		if id == "" || !skills.IsUserSelectableSystemSkill(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (s *agentsService) validateAgentEnabledSkillIDs(ctx context.Context, workspaceID, accountID string, skillIDs []string) error {
	normalized := normalizeAgentEnabledSkillIDs(skillIDs)
	if len(normalized) == 0 {
		return nil
	}
	candidates, err := s.listAgentSkillCandidatesForWorkspace(ctx, workspaceID, accountID)
	if err != nil {
		return fmt.Errorf("validate agent skills: %w", err)
	}
	available := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		available[strings.ToLower(strings.TrimSpace(candidate.SkillID))] = struct{}{}
	}
	for _, id := range normalized {
		if _, ok := available[strings.ToLower(strings.TrimSpace(id))]; !ok {
			return fmt.Errorf("skill %s is not available for agent", id)
		}
	}
	return nil
}

func normalizeAgentHomeTitle(input string) string {
	const maxHomeTitleRunes = 150
	title := strings.TrimSpace(input)
	if title == "" {
		return "title"
	}
	runes := []rune(title)
	if len(runes) > maxHomeTitleRunes {
		return string(runes[:maxHomeTitleRunes])
	}
	return title
}

func normalizeAgentOpeningStatement(input string) string {
	return strings.TrimSpace(strings.ReplaceAll(input, "\r\n", "\n"))
}

func normalizeAgentInputPlaceholder(input string) string {
	const maxPlaceholderRunes = 80
	placeholder := strings.TrimSpace(input)
	if placeholder == "" {
		return "输入指令..."
	}
	runes := []rune(placeholder)
	if len(runes) > maxPlaceholderRunes {
		return string(runes[:maxPlaceholderRunes])
	}
	return placeholder
}

func normalizeAgentThemeColor(input string) string {
	color := strings.TrimSpace(input)
	switch color {
	case "default", "blue", "emerald", "violet", "rose", "amber", "slate":
		return color
	default:
		return "default"
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func copyStringAnyMap(input map[string]interface{}) map[string]interface{} {
	if input == nil {
		return map[string]interface{}{}
	}
	out := make(map[string]interface{}, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func stringPtr(value string) *string {
	return &value
}

func nullableStringPtr(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func stringPtrValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
