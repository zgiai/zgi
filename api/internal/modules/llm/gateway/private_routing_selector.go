package gateway

import (
	"context"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	channelmodel "github.com/zgiai/zgi/api/internal/modules/llm/channel/model"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

const (
	routingMetricsWindow      = time.Hour
	routingRecentFailWindow   = 5 * time.Minute
	routingMaxSamplesPerRoute = 100
	routingMinSpeedSamples    = 3
)

type privateRoutingMetric struct {
	samples             int
	successes           int
	consecutiveFailures int
	recentFailures      int
	responseTimes       []float64
	priceUSD            *decimal.Decimal
}

type privateUsageBillRow struct {
	ChannelID        uuid.UUID `gorm:"column:channel_id"`
	Status           string    `gorm:"column:status"`
	ResponseTimeMS   int64     `gorm:"column:response_time_ms"`
	RequestCreatedAt time.Time `gorm:"column:request_created_at"`
}

type routeModelPriceRow struct {
	ID                    uuid.UUID       `gorm:"column:id"`
	Provider              string          `gorm:"column:provider"`
	InputPrice            decimal.Decimal `gorm:"column:input_price"`
	OutputPrice           decimal.Decimal `gorm:"column:output_price"`
	InputPriceConfigured  bool            `gorm:"column:input_price_configured"`
	OutputPriceConfigured bool            `gorm:"column:output_price_configured"`
}

type routeModelPriceOverrideRow struct {
	ModelID             uuid.UUID        `gorm:"column:model_id"`
	InputPriceOverride  *decimal.Decimal `gorm:"column:input_price_override"`
	OutputPriceOverride *decimal.Decimal `gorm:"column:output_price_override"`
}

func (r *ChannelRouter) selectRoutesByRoutingPolicy(
	ctx context.Context,
	organizationID uuid.UUID,
	modelName string,
	privateModel *llmmodel.CustomModel,
	routes []*channelmodel.LLMRoute,
	maxSelections int,
	policy routingPolicy,
) []*channelmodel.LLMRoute {
	if len(routes) == 0 || maxSelections <= 0 {
		return nil
	}

	metrics := r.loadPrivateRoutingMetrics(ctx, organizationID, modelName, routes)
	r.loadRoutePrices(ctx, organizationID, modelName, privateModel != nil, routes, policy, metrics)

	probeRoutes := make([]*channelmodel.LLMRoute, 0, len(routes))
	regularRoutes := make([]*channelmodel.LLMRoute, 0, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		if route.UpstreamProbe {
			probeRoutes = append(probeRoutes, route)
		} else {
			regularRoutes = append(regularRoutes, route)
		}
	}

	selected := make([]*channelmodel.LLMRoute, 0, maxSelections)
	if len(probeRoutes) > 0 {
		selected = append(selected, r.orderRoutesByRoutingPolicy(probeRoutes, metrics, policy, 1)...)
	}
	selected = append(selected, r.orderRoutesByRoutingPolicy(regularRoutes, metrics, policy, maxSelections-len(selected))...)
	return selected
}

func (r *ChannelRouter) orderRoutesByRoutingPolicy(routes []*channelmodel.LLMRoute, metrics map[uuid.UUID]*privateRoutingMetric, policy routingPolicy, limit int) []*channelmodel.LLMRoute {
	if len(routes) == 0 || limit <= 0 {
		return nil
	}
	groups := make(map[int][]*channelmodel.LLMRoute)
	priorities := make([]int, 0)
	for _, route := range routes {
		if _, ok := groups[route.Priority]; !ok {
			priorities = append(priorities, route.Priority)
		}
		groups[route.Priority] = append(groups[route.Priority], route)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(priorities)))

	result := make([]*channelmodel.LLMRoute, 0, min(limit, len(routes)))
	for _, priority := range priorities {
		group := groups[priority]
		r.rankRoutingGroup(group, metrics, policy)
		remaining := limit - len(result)
		if remaining <= 0 {
			break
		}
		if len(group) > remaining {
			group = group[:remaining]
		}
		result = append(result, group...)
	}
	return result
}

func (r *ChannelRouter) rankRoutingGroup(group []*channelmodel.LLMRoute, metrics map[uuid.UUID]*privateRoutingMetric, policy routingPolicy) {
	if len(group) <= 1 {
		return
	}
	neutralSpeed := neutralRoutingSpeed(group, metrics)
	less := func(left, right *channelmodel.LLMRoute) int {
		lm, rm := metrics[left.ID], metrics[right.ID]
		switch policy.Strategy {
		case routingStrategyPrice:
			if cmp := compareRoutePrice(lm, rm); cmp != 0 {
				return cmp
			}
			if cmp := compareRouteStability(lm, rm); cmp != 0 {
				return cmp
			}
		case routingStrategySpeed:
			if cmp := compareRouteSpeed(lm, rm, neutralSpeed); cmp != 0 {
				return cmp
			}
		default:
			if cmp := compareRouteStability(lm, rm); cmp != 0 {
				return cmp
			}
		}
		return 0
	}

	// Sort by policy first, then preserve weighted load balancing only inside
	// exact policy ties. A larger weight can never overtake a better score.
	sort.SliceStable(group, func(i, j int) bool { return less(group[i], group[j]) < 0 })
	for start := 0; start < len(group); {
		end := start + 1
		for end < len(group) && less(group[start], group[end]) == 0 && less(group[end], group[start]) == 0 {
			end++
		}
		weighted := r.selectWeightedRoutes(group[start:end], end-start)
		copy(group[start:end], weighted)
		start = end
	}
}

