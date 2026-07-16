package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/zgiai/zgi/api/pkg/response"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PricingOperation string

const (
	PricingOperationChat      PricingOperation = "chat"
	PricingOperationEmbedding PricingOperation = "embedding"
	PricingOperationRerank    PricingOperation = "rerank"
	PricingOperationImage     PricingOperation = "image_generation"
)

type PricingMeter string

const (
	PricingMeterInputToken  PricingMeter = "input_token"
	PricingMeterOutputToken PricingMeter = "output_token"
	PricingMeterImage       PricingMeter = "image"
)

type PricingSource string

const (
	PricingSourceUpstreamModelPrice  PricingSource = "upstream_model_price"
	PricingSourceAdminFallback       PricingSource = "admin_fallback"
	PricingSourceCodeDefaultFallback PricingSource = "code_default_fallback"
)

type UsageSource string

const (
	UsageSourceProviderUsage     UsageSource = "provider_usage"
	UsageSourceEstimatedUsage    UsageSource = "estimated_usage"
	UsageSourceRequestParameters UsageSource = "request_parameters"
)

type PricingFallbackRule struct {
	ID                  string           `json:"id"`
	Enabled             *bool            `json:"enabled,omitempty"`
	Operation           PricingOperation `json:"operation"`
	Meter               PricingMeter     `json:"meter"`
	Provider            string           `json:"provider,omitempty"`
	Model               string           `json:"model,omitempty"`
	Size                string           `json:"size,omitempty"`
	Quality             string           `json:"quality,omitempty"`
	Style               string           `json:"style,omitempty"`
	Unit                string           `json:"unit,omitempty"`
	PriceUSDPer1MTokens string           `json:"price_usd_per_1m_tokens,omitempty"`
	Credits             int64            `json:"credits,omitempty"`
	PricingSource       PricingSource    `json:"pricing_source,omitempty"`
}

type PricingFallbackConfigResponse struct {
	Enabled        bool                  `json:"enabled"`
	DefaultRules   []PricingFallbackRule `json:"default_rules"`
	OverrideRules  []PricingFallbackRule `json:"override_rules"`
	EffectiveRules []PricingFallbackRule `json:"effective_rules"`
}

type UpdatePricingFallbackRequest struct {
	Enabled       bool                  `json:"enabled"`
	OverrideRules []PricingFallbackRule `json:"override_rules"`
}

type pricingFallbackOverrideRecord struct {
	OrganizationID uuid.UUID      `gorm:"column:organization_id;primaryKey;type:uuid"`
	Enabled        bool           `gorm:"column:enabled;not null;default:false"`
	Rules          datatypes.JSON `gorm:"column:rules;type:jsonb;not null;default:'[]'"`
	UpdatedBy      string         `gorm:"column:updated_by;size:100"`
	CreatedAt      time.Time      `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP"`
	UpdatedAt      time.Time      `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP"`
}

func (pricingFallbackOverrideRecord) TableName() string {
	return "llm_pricing_fallback_overrides"
}

type PricingFallbackHandler struct {
	db *gorm.DB
}

func NewPricingFallbackHandler(db *gorm.DB) *PricingFallbackHandler {
	return &PricingFallbackHandler{db: db}
}

func RegisterPricingFallbackRoutes(group *gin.RouterGroup, handler *PricingFallbackHandler) {
	if group == nil || handler == nil {
		return
	}
	group.GET("/pricing/fallback", handler.Get)
	group.PUT("/pricing/fallback", handler.Update)
}

func (h *PricingFallbackHandler) Get(c *gin.Context) {
	organizationID, ok := pricingFallbackOrganizationIDFromContext(c)
	if !ok {
		return
	}
	config, err := LoadPricingFallbackConfig(c.Request.Context(), h.db, organizationID)
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, err.Error())
		return
	}
	response.Success(c, config)
}

