package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/agentmemory"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/suggestedquestions"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	sharedmodel "github.com/zgiai/zgi/api/internal/modules/shared/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
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

func (s *agentsService) validateAgentBindingGrantChanges(ctx context.Context, ag *Agent, cfg *AgentsConfig, accountID string, req dto.AgentConfigRequest) error {
	if ag == nil {
		return fmt.Errorf("agent is required")
	}
	previous := agentRuntimeModeFromConfig(cfg)
	workspaceID := ag.TenantID.String()
	organizationID := workspaceID
	if s.enterpriseService != nil {
		org, err := s.enterpriseService.GetOrganizationByWorkspaceID(ctx, workspaceID)
		if err != nil {
			return fmt.Errorf("resolve agent organization: %w", err)
		}
		if org != nil && strings.TrimSpace(org.ID) != "" {
			organizationID = strings.TrimSpace(org.ID)
		}
	}
	if bindingGrantNeedsRefresh(previous.KnowledgeDatasetIDs, previous.KnowledgeBoundByAccountID, previous.KnowledgeBoundAtUnix, req.KnowledgeDatasetIDs) {
		if err := s.validateKnowledgeBindingGrant(ctx, organizationID, workspaceID, accountID, req.KnowledgeDatasetIDs); err != nil {
			return err
		}
	}
	if databaseBindingGrantNeedsRefresh(previous.DatabaseBindings, previous.DatabaseBoundByAccountID, previous.DatabaseBoundAtUnix, req.DatabaseBindings) {
		if err := s.validateDatabaseBindingGrant(ctx, organizationID, accountID, req.DatabaseBindings); err != nil {
			return err
		}
	}
	if workflowBindingGrantNeedsRefresh(previous.WorkflowBindings, previous.WorkflowBoundByAccountID, previous.WorkflowBoundAtUnix, req.WorkflowBindings) {
		if err := s.validateWorkflowBindingGrant(ctx, workspaceID, req.WorkflowBindings); err != nil {
			return err
		}
	}
	return nil
}

func (s *agentsService) validateKnowledgeBindingGrant(ctx context.Context, organizationID, workspaceID, accountID string, datasetIDs []string) error {
	if len(normalizeStringIDs(datasetIDs)) == 0 {
		return nil
	}
	if s.knowledgeRetrievalService == nil {
		return fmt.Errorf("knowledge binding validation service is not configured")
	}
	scope := datasetservice.KnowledgeScope{
		OrganizationID: organizationID,
		WorkspaceID:    workspaceID,
		AccountID:      accountID,
	}
	if err := s.knowledgeRetrievalService.ValidateAccessibleDatasets(ctx, scope, datasetIDs); err != nil {
		return fmt.Errorf("validate knowledge binding: %w", err)
	}
	return nil
}

func (s *agentsService) validateDatabaseBindingGrant(ctx context.Context, organizationID, accountID string, bindings []dto.AgentDatabaseBinding) error {
	bindings = normalizeAgentDatabaseBindings(bindings)
	if len(bindings) == 0 {
		return nil
	}
	if s.dataSourceService == nil || s.enterpriseService == nil {
		return fmt.Errorf("database binding validation service is not configured")
	}
	for _, binding := range bindings {
		dataSource, err := s.dataSourceService.GetDataSourceByID(ctx, organizationID, binding.DataSourceID, accountID)
		if err != nil {
			return fmt.Errorf("load database %s: %w", binding.DataSourceID, err)
		}
		if dataSource == nil || strings.TrimSpace(dataSource.OrganizationID) != strings.TrimSpace(organizationID) {
			return fmt.Errorf("database %s not found", binding.DataSourceID)
		}
		workspaceID := strings.TrimSpace(organizationID)
		if dataSource.WorkspaceID != nil && strings.TrimSpace(*dataSource.WorkspaceID) != "" {
			workspaceID = strings.TrimSpace(*dataSource.WorkspaceID)
		}
		if err := s.requireDatabaseReadBindingPermission(ctx, organizationID, workspaceID, accountID); err != nil {
			return fmt.Errorf("database %s read binding: %w", binding.DataSourceID, err)
		}
		for _, tableID := range binding.TableIDs {
			table, err := s.dataSourceService.GetTable(ctx, organizationID, binding.DataSourceID, tableID, accountID)
			if err != nil {
				return fmt.Errorf("load table %s: %w", tableID, err)
			}
			if table == nil || strings.TrimSpace(table.DataSourceID) != binding.DataSourceID {
				return fmt.Errorf("table %s not found in database %s", tableID, binding.DataSourceID)
			}
		}
		if len(binding.WritableTableIDs) > 0 {
			if err := s.requireDatabaseWriteBindingPermission(ctx, organizationID, workspaceID, accountID); err != nil {
				return fmt.Errorf("database %s write binding: %w", binding.DataSourceID, err)
			}
		}
	}
	return nil
}

