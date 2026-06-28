package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	automationaction "github.com/zgiai/zgi/api/internal/modules/automation/service/action"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/gorm"
	"sort"
	"strings"
	"time"
)

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
		if err := s.validateDatabaseBindingGrant(ctx, organizationID, workspaceID, accountID, req.DatabaseBindings); err != nil {
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

func (s *agentsService) validateDatabaseBindingGrant(ctx context.Context, organizationID, workspaceID, accountID string, bindings []dto.AgentDatabaseBinding) error {
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
		sourceWorkspaceID := agentDataSourceWorkspaceID(organizationID, dataSource.WorkspaceID)
		if sourceWorkspaceID != strings.TrimSpace(workspaceID) {
			return fmt.Errorf("database %s not found in agent workspace", binding.DataSourceID)
		}
		if err := s.requireDatabaseReadBindingPermission(ctx, organizationID, sourceWorkspaceID, accountID); err != nil {
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
			if err := s.requireDatabaseWriteBindingPermission(ctx, organizationID, sourceWorkspaceID, accountID); err != nil {
				return fmt.Errorf("database %s write binding: %w", binding.DataSourceID, err)
			}
		}
	}
	return nil
}

func agentDataSourceWorkspaceID(organizationID string, workspaceID *string) string {
	if workspaceID != nil && strings.TrimSpace(*workspaceID) != "" {
		return strings.TrimSpace(*workspaceID)
	}
	return strings.TrimSpace(organizationID)
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
		if binding.VersionStrategy == automationaction.WorkflowVersionStrategyPinned {
			if strings.TrimSpace(binding.VersionUUID) == "" {
				return fmt.Errorf("workflow binding %s requires version_uuid", binding.BindingID)
			}
			candidate, ok, err := s.getPinnedAgentWorkflowBindingCandidate(ctx, workspaceID, binding)
			if err != nil {
				return fmt.Errorf("load pinned workflow binding %s: %w", binding.BindingID, err)
			}
			if !ok || strings.TrimSpace(candidate.AgentID) != strings.TrimSpace(binding.AgentID) {
				return fmt.Errorf("workflow binding %s pinned version is not available", binding.BindingID)
			}
			continue
		}
		candidate, ok := byBindingID[strings.TrimSpace(binding.BindingID)]
		if !ok || strings.TrimSpace(candidate.AgentID) != strings.TrimSpace(binding.AgentID) {
			return fmt.Errorf("workflow binding %s is not available", binding.BindingID)
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
		if bindings[idx].VersionStrategy == automationaction.WorkflowVersionStrategyPinned {
			pinned, pinnedOK, err := s.getPinnedAgentWorkflowBindingCandidate(ctx, workspaceID, bindings[idx])
			if err != nil || !pinnedOK {
				continue
			}
			applyAgentWorkflowBindingRuntimeInputs(&bindings[idx], pinned)
			continue
		}
		candidate, ok := byBindingID[bindings[idx].BindingID]
		if ok {
			applyAgentWorkflowBindingRuntimeInputs(&bindings[idx], candidate)
		}
	}
	return bindings
}

func applyAgentWorkflowBindingRuntimeInputs(binding *dto.AgentWorkflowBinding, candidate dto.AgentWorkflowBindingCandidate) {
	if binding == nil {
		return
	}
	if strings.TrimSpace(binding.AgentType) == "" {
		binding.AgentType = strings.TrimSpace(candidate.AgentType)
	}
	binding.StartInputs = cloneWorkflowStartInputs(candidate.StartInputs)
	binding.RequiredInputs = append([]string(nil), candidate.RequiredInputs...)
	binding.DefaultInputKey = strings.TrimSpace(candidate.DefaultInputKey)
}

