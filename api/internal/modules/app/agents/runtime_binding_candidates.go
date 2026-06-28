package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const (
	defaultAgentBindingCandidateLimit = 20
	maxAgentBindingCandidateLimit     = 100
)

func (s *agentsService) ListAgentKnowledgeCandidates(ctx context.Context, agentID, accountID string, req dto.AgentKnowledgeCandidatesRequest) (*dto.AgentKnowledgeCandidatesResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	limit := normalizeAgentBindingCandidateLimit(req.Limit)
	resp := &dto.AgentKnowledgeCandidatesResponse{
		AgentID:         ag.ID.String(),
		WorkspaceID:     ag.TenantID.String(),
		Query:           strings.TrimSpace(req.Query),
		Limit:           limit,
		IncludeSelected: req.IncludeSelected,
		Data:            []dto.AgentKnowledgeCandidate{},
	}
	if s.knowledgeRetrievalService == nil {
		return nil, fmt.Errorf("knowledge candidate service is not configured")
	}
	list, err := s.knowledgeRetrievalService.ListAccessibleDatasets(ctx, datasetservice.KnowledgeScope{
		WorkspaceID: ag.TenantID.String(),
		AccountID:   accountID,
	}, req.Query, limit)
	if err != nil {
		return nil, err
	}
	selected := selectedStringIDSet(agentRuntimeModeFromConfig(cfg).KnowledgeDatasetIDs)
	resp.Warnings = append(resp.Warnings, list.Warnings...)
	for _, item := range list.KnowledgeBases {
		datasetID := strings.TrimSpace(item.DatasetID)
		if datasetID == "" {
			continue
		}
		_, isSelected := selected[datasetID]
		if isSelected && !req.IncludeSelected {
			continue
		}
		resp.Data = append(resp.Data, dto.AgentKnowledgeCandidate{
			DatasetID:       datasetID,
			Name:            strings.TrimSpace(item.Name),
			Description:     strings.TrimSpace(item.Description),
			Provider:        strings.TrimSpace(item.Provider),
			EnableGraphFlow: item.EnableGraphFlow,
			Selected:        isSelected,
		})
	}
	resp.Count = len(resp.Data)
	return resp, nil
}

func (s *agentsService) ListAgentSkillCandidates(ctx context.Context, agentID, accountID string, req dto.AgentSkillCandidatesRequest) (*dto.AgentSkillCandidatesResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	limit := normalizeAgentBindingCandidateLimit(req.Limit)
	resp := &dto.AgentSkillCandidatesResponse{
		AgentID:         ag.ID.String(),
		WorkspaceID:     ag.TenantID.String(),
		Query:           strings.TrimSpace(req.Query),
		Limit:           limit,
		IncludeSelected: req.IncludeSelected,
		Data:            []dto.AgentSkillCandidate{},
	}
	candidates, err := s.listAgentSkillCandidatesForWorkspace(ctx, ag.TenantID.String(), accountID)
	if err != nil {
		return nil, err
	}
	selected := selectedStringIDSet(agentRuntimeModeFromConfig(cfg).EnabledSkillIDs)
	for _, item := range candidates {
		if !matchesAgentCandidateQuery(req.Query, item.SkillID, item.Name, item.Description, item.WhenToUse) {
			continue
		}
		_, isSelected := selected[strings.ToLower(strings.TrimSpace(item.SkillID))]
		if isSelected && !req.IncludeSelected {
			continue
		}
		item.Selected = isSelected
		resp.Data = append(resp.Data, item)
		if len(resp.Data) >= limit {
			break
		}
	}
	resp.Count = len(resp.Data)
	return resp, nil
}

func (s *agentsService) listAgentSkillCandidatesForWorkspace(ctx context.Context, workspaceID, accountID string) ([]dto.AgentSkillCandidate, error) {
	if s.chatRuntimeService == nil {
		return nil, fmt.Errorf("skill candidate service is not configured")
	}
	organizationID := s.organizationIDForAgentWorkspace(ctx, workspaceID)
	organizationUUID, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil {
		return nil, fmt.Errorf("invalid organization id")
	}
	accountUUID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return nil, fmt.Errorf("invalid account id")
	}
	metadata, err := s.chatRuntimeService.ListSkills(ctx, runtimeservice.Scope{
		OrganizationID: organizationUUID,
		AccountID:      accountUUID,
	})
	if err != nil {
		return nil, err
	}
	out := make([]dto.AgentSkillCandidate, 0, len(metadata))
	for _, item := range metadata {
		skillID := strings.ToLower(strings.TrimSpace(item.ID))
		if skillID == "" || skills.IsHiddenSystemSkill(skillID) {
			continue
		}
		if item.Status == skills.SkillStatusInvalid {
			continue
		}
		if !skills.SkillSupportsCaller(item.SupportedCallers, runtimemodel.ConversationCallerAgent) {
			continue
		}
		out = append(out, dto.AgentSkillCandidate{
			SkillID:          skillID,
			Name:             strings.TrimSpace(item.Name),
			Description:      strings.TrimSpace(item.Description),
			WhenToUse:        strings.TrimSpace(item.WhenToUse),
			Source:           strings.TrimSpace(item.Source),
			RuntimeType:      strings.TrimSpace(item.RuntimeType),
			HasTools:         item.HasTools,
			HasReferences:    item.HasReferences,
			HasScripts:       item.HasScripts,
			ScriptsSupported: item.ScriptsSupported,
			RequiredConfig:   append([]string(nil), item.RequiredConfig...),
		})
	}
	return out, nil
}

