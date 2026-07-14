package agents

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	datasetservice "github.com/zgiai/zgi/api/internal/modules/dataset/service"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"gorm.io/gorm/clause"
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
	page := normalizeAgentBindingCandidatePage(req.Page)
	resp := &dto.AgentKnowledgeCandidatesResponse{
		AgentID:         ag.ID.String(),
		WorkspaceID:     ag.TenantID.String(),
		Query:           strings.TrimSpace(req.Query),
		Page:            page,
		Limit:           limit,
		IncludeSelected: req.IncludeSelected,
		Data:            []dto.AgentKnowledgeCandidate{},
	}
	if s.knowledgeRetrievalService == nil {
		return nil, fmt.Errorf("knowledge candidate service is not configured")
	}
	selectedIDs := agentRuntimeModeFromConfig(cfg).KnowledgeDatasetIDs
	list, err := s.knowledgeRetrievalService.ListAccessibleDatasetCandidates(ctx, datasetservice.KnowledgeScope{
		WorkspaceID: ag.TenantID.String(),
		AccountID:   accountID,
	}, req.Query, selectedIDs, req.IncludeSelected, page, limit)
	if err != nil {
		return nil, err
	}
	selected := selectedStringIDSet(selectedIDs)
	resp.Total = int(list.Total)
	resp.HasMore = list.HasMore
	for _, item := range list.KnowledgeBases {
		datasetID := strings.TrimSpace(item.DatasetID)
		if datasetID == "" {
			continue
		}
		_, isSelected := selected[datasetID]
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
	page := normalizeAgentBindingCandidatePage(req.Page)
	resp := &dto.AgentSkillCandidatesResponse{
		AgentID:         ag.ID.String(),
		WorkspaceID:     ag.TenantID.String(),
		Query:           strings.TrimSpace(req.Query),
		Source:          strings.TrimSpace(req.Source),
		Page:            page,
		Limit:           limit,
		IncludeSelected: req.IncludeSelected,
		Data:            []dto.AgentSkillCandidate{},
	}
	candidates, err := s.listAgentSkillCandidatesForWorkspace(ctx, ag.TenantID.String(), accountID)
	if err != nil {
		return nil, err
	}
	selected := selectedStringIDSet(agentRuntimeModeFromConfig(cfg).EnabledSkillIDs)
	selectedCandidates := make([]dto.AgentSkillCandidate, 0, len(selected))
	availableCandidates := make([]dto.AgentSkillCandidate, 0, len(candidates))
	for _, item := range candidates {
		if !matchesAgentSkillSource(item.Source, req.Source) {
			continue
		}
		if !matchesAgentCandidateQuery(req.Query, item.SkillID, item.Name, item.Description, item.WhenToUse) {
			continue
		}
		_, isSelected := selected[strings.ToLower(strings.TrimSpace(item.SkillID))]
		if isSelected && !req.IncludeSelected {
			continue
		}
		item.Selected = isSelected
		if isSelected {
			selectedCandidates = append(selectedCandidates, item)
			continue
		}
		availableCandidates = append(availableCandidates, item)
	}
	sortAgentSkillCandidates(selectedCandidates)
	sortAgentSkillCandidates(availableCandidates)
	allCandidates := append(selectedCandidates, availableCandidates...)
	resp.Total = len(allCandidates)
	resp.Data = agentCandidatePage(allCandidates, page, limit)
	resp.HasMore = page*limit < resp.Total
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
		if skillID == "" {
			continue
		}
		if !item.Enabled || item.Status == skills.SkillStatusInvalid {
			continue
		}
		if !skills.SkillBindableToAgent(item) {
			continue
		}
		out = append(out, dto.AgentSkillCandidate{
			SkillID:          skillID,
			Name:             agentSkillCandidateDisplayName(item, skillID),
			Description:      strings.TrimSpace(item.Description),
			WhenToUse:        strings.TrimSpace(item.WhenToUse),
			Source:           strings.TrimSpace(item.Source),
			RuntimeType:      strings.TrimSpace(item.RuntimeType),
			HasTools:         item.HasTools,
			HasReferences:    item.HasReferences,
			HasScripts:       item.HasScripts,
			ScriptsSupported: item.ScriptsSupported,
			RequiredConfig:   append([]string(nil), item.RequiredConfig...),
			Display: dto.AgentSkillDisplayMetadata{
				Icon:        strings.TrimSpace(item.Display.Icon),
				Category:    strings.TrimSpace(item.Display.Category),
				Label:       cloneStringMap(item.Display.Label),
				Description: cloneStringMap(item.Display.Description),
				WhenToUse:   cloneStringMap(item.Display.WhenToUse),
				Tags:        cloneStringSliceMap(item.Display.Tags),
			},
		})
	}
	return out, nil
}

