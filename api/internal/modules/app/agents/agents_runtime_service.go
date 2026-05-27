package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/suggestedquestions"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

var (
	errAgentWebAppOffline      = errors.New("web app is offline")
	errAgentWebAppNotPublished = errors.New("agent web app has no published version")
)

func (s *agentsService) GetAgentConfig(ctx context.Context, agentID, accountID string) (*dto.AgentConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	return resp, nil
}

func (s *agentsService) GetAgentDraftRuntimeConfig(ctx context.Context, agentID, accountID string) (*dto.AgentDraftRuntimeConfigResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
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
	if _, err := applyAgentConfigRequestToDraft(cfg, req); err != nil {
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
	snapshot["agent_memory_slots"] = s.enabledAgentMemorySlotsForSnapshot(ctx, ag.ID)
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
	if err := s.agentsRepo.CreateAgentPublishedVersion(ctx, version); err != nil {
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
		SystemPrompt:             snapshot.SystemPrompt,
		ModelProvider:            snapshot.ModelProvider,
		Model:                    snapshot.Model,
		ModelParameters:          snapshot.ModelParameters,
		EnabledSkillIDs:          snapshot.EnabledSkillIDs,
		UseMemory:                false,
		AgentMemoryEnabled:       snapshot.AgentMemoryEnabled,
		FileUpload:               snapshot.FileUpload,
		HomeTitle:                snapshot.HomeTitle,
		InputPlaceholder:         snapshot.InputPlaceholder,
		ThemeColor:               snapshot.ThemeColor,
		SuggestedQuestions:       snapshot.SuggestedQuestions,
		KnowledgeDatasetIDs:      snapshot.KnowledgeDatasetIDs,
		KnowledgeRetrievalConfig: snapshot.KnowledgeRetrievalConfig,
	})
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
		_, err = s.agentMemoryService.ReplaceSlots(ctx, ag.ID, actorID, agentMemoryReplaceRequestFromConfig(snapshot.AgentMemorySlots))
		if err != nil {
			return nil, err
		}
	}
	resp := agentConfigResponse(ag.ID.String(), cfg)
	resp.AgentMemorySlots = s.agentMemorySlotsForDraft(ctx, ag.ID)
	resp.EnabledSkillIDs = applied.EnabledSkillIDs
	return resp, nil
}

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
	updated, err := s.agentMemoryService.ReplaceSlots(ctx, ag.ID, actorID, agentMemoryReplaceRequestFromConfig(slots))
	if err != nil {
		return nil, err
	}
	return agentMemorySlotConfigsFromResponses(updated), nil
}

func (s *agentsService) GetPublishedAgentWebAppConfig(ctx context.Context, webAppID string) (*dto.AgentWebAppRuntimeConfigResponse, error) {
	ag, err := s.agentsRepo.GetByWebAppID(ctx, webAppID)
	if err != nil {
		return nil, err
	}
	if NormalizeAgentWebAppStatus(ag.WebAppStatus) != AgentWebAppStatusActive {
		return nil, errAgentWebAppOffline
	}
	if ag.AgentsType != "AGENT" {
		return nil, fmt.Errorf("web app is not an AGENT runtime")
	}
	version, err := s.agentsRepo.GetLatestAgentPublishedVersion(ctx, ag.ID.String())
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, errAgentWebAppNotPublished
	}
	workspaceID := ag.TenantID.String()
	organizationID := workspaceID
	if s.enterpriseService != nil {
		org, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve web app organization: %w", err)
		}
		if org != nil {
			organizationID = org.ID
		}
	}
	cfg := agentConfigResponseFromSnapshot(ag.ID.String(), version.ConfigSnapshot)
	iconURL := ""
	if ag.IconType != nil && *ag.IconType == "image" && ag.Icon != nil && *ag.Icon != "" && s.fileService != nil {
		if fileURL, err := s.fileService.GetFileURL(ctx, *ag.Icon); err == nil {
			iconURL = fileURL
		}
	}
	return &dto.AgentWebAppRuntimeConfigResponse{
		AgentID:        ag.ID.String(),
		WebAppID:       ag.WebAppID.String(),
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		AgentType:      ag.AgentsType,
		Name:           ag.Name,
		Description:    ag.Description,
		Icon:           stringPtrValue(ag.Icon),
		IconType:       stringPtrValue(ag.IconType),
		IconURL:        iconURL,
		Version:        version.Version,
		VersionUUID:    version.VersionUUID.String(),
		Config:         *cfg,
	}, nil
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
	return req
}