func compareRoutePrice(left, right *privateRoutingMetric) int {
	leftPrice, rightPrice := routingPrice(left), routingPrice(right)
	if leftPrice == nil || rightPrice == nil {
		if leftPrice == rightPrice {
			return 0
		}
		if leftPrice != nil {
			return -1
		}
		return 1
	}
	return leftPrice.Cmp(*rightPrice)
}

func compareRouteStability(left, right *privateRoutingMetric) int {
	leftScore, rightScore := routingStabilityScore(left), routingStabilityScore(right)
	if math.Abs(leftScore-rightScore) > 1e-9 {
		if leftScore > rightScore {
			return -1
		}
		return 1
	}
	if routingConsecutiveFailures(left) != routingConsecutiveFailures(right) {
		if routingConsecutiveFailures(left) < routingConsecutiveFailures(right) {
			return -1
		}
		return 1
	}
	if routingRecentFailures(left) != routingRecentFailures(right) {
		if routingRecentFailures(left) < routingRecentFailures(right) {
			return -1
		}
		return 1
	}
	return 0
}

func compareRouteSpeed(left, right *privateRoutingMetric, neutral float64) int {
	leftSpeed := effectiveRoutingSpeed(left, neutral)
	rightSpeed := effectiveRoutingSpeed(right, neutral)
	if math.Abs(leftSpeed-rightSpeed) <= 1e-9 {
		return 0
	}
	if leftSpeed < rightSpeed {
		return -1
	}
	return 1
}

func routingPrice(metric *privateRoutingMetric) *decimal.Decimal {
	if metric == nil {
		return nil
	}
	return metric.priceUSD
}

func routingStabilityScore(metric *privateRoutingMetric) float64 {
	if metric == nil {
		return 0.5
	}
	return float64(metric.successes+1) / float64(metric.samples+2)
}

func routingConsecutiveFailures(metric *privateRoutingMetric) int {
	if metric == nil {
		return 0
	}
	return metric.consecutiveFailures
}

func routingRecentFailures(metric *privateRoutingMetric) int {
	if metric == nil {
		return 0
	}
	return metric.recentFailures
}

func effectiveRoutingSpeed(metric *privateRoutingMetric, neutral float64) float64 {
	if metric == nil || len(metric.responseTimes) < routingMinSpeedSamples {
		return neutral
	}
	return trimmedMean(metric.responseTimes)
}

func neutralRoutingSpeed(routes []*channelmodel.LLMRoute, metrics map[uuid.UUID]*privateRoutingMetric) float64 {
	values := make([]float64, 0, len(routes))
	for _, route := range routes {
		metric := metrics[route.ID]
		if metric != nil && len(metric.responseTimes) >= routingMinSpeedSamples {
			values = append(values, trimmedMean(metric.responseTimes))
		}
	}
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	mid := len(values) / 2
	if len(values)%2 == 1 {
		return values[mid]
	}
	return (values[mid-1] + values[mid]) / 2
}

func trimmedMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted) >= 5 {
		sorted = sorted[1 : len(sorted)-1]
	}
	total := 0.0
	for _, value := range sorted {
		total += value
	}
	return total / float64(len(sorted))
}