func matchesAgentSkillSource(candidateSource, requestedSource string) bool {
	requestedSource = strings.ToLower(strings.TrimSpace(requestedSource))
	if requestedSource == "" {
		return true
	}
	candidateSource = strings.ToLower(strings.TrimSpace(candidateSource))
	if requestedSource == "custom" {
		return candidateSource == "custom"
	}
	return candidateSource != "custom"
}

func sortAgentSkillCandidates(items []dto.AgentSkillCandidate) {
	sort.SliceStable(items, func(i, j int) bool {
		leftName := strings.ToLower(strings.TrimSpace(items[i].Name))
		rightName := strings.ToLower(strings.TrimSpace(items[j].Name))
		if leftName != rightName {
			return leftName < rightName
		}
		return items[i].SkillID < items[j].SkillID
	})
}

func cloneStringMap(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string]string, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneStringSliceMap(input map[string][]string) map[string][]string {
	if len(input) == 0 {
		return nil
	}
	out := make(map[string][]string, len(input))
	for key, value := range input {
		out[key] = append([]string(nil), value...)
	}
	return out
}

func agentCandidatePage[T any](items []T, page, limit int) []T {
	if len(items) == 0 {
		return []T{}
	}
	start := (page - 1) * limit
	if start >= len(items) {
		return []T{}
	}
	return items[start:min(start+limit, len(items))]
}

func agentSkillCandidateDisplayName(item skills.SkillDiscoveryMetadata, skillID string) string {
	for _, name := range []string{
		item.Display.Label["zh_Hans"],
		item.Display.Label["en_US"],
		item.Name,
		skillID,
	} {
		name = strings.TrimSpace(name)
		if name != "" {
			return name
		}
	}
	return ""
}

func (s *agentsService) ListAgentDatabaseCandidates(ctx context.Context, agentID, accountID string, req dto.AgentDatabaseCandidatesRequest) (*dto.AgentDatabaseCandidatesResponse, error) {
	ag, cfg, err := s.loadAuthorizedAgentRuntimeDraft(ctx, agentID, accountID, false)
	if err != nil {
		return nil, err
	}
	workspaceID := ag.TenantID.String()
	organizationID := s.organizationIDForAgentWorkspace(ctx, workspaceID)
	limit := normalizeAgentBindingCandidateLimit(req.Limit)
	page := normalizeAgentBindingCandidatePage(req.Page)
	resp := &dto.AgentDatabaseCandidatesResponse{
		AgentID:         ag.ID.String(),
		WorkspaceID:     workspaceID,
		Query:           strings.TrimSpace(req.Query),
		Page:            page,
		Limit:           limit,
		AvailableOnly:   req.AvailableOnly,
		IncludeSelected: req.IncludeSelected,
		RequireWrite:    req.RequireWrite,
		Data:            []dto.AgentDatabaseCandidate{},
	}
	if s.db == nil {
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
	selected := selectedDatabaseSourceSet(agentRuntimeModeFromConfig(cfg).DatabaseBindings)
	selectedIDs := stringSetKeys(selected)
	dbQuery := s.db.WithContext(ctx).
		Table("data_sources AS ds").
		Where("ds.organization_id = ? AND ds.workspace_id = ?", organizationID, workspaceID)
	if query := strings.TrimSpace(req.Query); query != "" {
		pattern := "%" + strings.ToLower(query) + "%"
		dbQuery = dbQuery.Where(
			"LOWER(COALESCE(ds.name, '')) LIKE ? OR LOWER(COALESCE(ds.description, '')) LIKE ? OR LOWER(COALESCE(ds.schema_name, '')) LIKE ?",
			pattern,
			pattern,
			pattern,
		)
	}
	if req.AvailableOnly {
		if req.IncludeSelected && len(selectedIDs) > 0 {
			dbQuery = dbQuery.Where(
				"(ds.id IN ? OR EXISTS (SELECT 1 FROM data_source_tables dst WHERE dst.data_source_id = ds.id))",
				selectedIDs,
			)
		} else {
			dbQuery = dbQuery.Where("EXISTS (SELECT 1 FROM data_source_tables dst WHERE dst.data_source_id = ds.id)")
		}
	}
	if !req.IncludeSelected && len(selectedIDs) > 0 {
		dbQuery = dbQuery.Where("ds.id NOT IN ?", selectedIDs)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count agent database candidates: %w", err)
	}
	if req.IncludeSelected && len(selectedIDs) > 0 {
		dbQuery = dbQuery.Clauses(clause.OrderBy{Expression: clause.Expr{
			SQL:                "CASE WHEN ds.id IN ? THEN 0 ELSE 1 END, LOWER(ds.name) ASC, ds.id ASC",
			Vars:               []interface{}{selectedIDs},
			WithoutParentheses: true,
		}})
	} else {
		dbQuery = dbQuery.Order("LOWER(ds.name) ASC, ds.id ASC")
	}
	var rows []agentDatabaseCandidateRow
	if err := dbQuery.
		Select(`ds.id AS data_source_id, ds.name, ds.description, ds.status, ds.workspace_id,
			ds.icon, ds.icon_type, ds.icon_background, ds.updated_at,
			(SELECT COUNT(*) FROM data_source_tables dst WHERE dst.data_source_id = ds.id) AS table_count`).
		Limit(limit).
		Offset((page - 1) * limit).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list agent database candidates: %w", err)
	}
	for _, item := range rows {
		_, isSelected := selected[strings.ToLower(strings.TrimSpace(item.DataSourceID))]
		resp.Data = append(resp.Data, dto.AgentDatabaseCandidate{
			DataSourceID:   strings.TrimSpace(item.DataSourceID),
			Name:           strings.TrimSpace(item.Name),
			Description:    strings.TrimSpace(item.Description),
			Status:         strings.TrimSpace(item.Status),
			WorkspaceID:    strings.TrimSpace(item.WorkspaceID),
			CanWrite:       canWrite,
			Icon:           stringPtrValue(item.Icon),
			IconType:       stringPtrValue(item.IconType),
			IconBackground: stringPtrValue(item.IconBackground),
			UpdatedAt:      item.UpdatedAt.Unix(),
			TableCount:     item.TableCount,
			Selected:       isSelected,
		})
	}
	resp.Total = int(total)
	resp.HasMore = int64(page*limit) < total
	resp.Count = len(resp.Data)
	return resp, nil
}

