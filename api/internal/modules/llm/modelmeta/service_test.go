package modelmeta

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
)

func TestModelMetaDataPriceConfiguredFromJSON(t *testing.T) {
	var explicitZero ModelMetaData
	if err := json.Unmarshal([]byte(`{"input_price":0,"output_price":0}`), &explicitZero); err != nil {
		t.Fatalf("unmarshal explicit zero: %v", err)
	}
	if explicitZero.InputPrice == nil || explicitZero.OutputPrice == nil {
		t.Fatalf("explicit zero prices should be present")
	}
	publishedZero := publishedModelFromMeta(&explicitZero)
	if !publishedZero.InputPriceConfigured || !publishedZero.OutputPriceConfigured {
		t.Fatalf("explicit zero prices should be marked configured")
	}
	if publishedZero.InputPrice != 0 || publishedZero.OutputPrice != 0 {
		t.Fatalf("explicit zero prices = %v/%v, want 0/0", publishedZero.InputPrice, publishedZero.OutputPrice)
	}

	var missing ModelMetaData
	if err := json.Unmarshal([]byte(`{}`), &missing); err != nil {
		t.Fatalf("unmarshal missing prices: %v", err)
	}
	if missing.InputPrice != nil || missing.OutputPrice != nil {
		t.Fatalf("missing prices should stay nil")
	}
	publishedMissing := publishedModelFromMeta(&missing)
	if publishedMissing.InputPriceConfigured || publishedMissing.OutputPriceConfigured {
		t.Fatalf("missing prices should not be marked configured")
	}
}

func TestModelMetaDiffDetectsPriceConfiguredStateChange(t *testing.T) {
	zero := 0.0
	local := &llmmodel.LLMModel{
		InputPrice:            decimal.Zero,
		OutputPrice:           decimal.Zero,
		InputPriceConfigured:  false,
		OutputPriceConfigured: true,
		CachedInputPrice:      decimal.Zero,
		InputModalities:       llmmodel.JSONArray{},
		OutputModalities:      llmmodel.JSONArray{},
		UseCases:              llmmodel.StringArray{},
		SupportedParameters:   llmmodel.ParameterDefinitions{},
		ConfigParameters:      llmmodel.ConfigParameters{},
	}
	remote := &ModelMetaData{
		InputPrice:  &zero,
		OutputPrice: nil,
	}
	svc := &Service{}

	if !svc.hasChanges(local, remote) {
		t.Fatalf("hasChanges = false, want true when configured flags differ")
	}
	fields := svc.computeDiffFields(local, remote)
	if !hasDiffField(fields, "input_price_configured") {
		t.Fatalf("diff fields = %#v, want input_price_configured", fields)
	}
	if !hasDiffField(fields, "output_price_configured") {
		t.Fatalf("diff fields = %#v, want output_price_configured", fields)
	}
}

func hasDiffField(fields []DiffField, name string) bool {
	for _, field := range fields {
		if field.Field == name {
			return true
		}
	}
	return false
}