func (r *ChannelRouter) loadPrivateRoutingMetrics(ctx context.Context, organizationID uuid.UUID, modelName string, routes []*channelmodel.LLMRoute) map[uuid.UUID]*privateRoutingMetric {
	result := make(map[uuid.UUID]*privateRoutingMetric, len(routes))
	ids := make([]uuid.UUID, 0, len(routes))
	for _, route := range routes {
		if route == nil {
			continue
		}
		result[route.ID] = &privateRoutingMetric{}
		if !isOfficialRoute(route) {
			ids = append(ids, route.ID)
		}
	}
	if r == nil || r.db == nil || len(ids) == 0 {
		return result
	}

	now := r.routingNow()
	var rows []privateUsageBillRow
	recentRows := r.db.WithContext(ctx).Table("llm_usage_bills").
		Select("channel_id, status, response_time_ms, request_created_at, ROW_NUMBER() OVER (PARTITION BY channel_id ORDER BY request_created_at DESC) AS sample_rank").
		Where("organization_id = ? AND channel_id IN ? AND model_name = ?", organizationID.String(), ids, strings.TrimSpace(modelName)).
		Where("billing_lane = ? AND use_system_provider = ? AND request_created_at >= ?", UsageBillingLanePrivate, false, now.Add(-routingMetricsWindow))
	err := r.db.WithContext(ctx).Table("(?) AS recent_private_usage", recentRows).
		Select("channel_id, status, response_time_ms, request_created_at").
		Where("sample_rank <= ?", routingMaxSamplesPerRoute).
		Order("channel_id ASC, request_created_at DESC").
		Find(&rows).Error
	if err != nil {
		return result
	}

	counts := make(map[uuid.UUID]int, len(ids))
	for _, row := range rows {
		metric := result[row.ChannelID]
		if metric == nil || counts[row.ChannelID] >= routingMaxSamplesPerRoute {
			continue
		}
		counts[row.ChannelID]++
		metric.samples++
		if row.Status == usageBillStatusSuccess {
			metric.successes++
		} else {
			if metric.successes == 0 {
				metric.consecutiveFailures++
			}
			if !row.RequestCreatedAt.Before(now.Add(-routingRecentFailWindow)) {
				metric.recentFailures++
			}
		}
		if row.ResponseTimeMS > 0 {
			metric.responseTimes = append(metric.responseTimes, float64(row.ResponseTimeMS))
		}
	}
	return result
}

func (r *ChannelRouter) loadRoutePrices(ctx context.Context, organizationID uuid.UUID, modelName string, custom bool, routes []*channelmodel.LLMRoute, policy routingPolicy, metrics map[uuid.UUID]*privateRoutingMetric) {
	if policy.Strategy != routingStrategyPrice || r == nil || r.db == nil {
		return
	}
	providers := make([]string, 0, len(routes))
	seen := make(map[string]struct{}, len(routes))
	for _, route := range routes {
		if route == nil || isOfficialRoute(route) {
			continue
		}
		provider := strings.TrimSpace(route.ChannelProvider)
		if provider == "" {
			continue
		}
		key := strings.ToLower(provider)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return
	}

	table := "llm_models"
	if custom {
		table = "llm_custom_models"
	}
	var rows []routeModelPriceRow
	query := r.db.WithContext(ctx).Table(table).
		Select("id, provider, input_price, output_price, input_price_configured, output_price_configured").
		Where("name = ? AND provider IN ? AND deleted_at IS NULL", strings.TrimSpace(modelName), providers)
	if custom {
		query = query.Where("organization_id = ?", organizationID.String())
	}
	if err := query.Find(&rows).Error; err != nil {
		return
	}

	if !custom && len(rows) > 0 {
		ids := make([]uuid.UUID, 0, len(rows))
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
		var overrides []routeModelPriceOverrideRow
		if err := r.db.WithContext(ctx).Table("llm_model_configs").
			Select("model_id, input_price_override, output_price_override").
			Where("organization_id = ? AND model_id IN ? AND deleted_at IS NULL", organizationID.String(), ids).
			Find(&overrides).Error; err == nil {
			byID := make(map[uuid.UUID]routeModelPriceOverrideRow, len(overrides))
			for _, override := range overrides {
				byID[override.ModelID] = override
			}
			for index := range rows {
				if override, ok := byID[rows[index].ID]; ok {
					if override.InputPriceOverride != nil {
						rows[index].InputPrice = *override.InputPriceOverride
						rows[index].InputPriceConfigured = true
					}
					if override.OutputPriceOverride != nil {
						rows[index].OutputPrice = *override.OutputPriceOverride
						rows[index].OutputPriceConfigured = true
					}
				}
			}
		}
	}

	prices := make(map[string]decimal.Decimal, len(rows))
	for _, row := range rows {
		requireOutput := policy.EstimatedOutputTokens > 0 && policy.Operation != PricingOperationEmbedding && policy.Operation != PricingOperationRerank
		if !row.InputPriceConfigured || (requireOutput && !row.OutputPriceConfigured) {
			continue
		}
		price := row.InputPrice.Mul(decimal.NewFromInt(int64(policy.EstimatedInputTokens))).Div(decimal.NewFromInt(1_000_000))
		if requireOutput {
			price = price.Add(row.OutputPrice.Mul(decimal.NewFromInt(int64(policy.EstimatedOutputTokens))).Div(decimal.NewFromInt(1_000_000)))
		}
		prices[strings.ToLower(strings.TrimSpace(row.Provider))] = price
	}
	for _, route := range routes {
		if route == nil {
			continue
		}
		if price, ok := prices[strings.ToLower(strings.TrimSpace(route.ChannelProvider))]; ok {
			value := price
			metrics[route.ID].priceUSD = &value
		}
	}
}

func (r *ChannelRouter) routingNow() time.Time {
	if r != nil && r.now != nil {
		return r.now().UTC()
	}
	return time.Now().UTC()
}