type agentDatabaseCandidateRow struct {
	DataSourceID   string    `gorm:"column:data_source_id"`
	Name           string    `gorm:"column:name"`
	Description    string    `gorm:"column:description"`
	Status         string    `gorm:"column:status"`
	WorkspaceID    string    `gorm:"column:workspace_id"`
	Icon           *string   `gorm:"column:icon"`
	IconType       *string   `gorm:"column:icon_type"`
	IconBackground *string   `gorm:"column:icon_background"`
	UpdatedAt      time.Time `gorm:"column:updated_at"`
	TableCount     int64     `gorm:"column:table_count"`
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
	page := normalizeAgentBindingCandidatePage(req.Page)
	resp := &dto.AgentDatabaseTablesResponse{
		AgentID:         ag.ID.String(),
		WorkspaceID:     workspaceID,
		DataSourceID:    dataSourceID,
		Query:           strings.TrimSpace(req.Query),
		Page:            page,
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
	if s.db == nil {
		return nil, fmt.Errorf("database table candidate service is not configured")
	}
	selected, writable := selectedDatabaseTableSets(agentRuntimeModeFromConfig(cfg).DatabaseBindings, dataSourceID)
	selectedIDs := stringSetKeys(selected)
	dbQuery := s.db.WithContext(ctx).
		Table("data_source_tables AS dst").
		Where("dst.data_source_id = ?", dataSourceID)
	if query := strings.TrimSpace(req.Query); query != "" {
		pattern := "%" + strings.ToLower(query) + "%"
		dbQuery = dbQuery.Where(
			"LOWER(COALESCE(dst.name, '')) LIKE ? OR LOWER(COALESCE(dst.description, '')) LIKE ? OR LOWER(COALESCE(dst.table_name, '')) LIKE ?",
			pattern,
			pattern,
			pattern,
		)
	}
	if !req.IncludeSelected && len(selectedIDs) > 0 {
		dbQuery = dbQuery.Where("dst.id NOT IN ?", selectedIDs)
	}
	var total int64
	if err := dbQuery.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("count agent database table candidates: %w", err)
	}
	if req.IncludeSelected && len(selectedIDs) > 0 {
		dbQuery = dbQuery.Clauses(clause.OrderBy{Expression: clause.Expr{
			SQL:                "CASE WHEN dst.id IN ? THEN 0 ELSE 1 END, LOWER(dst.name) ASC, dst.id ASC",
			Vars:               []interface{}{selectedIDs},
			WithoutParentheses: true,
		}})
	} else {
		dbQuery = dbQuery.Order("LOWER(dst.name) ASC, dst.id ASC")
	}
	var rows []agentDatabaseTableCandidateRow
	if err := dbQuery.
		Select("dst.id AS table_id, dst.data_source_id, dst.name, dst.description, dst.table_name AS physical_table_name, dst.updated_at").
		Limit(limit).
		Offset((page - 1) * limit).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list agent database table candidates: %w", err)
	}
	for _, table := range rows {
		tableID := strings.ToLower(strings.TrimSpace(table.TableID))
		_, isSelected := selected[tableID]
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
	}
	resp.Total = int(total)
	resp.HasMore = int64(page*limit) < total
	resp.Count = len(resp.Data)
	return resp, nil
}

