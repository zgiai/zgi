package provider

import (
	"encoding/json"
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
	var response adapter.VideoResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(body, &raw); err == nil {
		response.Raw = raw
	}
	if response.TaskID == "" {
		response.TaskID = response.ID
	}
	if response.VideoURL == "" && len(response.Data) > 0 {
		response.VideoURL = response.Data[0].URL
	}
	return &response, nil
}
