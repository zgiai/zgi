package channel_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/zgiai/ginext/internal/modules/llm/channel/dto"
)

// TestChannelViewFields tests that ChannelView includes expected fields and excludes is_official
func TestChannelViewFields(t *testing.T) {
	view := dto.ChannelView{
		ID:              uuid.New(),
		Name:            "Test Channel",
		Type:            "PRIVATE",
		ChannelProvider: "openai",
		Models:          []string{"gpt-4o"},
		IsEnabled:       true,
		Priority:        10,
		Weight:          50,
		CreatedAt:       1700000000,
		UpdatedAt:       1700000000,
	}

	data, err := json.Marshal(view)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	assert.Contains(t, result, "type")
	assert.Equal(t, "PRIVATE", result["type"])
	assert.Contains(t, result, "channel_provider")
	assert.Equal(t, "openai", result["channel_provider"])
	assert.NotContains(t, result, "supported_protocols")
	assert.NotContains(t, result, "is_official")
}

// TestPlatformChannelViewFields tests that PlatformChannelView contains per-route fields
func TestPlatformChannelViewFields(t *testing.T) {
	view := dto.PlatformChannelView{
		ID:         "test-uuid",
		Name:       "ZGI Cloud OpenAI",
		Provider:   "openai",
		ModelCount: 5,
		Priority:   100,
		Weight:     50,
		IsEnabled:  true,
		CreatedAt:  1700000000,
		UpdatedAt:  1700000000,
	}

	data, err := json.Marshal(view)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	// Should have per-route fields for load balancing
	assert.Contains(t, result, "id")
	assert.Contains(t, result, "name")
	assert.Contains(t, result, "model_count")
	assert.Contains(t, result, "priority")
	assert.Contains(t, result, "weight")
	assert.Contains(t, result, "is_enabled")

	// Should NOT have sensitive or irrelevant fields
	assert.NotContains(t, result, "type")
	assert.NotContains(t, result, "is_official")
	assert.NotContains(t, result, "api_key_masked")
	assert.NotContains(t, result, "api_base_url")
	assert.NotContains(t, result, "auto_ban")
	assert.NotContains(t, result, "models")
}

// TestChannelListResponseStructure tests the private channel list response (no ZGI_CLOUD)
func TestChannelListResponseStructure(t *testing.T) {
	resp := dto.ChannelListResponse{
		Channels: []*dto.ChannelView{
			{
				ID:              uuid.New(),
				Name:            "My Anthropic",
				ChannelProvider: "anthropic",
				Models:          []string{"claude-3-opus"},
				IsEnabled:       true,
				Priority:        50,
				Weight:          100,
				CreatedAt:       1700000000,
				UpdatedAt:       1700000000,
			},
		},
		Total:    1,
		Page:     1,
		PageSize: 20,
	}

	data, err := json.Marshal(resp)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	channels, ok := result["data"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, channels, 1)

	privateCh := channels[0].(map[string]interface{})
	assert.Equal(t, "anthropic", privateCh["channel_provider"])
	assert.NotContains(t, privateCh, "supported_protocols")
	assert.NotContains(t, privateCh, "is_official")

	// No summary block
	assert.NotContains(t, result, "summary")
	assert.Contains(t, result, "page")
	assert.Contains(t, result, "page_size")
}

// TestPlatformChannelListResponseStructure tests the platform channel list response
func TestPlatformChannelListResponseStructure(t *testing.T) {
	resp := dto.PlatformChannelListResponse{
		Channels: []*dto.PlatformChannelView{
			{
				ID:         "uuid-1",
				Name:       "ZGI Cloud OpenAI",
				Provider:   "openai",
				ModelCount: 5,
				Priority:   100,
				Weight:     50,
				IsEnabled:  true,
				CreatedAt:  1700000000,
				UpdatedAt:  1700000000,
			},
			{
				ID:         "uuid-2",
				Name:       "ZGI Cloud Anthropic",
				Provider:   "anthropic",
				ModelCount: 3,
				Priority:   80,
				Weight:     30,
				IsEnabled:  true,
				CreatedAt:  1700000000,
				UpdatedAt:  1700000000,
			},
		},
		Total: 2,
	}

	data, err := json.Marshal(resp)
	assert.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	assert.NoError(t, err)

	// Should be a list response
	channels, ok := result["list"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, channels, 2)
	assert.Equal(t, float64(2), result["total"])

	// Each channel should have id/priority/weight
	ch := channels[0].(map[string]interface{})
	assert.Contains(t, ch, "id")
	assert.Contains(t, ch, "priority")
	assert.Contains(t, ch, "weight")
	assert.Contains(t, ch, "model_count")
	assert.NotContains(t, ch, "models")
	assert.NotContains(t, ch, "api_key_masked")
}

// MockChannelListHandler simulates the private channel list API response
func MockChannelListHandler(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"code":    "0",
		"message": "success",
		"data": dto.ChannelListResponse{
			Channels: []*dto.ChannelView{
				{
					ID:              uuid.New(),
					Name:            "My OpenAI",
					ChannelProvider: "openai",
					Models:          []string{"gpt-4o"},
					IsEnabled:       true,
					Priority:        100,
					Weight:          100,
					CreatedAt:       1700000000,
					UpdatedAt:       1700000000,
				},
			},
			Total:    1,
			Page:     1,
			PageSize: 20,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// TestMockChannelListAPI tests the mock API response
func TestMockChannelListAPI(t *testing.T) {
	req := httptest.NewRequest("GET", "/console/api/llm/channels", nil)
	w := httptest.NewRecorder()

	MockChannelListHandler(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &result)
	assert.NoError(t, err)

	assert.Equal(t, "0", result["code"])

	data := result["data"].(map[string]interface{})
	channels := data["data"].([]interface{})
	assert.Len(t, channels, 1)

	channel := channels[0].(map[string]interface{})
	assert.Equal(t, "openai", channel["channel_provider"])
	assert.NotContains(t, channel, "supported_protocols")
	assert.NotContains(t, channel, "is_official")
	assert.NotContains(t, data, "summary")
}
