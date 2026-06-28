package skills

import (
	"context"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

const (
	SkillTime                   = "time"
	SkillCalculator             = "calculator"
	SkillConsoleNavigator       = "console-navigator"
	SkillAgentManagement        = "agent-management"
	SkillFileGenerator          = "file-generator"
	SkillFileManager            = "file-manager"
	SkillFileReader             = "file-reader"
	SkillPPTSlidePlanner        = "ppt-slide-planner"
	SkillWorkReport             = "work-report-generator"
	SkillSchedulePlanner        = "schedule-planner"
	SkillChartGenerator         = "chart-generator"
	SkillIntentRouter           = "intent-router"
	SkillArchitectureDiagram    = "architecture-diagram-generator"
	SkillContractFieldExtractor = "contract-field-extractor"
	SkillContentSummary         = "content-summary"
	SkillSensitiveRedaction     = "sensitive-redaction"
	SkillEmailWriting           = "email-writing"
	SkillDecisionSupport        = "decision-support"
	SkillMultiDocumentCompare   = "multi-document-compare"
	SkillResumeScreening        = "resume-screening"
	SkillResponseTonePolisher   = "response-tone-polisher"
	SkillFormInteraction        = "form-interaction"
	SkillImageGenerator         = "image-generator"
	SkillTicketRouting          = "ticket-routing"
	SkillPromptProfessionalizer = "prompt-professionalizer"
	SkillInternalKnowledge      = "internal-knowledge"
	SkillAgentKnowledge         = "agent-knowledge"
	SkillInternalDatabase       = "internal-database"
	SkillAgentDatabase          = "agent-database"
	SkillAgentWorkflow          = "agent-workflow"
	SkillAgentMemory            = "agent-memory"
	SkillUserMemory             = "user-memory"

	SkillSourceSystem = "system"
	SkillSourceCustom = "custom"

	SkillStatusActive  = "active"
	SkillStatusInvalid = "invalid"

	SkillRuntimeTypeTool   = "tool"
	SkillRuntimeTypePrompt = "prompt"
	SkillRuntimeTypeHybrid = "hybrid"

	SkillScriptToolRun = "run_script"

	SkillCallerAIChat   = "aichat"
	SkillCallerAgent    = "agent"
	SkillCallerWorkflow = "workflow"

	SkillRequiredConfigAgentKnowledge = "agent_knowledge"
	SkillRequiredConfigAgentDatabase  = "agent_database"
	SkillRequiredConfigAgentWorkflow  = "agent_workflow"
)

const (
	SkillExposureGeneral         = "general"
	SkillExposureSidebarManaged  = "sidebar_managed"
	SkillExposureSystemAsset     = "system_asset"
	SkillExposureAgentBound      = "agent_bound"
	SkillExposureWorkflowRuntime = "workflow_runtime"
	SkillExposureHiddenRuntime   = "hidden_runtime"
	SkillGovernanceRiskNone      = "none"
	SkillGovernanceRiskLow       = "low"
	SkillGovernanceRiskMedium    = "medium"
	SkillGovernanceRiskHigh      = "high"
	SkillGovernanceRiskMixed     = "mixed"
)

type SkillExposureProfile struct {
	Category            string
	UserSelectable      bool
	RuntimeManaged      bool
	SystemAsset         bool
	PageContextRequired bool
	GovernanceRisk      string
}

func IsHiddenSystemSkill(skillID string) bool {
	switch normalizeSkillID(skillID) {
	case SkillAgentManagement, SkillFileManager, SkillAgentKnowledge, SkillAgentDatabase, SkillAgentWorkflow, SkillAgentMemory, SkillUserMemory:
		return true
	default:
		return false
	}
}

func SystemSkillExposureProfile(skillID string) SkillExposureProfile {
	switch normalizeSkillID(skillID) {
	case SkillConsoleNavigator:
		return SkillExposureProfile{
			Category:            SkillExposureSidebarManaged,
			UserSelectable:      false,
			RuntimeManaged:      true,
			SystemAsset:         false,
			PageContextRequired: true,
			GovernanceRisk:      SkillGovernanceRiskLow,
		}
	case SkillFileManager:
		return SkillExposureProfile{
			Category:            SkillExposureSidebarManaged,
			UserSelectable:      false,
			RuntimeManaged:      true,
			SystemAsset:         true,
			PageContextRequired: true,
			GovernanceRisk:      SkillGovernanceRiskHigh,
		}
	case SkillAgentManagement:
		return SkillExposureProfile{
			Category:            SkillExposureSidebarManaged,
			UserSelectable:      false,
			RuntimeManaged:      true,
			SystemAsset:         true,
			PageContextRequired: true,
			GovernanceRisk:      SkillGovernanceRiskHigh,
		}
	case SkillInternalKnowledge:
		return SkillExposureProfile{
			Category:            SkillExposureSystemAsset,
			UserSelectable:      false,
			RuntimeManaged:      false,
			SystemAsset:         true,
			PageContextRequired: false,
			GovernanceRisk:      SkillGovernanceRiskLow,
		}
	case SkillInternalDatabase:
		return SkillExposureProfile{
			Category:            SkillExposureSystemAsset,
			UserSelectable:      false,
			RuntimeManaged:      false,
			SystemAsset:         true,
			PageContextRequired: false,
			GovernanceRisk:      SkillGovernanceRiskMixed,
		}
	case SkillAgentKnowledge, SkillAgentDatabase, SkillAgentWorkflow:
		return SkillExposureProfile{
			Category:            SkillExposureAgentBound,
			UserSelectable:      false,
			RuntimeManaged:      true,
			SystemAsset:         true,
			PageContextRequired: false,
			GovernanceRisk:      SkillGovernanceRiskMixed,
		}
	case SkillAgentMemory, SkillUserMemory:
		return SkillExposureProfile{
			Category:            SkillExposureHiddenRuntime,
			UserSelectable:      false,
			RuntimeManaged:      true,
			SystemAsset:         false,
			PageContextRequired: false,
			GovernanceRisk:      SkillGovernanceRiskLow,
		}
	default:
		return SkillExposureProfile{
			Category:            SkillExposureGeneral,
			UserSelectable:      true,
			RuntimeManaged:      false,
			SystemAsset:         false,
			PageContextRequired: false,
			GovernanceRisk:      SkillGovernanceRiskMixed,
		}
	}
}

func IsUserSelectableSystemSkill(skillID string) bool {
	return SystemSkillExposureProfile(skillID).UserSelectable
}

func IsSystemAssetSkill(skillID string) bool {
	return SystemSkillExposureProfile(skillID).SystemAsset
}

func IsRuntimeManagedSystemSkill(skillID string) bool {
	return SystemSkillExposureProfile(skillID).RuntimeManaged
}

func SkillSupportsCaller(supportedCallers []string, caller string) bool {
	caller = strings.ToLower(strings.TrimSpace(caller))
	if caller == "" || len(supportedCallers) == 0 {
		return true
	}
	for _, raw := range supportedCallers {
		if strings.EqualFold(strings.TrimSpace(raw), caller) {
			return true
		}
	}
	return false
}

type SkillToolDefinition struct {
	Name         string                   `json:"name" yaml:"name"`
	ProviderType tools.ToolProviderType   `json:"provider_type" yaml:"provider_type"`
	ProviderID   string                   `json:"provider_id" yaml:"provider_id"`
	Governance   *toolgovernance.Manifest `json:"governance,omitempty" yaml:"governance"`
}

type SkillFrontmatter struct {
	Name             string                             `yaml:"name"`
	Description      string                             `yaml:"description"`
	WhenToUse        string                             `yaml:"when_to_use"`
	ProviderType     tools.ToolProviderType             `yaml:"provider_type"`
	ProviderID       string                             `yaml:"provider_id"`
	Tools            []string                           `yaml:"tools"`
	ToolGovernance   map[string]toolgovernance.Manifest `yaml:"tool_governance"`
	RuntimeType      string                             `yaml:"runtime_type"`
	MaxCallsPerTurn  int                                `yaml:"max_calls_per_turn"`
	TimeoutSeconds   int                                `yaml:"timeout_seconds"`
	Display          SkillDisplayMetadata               `yaml:"display"`
	SupportedCallers []string                           `yaml:"supported_callers"`
	RequiredConfig   []string                           `yaml:"required_config"`
}

type SkillDisplayMetadata struct {
	Icon        string              `json:"icon" yaml:"icon"`
	Category    string              `json:"category" yaml:"category"`
	Label       map[string]string   `json:"label" yaml:"label"`
	Description map[string]string   `json:"description" yaml:"description"`
	WhenToUse   map[string]string   `json:"when_to_use" yaml:"when_to_use"`
	Tags        map[string][]string `json:"tags,omitempty" yaml:"tags"`
}

type SkillMetadata struct {
	ID               string               `json:"skill_id"`
	Source           string               `json:"source"`
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	WhenToUse        string               `json:"when_to_use"`
	Display          SkillDisplayMetadata `json:"display"`
	Tools            []string             `json:"tools"`
	RuntimeType      string               `json:"runtime_type"`
	References       []SkillReference     `json:"references,omitempty"`
	HasScripts       bool                 `json:"has_scripts"`
	ScriptsSupported bool                 `json:"scripts_supported"`
	MaxCallsPerTurn  int                  `json:"max_calls_per_turn"`
	TimeoutSeconds   int                  `json:"timeout_seconds"`
	RootPath         string               `json:"-"`
	SupportedCallers []string             `json:"supported_callers,omitempty"`
	RequiredConfig   []string             `json:"required_config,omitempty"`
}

type SkillPromptMetadata struct {
	ID               string `json:"skill_id"`
	Source           string `json:"source"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	WhenToUse        string `json:"when_to_use"`
	HasTools         bool   `json:"has_tools"`
	RuntimeType      string `json:"runtime_type"`
	HasReferences    bool   `json:"has_references"`
	HasScripts       bool   `json:"has_scripts"`
	ScriptsSupported bool   `json:"scripts_supported"`
	MaxCallsPerTurn  int    `json:"max_calls_per_turn"`
	TimeoutSeconds   int    `json:"timeout_seconds"`
}

type SkillMetadataPromptStats struct {
	EnabledCount int
	ExposedCount int
	OmittedCount int
	Truncated    bool
}

type SkillDiscoveryMetadata struct {
	ID               string               `json:"skill_id"`
	Source           string               `json:"source"`
	Name             string               `json:"name"`
	Description      string               `json:"description"`
	WhenToUse        string               `json:"when_to_use"`
	Display          SkillDisplayMetadata `json:"display"`
	RuntimeType      string               `json:"runtime_type"`
	Enabled          bool                 `json:"enabled"`
	HasTools         bool                 `json:"has_tools"`
	HasReferences    bool                 `json:"has_references"`
	HasScripts       bool                 `json:"has_scripts"`
	ScriptsSupported bool                 `json:"scripts_supported"`
	MaxCallsPerTurn  int                  `json:"max_calls_per_turn"`
	TimeoutSeconds   int                  `json:"timeout_seconds"`
	Status           string               `json:"status"`
	ValidationError  string               `json:"validation_error,omitempty"`
	SupportedCallers []string             `json:"supported_callers,omitempty"`
	RequiredConfig   []string             `json:"required_config,omitempty"`
}

type SkillDocument struct {
	Metadata     SkillMetadata         `json:"metadata"`
	Instructions string                `json:"instructions"`
	Tools        []SkillToolDefinition `json:"tools"`
}

type SkillReference struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	FullPath string `json:"-"`
}

type SkillTrace struct {
	Kind       string                   `json:"kind"`
	SkillID    string                   `json:"skill_id,omitempty"`
	ToolName   string                   `json:"tool_name,omitempty"`
	Title      string                   `json:"title,omitempty"`
	Message    string                   `json:"message,omitempty"`
	Status     string                   `json:"status"`
	DurationMS int64                    `json:"duration_ms,omitempty"`
	Arguments  map[string]interface{}   `json:"arguments,omitempty"`
	Result     map[string]interface{}   `json:"result,omitempty"`
	Governance *toolgovernance.Decision `json:"governance,omitempty"`
	Error      string                   `json:"error,omitempty"`
}

type ToolGovernanceRequest struct {
	Manifest         toolgovernance.Manifest
	SkillID          string
	ToolName         string
	ProviderType     tools.ToolProviderType
	ProviderID       string
	Arguments        map[string]interface{}
	ExecutionContext ExecutionContext
}

type ToolGovernanceGateway interface {
	DecideSkillTool(ctx context.Context, req ToolGovernanceRequest) (toolgovernance.Decision, error)
}

type SkillToolArgumentContract struct {
	SkillID     string                 `json:"skill_id"`
	ToolName    string                 `json:"tool_name"`
	Schema      map[string]interface{} `json:"schema"`
	Example     map[string]interface{} `json:"example,omitempty"`
	Description string                 `json:"description,omitempty"`
}

type ResolvedSkills struct {
	Skills []SkillDocument
}

type CustomSkillCatalogEntry struct {
	SkillID string
	Root    string
}

func (r *ResolvedSkills) Metadata() []SkillMetadata {
	if r == nil {
		return nil
	}
	out := make([]SkillMetadata, 0, len(r.Skills))
	for _, skill := range r.Skills {
		out = append(out, skill.Metadata)
	}
	return out
}

func (r *ResolvedSkills) PromptMetadata() []SkillPromptMetadata {
	if r == nil {
		return nil
	}
	out := make([]SkillPromptMetadata, 0, len(r.Skills))
	for _, skill := range r.Skills {
		out = append(out, skillPromptMetadata(skill))
	}
	return out
}

func (r *ResolvedSkills) SkillIDs() []string {
	if r == nil {
		return nil
	}
	out := make([]string, 0, len(r.Skills))
	for _, skill := range r.Skills {
		out = append(out, skill.Metadata.ID)
	}
	return out
}

func (r *ResolvedSkills) Get(skillID string) (*SkillDocument, bool) {
	if r == nil {
		return nil, false
	}
	normalized := normalizeSkillID(skillID)
	for idx := range r.Skills {
		if r.Skills[idx].Metadata.ID == normalized {
			return &r.Skills[idx], true
		}
	}
	return nil, false
}

func normalizeSkillID(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func skillPromptMetadata(skill SkillDocument) SkillPromptMetadata {
	metadata := skill.Metadata
	return SkillPromptMetadata{
		ID:               metadata.ID,
		Source:           metadata.Source,
		Name:             metadata.Name,
		Description:      metadata.Description,
		WhenToUse:        metadata.WhenToUse,
		HasTools:         len(skill.Tools) > 0,
		RuntimeType:      metadata.RuntimeType,
		HasReferences:    len(metadata.References) > 0,
		HasScripts:       metadata.HasScripts,
		ScriptsSupported: metadata.ScriptsSupported,
		MaxCallsPerTurn:  metadata.MaxCallsPerTurn,
		TimeoutSeconds:   metadata.TimeoutSeconds,
	}
}

func copyStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func copyStringAnyMap(values map[string]interface{}) map[string]interface{} {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]interface{}, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func skillDiscoveryMetadata(skill SkillDocument) SkillDiscoveryMetadata {
	metadata := skill.Metadata
	return SkillDiscoveryMetadata{
		ID:               metadata.ID,
		Source:           metadata.Source,
		Name:             metadata.Name,
		Description:      metadata.Description,
		WhenToUse:        metadata.WhenToUse,
		Display:          metadata.Display,
		RuntimeType:      metadata.RuntimeType,
		HasTools:         len(skill.Tools) > 0,
		HasReferences:    len(metadata.References) > 0,
		HasScripts:       metadata.HasScripts,
		ScriptsSupported: metadata.ScriptsSupported,
		MaxCallsPerTurn:  metadata.MaxCallsPerTurn,
		TimeoutSeconds:   metadata.TimeoutSeconds,
		Status:           SkillStatusActive,
		SupportedCallers: copyStringSlice(metadata.SupportedCallers),
		RequiredConfig:   copyStringSlice(metadata.RequiredConfig),
	}
}