func applyAgentConfigRequestToDraft(cfg *AgentsConfig, req dto.AgentConfigRequest) (dto.AgentConfigRequest, error) {
	if cfg == nil {
		return dto.AgentConfigRequest{}, fmt.Errorf("agent config is required")
	}
	runtimeCfg := normalizeAgentConfigRequest(req)
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
		EnabledSkillIDs:          runtimeCfg.EnabledSkillIDs,
		UseMemory:                false,
		AgentMemoryEnabled:       runtimeCfg.AgentMemoryEnabled,
		FileUploadEnabled:        runtimeCfg.FileUpload,
		HomeTitle:                runtimeCfg.HomeTitle,
		InputPlaceholder:         runtimeCfg.InputPlaceholder,
		ThemeColor:               runtimeCfg.ThemeColor,
		SuggestedQuestions:       runtimeCfg.SuggestedQuestions,
		KnowledgeDatasetIDs:      runtimeCfg.KnowledgeDatasetIDs,
		KnowledgeRetrievalConfig: runtimeCfg.KnowledgeRetrievalConfig,
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
	mode := dto.AgentRuntimeModeConfig{}
	if cfg != nil && cfg.AgentMode != nil && strings.TrimSpace(*cfg.AgentMode) != "" {
		_ = json.Unmarshal([]byte(*cfg.AgentMode), &mode)
	}
	resp := &dto.AgentConfigResponse{
		AgentID:                  agentID,
		ModelParameters:          params,
		EnabledSkillIDs:          normalizeAgentEnabledSkillIDs(mode.EnabledSkillIDs),
		UseMemory:                false,
		AgentMemoryEnabled:       mode.AgentMemoryEnabled,
		AgentMemorySlots:         normalizeAgentMemorySlotConfigs(mode.AgentMemorySlots),
		FileUpload:               mode.FileUploadEnabled,
		HomeTitle:                normalizeAgentHomeTitle(mode.HomeTitle),
		InputPlaceholder:         normalizeAgentInputPlaceholder(mode.InputPlaceholder),
		ThemeColor:               normalizeAgentThemeColor(mode.ThemeColor),
		SuggestedQuestions:       normalizeSuggestedQuestions(mode.SuggestedQuestions),
		KnowledgeDatasetIDs:      normalizeStringIDs(mode.KnowledgeDatasetIDs),
		KnowledgeRetrievalConfig: copyStringAnyMap(mode.KnowledgeRetrievalConfig),
	}
	if cfg != nil {
		resp.SystemPrompt = stringPtrValue(cfg.PrePrompt)
		resp.ModelProvider = stringPtrValue(cfg.ModelProvider)
		resp.Model = stringPtrValue(cfg.ModelVersionID)
		resp.UpdatedAt = cfg.UpdatedAt.Unix()
	}
	return resp
}

