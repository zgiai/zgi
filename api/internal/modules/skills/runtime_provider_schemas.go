package skills

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func (r *Runtime) ToolProviderConfigured(ctx context.Context, providerType tools.ToolProviderType, providerID string) bool {
	if r == nil || r.manager == nil {
		return false
	}
	_, err := r.manager.GetProvider(ctx, providerType, strings.TrimSpace(providerID), "")
	return err == nil
}

func (r *Runtime) attachProviderToolSchemas(ctx context.Context, doc *SkillDocument) {
	if r == nil || r.manager == nil || doc == nil {
		return
	}
	for index := range doc.Tools {
		definition := &doc.Tools[index]
		provider, err := r.manager.GetProvider(ctx, definition.ProviderType, definition.ProviderID, "")
		if err != nil {
			continue
		}
		tool, err := provider.GetTool(definition.Name)
		if err != nil {
			continue
		}
		entity := tool.GetEntity()
		definition.InputSchema = entity.InputSchema
		definition.OutputSchema = entity.OutputSchema
		definition.RuntimeDescription = strings.TrimSpace(entity.Description.LLM)
		if entity.Governance != nil {
			manifest := fallbackProviderGovernanceManifest(doc, *definition, entity)
			if definition.Governance != nil {
				manifest = *definition.Governance
			}
			if toolID := strings.TrimSpace(entity.Governance.ToolID); toolID != "" {
				manifest.ToolID = toolID
			}
			manifest.Effect = toolgovernance.Effect(entity.Governance.Effect)
			manifest.RiskLevel = toolgovernance.RiskLevel(entity.Governance.RiskLevel)
			manifest.DataEgress = entity.Governance.DataEgress
			manifest.ExternalDestination = entity.Governance.ExternalDestination
			manifest.SensitiveDataAllowed = entity.Governance.SensitiveDataAllowed
			manifest = toolgovernance.NormalizeManifest(manifest)
			definition.Governance = &manifest
		}
	}
}

func fallbackProviderGovernanceManifest(doc *SkillDocument, definition SkillToolDefinition, entity tools.ToolEntity) toolgovernance.Manifest {
	toolID := strings.TrimSpace(entity.Identity.Name)
	if toolID == "" {
		toolID = strings.TrimSpace(definition.Name)
	}
	providerID := strings.TrimSpace(definition.ProviderID)
	permissionScope := "integration:" + providerID + ":" + strings.TrimSpace(definition.Name)
	return toolgovernance.Manifest{
		ToolID:                 toolID,
		SkillID:                strings.TrimSpace(doc.Metadata.ID),
		Domain:                 "external_integration",
		AssetType:              "external_resource",
		ExternalSideEffect:     true,
		PermissionScopes:       []string{permissionScope},
		DefaultApprovalPolicy:  toolgovernance.ApprovalPolicyAlwaysAsk,
		AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic, toolgovernance.PermissionTierAdvanced, toolgovernance.PermissionTierFull},
		AuditRequired:          true,
	}
}
