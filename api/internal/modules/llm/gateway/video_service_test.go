package gateway

import (
	"testing"

	"github.com/google/uuid"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"gorm.io/datatypes"
)

func TestVideoPriceForSelectionMatchesFlatTokenRates(t *testing.T) {
	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{
			ID:       uuid.New(),
			Provider: "doubao",
			Model:    "doubao-seedance-2-0-mini-260615",
			Currency: "CNY",
			Pricing: datatypes.JSON([]byte(`{
				"video_generation": {
					"billing_unit": "million_video_tokens",
					"rates": [
						{"resolution":"480p","input_video":true,"price_per_million_tokens":14},
						{"resolution":"480p","input_video":false,"price_per_million_tokens":23}
					]
				}
			}`)),
		},
	}
	req := &adapter.VideoTaskRequest{AdditionalParameters: map[string]interface{}{
		"resolution":  "480p",
		"input_video": true,
	}}

	rate, resolution, inputVideo, mode, ok, err := videoPriceForSelection(selection, req, &adapter.VideoResponse{})
	if err != nil {
		t.Fatalf("videoPriceForSelection() error = %v", err)
	}
	if !ok || rate.String() != "14" || resolution != "480p" || !inputVideo || mode != "million_video_tokens" {
		t.Fatalf("videoPriceForSelection() = rate=%s resolution=%q inputVideo=%v mode=%q ok=%v", rate, resolution, inputVideo, mode, ok)
	}
}

func TestVideoPriceForSelectionMatchesPerSecondRates(t *testing.T) {
	selection := &ProviderSelection{
		Model: llmmodel.LLMModel{
			ID:       uuid.New(),
			Provider: "example",
			Model:    "video-per-second",
			Currency: "USD",
			Pricing: datatypes.JSON([]byte(`{
				"video_generation": {
					"billing_unit": "second",
					"rates": [
						{"resolution":"720p","price_per_second":0.02}
					]
				}
			}`)),
		},
	}
	req := &adapter.VideoTaskRequest{AdditionalParameters: map[string]interface{}{"resolution": "720p"}}

	rate, resolution, _, mode, ok, err := videoPriceForSelection(selection, req, &adapter.VideoResponse{})
	if err != nil {
		t.Fatalf("videoPriceForSelection() error = %v", err)
	}
	if !ok || rate.String() != "0.02" || resolution != "720p" || mode != "second" {
		t.Fatalf("videoPriceForSelection() = rate=%s resolution=%q mode=%q ok=%v", rate, resolution, mode, ok)
	}
}

func TestVideoPricingTokenCountReadsVideoTokenFields(t *testing.T) {
	resp := &adapter.VideoResponse{Raw: map[string]interface{}{
		"usage": map[string]interface{}{
			"completion_tokens": float64(12345),
			"total_tokens":      float64(12345),
		},
	}}
	if got := videoPricingTokenCount(nil, resp); got != 12345 {
		t.Fatalf("videoPricingTokenCount() = %d, want 12345", got)
	}
}

func TestVideoResponseFailureErrorPreservesUpstreamErrorMessage(t *testing.T) {
	resp := &adapter.VideoResponse{
		Status: "failed",
		Raw: map[string]interface{}{
			"error": map[string]interface{}{
				"code":    "InvalidParameter",
				"message": "Error while downloading image, error: expected the width to be at least 300px",
			},
		},
	}

	err := videoResponseFailureError(resp, "upstream video task failed")
	if err == nil {
		t.Fatal("videoResponseFailureError() error = nil")
	}
	const want = "upstream error: Error while downloading image, error: expected the width to be at least 300px"
	if err.Error() != want {
		t.Fatalf("videoResponseFailureError() = %q, want %q", err.Error(), want)
	}
}

func TestVideoCreateResponseRequiresTaskIDEvenWhenHTTP200BodyHasError(t *testing.T) {
	resp := &adapter.VideoResponse{
		Raw: map[string]interface{}{
			"error": map[string]interface{}{
				"message": "Error while downloading image",
			},
		},
	}

	if videoResponseHasTaskID(resp) {
		t.Fatal("videoResponseHasTaskID() = true, want false for response without task id")
	}
	if videoResponseFailureError(resp, "upstream video task id is empty").Error() != "upstream error: Error while downloading image" {
		t.Fatalf("videoResponseFailureError() did not preserve HTTP 200 error body message")
	}
}
