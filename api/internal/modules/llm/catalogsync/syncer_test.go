package catalogsync

import (
	"testing"

	"github.com/stretchr/testify/require"
	llmmodel "github.com/zgiai/zgi/api/internal/modules/llm/llmmodel/model"
	pb "github.com/zgiai/zgi/api/pkg/rpc/v1"
)

func TestPublishedModelCapabilityMapping(t *testing.T) {
	t.Run("endpoints", func(t *testing.T) {
		require.Nil(t, publishedModelEndpoints(nil))

		require.Equal(t, &llmmodel.ModelEndpoints{
			ChatCompletions:  true,
			Responses:        true,
			Realtime:         true,
			Assistants:       true,
			Batch:            true,
			Embeddings:       true,
			FineTuning:       true,
			ImageGeneration:  true,
			Vision:           true,
			SpeechGeneration: true,
			Transcription:    true,
			Translation:      true,
			Moderation:       true,
			Videos:           true,
			ImageEdit:        true,
		}, publishedModelEndpoints(&pb.CatalogModelEndpoints{
			ChatCompletions:  true,
			Responses:        true,
			Realtime:         true,
			Assistants:       true,
			Batch:            true,
			Embeddings:       true,
			FineTuning:       true,
			ImageGeneration:  true,
			Vision:           true,
			SpeechGeneration: true,
			Transcription:    true,
			Translation:      true,
			Moderation:       true,
			Videos:           true,
			ImageEdit:        true,
		}))
	})

	t.Run("features", func(t *testing.T) {
		require.Nil(t, publishedModelFeatures(nil))

		require.Equal(t, &llmmodel.ModelFeatures{
			Streaming:        true,
			FunctionCalling:  true,
			StructuredOutput: true,
			JsonMode:         true,
			Distillation:     true,
			Reasoning:        true,
			SystemPrompt:     true,
			Logprobs:         true,
			WebSearch:        true,
			FileSearch:       true,
			CodeInterpreter:  true,
			ComputerUse:      true,
			Mcp:              true,
			ReasoningEffort:  true,
			Attachment:       true,
		}, publishedModelFeatures(&pb.CatalogModelFeatures{
			Streaming:        true,
			FunctionCalling:  true,
			StructuredOutput: true,
			JsonMode:         true,
			Distillation:     true,
			Reasoning:        true,
			SystemPrompt:     true,
			Logprobs:         true,
			WebSearch:        true,
			FileSearch:       true,
			CodeInterpreter:  true,
			ComputerUse:      true,
			Mcp:              true,
			ReasoningEffort:  true,
			Attachment:       true,
		}))
	})

	t.Run("tools", func(t *testing.T) {
		require.Nil(t, publishedModelTools(nil))

		require.Equal(t, &llmmodel.ModelTools{
			WebSearch:         true,
			FileSearch:        true,
			ImageGeneration:   true,
			CodeInterpreter:   true,
			ComputerUse:       true,
			Mcp:               true,
			ParallelToolCalls: true,
		}, publishedModelTools(&pb.CatalogModelTools{
			WebSearch:         true,
			FileSearch:        true,
			ImageGeneration:   true,
			CodeInterpreter:   true,
			ComputerUse:       true,
			Mcp:               true,
			ParallelToolCalls: true,
		}))
	})

	t.Run("parameters", func(t *testing.T) {
		require.Nil(t, publishedModelParameters(nil))

		require.Equal(t, &llmmodel.ModelParameters{
			SupportsTemperature:      true,
			SupportsTopP:             true,
			SupportsPresencePenalty:  true,
			SupportsFrequencyPenalty: true,
			SupportsLogitBias:        true,
			SupportsSeed:             true,
			SupportsStop:             true,
			MaxStopSequences:         8,
		}, publishedModelParameters(&pb.CatalogModelParameters{
			Temperature:      true,
			TopP:             true,
			PresencePenalty:  true,
			FrequencyPenalty: true,
			LogitBias:        true,
			Seed:             true,
			Stop:             true,
			MaxStopSequences: 8,
		}))
	})
}

func TestCatalogFromResponseIncludesVendorDirectory(t *testing.T) {
	resp := &pb.GetPublishedCatalogResponse{
		Vendors: []*pb.CatalogVendor{{
			Vendor:       "openai",
			VendorName:   "OpenAI",
			CnName:       "OpenAI 中文名",
			EnName:       "OpenAI",
			Description:  "Console-managed vendor",
			Website:      "https://openai.com",
			CountryCode:  "US",
			Status:       "active",
			IsActive:     true,
			MetadataJson: `{"icon_key":"openai"}`,
		}},
	}

	catalog := catalogFromResponse(resp)
	require.Len(t, catalog.Vendors, 1)
	require.Equal(t, "openai", catalog.Vendors[0].Vendor)
	require.Equal(t, "OpenAI 中文名", catalog.Vendors[0].CNName)
	require.Equal(t, "openai", catalog.Vendors[0].Metadata["icon_key"])
}

