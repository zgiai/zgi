package gateway

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestAuthoritativeQuoteFloatRoundTripDoesNotIncreaseCredits(t *testing.T) {
	prices := []string{"0.01", "0.1", "0.15", "0.2", "0.3", "0.6", "1", "1.25", "2", "2.5", "3", "5", "10", "15", "60"}
	tokenCounts := []int{1, 2, 3, 7, 10, 11, 99, 100, 101, 999, 1000, 1001, 4096, 8191, 32768, 100000, 1000000}
	oneMillion := decimal.NewFromInt(1_000_000)
	for _, priceText := range prices {
		price := decimal.RequireFromString(priceText)
		for _, tokens := range tokenCounts {
			quotedUSD := price.Mul(decimal.NewFromInt(int64(tokens))).Div(oneMillion)
			quotedCredits := creditsFromUSD(quotedUSD)
			derivedPrice, resolved, err := deriveAuthoritativeTokenPrice(quotedUSD.InexactFloat64(), tokens)
			if err != nil || !resolved {
				t.Fatalf("derive price=%s tokens=%d: resolved=%v err=%v", priceText, tokens, resolved, err)
			}
			roundTripCredits := creditsFromUSD(derivedPrice.Mul(decimal.NewFromInt(int64(tokens))).Div(oneMillion))
			if roundTripCredits > quotedCredits {
				t.Fatalf("float round trip increased credits: price=%s tokens=%d quoted=%d roundtrip=%d usd=%s float=%.18g derived=%s", priceText, tokens, quotedCredits, roundTripCredits, quotedUSD, quotedUSD.InexactFloat64(), derivedPrice)
			}
		}
	}
}

func TestAuthoritativeDualQuoteFloatRoundTripDoesNotIncreaseCredits(t *testing.T) {
	prices := []string{"0.01", "0.1", "0.15", "0.3", "0.6", "1", "1.25", "2.5", "3", "10", "15", "60"}
	tokenCounts := []int{1, 3, 7, 99, 1000, 4096, 8191, 32768, 1000000}
	oneMillion := decimal.NewFromInt(1_000_000)
	for _, inputPriceText := range prices {
		inputPrice := decimal.RequireFromString(inputPriceText)
		for _, outputPriceText := range prices {
			outputPrice := decimal.RequireFromString(outputPriceText)
			for _, promptTokens := range tokenCounts {
				for _, completionTokens := range tokenCounts {
					inputUSD := inputPrice.Mul(decimal.NewFromInt(int64(promptTokens))).Div(oneMillion)
					outputUSD := outputPrice.Mul(decimal.NewFromInt(int64(completionTokens))).Div(oneMillion)
					quotedCredits := creditsFromUSD(inputUSD.Add(outputUSD))

					derivedInput, _, err := deriveAuthoritativeTokenPrice(inputUSD.InexactFloat64(), promptTokens)
					if err != nil {
						t.Fatalf("derive input price=%s tokens=%d: %v", inputPriceText, promptTokens, err)
					}
					derivedOutput, _, err := deriveAuthoritativeTokenPrice(outputUSD.InexactFloat64(), completionTokens)
					if err != nil {
						t.Fatalf("derive output price=%s tokens=%d: %v", outputPriceText, completionTokens, err)
					}
					roundTripUSD := derivedInput.Mul(decimal.NewFromInt(int64(promptTokens))).Div(oneMillion).
						Add(derivedOutput.Mul(decimal.NewFromInt(int64(completionTokens))).Div(oneMillion))
					roundTripCredits := creditsFromUSD(roundTripUSD)
					if roundTripCredits > quotedCredits {
						t.Fatalf("dual float round trip increased credits: input=%s/%d output=%s/%d quoted=%d roundtrip=%d", inputPriceText, promptTokens, outputPriceText, completionTokens, quotedCredits, roundTripCredits)
					}
				}
			}
		}
	}
}
