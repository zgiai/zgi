package pricing

import (
	"encoding/json"
	"testing"
)

func TestNormalizeUseCaseResponsesMapsToTextChat(t *testing.T) {
	if got := NormalizeUseCase("responses"); got != "text-chat" {
		t.Fatalf("NormalizeUseCase(responses) = %q, want text-chat", got)
	}
}

func TestMissingPricingForUseCasesModerationRequiresChatTokens(t *testing.T) {
	missing := MissingPricingForUseCases([]string{"moderation"}, json.RawMessage(`{"rules":[]}`), LegacyScalarPrices{})
	if len(missing) != 2 || missing[0].Operation != "chat" || missing[0].Meter != "input_token" || missing[1].Meter != "output_token" {
		t.Fatalf("missing = %#v, want chat input/output for moderation", missing)
	}
}

func TestMissingPricingAcceptsAlternativeRegisteredMeters(t *testing.T) {
	search := MissingPricingForUseCases([]string{"rerank"}, json.RawMessage(`{
		"rules":[{"operation":"rerank","meter":"search","unit":"search","price":"2","configured":true}]
	}`), LegacyScalarPrices{})
	if len(search) != 0 {
		t.Fatalf("rerank with search pricing missing = %#v, want none", search)
	}
	stt := MissingPricingForUseCases([]string{"speech-to-text"}, json.RawMessage(`{
		"metered":[{"operation":"transcription","meter":"input_token","base_unit":"per_1m_tokens","price":{"amount":"1","currency":"USD","per_quantity":1000000}}]
	}`), LegacyScalarPrices{})
	if len(stt) != 0 {
		t.Fatalf("stt with token pricing missing = %#v, want none", stt)
	}
}
