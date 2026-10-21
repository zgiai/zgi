package pricing

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// ToolPricingContract is the additive v1 tools block shared with Console.
// Missing and null prices resolve to free for backward compatibility.
type ToolPricingContract struct {
	SchemaVersion  int              `json:"schema_version,omitempty"`
	BillingLane    string           `json:"billing_lane,omitempty"`
	PricingVersion string           `json:"pricing_version,omitempty"`
	Tools          ToolPricingTools `json:"tools,omitempty"`
}

type ToolPricingTools struct {
	Default  map[string]*ToolPrice            `json:"default,omitempty"`
	Channels map[string]map[string]*ToolPrice `json:"channels,omitempty"`
}

type ToolPrice struct {
	Amount      string `json:"amount"`
	Currency    string `json:"currency"`
	Unit        string `json:"unit"`
	PerQuantity int64  `json:"per_quantity"`
}

type ResolvedToolPrice struct {
	Tool           string
	ChannelID      string
	BillingLane    string
	PricingVersion string
	Price          ToolPrice
	Source         string
}

var toolPriceAmountPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]+)?$`)
var toolPriceCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

func ValidateToolPricingContract(raw json.RawMessage) error {
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	var contract ToolPricingContract
	if err := json.Unmarshal(raw, &contract); err != nil {
		return fmt.Errorf("decode pricing contract: %w", err)
	}
	if contract.SchemaVersion != 0 && contract.SchemaVersion != 1 {
		return fmt.Errorf("unsupported pricing schema_version %d", contract.SchemaVersion)
	}
	if lane := strings.TrimSpace(contract.BillingLane); lane != "" && lane != "platform" && lane != "private" {
		return fmt.Errorf("pricing billing_lane must be platform or private")
	}
	if err := validateToolPriceMap(contract.Tools.Default, "tools.default"); err != nil {
		return err
	}
	for channelID, prices := range contract.Tools.Channels {
		if strings.TrimSpace(channelID) == "" {
			return fmt.Errorf("tools.channels contains an empty channel id")
		}
		if err := validateToolPriceMap(prices, "tools.channels["+channelID+"]"); err != nil {
			return err
		}
	}
	return nil
}

func validateToolPriceMap(prices map[string]*ToolPrice, path string) error {
	for tool, price := range prices {
		if tool != "web_search" && tool != "file_search" && tool != "code_interpreter" {
			return fmt.Errorf("%s contains unsupported tool %q", path, tool)
		}
		if price == nil {
			continue
		}
		if !toolPriceAmountPattern.MatchString(strings.TrimSpace(price.Amount)) || !toolPriceCurrencyPattern.MatchString(strings.TrimSpace(price.Currency)) || price.PerQuantity <= 0 {
			return fmt.Errorf("%s.%s has invalid amount, currency, or per_quantity", path, tool)
		}
		if tool == "code_interpreter" && (price.Unit != "session" || price.PerQuantity != 1) {
			return fmt.Errorf("%s.%s must use session and per_quantity 1", path, tool)
		}
		if tool != "code_interpreter" && (price.Unit != "call" || (price.PerQuantity != 1 && price.PerQuantity != 1000)) {
			return fmt.Errorf("%s.%s must use call and per_quantity 1 or 1000", path, tool)
		}
	}
	return nil
}

func ResolveToolPrice(raw json.RawMessage, lane, channelID, tool string) (ResolvedToolPrice, error) {
	var contract ToolPricingContract
	if len(strings.TrimSpace(string(raw))) > 0 {
		if err := json.Unmarshal(raw, &contract); err != nil {
			return ResolvedToolPrice{}, fmt.Errorf("decode pricing contract: %w", err)
		}
	}
	if err := ValidateToolPricingContract(raw); err != nil {
		return ResolvedToolPrice{}, err
	}
	if contract.BillingLane != "" && strings.TrimSpace(lane) != "" && contract.BillingLane != strings.TrimSpace(lane) {
		return ResolvedToolPrice{}, fmt.Errorf("pricing billing_lane %q does not match requested lane %q", contract.BillingLane, strings.TrimSpace(lane))
	}
	resolved := ResolvedToolPrice{Tool: tool, ChannelID: strings.TrimSpace(channelID), BillingLane: strings.TrimSpace(lane), PricingVersion: contract.PricingVersion, Source: "free"}
	if channelPrices, ok := contract.Tools.Channels[resolved.ChannelID]; ok {
		if price, configured := channelPrices[tool]; configured {
			if price == nil {
				return resolved, nil
			}
			resolved.Price, resolved.Source = *price, "channel"
			return resolved, nil
		}
	}
	if price := contract.Tools.Default[tool]; price != nil {
		resolved.Price, resolved.Source = *price, "default"
		return resolved, nil
	}
	return resolved, nil
}