func (s *agentsService) requireDatabaseReadBindingPermission(ctx context.Context, organizationID, workspaceID, accountID string) error {
	hasAIQuery, err := s.enterpriseService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionDatabaseAIQuery)
	if err != nil {
		return err
	}
	if !hasAIQuery {
		return fmt.Errorf("database ai query permission is required")
	}
	hasView, err := s.enterpriseService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionDatabaseView)
	if err != nil {
		return err
	}
	if !hasView {
		return fmt.Errorf("database view permission is required")
	}
	return nil
}

func (s *agentsService) requireDatabaseWriteBindingPermission(ctx context.Context, organizationID, workspaceID, accountID string) error {
	hasWrite, err := s.enterpriseService.CheckWorkspaceOrganizationAnyPermission(ctx, organizationID, workspaceID, accountID, workspacemodel.WorkspacePermissionDatabaseDataEdit, workspacemodel.WorkspacePermissionDatabaseManage)
	if err != nil {
		return err
	}
	if !hasWrite {
		return fmt.Errorf("database data edit or manage permission is required")
	}
	return nil
}

func (s *agentsService) validateWorkflowBindingGrant(ctx context.Context, workspaceID string, bindings []dto.AgentWorkflowBinding) error {
	bindings = normalizeAgentWorkflowBindings(bindings)
	if len(bindings) == 0 {
		return nil
	}
	candidates, err := s.listAgentWorkflowBindingCandidatesForWorkspace(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("load workflow binding candidates: %w", err)
	}
	byBindingID := make(map[string]dto.AgentWorkflowBindingCandidate, len(candidates))
	for _, candidate := range candidates {
		byBindingID[strings.TrimSpace(candidate.BindingID)] = candidate
	}
	for _, binding := range bindings {
		candidate, ok := byBindingID[strings.TrimSpace(binding.BindingID)]
		if !ok || strings.TrimSpace(candidate.AgentID) != strings.TrimSpace(binding.AgentID) {
			return fmt.Errorf("workflow binding %s is not available", binding.BindingID)
		}
		if binding.VersionStrategy == automationaction.WorkflowVersionStrategyPinned {
			if strings.TrimSpace(binding.VersionUUID) == "" {
				return fmt.Errorf("workflow binding %s requires version_uuid", binding.BindingID)
			}
			if strings.TrimSpace(binding.WorkflowID) != strings.TrimSpace(candidate.WorkflowID) || strings.TrimSpace(binding.VersionUUID) != strings.TrimSpace(candidate.VersionUUID) {
				return fmt.Errorf("workflow binding %s pinned version is not available", binding.BindingID)
			}
		}
	}
	return nil
}