type agentWorkflowCandidateRow struct {
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

func (s *agentsService) ListAgentWorkflowBindingCandidates(ctx context.Context, agentID, accountID string, req dto.AgentWorkflowBindingCandidatesRequest) (*dto.AgentWorkflowBindingCandidatesResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	items, err := s.listAgentWorkflowBindingCandidatesForWorkspace(ctx, ag.TenantID.String())
	if err != nil {
		return nil, err
	}
	items = filterAgentWorkflowBindingCandidates(items, agentRuntimeModeFromConfig(cfg), req)
	return &dto.AgentWorkflowBindingCandidatesResponse{
		AgentID:            ag.ID.String(),
		WorkspaceID:        ag.TenantID.String(),
		Query:              strings.TrimSpace(req.Query),
		AgentType:          strings.TrimSpace(req.AgentType),
		Limit:              normalizeAgentBindingCandidateLimit(req.Limit),
		IncludeSelected:    req.IncludeSelected,
		IncludeStartInputs: req.IncludeStartInputs,
		Count:              len(items),
		Data:               items,
	}, nil
}

func (s *agentsService) listAgentWorkflowBindingCandidatesForWorkspace(ctx context.Context, workspaceID string) ([]dto.AgentWorkflowBindingCandidate, error) {
	if s.db == nil {
		return nil, fmt.Errorf("database is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return []dto.AgentWorkflowBindingCandidate{}, nil
	}

	var rows []agentWorkflowCandidateRow
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
		items = append(items, s.agentWorkflowBindingCandidateFromRow(ctx, row))
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Label != items[j].Label {
			return items[i].Label < items[j].Label
		}
		return items[i].AgentID < items[j].AgentID
	})
	return items, nil
}

func (s *agentsService) getPinnedAgentWorkflowBindingCandidate(ctx context.Context, workspaceID string, binding dto.AgentWorkflowBinding) (dto.AgentWorkflowBindingCandidate, bool, error) {
	if s.db == nil {
		return dto.AgentWorkflowBindingCandidate{}, false, fmt.Errorf("database is required")
	}
	workspaceID = strings.TrimSpace(workspaceID)
	agentID := strings.TrimSpace(binding.AgentID)
	workflowID := strings.TrimSpace(binding.WorkflowID)
	versionUUID := strings.TrimSpace(binding.VersionUUID)
	if workspaceID == "" || agentID == "" || workflowID == "" || versionUUID == "" {
		return dto.AgentWorkflowBindingCandidate{}, false, nil
	}
	var row agentWorkflowCandidateRow
	err := s.db.WithContext(ctx).
		Table("workflows").
		Select("workflows.agent_id AS agent_id, workflows.id AS workflow_id, agents.agent_type AS agent_type, workflows.version_uuid AS version_uuid, workflows.version AS version, workflows.graph AS graph, agents.name AS label, agents.description AS description, agents.icon AS icon, agents.icon_type AS icon_type, workflows.created_at AS updated_at").
		Joins("JOIN agents ON agents.id = workflows.agent_id").
		Where("workflows.tenant_id = ? AND workflows.agent_id = ? AND workflows.id = ? AND workflows.version_uuid = ?", workspaceID, agentID, workflowID, versionUUID).
		Where("workflows.version != ?", "draft").
		Where("agents.deleted_at IS NULL AND agents.web_app_status = ?", AgentWebAppStatusActive).
		Where("agents.agent_type IN ?", []string{"WORKFLOW", "CONVERSATIONAL_WORKFLOW"}).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return dto.AgentWorkflowBindingCandidate{}, false, nil
	}
	if err != nil {
		return dto.AgentWorkflowBindingCandidate{}, false, err
	}
	return s.agentWorkflowBindingCandidateFromRow(ctx, row), true, nil
}

func (s *agentsService) agentWorkflowBindingCandidateFromRow(ctx context.Context, row agentWorkflowCandidateRow) dto.AgentWorkflowBindingCandidate {
	icon := stringPtrValue(row.Icon)
	iconType := stringPtrValue(row.IconType)
	iconURL := ""
	if iconType == "image" && icon != "" && s.fileService != nil {
		if fileURL, err := s.fileService.GetFileURL(ctx, icon); err == nil {
			iconURL = fileURL
		}
	}
	startInputs := workflowStartInputsFromGraph(row.Graph)
	agentID := strings.ToLower(strings.TrimSpace(row.AgentID))
	return dto.AgentWorkflowBindingCandidate{
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
	}
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
				Variable:            variable,
				Label:               strings.TrimSpace(stringFromMap(varMap, "label")),
				Type:                strings.TrimSpace(stringFromMap(varMap, "type")),
				Required:            boolFromMap(varMap, "required"),
				Default:             varMap["default"],
				DefaultDateTimeMode: strings.TrimSpace(stringFromMap(varMap, "default_datetime_mode")),
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