type agentDatabaseTableCandidateRow struct {
	TableID           string    `gorm:"column:table_id"`
	DataSourceID      string    `gorm:"column:data_source_id"`
	Name              string    `gorm:"column:name"`
	Description       string    `gorm:"column:description"`
	PhysicalTableName string    `gorm:"column:physical_table_name"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func stringSetKeys(input map[string]struct{}) []string {
	if len(input) == 0 {
		return nil
	}
	out := make([]string, 0, len(input))
	for value := range input {
		if value = strings.TrimSpace(value); value != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func filterAgentWorkflowBindingCandidates(items []dto.AgentWorkflowBindingCandidate, mode dto.AgentRuntimeModeConfig, req dto.AgentWorkflowBindingCandidatesRequest) []dto.AgentWorkflowBindingCandidate {
	items, _ = pageAgentWorkflowBindingCandidates(items, mode, req)
	return items
}

func pageAgentWorkflowBindingCandidates(items []dto.AgentWorkflowBindingCandidate, mode dto.AgentRuntimeModeConfig, req dto.AgentWorkflowBindingCandidatesRequest) ([]dto.AgentWorkflowBindingCandidate, int) {
	limit := normalizeAgentBindingCandidateLimit(req.Limit)
	page := normalizeAgentBindingCandidatePage(req.Page)
	selected := selectedWorkflowBindingSet(mode.WorkflowBindings)
	selectedItems := make([]dto.AgentWorkflowBindingCandidate, 0, len(selected))
	availableItems := make([]dto.AgentWorkflowBindingCandidate, 0, len(items))
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
		if isSelected {
			selectedItems = append(selectedItems, item)
			continue
		}
		availableItems = append(availableItems, item)
	}
	filtered := append(selectedItems, availableItems...)
	total := len(filtered)
	pageCount := agentBindingCandidatePageCount(total, limit)
	if total == 0 || page > pageCount {
		return []dto.AgentWorkflowBindingCandidate{}, total
	}
	start := (page - 1) * limit
	end := min(start+limit, total)
	return filtered[start:end], total
}

func normalizeAgentBindingCandidatePage(page int) int {
	if page <= 0 {
		return 1
	}
	return page
}

func agentBindingCandidatePageCount(total, limit int) int {
	if total <= 0 || limit <= 0 {
		return 0
	}
	return (total + limit - 1) / limit
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
	searchText := strings.Builder{}
	for _, value := range values {
		normalized := normalizeAgentCandidateSearchText(value)
		if strings.Contains(normalized, query) {
			return true
		}
		searchText.WriteByte(' ')
		searchText.WriteString(normalized)
	}
	tokens := agentCandidateQueryTokens(query)
	if len(tokens) == 0 {
		return true
	}
	search := searchText.String()
	for _, token := range tokens {
		if !agentCandidateSearchContainsToken(search, token) {
			return false
		}
	}
	return true
}

func normalizeAgentCandidateSearchText(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.NewReplacer("-", " ", "_", " ", "/", " ").Replace(value)))
}

func agentCandidateQueryTokens(query string) []string {
	fields := strings.FieldsFunc(normalizeAgentCandidateSearchText(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		if field == "" {
			continue
		}
		out = append(out, field)
	}
	return out
}

func agentCandidateSearchContainsToken(search, token string) bool {
	for _, variant := range agentCandidateQueryTokenVariants(token) {
		if strings.Contains(search, variant) {
			return true
		}
	}
	return false
}

func agentCandidateQueryTokenVariants(token string) []string {
	switch token {
	case "generation":
		return []string{"generation", "generate", "generator"}
	case "generate":
		return []string{"generate", "generator", "generation"}
	case "files":
		return []string{"files", "file"}
	default:
		return []string{token}
	}
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