func (s *agentsService) hydrateAgentWorkflowBindingTypes(ctx context.Context, workspaceID string, bindings []dto.AgentWorkflowBinding) []dto.AgentWorkflowBinding {
	bindings = normalizeAgentWorkflowBindings(bindings)
	if len(bindings) == 0 {
		return bindings
	}
	candidates, err := s.listAgentWorkflowBindingCandidatesForWorkspace(ctx, workspaceID)
	if err != nil {
		return bindings
	}
	typesByBindingID := make(map[string]string, len(candidates))
	for _, candidate := range candidates {
		if bindingID := strings.TrimSpace(candidate.BindingID); bindingID != "" {
			typesByBindingID[bindingID] = strings.TrimSpace(candidate.AgentType)
		}
	}
	for idx := range bindings {
		if strings.TrimSpace(bindings[idx].AgentType) != "" {
			continue
		}
		if agentType := typesByBindingID[bindings[idx].BindingID]; agentType != "" {
			bindings[idx].AgentType = agentType
		}
	}
	return bindings
}

func (s *agentsService) hydrateAgentWorkflowBindingRuntimeInputs(ctx context.Context, workspaceID string, bindings []dto.AgentWorkflowBinding) []dto.AgentWorkflowBinding {
	bindings = normalizeAgentWorkflowBindings(bindings)
	if len(bindings) == 0 {
		return bindings
	}
	candidates, err := s.listAgentWorkflowBindingCandidatesForWorkspace(ctx, workspaceID)
	if err != nil {
		return bindings
	}
	byBindingID := make(map[string]dto.AgentWorkflowBindingCandidate, len(candidates))
	for _, candidate := range candidates {
		if bindingID := strings.TrimSpace(candidate.BindingID); bindingID != "" {
			byBindingID[bindingID] = candidate
		}
	}
	for idx := range bindings {
		candidate, ok := byBindingID[bindings[idx].BindingID]
		if !ok {
			continue
		}
		if strings.TrimSpace(bindings[idx].AgentType) == "" {
			bindings[idx].AgentType = strings.TrimSpace(candidate.AgentType)
		}
		bindings[idx].StartInputs = cloneWorkflowStartInputs(candidate.StartInputs)
		bindings[idx].RequiredInputs = append([]string(nil), candidate.RequiredInputs...)
		bindings[idx].DefaultInputKey = strings.TrimSpace(candidate.DefaultInputKey)
	}
	return bindings
}

func (s *agentsService) ListAgentWorkflowBindingCandidates(ctx context.Context, agentID, accountID string) (*dto.AgentWorkflowBindingCandidatesResponse, error) {
	ag, _, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	items, err := s.listAgentWorkflowBindingCandidatesForWorkspace(ctx, ag.TenantID.String())
	if err != nil {
		return nil, err
	}
	return &dto.AgentWorkflowBindingCandidatesResponse{Data: items}, nil
}

