package gateway

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const defaultOrganizationRoutingStrategy = "stability"

const (
	routingStrategySpeed     = "speed"
	routingStrategyPrice     = "price"
	routingStrategyStability = "stability"
)

type routingPolicy struct {
	Strategy              string
	EstimatedInputTokens  int
	EstimatedOutputTokens int
	Operation             PricingOperation
}

type routingPolicyContextKey struct{}

type RoutingStrategyResolver struct {
	db *gorm.DB
}

func NewRoutingStrategyResolver(db *gorm.DB) *RoutingStrategyResolver {
	return &RoutingStrategyResolver{db: db}
}

func withRoutingTokenEstimate(ctx context.Context, inputTokens, outputTokens int, operation PricingOperation) context.Context {
	policy := routingPolicy{
		EstimatedInputTokens:  inputTokens,
		EstimatedOutputTokens: outputTokens,
		Operation:             operation,
	}
	return context.WithValue(ctx, routingPolicyContextKey{}, normalizeRoutingPolicy(policy))
}

func routingPolicyFromContext(ctx context.Context) routingPolicy {
	if ctx != nil {
		if policy, ok := ctx.Value(routingPolicyContextKey{}).(routingPolicy); ok {
			return normalizeRoutingPolicy(policy)
		}
	}
	return normalizeRoutingPolicy(routingPolicy{})
}

func normalizeRoutingPolicy(policy routingPolicy) routingPolicy {
	policy.Strategy = normalizeConsoleRoutingStrategy(policy.Strategy)
	if policy.EstimatedInputTokens == 0 && policy.EstimatedOutputTokens == 0 {
		policy.EstimatedInputTokens = 1000
		policy.EstimatedOutputTokens = 1000
	}
	if policy.EstimatedInputTokens < 0 {
		policy.EstimatedInputTokens = 1000
	}
	if policy.EstimatedOutputTokens < 0 {
		policy.EstimatedOutputTokens = 1000
	}
	if policy.Operation == "" {
		policy.Operation = PricingOperationChat
	}
	return policy
}

func (r *RoutingStrategyResolver) Resolve(ctx context.Context, organizationID uuid.UUID) routingPolicy {
	policy := routingPolicyFromContext(ctx)
	// Request context carries only workload estimates. The organization setting
	// is the sole authority for strategy; read failures must fail safe.
	policy.Strategy = defaultOrganizationRoutingStrategy
	if r == nil || r.db == nil || organizationID == uuid.Nil {
		return policy
	}

	var row struct {
		RoutingStrategy string `gorm:"column:llm_routing_strategy"`
	}
	if err := r.db.WithContext(ctx).
		Table("organizations").
		Select("llm_routing_strategy").
		Where("id = ?", organizationID.String()).
		Take(&row).Error; err != nil {
		return policy
	}
	policy.Strategy = normalizeConsoleRoutingStrategy(strings.TrimSpace(row.RoutingStrategy))
	return policy
}

func (s *llmGatewayServiceImpl) organizationRoutingStrategy(ctx context.Context, organizationID uuid.UUID) string {
	if s == nil {
		return defaultOrganizationRoutingStrategy
	}
	return NewRoutingStrategyResolver(s.db).Resolve(ctx, organizationID).Strategy
}
