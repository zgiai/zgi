package service

import (
	"context"
	"strings"

	"github.com/google/uuid"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	"github.com/zgiai/zgi/api/pkg/logger"
)

// The subjects market tools are bound to, as the market tool resolver names them.
const (
	MarketSubjectAgent        = "agent"
	MarketSubjectDigitalHuman = "digital_human"
)

// MarketTool is one ZGI Market tool bound to the current agent or digital human.
type MarketTool struct {
	// ProviderID names the subject and the listing for the tool manager.
	ProviderID  string
	Name        string
	Description string
	InputSchema map[string]interface{}
	// RiskLevel is read, write, or destructive.
	RiskLevel string
}

// MarketToolResolver lists the ZGI Market tools bound to an agent or a digital human.
type MarketToolResolver interface {
	SubjectMarketTools(ctx context.Context, organizationID uuid.UUID, subjectType string, subjectID uuid.UUID) ([]MarketTool, error)
}

const marketToolsInstructions = `These tools come from the ZGI Market MCP servers an administrator bound to you. Each call is
billed by ZGI Market. Use one only when it serves the user's request, pass exactly the arguments
its schema describes, and report what it returned. Tools that change data at the vendor need the
user's approval and may be refused when nobody can approve.`

// marketSubject names the subject of a caller that market tools can be bound to.
func marketSubject(caller Caller) (string, uuid.UUID, bool) {
	if caller.ID == nil || *caller.ID == uuid.Nil {
		return "", uuid.Nil, false
	}
	switch normalizeCallerType(caller.Type) {
	case runtimemodel.ConversationCallerAgent:
		return MarketSubjectAgent, *caller.ID, true
	case runtimemodel.ConversationCallerDigitalHuman:
		return MarketSubjectDigitalHuman, *caller.ID, true
	default:
		return "", uuid.Nil, false
	}
}

func (s *service) subjectMarketTools(ctx context.Context, scope Scope, caller Caller) []MarketTool {
	if s == nil || s.marketTools == nil {
		return nil
	}
	subjectType, subjectID, ok := marketSubject(caller)
	if !ok || scope.OrganizationID == uuid.Nil {
		return nil
	}
	found, err := s.marketTools.SubjectMarketTools(ctx, scope.OrganizationID, subjectType, subjectID)
	if err != nil {
		// Market tools are additive: a market that cannot be reached leaves the run without them.
		logger.WarnContext(ctx, "ZGI Market tools are unavailable for this run",
			"organization_id", scope.OrganizationID.String(), "subject_type", subjectType, "error", err.Error())
		return nil
	}
	return found
}

// marketSkillIDs reports the market Skill when the caller has market tools bound.
func (s *service) marketSkillIDs(ctx context.Context, scope Scope, caller Caller) []string {
	if len(s.subjectMarketTools(ctx, scope, caller)) == 0 {
		return nil
	}
	return []string{skills.SkillZGIMarketTools}
}

// overlayMarketTools adds the request-scoped market Skill to the resolved Skills of the run.
func (s *service) overlayMarketTools(ctx context.Context, prepared *PreparedChat, resolved *skills.ResolvedSkills) (*skills.ResolvedSkills, error) {
	if prepared == nil || prepared.parts == nil {
		return resolved, nil
	}
	parts := prepared.parts
	found := s.subjectMarketTools(ctx, prepared.Scope, prepared.Caller)
	if len(found) == 0 {
		parts.MarketSkillIDs = nil
		parts.SkillIDs = skillIDsWithout(parts.SkillIDs, []string{skills.SkillZGIMarketTools})
		parts.ToolSkillIDs = skillIDsWithout(parts.ToolSkillIDs, []string{skills.SkillZGIMarketTools})
		return resolved, nil
	}
	result := cloneResolvedSkills(resolved)
	result.Skills = append(result.Skills, MarketToolsSkillDocument(prepared.Caller, found))
	parts.MarketSkillIDs = []string{skills.SkillZGIMarketTools}
	parts.SkillIDs = mergeNormalizedSkillIDs(parts.SkillIDs, parts.MarketSkillIDs)
	parts.ToolSkillIDs = mergeNormalizedSkillIDs(parts.ToolSkillIDs, parts.MarketSkillIDs)
	parts.SkillMode = skillModeAuto
	return result, nil
}