func (s *agentsService) listAgentWorkflowBindingCandidatesForWorkspace(ctx context.Context, workspaceID string) ([]dto.AgentWorkflowBindingCandidate, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return []dto.AgentWorkflowBindingCandidate{}, nil
	}

	type workflowCandidateRow struct {
		AgentID     string  `gorm:"column:agent_id"`
		WorkflowID  string  `gorm:"column:workflow_id"`
		AgentType   string  `gorm:"column:agent_type"`
		VersionUUID string  `gorm:"column:version_uuid"`
		Version     string  `gorm:"column:version"`
		Graph       string  `gorm:"column:graph"`
		Label       string  `gorm:"column:label"`
		Description string  `gorm:"column:description"`
		Icon        *string `gorm:"column:icon"`
		IconType    *string `gorm:"column:icon_type"`
		UpdatedAt   time.Time
	}

	var rows []workflowCandidateRow
	if err := s.db.WithContext(ctx).
		Table("workflows").
		Select("workflows.agent_id AS agent_id, workflows.id AS workflow_id, agents.agent_type AS agent_type, workflows.version_uuid AS version_uuid, workflows.version AS version, workflows.graph AS graph, agents.name AS label, agents.description AS description, agents.icon AS icon, agents.icon_type AS icon_type, workflows.created_at AS updated_at").
		Joins("JOIN agents ON agents.id = workflows.agent_id").
		Where("workflows.tenant_id = ? AND workflows.version != ?", workspaceID, "draft").
		Where("agents.deleted_at IS NULL AND agents.web_app_status = ?", AgentWebAppStatusActive).
		Where("agents.agent_type IN ?", []string{"WORKFLOW", "CONVERSATIONAL_WORKFLOW"}).
		Order("workflows.agent_id ASC, workflows.created_at DESC").
		Scan(&rows).Error; err != nil {
		return nil, err
	}

	items := make([]dto.AgentWorkflowBindingCandidate, 0, len(rows))
	seen := map[string]struct{}{}
	for _, row := range rows {
		agentID := strings.ToLower(strings.TrimSpace(row.AgentID))
		if agentID == "" {
			continue
		}
		if _, ok := seen[agentID]; ok {
			continue
		}
		seen[agentID] = struct{}{}
		icon := stringPtrValue(row.Icon)
		iconType := stringPtrValue(row.IconType)
		iconURL := ""
		if iconType == "image" && icon != "" && s.fileService != nil {
			if fileURL, err := s.fileService.GetFileURL(ctx, icon); err == nil {
				iconURL = fileURL
			}
		}
		startInputs := workflowStartInputsFromGraph(row.Graph)
		items = append(items, dto.AgentWorkflowBindingCandidate{
			BindingID:       agentID,
			Label:           strings.TrimSpace(row.Label),
			Description:     strings.TrimSpace(row.Description),
			AgentID:         agentID,
			WorkflowID:      strings.ToLower(strings.TrimSpace(row.WorkflowID)),
			AgentType:       strings.TrimSpace(row.AgentType),
			VersionStrategy: automationaction.WorkflowVersionStrategyLatestPublished,
			VersionUUID:     strings.ToLower(strings.TrimSpace(row.VersionUUID)),
			Version:         strings.TrimSpace(row.Version),
			Icon:            icon,
			IconType:        iconType,
			IconURL:         iconURL,
			UpdatedAt:       row.UpdatedAt.Unix(),
			StartInputs:     startInputs,
			RequiredInputs:  requiredWorkflowStartInputNames(startInputs),
			DefaultInputKey: defaultWorkflowStartInputKey(startInputs),
		})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Label != items[j].Label {
			return items[i].Label < items[j].Label
		}
		return items[i].AgentID < items[j].AgentID
	})
	return items, nil
}

func (s *agentsService) PublishAgent(ctx context.Context, agentID, accountID string, req dto.PublishAgentRequest) (*dto.PublishAgentResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, true)
	if err != nil {
		return nil, err
	}
	snapshot := agentConfigSnapshot(ag.ID.String(), cfg)
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
	cfg.WorkflowBindings = s.hydrateAgentWorkflowBindingRuntimeInputs(ctx, workspaceID, cfg.WorkflowBindings)
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

