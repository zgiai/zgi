package toolgovernance

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	FrozenInvocationStatusPending  = "pending"
	FrozenInvocationStatusApproved = "approved"
	FrozenInvocationStatusRejected = "rejected"
	FrozenInvocationStatusExecuted = "executed"
	FrozenInvocationStatusFailed   = "failed"
)

type FrozenInvocation struct {
	ID             string                 `json:"id,omitempty"`
	IdempotencyKey string                 `json:"idempotency_key,omitempty"`
	Hash           string                 `json:"hash,omitempty"`
	Status         string                 `json:"status"`
	CorrelationID  string                 `json:"correlation_id,omitempty"`
	SkillID        string                 `json:"skill_id"`
	ToolName       string                 `json:"tool_name"`
	ToolID         string                 `json:"tool_id,omitempty"`
	ProviderType   string                 `json:"provider_type,omitempty"`
	ProviderID     string                 `json:"provider_id,omitempty"`
	Effect         Effect                 `json:"effect,omitempty"`
	AssetType      string                 `json:"asset_type,omitempty"`
	RiskLevel      RiskLevel              `json:"risk_level,omitempty"`
	Arguments      map[string]interface{} `json:"arguments"`
	Assets         []AssetRef             `json:"assets,omitempty"`
	ExpectedAssets []AssetRef             `json:"expected_assets,omitempty"`
	CreatedAt      *time.Time             `json:"created_at,omitempty"`
	ExpiresAt      *time.Time             `json:"expires_at,omitempty"`
}

type FrozenInvocationRequest struct {
	CorrelationID  string
	Manifest       Manifest
	SkillID        string
	ToolName       string
	ProviderType   string
	ProviderID     string
	Arguments      map[string]interface{}
	Assets         []AssetRef
	ExpectedAssets []AssetRef
	Now            time.Time
	TTL            time.Duration
}

func NewFrozenInvocation(req FrozenInvocationRequest) FrozenInvocation {
	now := req.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = DefaultSessionGrantTTL
	}
	expiresAt := now.Add(ttl)
	manifest := NormalizeManifest(req.Manifest)
	invocation := FrozenInvocation{
		Status:         FrozenInvocationStatusPending,
		CorrelationID:  strings.TrimSpace(req.CorrelationID),
		SkillID:        firstNonEmptyString(req.SkillID, manifest.SkillID),
		ToolName:       strings.TrimSpace(req.ToolName),
		ToolID:         manifest.ToolID,
		ProviderType:   strings.TrimSpace(req.ProviderType),
		ProviderID:     strings.TrimSpace(req.ProviderID),
		Effect:         manifest.Effect,
		AssetType:      manifest.AssetType,
		RiskLevel:      manifest.RiskLevel,
		Arguments:      cloneStringAnyMap(req.Arguments),
		Assets:         sortedAssets(normalizeAssets(req.Assets)),
		ExpectedAssets: sortedAssets(normalizeAssets(req.ExpectedAssets)),
		CreatedAt:      &now,
		ExpiresAt:      &expiresAt,
	}
	invocation.Hash = FrozenInvocationHash(invocation)
	shortHash := shortFrozenInvocationHash(invocation.Hash)
	if invocation.CorrelationID != "" {
		invocation.ID = "frozen:" + invocation.CorrelationID
		invocation.IdempotencyKey = "tool_governance:" + invocation.CorrelationID + ":" + shortHash
	} else {
		invocation.ID = "frozen:" + shortHash
		invocation.IdempotencyKey = "tool_governance:" + shortHash
	}
	return invocation
}

func NormalizeFrozenInvocation(invocation FrozenInvocation) FrozenInvocation {
	invocation.CorrelationID = strings.TrimSpace(invocation.CorrelationID)
	invocation.SkillID = strings.TrimSpace(invocation.SkillID)
	invocation.ToolName = strings.TrimSpace(invocation.ToolName)
	invocation.ToolID = strings.TrimSpace(invocation.ToolID)
	invocation.ProviderType = strings.TrimSpace(invocation.ProviderType)
	invocation.ProviderID = strings.TrimSpace(invocation.ProviderID)
	invocation.Effect = NormalizeEffect(invocation.Effect)
	invocation.AssetType = normalizeAssetType(invocation.AssetType)
	invocation.RiskLevel = NormalizeRiskLevel(invocation.RiskLevel)
	invocation.Status = normalizeFrozenInvocationStatus(invocation.Status)
	invocation.Arguments = cloneStringAnyMap(invocation.Arguments)
	if invocation.Arguments == nil {
		invocation.Arguments = map[string]interface{}{}
	}
	invocation.Assets = sortedAssets(normalizeAssets(invocation.Assets))
	invocation.ExpectedAssets = sortedAssets(normalizeAssets(invocation.ExpectedAssets))
	return invocation
}