func (h *PricingFallbackHandler) Update(c *gin.Context) {
	var req UpdatePricingFallbackRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		response.Fail(c, response.ErrInvalidParams)
		return
	}
	organizationID, ok := pricingFallbackOrganizationIDFromContext(c)
	if !ok {
		return
	}
	config, err := SavePricingFallbackConfig(c.Request.Context(), h.db, organizationID, req, c.GetString("account_id"))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParams, err.Error())
		return
	}
	response.Success(c, config)
}

func pricingFallbackOrganizationIDFromContext(c *gin.Context) (uuid.UUID, bool) {
	organizationID, err := uuid.Parse(strings.TrimSpace(c.GetString("organization_id")))
	if err != nil {
		response.FailWithMessage(c, response.ErrInvalidParam, "invalid organization_id")
		return uuid.Nil, false
	}
	return organizationID, true
}

func DefaultPricingFallbackRules() []PricingFallbackRule {
	return []PricingFallbackRule{
		tokenFallbackRule("token.chat.input.default", PricingOperationChat, PricingMeterInputToken, "1"),
		tokenFallbackRule("token.chat.output.default", PricingOperationChat, PricingMeterOutputToken, "2"),
		tokenFallbackRule("token.embedding.input.default", PricingOperationEmbedding, PricingMeterInputToken, "1"),
		tokenFallbackRule("token.rerank.input.default", PricingOperationRerank, PricingMeterInputToken, "1"),
		imageFallbackRule("image.qwen.default", "qwen", 160),
		imageFallbackRule("image.doubao.default", "doubao", 100),
		imageFallbackRule("image.openai.default", "openai", 200),
		imageFallbackRule("image.gcp.default", "gcp", 180),
		imageFallbackRule("image.midjourney.default", "midjourney", 300),
		imageFallbackRule("image.generic.default", "*", 200),
	}
}

func tokenFallbackRule(id string, operation PricingOperation, meter PricingMeter, price string) PricingFallbackRule {
	return PricingFallbackRule{
		ID:                  id,
		Enabled:             boolPtr(true),
		Operation:           operation,
		Meter:               meter,
		Unit:                "usd_per_1m_tokens",
		PriceUSDPer1MTokens: price,
		PricingSource:       PricingSourceCodeDefaultFallback,
	}
}

func imageFallbackRule(id string, provider string, credits int64) PricingFallbackRule {
	return PricingFallbackRule{
		ID:            id,
		Enabled:       boolPtr(true),
		Operation:     PricingOperationImage,
		Meter:         PricingMeterImage,
		Provider:      provider,
		Unit:          "credits_per_image",
		Credits:       credits,
		PricingSource: PricingSourceCodeDefaultFallback,
	}
}

func LoadPricingFallbackConfig(ctx context.Context, db *gorm.DB, organizationID uuid.UUID) (PricingFallbackConfigResponse, error) {
	defaultRules := DefaultPricingFallbackRules()
	config := PricingFallbackConfigResponse{
		Enabled:        false,
		DefaultRules:   defaultRules,
		OverrideRules:  []PricingFallbackRule{},
		EffectiveRules: effectivePricingFallbackRules(defaultRules, nil),
	}
	if db == nil || organizationID == uuid.Nil {
		return config, nil
	}

	record, found, err := loadPricingFallbackOverrideRecord(ctx, db, organizationID)
	if err != nil {
		if isMissingPricingTableError(err, "llm_pricing_fallback_overrides") {
			return config, nil
		}
		return PricingFallbackConfigResponse{}, err
	}
	if found {
		config.Enabled = record.Enabled
		var rules []PricingFallbackRule
		if len(record.Rules) > 0 && string(record.Rules) != "null" {
			if err := json.Unmarshal(record.Rules, &rules); err != nil {
				return PricingFallbackConfigResponse{}, fmt.Errorf("invalid pricing fallback override rules: %w", err)
			}
		}
		config.OverrideRules = markPricingRuleSource(rules, PricingSourceAdminFallback)
	}
	config.EffectiveRules = effectivePricingFallbackRules(defaultRules, config.OverrideRules)
	return config, nil
}