func normalizeAgentDatabaseBindings(input []dto.AgentDatabaseBinding) []dto.AgentDatabaseBinding {
	type bindingTables struct {
		readable map[string]struct{}
		writable map[string]struct{}
	}
	byDataSource := map[string]bindingTables{}
	for _, binding := range input {
		dataSourceID := strings.ToLower(strings.TrimSpace(binding.DataSourceID))
		if dataSourceID == "" {
			continue
		}
		tableIDs := normalizeStringIDs(binding.TableIDs)
		if len(tableIDs) == 0 {
			continue
		}
		tables, ok := byDataSource[dataSourceID]
		if !ok {
			tables = bindingTables{readable: map[string]struct{}{}, writable: map[string]struct{}{}}
		}
		for _, tableID := range tableIDs {
			tables.readable[tableID] = struct{}{}
		}
		for _, tableID := range normalizeStringIDs(binding.WritableTableIDs) {
			if _, ok := tables.readable[tableID]; ok {
				tables.writable[tableID] = struct{}{}
			}
		}
		byDataSource[dataSourceID] = tables
	}
	dataSourceIDs := make([]string, 0, len(byDataSource))
	for dataSourceID := range byDataSource {
		dataSourceIDs = append(dataSourceIDs, dataSourceID)
	}
	sort.Strings(dataSourceIDs)
	out := make([]dto.AgentDatabaseBinding, 0, len(dataSourceIDs))
	for _, dataSourceID := range dataSourceIDs {
		tableIDs := make([]string, 0, len(byDataSource[dataSourceID].readable))
		for tableID := range byDataSource[dataSourceID].readable {
			tableIDs = append(tableIDs, tableID)
		}
		sort.Strings(tableIDs)
		writableTableIDs := make([]string, 0, len(byDataSource[dataSourceID].writable))
		for tableID := range byDataSource[dataSourceID].writable {
			if _, ok := byDataSource[dataSourceID].readable[tableID]; ok {
				writableTableIDs = append(writableTableIDs, tableID)
			}
		}
		sort.Strings(writableTableIDs)
		binding := dto.AgentDatabaseBinding{
			DataSourceID: dataSourceID,
			TableIDs:     tableIDs,
		}
		if len(writableTableIDs) > 0 {
			binding.WritableTableIDs = writableTableIDs
		}
		out = append(out, binding)
	}
	return out
}

func normalizeAgentWorkflowBindings(input []dto.AgentWorkflowBinding) []dto.AgentWorkflowBinding {
	byBindingID := map[string]dto.AgentWorkflowBinding{}
	for _, binding := range input {
		bindingID := strings.ToLower(strings.TrimSpace(binding.BindingID))
		if bindingID == "" {
			continue
		}
		agentID := strings.ToLower(strings.TrimSpace(binding.AgentID))
		workflowID := strings.ToLower(strings.TrimSpace(binding.WorkflowID))
		if agentID == "" || workflowID == "" {
			continue
		}
		versionStrategy := strings.TrimSpace(binding.VersionStrategy)
		if versionStrategy == "" {
			versionStrategy = automationaction.WorkflowVersionStrategyLatestPublished
		}
		if versionStrategy != automationaction.WorkflowVersionStrategyLatestPublished && versionStrategy != automationaction.WorkflowVersionStrategyPinned {
			continue
		}
		versionUUID := strings.ToLower(strings.TrimSpace(binding.VersionUUID))
		if versionStrategy != automationaction.WorkflowVersionStrategyPinned {
			versionUUID = ""
		}
		timeoutSeconds := binding.TimeoutSeconds
		if timeoutSeconds < 0 {
			timeoutSeconds = 0
		}
		byBindingID[bindingID] = dto.AgentWorkflowBinding{
			BindingID:       bindingID,
			Label:           strings.TrimSpace(binding.Label),
			Description:     strings.TrimSpace(binding.Description),
			AgentID:         agentID,
			WorkflowID:      workflowID,
			AgentType:       strings.TrimSpace(binding.AgentType),
			VersionStrategy: versionStrategy,
			VersionUUID:     versionUUID,
			TimeoutSeconds:  timeoutSeconds,
		}
	}
	ids := make([]string, 0, len(byBindingID))
	for id := range byBindingID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]dto.AgentWorkflowBinding, 0, len(ids))
	for _, id := range ids {
		out = append(out, byBindingID[id])
	}
	return out
}

