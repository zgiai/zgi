package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	errAgentWebAppOffline      = errors.New("web app is offline")
	errAgentWebAppNotPublished = errors.New("agent web app has no published version")
	errAgentPromptTooLong      = errors.New("agent system prompt is too long")
)

func (s *agentsService) GetAgentConfig(ctx context.Context, agentID, accountID string) (*dto.AgentConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
	resp.WorkflowBindings = s.hydrateAgentWorkflowBindingRuntimeInputs(ctx, ag.TenantID.String(), resp.WorkflowBindings)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
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
	return &dto.AgentDraftRuntimeConfigResponse{
		AgentID:     ag.ID.String(),
		WorkspaceID: ag.TenantID.String(),
		Config:      *resp,
	}, nil
}

func (s *agentsService) UpdateAgentConfig(ctx context.Context, agentID, accountID string, req dto.AgentConfigRequest) (*dto.AgentConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	runtimeReq := normalizeAgentConfigRequest(req)
	runtimeReq.WorkflowBindings = s.hydrateAgentWorkflowBindingTypes(ctx, ag.TenantID.String(), runtimeReq.WorkflowBindings)
	if err := s.validateAgentEnabledSkillIDs(ctx, ag.TenantID.String(), accountID, runtimeReq.EnabledSkillIDs); err != nil {
		return nil, err
	}
	if err := s.validateAgentBindingGrantChanges(ctx, ag, cfg, accountID, runtimeReq); err != nil {
		return nil, err
	}
	if _, err := applyAgentConfigRequestToDraft(cfg, runtimeReq, accountID); err != nil {
		return nil, err
	}
	if uid, err := uuid.Parse(accountID); err == nil {
		cfg.UpdatedBy = &uid
	}
	if err := s.agentsRepo.UpdateAgentsConfig(ctx, cfg); err != nil {
		return nil, err
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	return resp, nil
}

func (s *agentsService) PublishAgent(ctx context.Context, agentID, accountID string, req dto.PublishAgentRequest) (*dto.PublishAgentResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	snapshot := agentConfigSnapshot(ag.ID.String(), cfg)
	snapshot["supports_vision"] = s.resolveAgentModelSupportsVision(
		ctx,
		s.organizationIDForAgentWorkspace(ctx, ag.TenantID.String()),
		stringFromSnapshot(snapshot, "model_provider"),
		stringFromSnapshot(snapshot, "model"),
	)
	if err := validateAgentSystemPromptSource(stringFromSnapshot(snapshot, "system_prompt")); err != nil {
		return nil, err
	}
	currentMemorySlots, err := s.loadAgentMemorySlotsForDraft(ctx, ag.ID)
	if err != nil {
		return nil, fmt.Errorf("load agent memory slots for publish: %w", err)
	}
	snapshotMemorySlots := []dto.AgentMemorySlotConfig{}
	if enabled, _ := snapshot["agent_memory_enabled"].(bool); enabled {
		snapshotMemorySlots = agentMemorySnapshotSlots(enabledAgentMemorySlots(currentMemorySlots))
	}
	snapshot["agent_memory_slots"] = snapshotMemorySlots
	now := time.Now()
	versionUUID := uuid.New()
	version := &AgentPublishedVersion{
		AgentID:        ag.ID,
		WorkspaceID:    ag.TenantID,
		Version:        now.Format("20060102150405"),
		VersionUUID:    versionUUID,
		ConfigSnapshot: snapshot,
		Description:    strings.TrimSpace(req.Description),
		CreatedAt:      now,
	}
	if uid, err := uuid.Parse(accountID); err == nil {
		version.CreatedBy = &uid
	}
	if err := s.createAgentPublishedVersion(ctx, version, ag.ID, currentMemorySlots); err != nil {
		return nil, err
	}
	return &dto.PublishAgentResponse{
		AgentID:     ag.ID.String(),
		VersionUUID: versionUUID.String(),
		Version:     version.Version,
		WebAppID:    ag.WebAppID.String(),
		PublishedAt: now.Unix(),
	}, nil
}

func (s *agentsService) createAgentPublishedVersion(ctx context.Context, version *AgentPublishedVersion, agentID uuid.UUID, currentMemorySlots []dto.AgentMemorySlotConfig) error {
	if version == nil {
		return fmt.Errorf("agent published version is required")
	}
	if s.db == nil {
		return fmt.Errorf("database is required")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(version).Error; err != nil {
			return fmt.Errorf("failed to create agent published version: %w", err)
		}
		if s.agentMemoryService == nil {
			return nil
		}
		memoryService := agentmemory.NewService(tx)
		if err := memoryService.ClearValuesNotInKeys(ctx, agentID, agentMemoryKeys(currentMemorySlots)); err != nil {
			return fmt.Errorf("clear removed agent memory values: %w", err)
		}
		return nil
	})
}

