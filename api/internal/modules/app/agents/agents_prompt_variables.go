package agents

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var agentPromptVariablePattern = regexp.MustCompile(`(?s)<zgi:(slot|knowledge|skill|database|table)\b([^>]*)>(.*?)</zgi:(slot|knowledge|skill|database|table)>`)
var agentPromptVariableAttrPattern = regexp.MustCompile(`([a-zA-Z_][\w-]*)="([^"]*)"`)

const (
	agentSystemPromptMaxLength    = 16000
	agentSystemPromptRawMaxLength = 50000
)

type agentPromptDatasetSummary struct {
	ID          string
	Name        string
	Description string
}

type agentPromptDatabaseSummary struct {
	ID          string
	Name        string
	Description string
	SchemaName  string
	Tables      []agentPromptTableSummary
}

type agentPromptTableSummary struct {
	ID           string
	DataSourceID string
	Name         string
	Description  string
	Writable     bool
}

func (h *AgentsHandler) agentRunConfig(ctx context.Context, scope runtimeservice.Scope, agentID, systemPromptVersion string, cfg dto.AgentConfigResponse, agentMemoryUserScope string) (runtimeservice.RunConfig, error) {
	cfg.SystemPrompt = h.resolveAgentSystemPrompt(ctx, scope, cfg)
	if err := validateAgentResolvedSystemPrompt(cfg.SystemPrompt); err != nil {
		return runtimeservice.RunConfig{}, err
	}
	return agentRunConfig(agentID, systemPromptVersion, cfg, agentMemoryUserScope), nil
}

func validateAgentSystemPromptSource(source string) error {
	if promptRuneLength(source) > agentSystemPromptRawMaxLength {
		return fmt.Errorf("%w: serialized prompt exceeds %d characters", errAgentPromptTooLong, agentSystemPromptRawMaxLength)
	}
	length := agentPromptEffectiveLength(source)
	if length > agentSystemPromptMaxLength {
		return fmt.Errorf("%w: effective prompt length %d exceeds %d characters", errAgentPromptTooLong, length, agentSystemPromptMaxLength)
	}
	return nil
}

func validateAgentResolvedSystemPrompt(source string) error {
	length := promptRuneLength(source)
	if length > agentSystemPromptMaxLength {
		return fmt.Errorf("%w: resolved prompt length %d exceeds %d characters", errAgentPromptTooLong, length, agentSystemPromptMaxLength)
	}
	return nil
}

func agentPromptEffectiveLength(source string) int {
	source = strings.TrimSpace(source)
	if source == "" {
		return 0
	}

	length := 0
	lastIndex := 0
	matches := agentPromptVariablePattern.FindAllStringSubmatchIndex(source, -1)
	for _, match := range matches {
		if len(match) < 10 {
			continue
		}
		if match[0] > lastIndex {
			length += promptRuneLength(source[lastIndex:match[0]])
		}
		openKind := source[match[2]:match[3]]
		closeKind := source[match[8]:match[9]]
		if openKind == closeKind {
			content := html.UnescapeString(source[match[6]:match[7]])
			length += promptRuneLength(content)
		} else {
			length += promptRuneLength(source[match[0]:match[1]])
		}
		lastIndex = match[1]
	}
	if lastIndex < len(source) {
		length += promptRuneLength(source[lastIndex:])
	}
	return length
}

func promptRuneLength(source string) int {
	return len([]rune(source))
}

func (h *AgentsHandler) resolveAgentSystemPrompt(ctx context.Context, scope runtimeservice.Scope, cfg dto.AgentConfigResponse) string {
	source := strings.TrimSpace(cfg.SystemPrompt)
	if source == "" || !agentPromptVariablePattern.MatchString(source) {
		return source
	}

	datasets := h.agentPromptDatasets(ctx, scope, cfg.KnowledgeDatasetIDs)
	skillMetadata := h.agentPromptSkills(ctx, scope, cfg.EnabledSkillIDs)
	databases := h.agentPromptDatabases(ctx, scope, cfg.DatabaseBindings)

	return agentPromptVariablePattern.ReplaceAllStringFunc(source, func(token string) string {
		matches := agentPromptVariablePattern.FindStringSubmatch(token)
		if len(matches) < 5 || strings.TrimSpace(matches[1]) != strings.TrimSpace(matches[4]) {
			return agentPromptDisabledCapability(token)
		}
		blockType := strings.TrimSpace(matches[1])
		attrs := parseAgentPromptVariableAttrs(matches[2])
		content := strings.TrimSpace(html.UnescapeString(matches[3]))
		switch blockType {
		case "slot":
			return content
		case "knowledge":
			return renderAgentPromptKnowledgeVariable(attrs["id"], datasets)
		case "skill":
			return renderAgentPromptSkillVariable(attrs["id"], skillMetadata)
		case "database":
			return renderAgentPromptDatabaseVariable(attrs["id"], databases)
		case "table":
			return renderAgentPromptTableVariable(attrs["id"], databases)
		}
		return agentPromptDisabledCapability(token)
	})
}