func (s *agentsService) ListAgentDatabaseCandidates(ctx context.Context, agentID, accountID string, req dto.AgentDatabaseCandidatesRequest) (*dto.AgentDatabaseCandidatesResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	workspaceID := ag.TenantID.String()
	organizationID := s.organizationIDForAgentWorkspace(ctx, workspaceID)
	limit := normalizeAgentBindingCandidateLimit(req.Limit)
	resp := &dto.AgentDatabaseCandidatesResponse{
		AgentID:         ag.ID.String(),
		WorkspaceID:     workspaceID,
		Query:           strings.TrimSpace(req.Query),
		Limit:           limit,
		IncludeSelected: req.IncludeSelected,
		RequireWrite:    req.RequireWrite,
		Data:            []dto.AgentDatabaseCandidate{},
	}
	if s.dataSourceService == nil {
		return nil, fmt.Errorf("database candidate service is not configured")
	}
	if s.enterpriseService != nil {
		if err := s.requireDatabaseReadBindingPermission(ctx, organizationID, workspaceID, accountID); err != nil {
			resp.Warnings = append(resp.Warnings, err.Error())
			return resp, nil
		}
	}
	canWrite := true
	if s.enterpriseService != nil {
		if err := s.requireDatabaseWriteBindingPermission(ctx, organizationID, workspaceID, accountID); err != nil {
			canWrite = false
			if req.RequireWrite {
				resp.Warnings = append(resp.Warnings, err.Error())
				return resp, nil
			}
		}
	}
	dataSources, err := s.dataSourceService.ListDataSources(ctx, organizationID, accountID, []string{workspaceID})
	if err != nil {
		return nil, err
	}
	selected := selectedDatabaseSourceSet(agentRuntimeModeFromConfig(cfg).DatabaseBindings)
	for _, item := range dataSources {
		if item == nil {
			continue
		}
		if agentDataSourceWorkspaceID(organizationID, item.WorkspaceID) != workspaceID {
			continue
		}
		if !matchesAgentCandidateQuery(req.Query, item.Name, item.Description) {
			continue
		}
		_, isSelected := selected[strings.ToLower(strings.TrimSpace(item.ID))]
		if isSelected && !req.IncludeSelected {
			continue
		}
		resp.Data = append(resp.Data, dto.AgentDatabaseCandidate{
			DataSourceID:   strings.TrimSpace(item.ID),
			Name:           strings.TrimSpace(item.Name),
			Description:    strings.TrimSpace(item.Description),
			Status:         strings.TrimSpace(item.Status),
			WorkspaceID:    agentDataSourceWorkspaceID(organizationID, item.WorkspaceID),
			CanEdit:        item.CanEdit,
			CanWrite:       canWrite,
			Icon:           stringPtrValue(item.Icon),
			IconType:       stringPtrValue(item.IconType),
			IconBackground: stringPtrValue(item.IconBackground),
			UpdatedAt:      item.UpdatedAt.Unix(),
			Selected:       isSelected,
		})
		if len(resp.Data) >= limit {
			break
		}
	}
	resp.Count = len(resp.Data)
	return resp, nil
}

