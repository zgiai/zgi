package modelmeta

import (
	"encoding/json"
	"testing"

	"github.com/shopspring/decimal"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	"gorm.io/datatypes"
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

func TestNormalizeRemotePricePreservesOfficialTwelveDecimalPrice(t *testing.T) {
	got := normalizeRemotePrice(0.000180612345)
	want := decimal.RequireFromString("0.000180612345")
	if !got.Equal(want) {
		t.Fatalf("normalizeRemotePrice() = %s, want %s", got, want)
	}
}

func TestModelMetaDiffDetectsStructuredPricingChange(t *testing.T) {
	local := &llmmodel.LLMModel{
		Pricing:          datatypes.JSON(`{"deployment_scope":"global","price_per_image":0.2}`),
		InputModalities:  llmmodel.JSONArray{},
		OutputModalities: llmmodel.JSONArray{},
		UseCases:         llmmodel.StringArray{string(llmmodel.UseCaseTextChat)},
	}
	remote := &ModelMetaData{
		Pricing: json.RawMessage(`{"deployment_scope":"global","price_per_image":0.25}`),
	}
	svc := &Service{}

	if !svc.hasChanges(local, remote) {
		t.Fatal("hasChanges = false, want true when structured pricing differs")
	}
	if fields := svc.computeDiffFields(local, remote); !hasDiffField(fields, "pricing") {
		t.Fatalf("diff fields = %#v, want pricing", fields)
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

func TestModelMetaDiffDetectsDefaultParameterCapabilitiesChange(t *testing.T) {
	local := &llmmodel.LLMModel{
		UseCases:              llmmodel.StringArray{string(llmmodel.UseCaseVideoGen)},
		InputModalities:       llmmodel.JSONArray{"text", "image"},
		OutputModalities:      llmmodel.JSONArray{"video"},
		SupportedParameters:   llmmodel.ParameterDefinitions{},
		ConfigParameters:      llmmodel.ConfigParameters{},
		DefaultParameters:     llmmodel.JSONObject{},
		InputPrice:            decimal.Zero,
		OutputPrice:           decimal.Zero,
		CachedInputPrice:      decimal.Zero,
		InputPriceConfigured:  false,
		OutputPriceConfigured: false,
	}
	remote := &ModelMetaData{
		UseCases:         []string{string(llmmodel.UseCaseVideoGen)},
		InputModalities:  []string{"text", "image"},
		OutputModalities: []string{"video"},
		Capabilities: map[string]interface{}{
			"video": map[string]interface{}{
				"reference_modes": []interface{}{"auto", "first_last_frame"},
				"references": map[string]interface{}{
					"image_max_items": float64(2),
				},
			},
		},
	}
	svc := &Service{}

	if !svc.hasChanges(local, remote) {
		t.Fatal("hasChanges = false, want true when capabilities/default_parameters differ")
	}
	if fields := svc.computeDiffFields(local, remote); !hasDiffField(fields, "default_parameters") {
		t.Fatalf("diff fields = %#v, want default_parameters", fields)
	}
}

func TestModelMetaTopLevelVideoIsPublishedAsCapability(t *testing.T) {
	remote := &ModelMetaData{
		UseCases:         []string{string(llmmodel.UseCaseVideoGen)},
		InputModalities:  []string{"text", "image", "video", "audio"},
		OutputModalities: []string{"video", "audio"},
		Video: map[string]interface{}{
			"modes":       []interface{}{"first_last_frame", "omni_reference"},
			"resolutions": []interface{}{"480p", "720p", "1080p", "4k"},
			"duration": map[string]interface{}{
				"mode":         "range",
				"min_seconds":  float64(4),
				"max_seconds":  float64(15),
				"step_seconds": float64(1),
			},
			"audio": map[string]interface{}{
				"input":      true,
				"generation": true,
			},
		},
	}

	published := publishedModelFromMeta(remote)
	capabilities := published.DefaultParameters["capabilities"].(map[string]interface{})
	video := capabilities["video"].(map[string]interface{})

	if got := video["resolutions"]; got == nil {
		t.Fatalf("published video resolutions missing: %#v", published.DefaultParameters)
	}
	if got := video["duration"]; got == nil {
		t.Fatalf("published video duration missing: %#v", published.DefaultParameters)
	}

	local := &llmmodel.LLMModel{
		UseCases:              llmmodel.StringArray{string(llmmodel.UseCaseVideoGen)},
		InputModalities:       llmmodel.JSONArray{"text", "image", "video", "audio"},
		OutputModalities:      llmmodel.JSONArray{"video", "audio"},
		SupportedParameters:   llmmodel.ParameterDefinitions{},
		ConfigParameters:      llmmodel.ConfigParameters{},
		DefaultParameters:     llmmodel.JSONObject{},
		InputPrice:            decimal.Zero,
		OutputPrice:           decimal.Zero,
		CachedInputPrice:      decimal.Zero,
		InputPriceConfigured:  false,
		OutputPriceConfigured: false,
	}
	svc := &Service{}

	if !svc.hasChanges(local, remote) {
		t.Fatal("hasChanges = false, want true when top-level video capabilities differ")
	}
	if fields := svc.computeDiffFields(local, remote); !hasDiffField(fields, "default_parameters") {
		t.Fatalf("diff fields = %#v, want default_parameters", fields)
	}
}