func (s *agentsService) ListAgentPublishedVersions(ctx context.Context, agentID, accountID string, page, limit int) (*dto.AgentPublishedVersionsResponse, error) {
	_, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
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

func (s *agentsService) RollbackAgentPublishedVersion(ctx context.Context, agentID, accountID string, req dto.RollbackAgentPublishedVersionRequest) (*dto.AgentConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	versionID := strings.TrimSpace(req.VersionID)
	if versionID == "" {
		return nil, fmt.Errorf("version id is required")
	}
	version, err := s.agentsRepo.GetAgentPublishedVersionByID(ctx, agentID, versionID)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("published version not found")
	}
	snapshot := agentConfigResponseFromSnapshot(agentID, version.ConfigSnapshot)
	applied, err := applyAgentConfigRequestToDraft(cfg, dto.AgentConfigRequest{
		SystemPrompt:              snapshot.SystemPrompt,
		ModelProvider:             snapshot.ModelProvider,
		Model:                     snapshot.Model,
		ModelParameters:           snapshot.ModelParameters,
		EnabledSkillIDs:           snapshot.EnabledSkillIDs,
		UseMemory:                 false,
		AgentMemoryEnabled:        snapshot.AgentMemoryEnabled,
		FileUpload:                snapshot.FileUpload,
		HomeTitle:                 snapshot.HomeTitle,
		InputPlaceholder:          snapshot.InputPlaceholder,
		ThemeColor:                snapshot.ThemeColor,
		SuggestedQuestions:        snapshot.SuggestedQuestions,
		KnowledgeDatasetIDs:       snapshot.KnowledgeDatasetIDs,
		KnowledgeBoundByAccountID: snapshot.KnowledgeBoundByAccountID,
		KnowledgeBoundAtUnix:      snapshot.KnowledgeBoundAtUnix,
		KnowledgeRetrievalConfig:  snapshot.KnowledgeRetrievalConfig,
		DatabaseBindings:          snapshot.DatabaseBindings,
		DatabaseBoundByAccountID:  snapshot.DatabaseBoundByAccountID,
		DatabaseBoundAtUnix:       snapshot.DatabaseBoundAtUnix,
		WorkflowBindings:          snapshot.WorkflowBindings,
		WorkflowBoundByAccountID:  snapshot.WorkflowBoundByAccountID,
		WorkflowBoundAtUnix:       snapshot.WorkflowBoundAtUnix,
	}, accountID)
	if err != nil {
		return nil, err
	}
	if uid, err := uuid.Parse(accountID); err == nil {
		cfg.UpdatedBy = &uid
	}
	if err := s.agentsRepo.UpdateAgentsConfig(ctx, cfg); err != nil {
		return nil, err
	}
	if s.agentMemoryService != nil {
		actorID, _ := uuid.Parse(accountID)
		_, err = s.agentMemoryService.ReplaceSlots(ctx, ag.ID, actorID, agentMemoryReplaceRequestFromConfig(snapshot.AgentMemorySlots, false))
		if err != nil {
			return nil, err
		}
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	resp.EnabledSkillIDs = applied.EnabledSkillIDs
	return resp, nil
}

func (s *agentsService) loadAuthorizedAgentRuntimeDraft(ctx context.Context, agentID, accountID string, ensureConfig bool) (*Agent, *AgentsConfig, error) {
	ag, cfg, err := s.loadAgentRuntimeDraft(ctx, agentID)
	if err != nil {
		return nil, nil, err
	}
	if ag.AgentsType != "AGENT" {
		return nil, nil, fmt.Errorf("agent runtime config is only available for AGENT type")
	}
	if err := s.ensureCanManageAgent(ctx, ag, accountID); err != nil {
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

func (s *agentsService) ensureAgentEditor(ctx context.Context, accountID string) error {
	if strings.TrimSpace(accountID) == "" {
		return fmt.Errorf("account id is required")
	}
	if s.accountService == nil {
		return nil
	}
	ok, err := s.accountService.IsEditor(ctx, accountID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("permission denied")
	}
	return nil
}

func normalizeAgentConfigRequest(req dto.AgentConfigRequest) dto.AgentConfigRequest {
	req.SystemPrompt = strings.TrimSpace(req.SystemPrompt)
	req.ModelProvider = strings.TrimSpace(req.ModelProvider)
	req.Model = strings.TrimSpace(req.Model)
	req.HomeTitle = normalizeAgentHomeTitle(req.HomeTitle)
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
	knowledgeBoundByAccountID, knowledgeBoundAtUnix := bindingGrantForStringIDs(previousMode.KnowledgeDatasetIDs, previousMode.KnowledgeBoundByAccountID, previousMode.KnowledgeBoundAtUnix, runtimeCfg.KnowledgeDatasetIDs, actorAccountID, nowUnix)
	databaseBoundByAccountID, databaseBoundAtUnix := bindingGrantForDatabaseBindings(previousMode.DatabaseBindings, previousMode.DatabaseBoundByAccountID, previousMode.DatabaseBoundAtUnix, runtimeCfg.DatabaseBindings, actorAccountID, nowUnix)
	workflowBoundByAccountID, workflowBoundAtUnix := bindingGrantForWorkflowBindings(previousMode.WorkflowBindings, previousMode.WorkflowBoundByAccountID, previousMode.WorkflowBoundAtUnix, runtimeCfg.WorkflowBindings, actorAccountID, nowUnix)
	if strings.TrimSpace(runtimeCfg.KnowledgeBoundByAccountID) != "" && runtimeCfg.KnowledgeBoundAtUnix > 0 {
		knowledgeBoundByAccountID = strings.TrimSpace(runtimeCfg.KnowledgeBoundByAccountID)
		knowledgeBoundAtUnix = runtimeCfg.KnowledgeBoundAtUnix
	}
	if strings.TrimSpace(runtimeCfg.DatabaseBoundByAccountID) != "" && runtimeCfg.DatabaseBoundAtUnix > 0 {
		databaseBoundByAccountID = strings.TrimSpace(runtimeCfg.DatabaseBoundByAccountID)
		databaseBoundAtUnix = runtimeCfg.DatabaseBoundAtUnix
	}
	if strings.TrimSpace(runtimeCfg.WorkflowBoundByAccountID) != "" && runtimeCfg.WorkflowBoundAtUnix > 0 {
		workflowBoundByAccountID = strings.TrimSpace(runtimeCfg.WorkflowBoundByAccountID)
		workflowBoundAtUnix = runtimeCfg.WorkflowBoundAtUnix
	}
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
	return resp
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
		if id == "" || skills.IsHiddenSystemSkill(id) {
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
	const maxHomeTitleRunes = 40
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