// MarketToolsSkillDocument builds the market Skill. Tool names stay the vendor's unless two bound
// listings share one; the provider identity always names the listing.
func MarketToolsSkillDocument(caller Caller, found []MarketTool) skills.SkillDocument {
	supportedCallers := []string{skills.SkillCallerAgent}
	if normalizeCallerType(caller.Type) == runtimemodel.ConversationCallerDigitalHuman {
		supportedCallers = []string{skills.SkillCallerDigitalHuman}
	}
	counts := make(map[string]int, len(found))
	for _, tool := range found {
		counts[tool.Name]++
	}
	definitions := make([]skills.SkillToolDefinition, 0, len(found))
	names := make([]string, 0, len(found))
	for _, tool := range found {
		name := tool.Name
		if counts[name] > 1 {
			slug := tool.ProviderID[strings.LastIndex(tool.ProviderID, ":")+1:]
			name = strings.ReplaceAll(slug, "-", "_") + "__" + name
		}
		description := strings.TrimSpace(tool.Description)
		if description == "" {
			description = "A ZGI Market tool."
		}
		definitions = append(definitions, skills.SkillToolDefinition{
			Name:               name,
			ProviderType:       tools.ToolProviderTypeMarket,
			ProviderID:         tool.ProviderID,
			InputSchema:        tool.InputSchema,
			RuntimeDescription: description,
			Governance:         marketToolGovernance(name, tool.RiskLevel),
		})
		names = append(names, name)
	}
	return skills.SkillDocument{
		Metadata: skills.SkillMetadata{
			ID: skills.SkillZGIMarketTools, Source: skills.SkillSourceSystem,
			Name: "ZGI Market tools", Description: "Tools of the ZGI Market MCP servers bound to you.",
			WhenToUse:   "Use when one of the bound ZGI Market tools can satisfy the current request.",
			RuntimeType: skills.SkillRuntimeTypeTool, MaxCallsPerTurn: 20, TimeoutSeconds: 90,
			SupportedCallers: supportedCallers, Tools: names,
		},
		Instructions: marketToolsInstructions,
		Tools:        definitions,
	}
}

// marketToolGovernance maps the market's risk level onto tool governance. A read tool runs
// without asking. A write tool always asks, and a destructive one asks for every call; where
// nobody can approve, such as an unattended digital human, governance refuses them.
func marketToolGovernance(toolName, riskLevel string) *toolgovernance.Manifest {
	manifest := &toolgovernance.Manifest{
		ToolID: "zgi-market." + toolName, SkillID: skills.SkillZGIMarketTools, Domain: "zgi_market",
		AssetType: "market_tool", DataEgress: true, ExternalDestination: "zgi-market",
		PermissionScopes:       []string{"zgi_market:tool"},
		AllowedPermissionTiers: []toolgovernance.PermissionTier{toolgovernance.PermissionTierBasic, toolgovernance.PermissionTierAdvanced, toolgovernance.PermissionTierFull},
		AuditRequired:          true,
	}
	switch riskLevel {
	case "read":
		manifest.Effect, manifest.RiskLevel = toolgovernance.EffectRead, toolgovernance.RiskLevelLow
		manifest.DefaultApprovalPolicy = toolgovernance.ApprovalPolicyAutoByPermissionTier
		manifest.Reversible = true
	case "destructive":
		manifest.Effect, manifest.RiskLevel = toolgovernance.EffectDelete, toolgovernance.RiskLevelHigh
		manifest.DefaultApprovalPolicy = toolgovernance.ApprovalPolicyAlwaysAsk
		manifest.ApprovalEveryInvocation = true
		manifest.ExternalSideEffect = true
	default:
		manifest.Effect, manifest.RiskLevel = toolgovernance.EffectInvoke, toolgovernance.RiskLevelMedium
		manifest.DefaultApprovalPolicy = toolgovernance.ApprovalPolicyAlwaysAsk
		manifest.ExternalSideEffect = true
	}
	return manifest
}