func TestPublishedModelLifecycleFieldsMapping(t *testing.T) {
	resp := &pb.GetPublishedCatalogResponse{
		Version:     12,
		PublishedAt: 1700000000000,
		Models: []*pb.CatalogModel{
			{
				Provider:            "deepseek",
				Vendor:              "deepseek",
				Model:               "deepseek-chat",
				ModelName:           "DeepSeek Chat",
				Status:              "deprecated",
				ReplacementProvider: "deepseek",
				ReplacementModel:    "deepseek-v4-flash",
				DeprecationReason:   "Compatibility model is deprecated.",
			},
			{
				Provider:  "deepseek",
				Model:     "deepseek-old",
				ModelName: "DeepSeek Old",
				Status:    "deprecated",
			},
		},
	}

	catalog := catalogFromResponse(resp)
	require.Len(t, catalog.Models, 2)
	require.Equal(t, "deepseek", catalog.Models[0].Vendor)
	require.Equal(t, "deepseek", catalog.Models[0].ReplacementProvider)
	require.Equal(t, "deepseek-v4-flash", catalog.Models[0].ReplacementModel)
	require.Equal(t, "Compatibility model is deprecated.", catalog.Models[0].DeprecationReason)
	require.Empty(t, catalog.Models[1].ReplacementProvider)
	require.Empty(t, catalog.Models[1].ReplacementModel)
	require.Empty(t, catalog.Models[1].DeprecationReason)
}

func TestPublishedModelPricingMapping(t *testing.T) {
	resp := &pb.GetPublishedCatalogResponse{
		Version:     13,
		PublishedAt: 1700000000000,
		Models: []*pb.CatalogModel{{
			Provider:    "openai",
			Model:       "gpt-5.5",
			PricingJson: `{"deployment_scope":"global","token_tiers":[{"min_input_tokens":0,"max_input_tokens":272000,"input_price_per_million":5,"output_price_per_million":30}]}`,
		}},
	}

	catalog := catalogFromResponse(resp)
	require.Len(t, catalog.Models, 1)
	require.JSONEq(t, resp.Models[0].GetPricingJson(), string(catalog.Models[0].Pricing))
}

func TestPublishedModelConfigPayloadCarriesDefaultParameters(t *testing.T) {
	resp := &pb.GetPublishedCatalogResponse{
		Version:     14,
		PublishedAt: 1700000000000,
		Models: []*pb.CatalogModel{{
			Provider: "doubao",
			Model:    "doubao-seedance-2-0-mini-260615",
			ConfigParametersJson: `{
				"config_parameters":[{"name":"quality","template_key":"quality","type":"string","default":"standard"}],
				"default_parameters":{"capabilities":{"video":{"resolutions":["480p","720p"],"duration":{"min_seconds":4,"max_seconds":15,"step_seconds":1}}}}
			}`,
		}},
	}

	catalog := catalogFromResponse(resp)
	require.Len(t, catalog.Models, 1)
	require.JSONEq(t, `[{"name":"quality","template_key":"quality","type":"string","required":false,"default":"standard"}]`, string(catalog.Models[0].ConfigParameters))
	require.Equal(t, []interface{}{"480p", "720p"}, catalog.Models[0].DefaultParameters["capabilities"].(map[string]interface{})["video"].(map[string]interface{})["resolutions"])
}

func TestPublishedModelConfigParametersPreserveVoiceCatalog(t *testing.T) {
	const configParameters = `[{"name":"voice","template_key":"voice","label":{"en_US":"Voice"},"type":"string","help":{"en_US":"Select a voice"},"required":true,"default":"voice-a","options":["voice-a"],"option_labels":{"voice-a":{"en_US":"Voice A"}}}]`
	resp := &pb.GetPublishedCatalogResponse{
		Version:     14,
		PublishedAt: 1700000000000,
		Models: []*pb.CatalogModel{{
			Provider:             "doubao",
			Model:                "seed-tts-2.0",
			ConfigParametersJson: configParameters,
		}},
	}

	catalog := catalogFromResponse(resp)
	require.Len(t, catalog.Models, 1)
	params, err := llmmodel.NormalizeConfigParametersJSON(catalog.Models[0].ConfigParameters)
	require.NoError(t, err)
	require.Len(t, params, 1)
	require.Equal(t, []string{"voice-a"}, params[0].Options)
	require.Equal(t, "Voice A", params[0].OptionLabels["voice-a"].EnUS)
}