func workflowStartInputsFromGraph(graph string) []dto.AgentWorkflowStartInput {
	graph = strings.TrimSpace(graph)
	if graph == "" {
		return nil
	}
	var payload struct {
		Nodes []struct {
			Data map[string]interface{} `json:"data"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal([]byte(graph), &payload); err != nil {
		return nil
	}
	for _, node := range payload.Nodes {
		if !strings.EqualFold(strings.TrimSpace(stringFromMap(node.Data, "type")), "start") {
			continue
		}
		rawVariables, ok := node.Data["variables"].([]interface{})
		if !ok {
			return nil
		}
		inputs := make([]dto.AgentWorkflowStartInput, 0, len(rawVariables))
		seen := map[string]struct{}{}
		for _, raw := range rawVariables {
			varMap, ok := raw.(map[string]interface{})
			if !ok {
				continue
			}
			variable := strings.TrimSpace(stringFromMap(varMap, "variable"))
			if variable == "" {
				continue
			}
			if _, exists := seen[variable]; exists {
				continue
			}
			seen[variable] = struct{}{}
			inputs = append(inputs, dto.AgentWorkflowStartInput{
				Variable: variable,
				Label:    strings.TrimSpace(stringFromMap(varMap, "label")),
				Type:     strings.TrimSpace(stringFromMap(varMap, "type")),
				Required: boolFromMap(varMap, "required"),
			})
		}
		return inputs
	}
	return nil
}

func requiredWorkflowStartInputNames(inputs []dto.AgentWorkflowStartInput) []string {
	out := make([]string, 0, len(inputs))
	for _, input := range inputs {
		variable := strings.TrimSpace(input.Variable)
		if variable == "" || !input.Required {
			continue
		}
		out = append(out, variable)
	}
	return out
}

func defaultWorkflowStartInputKey(inputs []dto.AgentWorkflowStartInput) string {
	if len(inputs) == 0 {
		return "query"
	}
	required := requiredWorkflowStartInputNames(inputs)
	if len(required) == 1 {
		return required[0]
	}
	for _, input := range inputs {
		if strings.EqualFold(strings.TrimSpace(input.Variable), "query") {
			return "query"
		}
	}
	if len(inputs) == 1 {
		return strings.TrimSpace(inputs[0].Variable)
	}
	return ""
}

func cloneWorkflowStartInputs(inputs []dto.AgentWorkflowStartInput) []dto.AgentWorkflowStartInput {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]dto.AgentWorkflowStartInput, len(inputs))
	copy(out, inputs)
	return out
}

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func boolFromMap(values map[string]interface{}, key string) bool {
	if values == nil {
		return false
	}
	value, ok := values[key]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return strings.EqualFold(strings.TrimSpace(typed), "true")
	default:
		return false
	}
}

func agentRuntimeModeFromConfig(cfg *AgentsConfig) dto.AgentRuntimeModeConfig {
	mode := dto.AgentRuntimeModeConfig{}
	if cfg == nil || cfg.AgentMode == nil || strings.TrimSpace(*cfg.AgentMode) == "" {
		return mode
	}
	_ = json.Unmarshal([]byte(*cfg.AgentMode), &mode)
	return mode
}

func bindingGrantNeedsRefresh(previous []string, previousActor string, previousAtUnix int64, current []string) bool {
	current = normalizeStringIDs(current)
	if len(current) == 0 {
		return false
	}
	if strings.TrimSpace(previousActor) == "" || previousAtUnix <= 0 {
		return true
	}
	return !stringIDsEqual(normalizeStringIDs(previous), current)
}

func databaseBindingGrantNeedsRefresh(previous []dto.AgentDatabaseBinding, previousActor string, previousAtUnix int64, current []dto.AgentDatabaseBinding) bool {
	current = normalizeAgentDatabaseBindings(current)
	if len(current) == 0 {
		return false
	}
	if strings.TrimSpace(previousActor) == "" || previousAtUnix <= 0 {
		return true
	}
	return !databaseBindingsEqual(normalizeAgentDatabaseBindings(previous), current)
}

func workflowBindingGrantNeedsRefresh(previous []dto.AgentWorkflowBinding, previousActor string, previousAtUnix int64, current []dto.AgentWorkflowBinding) bool {
	current = normalizeAgentWorkflowBindings(current)
	if len(current) == 0 {
		return false
	}
	if strings.TrimSpace(previousActor) == "" || previousAtUnix <= 0 {
		return true
	}
	return !workflowBindingsEqual(normalizeAgentWorkflowBindings(previous), current)
}

func bindingGrantForStringIDs(previous []string, previousActor string, previousAtUnix int64, current []string, actorAccountID string, nowUnix int64) (string, int64) {
	current = normalizeStringIDs(current)
	previous = normalizeStringIDs(previous)
	grant := runtimeservice.ResolveBoundResourceGrant(
		runtimeservice.NewBoundResourceGrant(previousActor, previousAtUnix),
		len(current) > 0,
		stringIDsEqual(previous, current),
		actorAccountID,
		nowUnix,
	)
	return grant.BoundByAccountID, grant.BoundAtUnix
}

func bindingGrantForDatabaseBindings(previous []dto.AgentDatabaseBinding, previousActor string, previousAtUnix int64, current []dto.AgentDatabaseBinding, actorAccountID string, nowUnix int64) (string, int64) {
	current = normalizeAgentDatabaseBindings(current)
	previous = normalizeAgentDatabaseBindings(previous)
	grant := runtimeservice.ResolveBoundResourceGrant(
		runtimeservice.NewBoundResourceGrant(previousActor, previousAtUnix),
		len(current) > 0,
		databaseBindingsEqual(previous, current),
		actorAccountID,
		nowUnix,
	)
	return grant.BoundByAccountID, grant.BoundAtUnix
}

func bindingGrantForWorkflowBindings(previous []dto.AgentWorkflowBinding, previousActor string, previousAtUnix int64, current []dto.AgentWorkflowBinding, actorAccountID string, nowUnix int64) (string, int64) {
	current = normalizeAgentWorkflowBindings(current)
	previous = normalizeAgentWorkflowBindings(previous)
	grant := runtimeservice.ResolveBoundResourceGrant(
		runtimeservice.NewBoundResourceGrant(previousActor, previousAtUnix),
		len(current) > 0,
		workflowBindingsEqual(previous, current),
		actorAccountID,
		nowUnix,
	)
	return grant.BoundByAccountID, grant.BoundAtUnix
}

func stringIDsEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func databaseBindingsEqual(left []dto.AgentDatabaseBinding, right []dto.AgentDatabaseBinding) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].DataSourceID != right[i].DataSourceID || !stringIDsEqual(left[i].TableIDs, right[i].TableIDs) || !stringIDsEqual(left[i].WritableTableIDs, right[i].WritableTableIDs) {
			return false
		}
	}
	return true
}

func workflowBindingsEqual(left []dto.AgentWorkflowBinding, right []dto.AgentWorkflowBinding) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i].BindingID != right[i].BindingID ||
			left[i].Label != right[i].Label ||
			left[i].Description != right[i].Description ||
			left[i].AgentID != right[i].AgentID ||
			left[i].WorkflowID != right[i].WorkflowID ||
			left[i].AgentType != right[i].AgentType ||
			left[i].VersionStrategy != right[i].VersionStrategy ||
			left[i].VersionUUID != right[i].VersionUUID ||
			left[i].TimeoutSeconds != right[i].TimeoutSeconds {
			return false
		}
	}
	return true
}

func agentDatabaseBindingsFromSnapshot(value interface{}) []dto.AgentDatabaseBinding {
	payload, err := json.Marshal(value)
	if err != nil {
		return []dto.AgentDatabaseBinding{}
	}
	var bindings []dto.AgentDatabaseBinding
	if err := json.Unmarshal(payload, &bindings); err != nil {
		return []dto.AgentDatabaseBinding{}
	}
	return normalizeAgentDatabaseBindings(bindings)
}

func agentWorkflowBindingsFromSnapshot(value interface{}) []dto.AgentWorkflowBinding {
	payload, err := json.Marshal(value)
	if err != nil {
		return []dto.AgentWorkflowBinding{}
	}
	var bindings []dto.AgentWorkflowBinding
	if err := json.Unmarshal(payload, &bindings); err != nil {
		return []dto.AgentWorkflowBinding{}
	}
	return normalizeAgentWorkflowBindings(bindings)
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
