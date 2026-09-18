package gateway

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"go.opentelemetry.io/otel/attribute"
	"gorm.io/datatypes"
)

func pricingSnapshotMap(snapshot datatypes.JSON) map[string]interface{} {
	values := map[string]interface{}{}
	if len(snapshot) > 0 && string(snapshot) != "null" {
		_ = json.Unmarshal(snapshot, &values)
	}
	return values
}

func pricingSnapshotString(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	raw, ok := values[key]
	if !ok || raw == nil {
		return ""
	}
	switch typed := raw.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(strings.Trim(fmtAny(typed), `"`))
	}
}

func fmtAny(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		data, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(data)
	}
}

func mergePricingSnapshot(bc *BillingContext, patch map[string]interface{}) {
	if bc == nil || len(patch) == 0 {
		return
	}
	values := pricingSnapshotMap(bc.PricingSnapshot)
	changed := false
	for key, value := range patch {
		if value == nil {
			continue
		}
		text := strings.TrimSpace(fmtAny(value))
		if text == "" {
			continue
		}
		if pricingSnapshotString(values, key) == text {
			continue
		}
		values[key] = text
		changed = true
	}
	if changed {
		bc.PricingSnapshot = buildPricingSnapshot(values)
	}
}

func billingContextRouteID(bc *BillingContext) string {
	if bc == nil {
		return ""
	}
	if bc.ChannelID != nil && *bc.ChannelID != uuid.Nil {
		return bc.ChannelID.String()
	}
	if bc.RouteID != nil && *bc.RouteID != uuid.Nil {
		return bc.RouteID.String()
	}
	return ""
}

