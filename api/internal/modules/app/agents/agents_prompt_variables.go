package agents

import (
	"context"
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/google/uuid"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
)

var agentPromptVariablePattern = regexp.MustCompile(`(?s)<zgi:(slot|knowledge|skill)\b([^>]*)>(.*?)</zgi:(slot|knowledge|skill)>`)
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