func parseAgentPromptVariableAttrs(input string) map[string]string {
	out := map[string]string{}
	matches := agentPromptVariableAttrPattern.FindAllStringSubmatch(input, -1)
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		key := strings.TrimSpace(match[1])
		if key == "" {
			continue
		}
		out[key] = html.UnescapeString(match[2])
	}
	return out
}

func (h *AgentsHandler) agentPromptDatasets(ctx context.Context, scope runtimeservice.Scope, datasetIDs []string) map[string]agentPromptDatasetSummary {
	ids := normalizeAgentPromptIDs(datasetIDs)
	if len(ids) == 0 || h == nil || h.db == nil {
		return map[string]agentPromptDatasetSummary{}
	}
	query := h.db.WithContext(ctx).
		Table("datasets").
		Select("id, name, COALESCE(description, '') AS description").
		Where("id IN ?", ids)
	if scope.OrganizationID != uuid.Nil {
		query = query.Where("organization_id = ?", scope.OrganizationID.String())
	}
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		query = query.Where("workspace_id = ?", scope.WorkspaceID.String())
	}

	var rows []agentPromptDatasetSummary
	if err := query.Find(&rows).Error; err != nil {
		logger.WarnContext(ctx, "failed to resolve agent prompt knowledge variables", err)
		return map[string]agentPromptDatasetSummary{}
	}
	out := make(map[string]agentPromptDatasetSummary, len(rows))
	for _, row := range rows {
		row.ID = strings.TrimSpace(row.ID)
		if row.ID == "" {
			continue
		}
		out[row.ID] = row
	}
	return out
}

func (h *AgentsHandler) agentPromptSkills(ctx context.Context, scope runtimeservice.Scope, skillIDs []string) map[string]skills.SkillDiscoveryMetadata {
	ids := normalizeAgentPromptIDs(skillIDs)
	if len(ids) == 0 || h == nil || h.chatRuntimeService == nil {
		return map[string]skills.SkillDiscoveryMetadata{}
	}
	catalog, err := h.chatRuntimeService.ListSkills(ctx, scope)
	if err != nil {
		logger.WarnContext(ctx, "failed to resolve agent prompt skill variables", err)
		return map[string]skills.SkillDiscoveryMetadata{}
	}
	allowed := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		allowed[id] = struct{}{}
	}
	out := make(map[string]skills.SkillDiscoveryMetadata, len(ids))
	for _, item := range catalog {
		id := strings.TrimSpace(item.ID)
		if _, ok := allowed[id]; !ok {
			continue
		}
		out[id] = item
	}
	return out
}