// applyRouteAttributionSnapshot fills durable actual routing fields in
// pricing_snapshot when upstream settlement headers did not provide them.
func applyRouteAttributionSnapshot(bc *BillingContext, routeName string) {
	if bc == nil {
		return
	}
	values := pricingSnapshotMap(bc.PricingSnapshot)
	patch := map[string]interface{}{}

	// Platform-lane traffic is served through the ZGI Cloud aggregation route,
	// whose ChannelProvider is the platform adapter ("zgi-cloud"), not the
	// upstream supplier. Attribute the billed supplier instead, and do not
	// stamp the platform route name as the upstream channel: Console owns the
	// real channel identity and reports it through settlement headers when
	// available.
	providerFallback := strings.TrimSpace(bc.ChannelProvider)
	channelFallback := strings.TrimSpace(routeName)
	if bc.UseSystemProvider {
		providerFallback = firstNonEmptyString(strings.TrimSpace(bc.ProviderName), strings.TrimSpace(bc.ChannelProvider))
		channelFallback = ""
	}
	actualProvider := firstNonEmptyString(
		pricingSnapshotString(values, "actual_provider"),
		providerFallback,
	)
	if actualProvider != "" && pricingSnapshotString(values, "actual_provider") == "" {
		patch["actual_provider"] = actualProvider
	}

	routeID := billingContextRouteID(bc)
	if routeID != "" && pricingSnapshotString(values, "actual_channel_id") == "" {
		patch["actual_channel_id"] = routeID
	}

	actualChannelName := firstNonEmptyString(
		pricingSnapshotString(values, "actual_channel_name"),
		channelFallback,
	)
	if actualChannelName != "" && pricingSnapshotString(values, "actual_channel_name") == "" {
		patch["actual_channel_name"] = actualChannelName
	}

	if strings.TrimSpace(bc.ModelName) != "" && pricingSnapshotString(values, "actual_upstream_model") == "" {
		patch["actual_upstream_model"] = strings.TrimSpace(bc.ModelName)
	}

	mergePricingSnapshot(bc, patch)
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (b *BillingService) loadRouteName(ctx context.Context, routeID *uuid.UUID) string {
	if b == nil || b.db == nil || routeID == nil || *routeID == uuid.Nil {
		return ""
	}
	if !b.db.Migrator().HasTable("llm_routes") {
		return ""
	}
	var name string
	if err := b.db.WithContext(ctx).
		Table("llm_routes").
		Select("name").
		Where("id = ?", routeID.String()).
		Scan(&name).Error; err != nil {
		return ""
	}
	return strings.TrimSpace(name)
}

func (b *BillingService) enrichBillingContextCatalogFields(ctx context.Context, bc *BillingContext) {
	if b == nil || b.db == nil || bc == nil {
		return
	}
	if bc.ModelID != uuid.Nil && strings.TrimSpace(bc.ModelName) == "" && b.db.Migrator().HasTable("llm_models") {
		var row struct {
			Model    string
			Provider string
		}
		if err := b.db.WithContext(ctx).
			Table("llm_models").
			Select("name AS model, provider").
			Where("id = ?", bc.ModelID).
			Take(&row).Error; err == nil {
			bc.ModelName = strings.TrimSpace(row.Model)
			if strings.TrimSpace(bc.ProviderName) == "" {
				bc.ProviderName = strings.TrimSpace(row.Provider)
			}
		}
	}
	if bc.ProviderID != uuid.Nil && strings.TrimSpace(bc.ProviderName) == "" && b.db.Migrator().HasTable("llm_providers") {
		var provider string
		if err := b.db.WithContext(ctx).
			Table("llm_providers").
			Select("provider").
			Where("id = ?", bc.ProviderID).
			Scan(&provider).Error; err == nil {
			bc.ProviderName = strings.TrimSpace(provider)
		}
	}
}

func (b *BillingService) applyRouteAttributionWithLookup(ctx context.Context, bc *BillingContext) {
	if b == nil || bc == nil {
		return
	}
	routeID := bc.RouteID
	if routeID == nil {
		routeID = bc.ChannelID
	}
	applyRouteAttributionSnapshot(bc, b.loadRouteName(ctx, routeID))
}

func settlementFromQueryResult(result *settlementQueryResponse) *adapter.SettlementResult {
	if result == nil {
		return nil
	}
	settlement := &adapter.SettlementResult{
		SettlementID:   strings.TrimSpace(result.SettlementID),
		OfficialPoints: result.SettledCredits,
		Status:         strings.TrimSpace(result.SettlementStatus),
	}
	if len(result.PricingSnapshot) == 0 || !json.Valid(result.PricingSnapshot) {
		return settlement
	}
	values := map[string]interface{}{}
	if err := json.Unmarshal(result.PricingSnapshot, &values); err != nil {
		return settlement
	}
	for key, field := range map[string]string{
		"actual_channel_id":     "ActualChannelID",
		"actual_channel_name":   "ActualChannelName",
		"actual_provider":       "ActualProvider",
		"actual_protocol":       "ActualProtocol",
		"actual_upstream_model": "ActualUpstreamModel",
		"total_cost_usd":        "TotalCostUSD",
		"total_cost_cny":        "TotalCostCNY",
		"cny_per_usd":           "CNYPerUSD",
	} {
		if text := pricingSnapshotString(values, key); text != "" {
			switch field {
			case "ActualChannelID":
				settlement.ActualChannelID = text
			case "ActualChannelName":
				settlement.ActualChannelName = text
			case "ActualProvider":
				settlement.ActualProvider = text
			case "ActualProtocol":
				settlement.ActualProtocol = text
			case "ActualUpstreamModel":
				settlement.ActualUpstreamModel = text
			case "TotalCostUSD":
				settlement.TotalCostUSD = text
			case "TotalCostCNY":
				settlement.TotalCostCNY = text
			case "CNYPerUSD":
				settlement.CNYPerUSD = text
			}
		}
	}
	return settlement
}

func isMediaPricingOperation(operation PricingOperation) bool {
	switch operation {
	case PricingOperationImage, PricingOperationMusic, PricingOperationSpeech, PricingOperationTranscription, PricingOperationLyrics:
		return true
	default:
		return false
	}
}

func finalizeMediaTimingSnapshot(bc *BillingContext) {
	if bc == nil || !isMediaPricingOperation(bc.PricingOperation) {
		return
	}
	startedAt := bc.RequestCreatedAt.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
		bc.RequestCreatedAt = startedAt
	}
	finishedAt := bc.SettledAt.UTC()
	if finishedAt.IsZero() {
		finishedAt = time.Now().UTC()
		bc.SettledAt = finishedAt
	}
	if finishedAt.Before(startedAt) {
		finishedAt = startedAt
		bc.SettledAt = finishedAt
	}
	if bc.ResponseTime <= 0 {
		bc.ResponseTime = finishedAt.Sub(startedAt).Milliseconds()
	}
	mergePricingSnapshot(bc, map[string]interface{}{
		"media_started_at":   startedAt.Format(time.RFC3339Nano),
		"media_completed_at": finishedAt.Format(time.RFC3339Nano),
		"media_duration_ms":  bc.ResponseTime,
		"media_operation":    string(bc.PricingOperation),
	})
}

func routeAttributionAttributes(bc *BillingContext) []attribute.KeyValue {
	if bc == nil {
		return nil
	}
	values := pricingSnapshotMap(bc.PricingSnapshot)
	attrs := make([]attribute.KeyValue, 0, 5)
	for key, attributeKey := range map[string]string{
		"actual_channel_id":     "zgi.actual_channel_id",
		"actual_channel_name":   "zgi.actual_channel_name",
		"actual_provider":       "zgi.actual_provider",
		"actual_protocol":       "zgi.actual_protocol",
		"actual_upstream_model": "zgi.actual_upstream_model",
	} {
		if value := pricingSnapshotString(values, key); value != "" {
			attrs = append(attrs, attribute.String(attributeKey, value))
		}
	}
	return attrs
}