func SavePricingFallbackConfig(ctx context.Context, db *gorm.DB, organizationID uuid.UUID, req UpdatePricingFallbackRequest, updatedBy string) (PricingFallbackConfigResponse, error) {
	if db == nil {
		return PricingFallbackConfigResponse{}, fmt.Errorf("database is not configured")
	}
	if organizationID == uuid.Nil {
		return PricingFallbackConfigResponse{}, fmt.Errorf("organization_id is required")
	}
	rules, err := normalizePricingFallbackOverrideRules(req.OverrideRules)
	if err != nil {
		return PricingFallbackConfigResponse{}, err
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return PricingFallbackConfigResponse{}, fmt.Errorf("marshal pricing fallback override rules: %w", err)
	}
	now := time.Now().UTC()
	values := map[string]interface{}{
		"organization_id": organizationID,
		"enabled":         req.Enabled,
		"rules":           datatypes.JSON(raw),
		"updated_by":      strings.TrimSpace(updatedBy),
		"created_at":      now,
		"updated_at":      now,
	}
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "organization_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"enabled":    req.Enabled,
				"rules":      datatypes.JSON(raw),
				"updated_by": strings.TrimSpace(updatedBy),
				"updated_at": now,
			}),
		}).
		Model(&pricingFallbackOverrideRecord{}).
		Create(values).Error; err != nil {
		return PricingFallbackConfigResponse{}, fmt.Errorf("save pricing fallback override: %w", err)
	}
	return LoadPricingFallbackConfig(ctx, db, organizationID)
}

func loadPricingFallbackOverrideRecord(ctx context.Context, db *gorm.DB, organizationID uuid.UUID) (*pricingFallbackOverrideRecord, bool, error) {
	var record pricingFallbackOverrideRecord
	err := db.WithContext(ctx).
		Where("organization_id = ?", organizationID).
		First(&record).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return &record, true, nil
}

func effectivePricingFallbackRules(defaultRules, overrideRules []PricingFallbackRule) []PricingFallbackRule {
	effective := make([]PricingFallbackRule, 0, len(overrideRules)+len(defaultRules))
	seen := make(map[string]struct{}, len(overrideRules))
	for _, rule := range markPricingRuleSource(overrideRules, PricingSourceAdminFallback) {
		key := pricingFallbackRuleTargetKey(rule)
		if _, ok := seen[key]; ok {
			continue
		}
		effective = append(effective, rule)
		seen[key] = struct{}{}
	}
	for _, rule := range markPricingRuleSource(defaultRules, PricingSourceCodeDefaultFallback) {
		key := pricingFallbackRuleTargetKey(rule)
		if _, ok := seen[key]; ok {
			continue
		}
		effective = append(effective, rule)
		seen[key] = struct{}{}
	}
	return effective
}

func pricingFallbackRuleTargetKey(rule PricingFallbackRule) string {
	parts := []string{
		strings.ToLower(strings.TrimSpace(string(rule.Operation))),
		strings.ToLower(strings.TrimSpace(string(rule.Meter))),
		normalizePricingFallbackTargetPart(rule.Provider),
		normalizePricingFallbackTargetPart(rule.Model),
		normalizePricingFallbackTargetPart(rule.Size),
		normalizePricingFallbackTargetPart(rule.Quality),
		normalizePricingFallbackTargetPart(rule.Style),
	}
	return strings.Join(parts, "\x00")
}

func normalizePricingFallbackTargetPart(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" || value == "*" {
		return "*"
	}
	return value
}

func markPricingRuleSource(rules []PricingFallbackRule, source PricingSource) []PricingFallbackRule {
	out := make([]PricingFallbackRule, 0, len(rules))
	for _, rule := range rules {
		rule.PricingSource = source
		out = append(out, rule)
	}
	return out
}