func agentConfigSnapshot(agentID string, cfg *AgentsConfig) map[string]interface{} {
	resp := agentConfigResponse(agentID, cfg)
	return map[string]interface{}{
		"agent_id":                   resp.AgentID,
		"system_prompt":              resp.SystemPrompt,
		"model_provider":             resp.ModelProvider,
		"model":                      resp.Model,
		"model_parameters":           resp.ModelParameters,
		"enabled_skill_ids":          resp.EnabledSkillIDs,
		"use_memory":                 false,
		"agent_memory_enabled":       resp.AgentMemoryEnabled,
		"agent_memory_slots":         normalizeAgentMemorySlotConfigs(resp.AgentMemorySlots),
		"file_upload_enabled":        resp.FileUpload,
		"home_title":                 resp.HomeTitle,
		"input_placeholder":          resp.InputPlaceholder,
		"theme_color":                resp.ThemeColor,
		"suggested_questions":        resp.SuggestedQuestions,
		"knowledge_dataset_ids":      resp.KnowledgeDatasetIDs,
		"knowledge_retrieval_config": resp.KnowledgeRetrievalConfig,
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
	if cfg, ok := snapshot["knowledge_retrieval_config"].(map[string]interface{}); ok {
		resp.KnowledgeRetrievalConfig = copyStringAnyMap(cfg)
	}
	return resp
}

func (s *agentsService) agentMemorySlotsForDraft(ctx context.Context, agentID uuid.UUID) []dto.AgentMemorySlotConfig {
	if s.agentMemoryService == nil || agentID == uuid.Nil {
		return []dto.AgentMemorySlotConfig{}
	}
	slots, err := s.agentMemoryService.ListSlots(ctx, agentID)
	if err != nil {
		return []dto.AgentMemorySlotConfig{}
	}
	return agentMemorySlotConfigsFromResponses(slots)
}

func (s *agentsService) enabledAgentMemorySlotsForSnapshot(ctx context.Context, agentID uuid.UUID) []dto.AgentMemorySlotConfig {
	slots := s.agentMemorySlotsForDraft(ctx, agentID)
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
		out = append(out, dto.AgentMemorySlotConfig{
			ID:          slot.ID,
			Key:         strings.TrimSpace(slot.Key),
			Description: strings.TrimSpace(slot.Description),
			MaxChars:    slot.MaxChars,
			Enabled:     slot.Enabled,
			SortOrder:   slot.SortOrder,
			CreatedAt:   slot.CreatedAt,
			UpdatedAt:   slot.UpdatedAt,
		})
	}
	return normalizeAgentMemorySlotConfigs(out)
}

