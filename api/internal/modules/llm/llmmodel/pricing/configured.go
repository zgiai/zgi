package pricing

import (
	"encoding/json"

	"github.com/shopspring/decimal"
)

// ResolvedConfigured carries equivalent configured flags derived from structured
// pricing and legacy projections. Organization overrides are applied separately.
type ResolvedConfigured struct {
	Input        bool
	Output       bool
	CacheRead    bool
	CacheWrite   bool
	CacheWrite5m bool
	CacheWrite1h bool
}

// ResolveConfigured derives configured flags for official, custom, and private
// models from pricing JSON. Scalar columns are not consulted directly; callers
// must fold organization overrides before billing.
func ResolveConfigured(raw json.RawMessage) ResolvedConfigured {
	envelope := parsePricingEnvelope(raw)
	return ResolvedConfigured{
		Input:        resolveTokenMeterConfigured(envelope, "input_token"),
		Output:       resolveTokenMeterConfigured(envelope, "output_token"),
		CacheRead:    resolveTokenMeterConfigured(envelope, "cache_read_token"),
		CacheWrite:   resolveTokenMeterConfigured(envelope, "cache_write_token"),
		CacheWrite5m: resolveCacheWrite5mConfigured(envelope),
		CacheWrite1h: resolveTokenMeterConfigured(envelope, "cache_write_1h_token"),
	}
}

// ApplyOrganizationOverrides marks configured flags for organization price overrides.
func ApplyOrganizationOverrides(configured ResolvedConfigured, overrides OrganizationOverrides) ResolvedConfigured {
	if overrides.Input {
		configured.Input = true
	}
	if overrides.Output {
		configured.Output = true
	}
	if overrides.CacheRead {
		configured.CacheRead = true
	}
	if overrides.CacheWrite {
		configured.CacheWrite = true
	}
	if overrides.CacheWrite5m {
		configured.CacheWrite5m = true
	}
	if overrides.CacheWrite1h {
		configured.CacheWrite1h = true
	}
	return configured
}

// OrganizationOverrides tracks which token prices were overridden at the tenant level.
type OrganizationOverrides struct {
	Input        bool
	Output       bool
	CacheRead    bool
	CacheWrite   bool
	CacheWrite5m bool
	CacheWrite1h bool
}

// DecimalPtr returns a float64 pointer when the decimal represents an explicit price.
func DecimalPtr(value decimal.Decimal) *float64 {
	f, _ := value.Float64()
	return floatPtr(f)
}

func resolveTokenMeterConfigured(envelope pricingEnvelope, meter string) bool {
	if envelope.present {
		for _, rule := range envelope.Rules {
			if rule.Meter != meter {
				continue
			}
			if ruleConfigured(rule) {
				return true
			}
			if hasRulePrice(rule) {
				return true
			}
		}
	}
	if len(envelope.TokenTiers) == 0 {
		return false
	}
	tier := envelope.TokenTiers[0]
	switch meter {
	case "input_token":
		return tier.InputPricePerMillion != nil
	case "output_token":
		return tier.OutputPricePerMillion != nil
	case "cache_read_token":
		return tier.CachedInputPricePerMillion != nil
	case "cache_write_token":
		return tier.CacheWritePricePerMillion != nil
	case "cache_write_5m_token":
		return tier.CacheWrite5mPricePerMillion != nil
	case "cache_write_1h_token":
		return tier.CacheWrite1hPricePerMillion != nil
	default:
		return false
	}
}

func resolveCacheWrite5mConfigured(envelope pricingEnvelope) bool {
	if resolveTokenMeterConfigured(envelope, "cache_write_5m_token") {
		return true
	}
	return resolveTokenMeterConfigured(envelope, "cache_write_token")
}