func FrozenInvocationHash(invocation FrozenInvocation) string {
	normalized := NormalizeFrozenInvocation(invocation)
	payload := frozenInvocationCanonicalPayload{
		SkillID:        normalized.SkillID,
		ToolName:       normalized.ToolName,
		ToolID:         normalized.ToolID,
		ProviderType:   normalized.ProviderType,
		ProviderID:     normalized.ProviderID,
		Effect:         normalized.Effect,
		AssetType:      normalized.AssetType,
		RiskLevel:      normalized.RiskLevel,
		Arguments:      normalized.Arguments,
		Assets:         normalized.Assets,
		ExpectedAssets: normalized.ExpectedAssets,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func FrozenInvocationHashMatches(invocation FrozenInvocation) bool {
	expected := strings.TrimSpace(invocation.Hash)
	if expected == "" {
		return false
	}
	return expected == FrozenInvocationHash(invocation)
}

type frozenInvocationCanonicalPayload struct {
	SkillID        string                 `json:"skill_id"`
	ToolName       string                 `json:"tool_name"`
	ToolID         string                 `json:"tool_id,omitempty"`
	ProviderType   string                 `json:"provider_type,omitempty"`
	ProviderID     string                 `json:"provider_id,omitempty"`
	Effect         Effect                 `json:"effect,omitempty"`
	AssetType      string                 `json:"asset_type,omitempty"`
	RiskLevel      RiskLevel              `json:"risk_level,omitempty"`
	Arguments      map[string]interface{} `json:"arguments"`
	Assets         []AssetRef             `json:"assets,omitempty"`
	ExpectedAssets []AssetRef             `json:"expected_assets,omitempty"`
}

func normalizeFrozenInvocationStatus(status string) string {
	switch strings.TrimSpace(status) {
	case FrozenInvocationStatusApproved:
		return FrozenInvocationStatusApproved
	case FrozenInvocationStatusRejected:
		return FrozenInvocationStatusRejected
	case FrozenInvocationStatusExecuted:
		return FrozenInvocationStatusExecuted
	case FrozenInvocationStatusFailed:
		return FrozenInvocationStatusFailed
	default:
		return FrozenInvocationStatusPending
	}
}

func shortFrozenInvocationHash(hash string) string {
	hash = strings.TrimSpace(strings.TrimPrefix(hash, "sha256:"))
	if len(hash) > 16 {
		return hash[:16]
	}
	if hash != "" {
		return hash
	}
	return "unknown"
}

func sortedAssets(assets []AssetRef) []AssetRef {
	if len(assets) == 0 {
		return nil
	}
	out := append([]AssetRef(nil), assets...)
	sort.Slice(out, func(i, j int) bool {
		return assetSortKey(out[i]) < assetSortKey(out[j])
	})
	return out
}

func assetSortKey(asset AssetRef) string {
	return strings.Join([]string{
		strings.TrimSpace(asset.Type),
		strings.TrimSpace(asset.WorkspaceID),
		strings.TrimSpace(asset.ID),
		strings.ToLower(strings.TrimSpace(asset.Name)),
	}, "|")
}

func cloneStringAnyMap(input map[string]interface{}) map[string]interface{} {
	if len(input) == 0 {
		return nil
	}
	data, err := json.Marshal(input)
	if err != nil {
		out := make(map[string]interface{}, len(input))
		for key, value := range input {
			out[key] = value
		}
		return out
	}
	var output map[string]interface{}
	if err := json.Unmarshal(data, &output); err != nil {
		out := make(map[string]interface{}, len(input))
		for key, value := range input {
			out[key] = value
		}
		return out
	}
	return output
}

func FrozenInvocationFromAny(value interface{}) (FrozenInvocation, bool, error) {
	if value == nil {
		return FrozenInvocation{}, false, nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return FrozenInvocation{}, false, fmt.Errorf("marshal frozen invocation: %w", err)
	}
	var invocation FrozenInvocation
	if err := json.Unmarshal(data, &invocation); err != nil {
		return FrozenInvocation{}, false, fmt.Errorf("unmarshal frozen invocation: %w", err)
	}
	invocation = NormalizeFrozenInvocation(invocation)
	if invocation.SkillID == "" && invocation.ToolName == "" && len(invocation.Arguments) == 0 {
		return FrozenInvocation{}, false, nil
	}
	return invocation, true, nil
}