func agentMemoryReplaceRequestFromConfig(slots []dto.AgentMemorySlotConfig) agentmemory.ReplaceSlotsRequest {
	normalized := normalizeAgentMemorySlotConfigs(slots)
	req := agentmemory.ReplaceSlotsRequest{Slots: make([]agentmemory.SlotUpsertRequest, 0, len(normalized))}
	for _, slot := range normalized {
		enabled := slot.Enabled
		req.Slots = append(req.Slots, agentmemory.SlotUpsertRequest{
			Key:         slot.Key,
			Description: slot.Description,
			MaxChars:    slot.MaxChars,
			Enabled:     &enabled,
			SortOrder:   slot.SortOrder,
		})
	}
	return req
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
		maxChars := slot.MaxChars
		if maxChars <= 0 {
			maxChars = 1000
		}
		if maxChars > 4000 {
			maxChars = 4000
		}
		sortOrder := slot.SortOrder
		if sortOrder == 0 {
			sortOrder = i
		}
		out = append(out, dto.AgentMemorySlotConfig{
			ID:          strings.TrimSpace(slot.ID),
			Key:         key,
			Description: strings.TrimSpace(slot.Description),
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

func (s *agentsService) GenerateAgentSuggestedQuestions(ctx context.Context, agentID, accountID string, req *dto.GenerateAgentSuggestedQuestionsRequest) (*dto.GenerateSuggestedQuestionsResponse, error) {
	if req == nil {
		req = &dto.GenerateAgentSuggestedQuestionsRequest{}
	}
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	if s.llmClient == nil {
		return nil, fmt.Errorf("llm client is not configured")
	}

	workspaceID := ag.TenantID.String()
	organizationID := workspaceID
	if s.enterpriseService != nil {
		if org, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID); err == nil && org != nil && org.ID != "" {
			organizationID = org.ID
		}
	}

	cfgResp := agentConfigResponse(ag.ID.String(), cfg)
	provider, model, err := s.resolveAgentSuggestedQuestionsModel(ctx, organizationID, firstNonEmpty(req.Provider, cfgResp.ModelProvider), firstNonEmpty(req.Model, cfgResp.Model))
	if err != nil {
		return nil, err
	}

	systemPrompt := strings.TrimSpace(req.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = cfgResp.SystemPrompt
	}
	homeTitle := normalizeAgentHomeTitle(firstNonEmpty(req.HomeTitle, cfgResp.HomeTitle))

	capabilities := make([]suggestedquestions.CapabilitySummary, 0, len(req.Skills)+len(req.KnowledgeRefs))
	for _, skill := range req.Skills {
		title := strings.TrimSpace(firstNonEmpty(skill.Name, skill.ID))
		if title == "" {
			continue
		}
		capabilities = append(capabilities, suggestedquestions.CapabilitySummary{
			Type:       "skill",
			Title:      cleanAgentContextText(title, 80),
			Dependency: cleanAgentContextText(skill.Description, 160),
		})
	}
	for _, ref := range req.KnowledgeRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		capabilities = append(capabilities, suggestedquestions.CapabilitySummary{
			Type:  "knowledge_ref",
			Title: cleanAgentContextText(ref, 120),
		})
	}
	if len(capabilities) > 12 {
		capabilities = capabilities[:12]
	}

	generator := suggestedquestions.NewGenerator(s.llmClient)
	result, err := generator.Generate(ctx, suggestedquestions.GenerateRequest{
		Context: suggestedquestions.WorkflowContext{
			Locale:            req.Locale,
			AgentName:         ag.Name,
			AgentDescription:  cleanAgentContextText(ag.Description, 300),
			WorkflowType:      "AGENT",
			OpeningStatement:  homeTitle,
			ExistingQuestions: normalizeSuggestedQuestions(req.ExistingQuestions),
			LLMPrompts: []suggestedquestions.PromptSummary{{
				NodeTitle: "System prompt",
				Role:      "system",
				Text:      cleanAgentContextText(systemPrompt, 1200),
				Model:     model,
			}},
			Capabilities: capabilities,
		},
		Count:          req.Count,
		Provider:       provider,
		Model:          model,
		AgentID:        agentID,
		WorkspaceID:    workspaceID,
		OrganizationID: organizationID,
		AccountID:      accountID,
		AppType:        "agent",
	})
	if err != nil {
		return nil, err
	}

	questions := make([]dto.SuggestedQuestionCandidate, 0, len(result.Questions))
	for _, question := range result.Questions {
		questions = append(questions, dto.SuggestedQuestionCandidate{
			Text:   question.Text,
			Reason: question.Reason,
		})
	}

	return &dto.GenerateSuggestedQuestionsResponse{
		Questions: questions,
		Warnings:  result.Warnings,
		Provider:  result.Provider,
		Model:     result.Model,
	}, nil
}

func (s *agentsService) resolveAgentSuggestedQuestionsModel(ctx context.Context, organizationID, explicitProvider, explicitModel string) (string, string, error) {
	provider := strings.TrimSpace(explicitProvider)
	model := strings.TrimSpace(explicitModel)
	if model != "" {
		return provider, model, nil
	}
	if s.defaultModelResolver == nil || strings.TrimSpace(organizationID) == "" {
		return "", "", suggestedquestions.ErrModelNotConfigured
	}
	resolved, err := s.defaultModelResolver.ResolveModelType(ctx, organizationID, nil, nil, sharedmodel.ModelTypeLLM)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve default LLM model: %w", err)
	}
	if resolved == nil || strings.TrimSpace(resolved.Model) == "" {
		return "", "", suggestedquestions.ErrModelNotConfigured
	}
	return strings.TrimSpace(resolved.Provider), strings.TrimSpace(resolved.Model), nil
}

func isAgentSuggestedQuestionsConfigurationError(err error) bool {
	return errors.Is(err, suggestedquestions.ErrModelNotConfigured)
}

func isAgentSuggestedQuestionsModelOutputError(err error) bool {
	return errors.Is(err, suggestedquestions.ErrModelOutputInvalid)
}

func stringFromSnapshot(snapshot map[string]interface{}, key string) string {
	if value, ok := snapshot[key].(string); ok {
		return value
	}
	return ""
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

func normalizeSuggestedQuestions(input []string) []string {
	out := make([]string, 0, len(input))
	for _, raw := range input {
		item := strings.TrimSpace(raw)
		if item == "" {
			continue
		}
		if len([]rune(item)) > 200 {
			runes := []rune(item)
			item = string(runes[:200])
		}
		out = append(out, item)
		if len(out) >= 6 {
			break
		}
	}
	return out
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

func cleanAgentContextText(input string, maxRunes int) string {
	text := strings.TrimSpace(input)
	if maxRunes <= 0 {
		return text
	}
	runes := []rune(text)
	if len(runes) > maxRunes {
		return string(runes[:maxRunes])
	}
	return text
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