func (h *AgentsHandler) agentPromptDatabases(ctx context.Context, scope runtimeservice.Scope, bindings []dto.AgentDatabaseBinding) map[string]agentPromptDatabaseSummary {
	boundDataSources, boundTables, writableTables := normalizeAgentPromptDatabaseBindings(bindings)
	if len(boundDataSources) == 0 || len(boundTables) == 0 || h == nil || h.db == nil {
		return map[string]agentPromptDatabaseSummary{}
	}

	var dbRows []agentPromptDatabaseSummary
	query := h.db.WithContext(ctx).
		Table("data_sources").
		Select("id, name, COALESCE(description, '') AS description, COALESCE(schema_name, '') AS schema_name").
		Where("id IN ?", boundDataSources)
	if scope.OrganizationID != uuid.Nil {
		query = query.Where("organization_id = ?", scope.OrganizationID.String())
	}
	if scope.WorkspaceID != nil && *scope.WorkspaceID != uuid.Nil {
		query = query.Where("workspace_id = ?", scope.WorkspaceID.String())
	}
	if err := query.Find(&dbRows).Error; err != nil {
		logger.WarnContext(ctx, "failed to resolve agent prompt database variables", err)
		return map[string]agentPromptDatabaseSummary{}
	}

	out := make(map[string]agentPromptDatabaseSummary, len(dbRows))
	loadedDataSourceIDs := make([]string, 0, len(dbRows))
	for _, row := range dbRows {
		row.ID = strings.TrimSpace(row.ID)
		if row.ID == "" {
			continue
		}
		out[row.ID] = row
		loadedDataSourceIDs = append(loadedDataSourceIDs, row.ID)
	}
	if len(loadedDataSourceIDs) == 0 {
		return out
	}

	tableIDs := make([]string, 0, len(boundTables))
	for tableID := range boundTables {
		tableIDs = append(tableIDs, tableID)
	}
	var tableRows []agentPromptTableSummary
	tableQuery := h.db.WithContext(ctx).
		Table("data_source_tables").
		Select("id, data_source_id, name, COALESCE(description, '') AS description").
		Where("data_source_id IN ?", loadedDataSourceIDs).
		Where("id IN ?", tableIDs)
	if scope.OrganizationID != uuid.Nil {
		tableQuery = tableQuery.Where("organization_id = ?", scope.OrganizationID.String())
	}
	if err := tableQuery.Find(&tableRows).Error; err != nil {
		logger.WarnContext(ctx, "failed to resolve agent prompt database table variables", err)
		return out
	}

	for _, table := range tableRows {
		table.ID = strings.TrimSpace(table.ID)
		table.DataSourceID = strings.TrimSpace(table.DataSourceID)
		if table.ID == "" || table.DataSourceID == "" {
			continue
		}
		table.Writable = writableTables[table.DataSourceID+":"+table.ID]
		dbSummary, ok := out[table.DataSourceID]
		if !ok {
			continue
		}
		dbSummary.Tables = append(dbSummary.Tables, table)
		out[table.DataSourceID] = dbSummary
	}
	for id, dbSummary := range out {
		sortAgentPromptTables(dbSummary.Tables)
		out[id] = dbSummary
	}
	return out
}

func renderAgentPromptKnowledgeVariable(key string, datasets map[string]agentPromptDatasetSummary) string {
	id := strings.TrimSpace(key)
	if id == "" {
		return agentPromptDisabledCapability("knowledge")
	}
	if item, ok := datasets[id]; ok {
		return renderAgentPromptDataset(item)
	}
	return agentPromptDisabledCapability("knowledge." + id)
}

func renderAgentPromptSkillVariable(key string, metadata map[string]skills.SkillDiscoveryMetadata) string {
	if item, ok := metadata[key]; ok {
		return renderAgentPromptSkill(item)
	}
	return agentPromptDisabledCapability("skill." + key)
}

func renderAgentPromptDatabaseVariable(key string, databases map[string]agentPromptDatabaseSummary) string {
	id := strings.TrimSpace(key)
	if id == "" {
		return agentPromptDisabledCapability("database")
	}
	if item, ok := databases[id]; ok {
		return renderAgentPromptDatabase(item)
	}
	return agentPromptDisabledCapability("database." + id)
}

func renderAgentPromptTableVariable(key string, databases map[string]agentPromptDatabaseSummary) string {
	dataSourceID, tableID := splitAgentPromptTableKey(key)
	if tableID == "" {
		return agentPromptDisabledCapability("table." + key)
	}
	for _, database := range databases {
		if dataSourceID != "" && database.ID != dataSourceID {
			continue
		}
		for _, table := range database.Tables {
			if table.ID == tableID {
				return renderAgentPromptTable(database, table)
			}
		}
	}
	return agentPromptDisabledCapability("table." + key)
}

func renderAgentPromptDataset(item agentPromptDatasetSummary) string {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = item.ID
	}
	desc := strings.TrimSpace(item.Description)
	if desc == "" {
		return fmt.Sprintf("%s (ID: %s)", name, item.ID)
	}
	return fmt.Sprintf("%s (ID: %s): %s", name, item.ID, desc)
}

func renderAgentPromptDatabase(item agentPromptDatabaseSummary) string {
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = "Unnamed database"
	}
	parts := []string{fmt.Sprintf("Database: %s", name)}
	if schema := strings.TrimSpace(item.SchemaName); schema != "" {
		parts = append(parts, "Schema: "+schema)
	}
	if desc := strings.TrimSpace(item.Description); desc != "" {
		parts = append(parts, "Description: "+desc)
	}
	if len(item.Tables) > 0 {
		tableLines := make([]string, 0, len(item.Tables))
		for _, table := range item.Tables {
			tableLines = append(tableLines, "- "+renderAgentPromptTableLine(table))
		}
		parts = append(parts, "Bound tables:\n"+strings.Join(tableLines, "\n"))
	}
	return strings.Join(parts, "\n")
}