func normalizePricingFallbackOverrideRules(rules []PricingFallbackRule) ([]PricingFallbackRule, error) {
	normalized := make([]PricingFallbackRule, 0, len(rules))
	seenIDs := make(map[string]struct{}, len(rules))
	seenTargets := make(map[string]struct{}, len(rules))
	for i, rule := range rules {
		rule.ID = strings.TrimSpace(rule.ID)
		if rule.ID == "" {
			return nil, fmt.Errorf("override_rules[%d].id is required", i)
		}
		if _, ok := seenIDs[rule.ID]; ok {
			return nil, fmt.Errorf("override_rules[%d].id is duplicated", i)
		}
		seenIDs[rule.ID] = struct{}{}
		if rule.Enabled == nil {
			rule.Enabled = boolPtr(true)
		}
		rule.Operation = PricingOperation(strings.TrimSpace(string(rule.Operation)))
		rule.Meter = PricingMeter(strings.TrimSpace(string(rule.Meter)))
		rule.Provider = strings.TrimSpace(rule.Provider)
		rule.Model = strings.TrimSpace(rule.Model)
		rule.Size = strings.TrimSpace(rule.Size)
		rule.Quality = strings.TrimSpace(rule.Quality)
		rule.Style = strings.TrimSpace(rule.Style)
		rule.Unit = strings.TrimSpace(rule.Unit)
		rule.PriceUSDPer1MTokens = strings.TrimSpace(rule.PriceUSDPer1MTokens)
		rule.PricingSource = PricingSourceAdminFallback
		if err := validatePricingFallbackRule(rule, i); err != nil {
			return nil, err
		}
		targetKey := pricingFallbackRuleTargetKey(rule)
		if _, ok := seenTargets[targetKey]; ok {
			return nil, fmt.Errorf("override_rules[%d] target is duplicated", i)
		}
		seenTargets[targetKey] = struct{}{}
		normalized = append(normalized, rule)
	}
	return normalized, nil
}

func validatePricingFallbackRule(rule PricingFallbackRule, index int) error {
	switch rule.Operation {
	case PricingOperationChat, PricingOperationEmbedding, PricingOperationRerank, PricingOperationImage:
	default:
		return fmt.Errorf("override_rules[%d].operation is invalid", index)
	}
	switch rule.Meter {
	case PricingMeterInputToken, PricingMeterOutputToken:
		if rule.Operation == PricingOperationImage {
			return fmt.Errorf("override_rules[%d].meter is invalid for image pricing", index)
		}
		if rule.PriceUSDPer1MTokens == "" {
			return fmt.Errorf("override_rules[%d].price_usd_per_1m_tokens is required", index)
		}
		price, err := decimal.NewFromString(rule.PriceUSDPer1MTokens)
		if err != nil || price.IsNegative() {
			return fmt.Errorf("override_rules[%d].price_usd_per_1m_tokens is invalid", index)
		}
	case PricingMeterImage:
		if rule.Operation != PricingOperationImage {
			return fmt.Errorf("override_rules[%d].meter is invalid for token pricing", index)
		}
		if rule.Credits <= 0 {
			return fmt.Errorf("override_rules[%d].credits must be greater than zero", index)
		}
	default:
		return fmt.Errorf("override_rules[%d].meter is invalid", index)
	}
	return nil
}

func pricingFallbackRuleEnabled(rule PricingFallbackRule) bool {
	return rule.Enabled == nil || *rule.Enabled
}

func parseFallbackTokenPrice(rule PricingFallbackRule) (decimal.Decimal, error) {
	price, err := decimal.NewFromString(strings.TrimSpace(rule.PriceUSDPer1MTokens))
	if err != nil {
		return decimal.Zero, fmt.Errorf("invalid fallback token price for rule %q: %w", rule.ID, err)
	}
	if price.IsNegative() {
		return decimal.Zero, fmt.Errorf("invalid fallback token price for rule %q: must be greater than or equal to zero", rule.ID)
	}
	return price, nil
}

func boolPtr(v bool) *bool {
	return &v
}