func (s *agentsService) ListAgentDatabaseTables(ctx context.Context, agentID, accountID string, req dto.AgentDatabaseTablesRequest) (*dto.AgentDatabaseTablesResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	workspaceID := ag.TenantID.String()
	organizationID := s.organizationIDForAgentWorkspace(ctx, workspaceID)
	dataSourceID := strings.ToLower(strings.TrimSpace(req.DataSourceID))
	if dataSourceID == "" {
		return nil, fmt.Errorf("data_source_id is required")
	}
	limit := normalizeAgentBindingCandidateLimit(req.Limit)
	resp := &dto.AgentDatabaseTablesResponse{
		AgentID:         ag.ID.String(),
		WorkspaceID:     workspaceID,
		DataSourceID:    dataSourceID,
		Query:           strings.TrimSpace(req.Query),
		Limit:           limit,
		IncludeColumns:  req.IncludeColumns,
		IncludeSelected: req.IncludeSelected,
		Data:            []dto.AgentDatabaseTableCandidate{},
	}
	if s.dataSourceService == nil {
		return nil, fmt.Errorf("database candidate service is not configured")
	}
	dataSource, err := s.dataSourceService.GetDataSourceByID(ctx, organizationID, dataSourceID, accountID)
	if err != nil {
		return nil, fmt.Errorf("load database %s: %w", dataSourceID, err)
	}
	if dataSource == nil || strings.TrimSpace(dataSource.OrganizationID) != strings.TrimSpace(organizationID) {
		return nil, fmt.Errorf("database %s not found", dataSourceID)
	}
	if agentDataSourceWorkspaceID(organizationID, dataSource.WorkspaceID) != workspaceID {
		return nil, fmt.Errorf("database %s not found in agent workspace", dataSourceID)
	}
	if s.enterpriseService != nil {
		if err := s.requireDatabaseReadBindingPermission(ctx, organizationID, workspaceID, accountID); err != nil {
			return nil, fmt.Errorf("database %s read binding: %w", dataSourceID, err)
		}
	}
	tables, err := s.dataSourceService.ListTables(ctx, organizationID, dataSourceID, accountID)
	if err != nil {
		return nil, err
	}
	selected, writable := selectedDatabaseTableSets(agentRuntimeModeFromConfig(cfg).DatabaseBindings, dataSourceID)
	for _, table := range tables {
		if table == nil || strings.TrimSpace(table.DataSourceID) != dataSourceID {
			continue
		}
		if !matchesAgentCandidateQuery(req.Query, table.Name, table.Description, table.PhysicalTableName) {
			continue
		}
		tableID := strings.ToLower(strings.TrimSpace(table.ID))
		_, isSelected := selected[tableID]
		if isSelected && !req.IncludeSelected {
			continue
		}
		candidate := dto.AgentDatabaseTableCandidate{
			TableID:           tableID,
			DataSourceID:      dataSourceID,
			Name:              strings.TrimSpace(table.Name),
			Description:       strings.TrimSpace(table.Description),
			PhysicalTableName: strings.TrimSpace(table.PhysicalTableName),
			UpdatedAt:         table.UpdatedAt.Unix(),
			Selected:          isSelected,
			Writable:          writable[tableID],
		}
		if req.IncludeColumns {
			columns, err := s.dataSourceService.GetTableColumns(ctx, organizationID, dataSourceID, tableID, false)
			if err != nil {
				return nil, fmt.Errorf("load columns for table %s: %w", tableID, err)
			}
			candidate.Columns = columns.Columns
		}
		resp.Data = append(resp.Data, candidate)
		if len(resp.Data) >= limit {
			break
		}
	}
	resp.Count = len(resp.Data)
	return resp, nil
}

func filterAgentWorkflowBindingCandidates(items []dto.AgentWorkflowBindingCandidate, mode dto.AgentRuntimeModeConfig, req dto.AgentWorkflowBindingCandidatesRequest) []dto.AgentWorkflowBindingCandidate {
	limit := normalizeAgentBindingCandidateLimit(req.Limit)
	selected := selectedWorkflowBindingSet(mode.WorkflowBindings)
	out := make([]dto.AgentWorkflowBindingCandidate, 0, min(len(items), limit))
	for _, item := range items {
		if !matchesAgentCandidateQuery(req.Query, item.Label, item.Description, item.BindingID, item.AgentID) {
			continue
		}
		if agentType := strings.TrimSpace(req.AgentType); agentType != "" && !strings.EqualFold(strings.TrimSpace(item.AgentType), agentType) {
			continue
		}
		_, isSelected := selected[strings.ToLower(strings.TrimSpace(item.BindingID))]
		if isSelected && !req.IncludeSelected {
			continue
		}
		item.Selected = isSelected
		if !req.IncludeStartInputs {
			item.StartInputs = nil
			item.RequiredInputs = nil
			item.DefaultInputKey = ""
		}
		out = append(out, item)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func normalizeAgentBindingCandidateLimit(limit int) int {
	if limit <= 0 {
		return defaultAgentBindingCandidateLimit
	}
	if limit > maxAgentBindingCandidateLimit {
		return maxAgentBindingCandidateLimit
	}
	return limit
}

func matchesAgentCandidateQuery(query string, values ...string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(strings.TrimSpace(value)), query) {
			return true
		}
	}
	return false
}

func selectedStringIDSet(ids []string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range normalizeStringIDs(ids) {
		out[id] = struct{}{}
	}
	return out
}

func selectedDatabaseSourceSet(bindings []dto.AgentDatabaseBinding) map[string]struct{} {
	out := map[string]struct{}{}
	for _, binding := range normalizeAgentDatabaseBindings(bindings) {
		out[binding.DataSourceID] = struct{}{}
	}
	return out
}

func selectedDatabaseTableSets(bindings []dto.AgentDatabaseBinding, dataSourceID string) (map[string]struct{}, map[string]bool) {
	selected := map[string]struct{}{}
	writable := map[string]bool{}
	dataSourceID = strings.ToLower(strings.TrimSpace(dataSourceID))
	for _, binding := range normalizeAgentDatabaseBindings(bindings) {
		if binding.DataSourceID != dataSourceID {
			continue
		}
		for _, tableID := range binding.TableIDs {
			selected[tableID] = struct{}{}
		}
		for _, tableID := range binding.WritableTableIDs {
			writable[tableID] = true
		}
	}
	return selected, writable
}

func selectedWorkflowBindingSet(bindings []dto.AgentWorkflowBinding) map[string]struct{} {
	out := map[string]struct{}{}
	for _, binding := range normalizeAgentWorkflowBindings(bindings) {
		out[binding.BindingID] = struct{}{}
	}
	return out
}