func renderAgentPromptTable(database agentPromptDatabaseSummary, table agentPromptTableSummary) string {
	dbName := strings.TrimSpace(database.Name)
	if dbName == "" {
		dbName = "Unnamed database"
	}
	parts := []string{
		fmt.Sprintf("Data table: %s", renderAgentPromptTableName(table)),
		"Database: " + dbName,
	}
	if schema := strings.TrimSpace(database.SchemaName); schema != "" {
		parts = append(parts, "Schema: "+schema)
	}
	if desc := strings.TrimSpace(table.Description); desc != "" {
		parts = append(parts, "Description: "+desc)
	}
	if table.Writable {
		parts = append(parts, "Write access: enabled")
	} else {
		parts = append(parts, "Write access: disabled")
	}
	return strings.Join(parts, "\n")
}

func renderAgentPromptTableLine(table agentPromptTableSummary) string {
	line := renderAgentPromptTableName(table)
	if desc := strings.TrimSpace(table.Description); desc != "" {
		line += ": " + desc
	}
	if table.Writable {
		line += " (writable)"
	}
	return line
}

func renderAgentPromptTableName(table agentPromptTableSummary) string {
	name := strings.TrimSpace(table.Name)
	if name == "" {
		name = "Unnamed table"
	}
	return name
}

func renderAgentPromptSkill(item skills.SkillDiscoveryMetadata) string {
	name := skillPromptName(item)
	desc := firstNonEmptyAgentPrompt(skillPromptLocaleText(item.Display.Description), item.Description, item.WhenToUse, item.ID)
	if desc == "" || desc == item.ID {
		return fmt.Sprintf("%s (ID: %s)", name, item.ID)
	}
	return fmt.Sprintf("%s (ID: %s): %s", name, item.ID, desc)
}

func skillPromptName(item skills.SkillDiscoveryMetadata) string {
	return firstNonEmptyAgentPrompt(skillPromptLocaleText(item.Display.Label), item.Name, item.ID)
}

func skillPromptLocaleText(values map[string]string) string {
	return firstNonEmptyAgentPrompt(values["zh_Hans"], values["zh-Hans"], values["en_US"], values["en-US"])
}

func agentPromptDisabledCapability(token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		token = "unknown"
	}
	return fmt.Sprintf("[该能力当前未启用: %s]", token)
}

func normalizeAgentPromptIDs(input []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(input))
	for _, raw := range input {
		id := strings.TrimSpace(raw)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func firstNonEmptyAgentPrompt(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeAgentPromptDatabaseBindings(bindings []dto.AgentDatabaseBinding) ([]string, map[string]string, map[string]bool) {
	dataSources := make([]string, 0, len(bindings))
	dataSourceSeen := map[string]struct{}{}
	tableToDataSource := map[string]string{}
	writableTables := map[string]bool{}
	for _, binding := range bindings {
		dataSourceID := strings.TrimSpace(binding.DataSourceID)
		if dataSourceID == "" {
			continue
		}
		if _, ok := dataSourceSeen[dataSourceID]; !ok {
			dataSources = append(dataSources, dataSourceID)
			dataSourceSeen[dataSourceID] = struct{}{}
		}
		tableSet := map[string]struct{}{}
		for _, rawTableID := range binding.TableIDs {
			tableID := strings.TrimSpace(rawTableID)
			if tableID == "" {
				continue
			}
			tableSet[tableID] = struct{}{}
			tableToDataSource[tableID] = dataSourceID
		}
		for _, rawTableID := range binding.WritableTableIDs {
			tableID := strings.TrimSpace(rawTableID)
			if _, ok := tableSet[tableID]; ok {
				writableTables[dataSourceID+":"+tableID] = true
			}
		}
	}
	return dataSources, tableToDataSource, writableTables
}

func splitAgentPromptTableKey(key string) (string, string) {
	trimmed := strings.TrimSpace(key)
	if trimmed == "" {
		return "", ""
	}
	for _, separator := range []string{":", "/"} {
		if strings.Contains(trimmed, separator) {
			parts := strings.SplitN(trimmed, separator, 2)
			return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		}
	}
	return "", trimmed
}

func sortAgentPromptTables(tables []agentPromptTableSummary) {
	sort.SliceStable(tables, func(i, j int) bool {
		left := strings.ToLower(renderAgentPromptTableName(tables[i]))
		right := strings.ToLower(renderAgentPromptTableName(tables[j]))
		if left == right {
			return tables[i].ID < tables[j].ID
		}
		return left < right
	})
}
