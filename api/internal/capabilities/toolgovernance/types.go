package toolgovernance

import "time"

type Effect string

const (
	EffectNone         Effect = "none"
	EffectRead         Effect = "read"
	EffectCreate       Effect = "create"
	EffectUpdate       Effect = "update"
	EffectDelete       Effect = "delete"
	EffectPublish      Effect = "publish"
	EffectInvoke       Effect = "invoke"
	EffectSchedule     Effect = "schedule"
	EffectExternalSend Effect = "external_send"
)

type RiskLevel string

const (
	RiskLevelLow      RiskLevel = "low"
	RiskLevelMedium   RiskLevel = "medium"
	RiskLevelHigh     RiskLevel = "high"
	RiskLevelCritical RiskLevel = "critical"
)

type PermissionTier string

const (
	PermissionTierBasic    PermissionTier = "basic"
	PermissionTierAdvanced PermissionTier = "advanced"
	PermissionTierFull     PermissionTier = "full"
)

type ApprovalPolicy string

const (
	ApprovalPolicyAutoByPermissionTier ApprovalPolicy = "auto_by_permission_tier"
	ApprovalPolicyAlwaysAsk            ApprovalPolicy = "always_ask"
	ApprovalPolicyNeverAsk             ApprovalPolicy = "never_ask"
)

type DecisionStatus string

const (
	DecisionStatusAllowed         DecisionStatus = "allowed"
	DecisionStatusNeedsApproval   DecisionStatus = "needs_approval"
	DecisionStatusDenied          DecisionStatus = "denied"
	DecisionStatusNeedsResolution DecisionStatus = "needs_resolution"
	DecisionStatusBlocked         DecisionStatus = "blocked"
)

const ApprovalEventTypeAssetToolApproval = "asset_tool_approval"

type Manifest struct {
	ToolID                  string           `json:"tool_id" yaml:"tool_id"`
	SkillID                 string           `json:"skill_id,omitempty" yaml:"skill_id"`
	Domain                  string           `json:"domain,omitempty" yaml:"domain"`
	Effect                  Effect           `json:"effect" yaml:"effect"`
	AssetType               string           `json:"asset_type,omitempty" yaml:"asset_type"`
	RiskLevel               RiskLevel        `json:"risk_level" yaml:"risk_level"`
	RequiresAssetResolution bool             `json:"requires_asset_resolution" yaml:"requires_asset_resolution"`
	Reversible              bool             `json:"reversible" yaml:"reversible"`
	BulkSensitive           bool             `json:"bulk_sensitive" yaml:"bulk_sensitive"`
	ExternalSideEffect      bool             `json:"external_side_effect" yaml:"external_side_effect"`
	PermissionScopes        []string         `json:"permission_scopes,omitempty" yaml:"permission_scopes"`
	DefaultApprovalPolicy   ApprovalPolicy   `json:"default_approval_policy" yaml:"default_approval_policy"`
	AllowedPermissionTiers  []PermissionTier `json:"allowed_permission_tiers,omitempty" yaml:"allowed_permission_tiers"`
	AuditRequired           bool             `json:"audit_required" yaml:"audit_required"`
	IdempotencyRequired     bool             `json:"idempotency_required" yaml:"idempotency_required"`
}

type AssetRef struct {
	ID          string                 `json:"id,omitempty"`
	Type        string                 `json:"type,omitempty"`
	Name        string                 `json:"name,omitempty"`
	WorkspaceID string                 `json:"workspace_id,omitempty"`
	Source      string                 `json:"source,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

type SessionGrant struct {
	ConversationID        string     `json:"conversation_id"`
	ToolID                string     `json:"tool_id"`
	Effect                Effect     `json:"effect"`
	AssetType             string     `json:"asset_type,omitempty"`
	Assets                []AssetRef `json:"assets,omitempty"`
	RiskLevel             RiskLevel  `json:"risk_level"`
	ApprovalCorrelationID string     `json:"approval_correlation_id,omitempty"`
	GrantedAt             time.Time  `json:"granted_at,omitempty"`
	ExpiresAt             time.Time  `json:"expires_at,omitempty"`
}

type ApprovalEvent struct {
	Type               string         `json:"type"`
	CorrelationID      string         `json:"correlation_id"`
	ToolID             string         `json:"tool_id"`
	SkillID            string         `json:"skill_id,omitempty"`
	Domain             string         `json:"domain,omitempty"`
	Effect             Effect         `json:"effect"`
	AssetType          string         `json:"asset_type,omitempty"`
	RiskLevel          RiskLevel      `json:"risk_level"`
	Assets             []AssetRef     `json:"assets,omitempty"`
	Reversible         bool           `json:"reversible"`
	BulkSensitive      bool           `json:"bulk_sensitive"`
	ExternalSideEffect bool           `json:"external_side_effect"`
	PermissionTier     PermissionTier `json:"permission_tier"`
	Grant              SessionGrant   `json:"grant"`
}

type Decision struct {
	Status                  DecisionStatus         `json:"status"`
	RequiresApproval        bool                   `json:"requires_approval"`
	Reason                  string                 `json:"reason,omitempty"`
	CorrelationID           string                 `json:"correlation_id"`
	ApprovedByCorrelationID string                 `json:"approved_by_correlation_id,omitempty"`
	MatchedGrant            *SessionGrant          `json:"matched_grant,omitempty"`
	Manifest                Manifest               `json:"manifest"`
	Assets                  []AssetRef             `json:"assets,omitempty"`
	ExpectedAssets          []AssetRef             `json:"expected_assets,omitempty"`
	ApprovalEvent           *ApprovalEvent         `json:"approval_event,omitempty"`
	AssetOperationAudit     map[string]interface{} `json:"asset_operation_audit,omitempty"`
	ModelFeedback           map[string]interface{} `json:"model_feedback,omitempty"`
}

type Request struct {
	Manifest       Manifest
	PermissionTier PermissionTier
	ConversationID string
	Assets         []AssetRef
	ExpectedAssets []AssetRef
	SessionGrants  []SessionGrant
	CorrelationID  string
}

type Policy struct {
	DefaultPermissionTier              PermissionTier
	CriticalRiskBlocked                bool
	HighRiskRequiresApproval           bool
	CriticalRiskRequiresApproval       bool
	DeleteRequiresApproval             bool
	BulkSensitiveRequiresApproval      bool
	ExternalSideEffectRequiresApproval bool
}
