package service

import (
	"strings"

	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

// splitConnectorNativeToolProjections separates Skill instruction activation
// from per-operation schema selection. The input is the freshly authorized
// Connector snapshot overlaid on ordinary Skills. Only the projection/session
// copy omits native definitions; execution retains their provider identities.
// This lets direct functions share the existing relevance, alias, capacity,
// and context-budget path instead of activating every schema in one group.
func splitConnectorNativeToolProjections(resolved *skills.ResolvedSkills) (*skills.ResolvedSkills, []skills.NativeToolProjection) {
	projectionResolved := cloneResolvedSkills(resolved)
	projections := make([]skills.NativeToolProjection, 0)
	for index := range projectionResolved.Skills {
		doc := &projectionResolved.Skills[index]
		if !hasConnectorNativeTools(doc) {
			continue
		}
		definitions := make([]skills.SkillToolDefinition, 0, len(doc.Tools))
		toolNames := make([]string, 0, len(doc.Tools))
		for _, definition := range doc.Tools {
			if !definition.ConnectorNative {
				definitions = append(definitions, definition)
				toolNames = appendUniqueStrings(toolNames, definition.Name)
				continue
			}
			if tools.NormalizeToolProviderType(definition.ProviderType) != tools.ToolProviderTypeConnector ||
				strings.TrimSpace(definition.ProviderID) == "" || strings.TrimSpace(definition.ConnectorOperationID) == "" {
				continue
			}
			projections = append(projections, skills.NativeToolProjection{
				Name:        strings.TrimSpace(definition.Name),
				NameScope:   definition.ProviderID + "/" + definition.ConnectorOperationID,
				Description: strings.TrimSpace(definition.RuntimeDescription),
				InputSchema: cloneConnectorSchema(definition.InputSchema),
				Binding: skills.NativeToolBinding{
					SkillID:            doc.Metadata.ID,
					ToolName:           definition.Name,
					ConnectorOperation: &skills.NativeConnectorOperation{ConnectorID: definition.ProviderID, OperationID: definition.ConnectorOperationID},
				},
			})
		}
		doc.Tools = definitions
		doc.Metadata.Tools = toolNames
	}
	return projectionResolved, projections
}

func nativeProjectionsForActiveSkills(projections []skills.NativeToolProjection, activeSkillIDs []string) []skills.NativeToolProjection {
	result := make([]skills.NativeToolProjection, 0, len(projections))
	for _, projection := range projections {
		if containsNativeSkillID(activeSkillIDs, projection.Binding.SkillID) {
			result = append(result, projection)
		}
	}
	return result
}

// The native loop also accepts direct names from loaded Skills for backwards
// compatibility. Remove unexposed Connector definitions from that lookup so a
// guessed name cannot bypass schema or capacity selection. The retained
// definitions still go through fresh runtime identity and risk checks.
func restrictConnectorNativeExecutionTools(resolved *skills.ResolvedSkills, toolSet skills.NativeToolSet) *skills.ResolvedSkills {
	type identity struct{ skillID, toolName string }
	exposed := make(map[identity]struct{}, len(toolSet.ToolBindings))
	for _, binding := range toolSet.ToolBindings {
		exposed[identity{strings.ToLower(strings.TrimSpace(binding.SkillID)), strings.TrimSpace(binding.ToolName)}] = struct{}{}
	}
	result := cloneResolvedSkills(resolved)
	for index := range result.Skills {
		doc := &result.Skills[index]
		if !hasConnectorNativeTools(doc) {
			continue
		}
		definitions := make([]skills.SkillToolDefinition, 0, len(doc.Tools))
		toolNames := make([]string, 0, len(doc.Tools))
		for _, definition := range doc.Tools {
			if definition.ConnectorNative {
				if _, ok := exposed[identity{strings.ToLower(strings.TrimSpace(doc.Metadata.ID)), strings.TrimSpace(definition.Name)}]; !ok {
					continue
				}
			}
			definitions = append(definitions, definition)
			toolNames = appendUniqueStrings(toolNames, definition.Name)
		}
		doc.Tools = definitions
		doc.Metadata.Tools = toolNames
	}
	return result
}

func onlyConnectorNativeTools(doc *skills.SkillDocument) bool {
	if doc == nil || len(doc.Tools) == 0 {
		return false
	}
	for _, definition := range doc.Tools {
		if !definition.ConnectorNative {
			return false
		}
	}
	return true
}

func hasConnectorNativeTools(doc *skills.SkillDocument) bool {
	if doc != nil {
		for _, definition := range doc.Tools {
			if definition.ConnectorNative {
				return true
			}
		}
	}
	return false
}
