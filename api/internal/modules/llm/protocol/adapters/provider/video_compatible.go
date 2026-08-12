package provider

import (
	"encoding/json"
	"fmt"
	"strings"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

func buildOpenAICompatibleVideoPayload(request *adapter.VideoRequest) map[string]any {
	payload := map[string]any{}
	if request == nil {
		return payload
	}
	if strings.TrimSpace(request.Model) != "" {
		payload["model"] = request.Model
	}
	if strings.TrimSpace(request.Prompt) != "" {
		payload["prompt"] = request.Prompt
	}
	if strings.TrimSpace(request.ImageURL) != "" {
		payload["image_url"] = request.ImageURL
	}
	if len(request.ImageURLs) > 0 {
		payload["image_urls"] = request.ImageURLs
	}
	if strings.TrimSpace(request.FirstFrameURL) != "" {
		payload["first_frame_url"] = request.FirstFrameURL
	}
	if strings.TrimSpace(request.LastFrameURL) != "" {
		payload["last_frame_url"] = request.LastFrameURL
	}
	if strings.TrimSpace(request.VideoURL) != "" {
		payload["video_url"] = request.VideoURL
	}
	if strings.TrimSpace(request.AudioURL) != "" {
		payload["audio_url"] = request.AudioURL
	}
	if strings.TrimSpace(request.NegativePrompt) != "" {
		payload["negative_prompt"] = request.NegativePrompt
	}
	if strings.TrimSpace(request.Size) != "" {
		payload["size"] = request.Size
	}
	if strings.TrimSpace(request.Ratio) != "" {
		payload["ratio"] = request.Ratio
	}
	if strings.TrimSpace(request.Resolution) != "" {
		payload["resolution"] = request.Resolution
	}
	if request.Duration != nil {
		payload["duration"] = *request.Duration
	}
	if request.N != nil {
		payload["n"] = *request.N
	}
	if request.GenerateAudio != nil {
		payload["generate_audio"] = *request.GenerateAudio
	}
	if request.PromptExtend != nil {
		payload["prompt_extend"] = *request.PromptExtend
	}
	if request.Watermark != nil {
		payload["watermark"] = *request.Watermark
	}
	if strings.TrimSpace(request.CallbackURL) != "" {
		payload["callback_url"] = request.CallbackURL
	}
	if strings.TrimSpace(request.User) != "" {
		payload["user"] = request.User
	}
	for k, v := range request.AdditionalParameters {
		payload[k] = v
	}
	return payload
}

func decodeOpenAICompatibleVideoResponse(body []byte) (*adapter.VideoResponse, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, err
	}
	if message := openAICompatibleVideoErrorMessage(raw); message != "" {
		return nil, fmt.Errorf("upstream error: %s", message)
	}

	var response adapter.VideoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	response.Raw = raw
	if response.TaskID == "" {
		response.TaskID = response.ID
	}
	if response.VideoURL == "" && len(response.Data) > 0 {
		response.VideoURL = response.Data[0].URL
	}
	return &response, nil
}

func openAICompatibleVideoErrorMessage(raw map[string]interface{}) string {
	if raw == nil {
		return ""
	}
	if message := openAICompatibleVideoErrorMessageFromAny(raw["error"]); message != "" {
		return message
	}
	if data, ok := raw["data"]; ok {
		if message := openAICompatibleVideoErrorMessageFromAny(data); message != "" {
			return message
		}
	}
	return ""
}

func openAICompatibleVideoErrorMessageFromAny(value interface{}) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case map[string]interface{}:
		for _, key := range []string{"message", "msg", "error_message", "errorMessage"} {
			if message := openAICompatibleVideoErrorMessageFromAny(typed[key]); message != "" {
				return message
			}
		}
		if message := openAICompatibleVideoErrorMessageFromAny(typed["error"]); message != "" {
			return message
		}
	case []interface{}:
		for _, item := range typed {
			if message := openAICompatibleVideoErrorMessageFromAny(item); message != "" {
				return message
			}
		}
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		var mapped map[string]interface{}
		if err := json.Unmarshal(raw, &mapped); err != nil {
			return ""
		}
		return openAICompatibleVideoErrorMessageFromAny(mapped)
	}
	return ""
}
