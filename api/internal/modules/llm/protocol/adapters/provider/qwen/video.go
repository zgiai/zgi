package qwen

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	adapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
)

const qwenVideoSynthesisPath = "/services/aigc/video-generation/video-synthesis"

// CreateVideo submits a native DashScope/Wan asynchronous video task.  Video
// generation is deliberately kept on the native endpoint; the compatible
// endpoint does not define the task lifecycle used by Wan.
func (a *Adapter) CreateVideo(ctx context.Context, request *adapter.VideoRequest) (*adapter.VideoResponse, error) {
	if request == nil {
		return nil, fmt.Errorf("%w: video request is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Model) == "" {
		return nil, fmt.Errorf("%w: video model is required", adapter.ErrInvalidRequest)
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return nil, fmt.Errorf("%w: video prompt is required", adapter.ErrInvalidRequest)
	}

	payload := buildQwenVideoPayload(request)
	headers := a.aliyunJSONHeaders()
	headers["X-DashScope-Async"] = "enable"
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "POST", a.nativeBaseURL()+qwenVideoSynthesisPath, headers, payload)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, a.handleError(statusCode, respBody)
	}
	return decodeQwenVideoResponse(respBody)
}

// GetVideoTask queries the native DashScope task endpoint.
func (a *Adapter) GetVideoTask(ctx context.Context, request *adapter.VideoTaskRequest) (*adapter.VideoResponse, error) {
	if request == nil || strings.TrimSpace(request.TaskID) == "" {
		return nil, fmt.Errorf("%w: video task_id is required", adapter.ErrInvalidRequest)
	}
	endpoint := a.nativeBaseURL() + "/tasks/" + url.PathEscape(strings.TrimSpace(request.TaskID))
	respBody, statusCode, err := a.httpClient.DoRequest(ctx, "GET", endpoint, a.aliyunJSONHeaders(), nil)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	if statusCode < 200 || statusCode >= 300 {
		return nil, a.handleError(statusCode, respBody)
	}
	return decodeQwenVideoResponse(respBody)
}

func buildQwenVideoPayload(request *adapter.VideoRequest) map[string]any {
	input := map[string]any{}
	if value := strings.TrimSpace(request.Prompt); value != "" {
		input["prompt"] = value
	}
	if value := strings.TrimSpace(request.ImageURL); value != "" {
		input["img_url"] = value
	}
	if value := strings.TrimSpace(request.FirstFrameURL); value != "" {
		input["first_frame_url"] = value
	}
	if value := strings.TrimSpace(request.LastFrameURL); value != "" {
		input["last_frame_url"] = value
	}
	parameters := map[string]any{}
	if value := strings.TrimSpace(request.Size); value != "" {
		parameters["size"] = strings.ReplaceAll(value, "x", "*")
	}
	if value := strings.TrimSpace(request.Resolution); value != "" {
		parameters["resolution"] = value
	}
	if value := strings.TrimSpace(request.Ratio); value != "" {
		parameters["ratio"] = value
	}
	if request.Duration != nil {
		parameters["duration"] = *request.Duration
	}
	if request.N != nil {
		parameters["n"] = *request.N
	}
	if request.PromptExtend != nil {
		parameters["prompt_extend"] = *request.PromptExtend
	}
	if request.Watermark != nil {
		parameters["watermark"] = *request.Watermark
	}
	if request.NegativePrompt != "" {
		parameters["negative_prompt"] = request.NegativePrompt
	}
	if request.GenerateAudio != nil {
		parameters["generate_audio"] = *request.GenerateAudio
	}
	for key, value := range request.AdditionalParameters {
		parameters[key] = value
	}
	return map[string]any{
		"model":      strings.TrimSpace(request.Model),
		"input":      input,
		"parameters": parameters,
	}
}

// decodeQwenVideoResponse intentionally checks output.task_id before the
// top-level request_id. DashScope returns request_id for tracing and task_id
// for polling; confusing the two leaves the task permanently UNKNOWN.
func decodeQwenVideoResponse(body []byte) (*adapter.VideoResponse, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if message := qwenVideoErrorMessage(raw); message != "" {
		return nil, fmt.Errorf("upstream error: %s", message)
	}
	response := &adapter.VideoResponse{Raw: raw, Created: time.Now().Unix()}
	if output, ok := raw["output"].(map[string]any); ok {
		response.TaskID = qwenString(output["task_id"])
		if response.TaskID == "" {
			response.TaskID = qwenString(output["taskId"])
		}
		response.Status = qwenString(output["task_status"])
		if response.Status == "" {
			response.Status = qwenString(output["status"])
		}
		response.VideoURL = qwenString(output["video_url"])
		if response.VideoURL == "" {
			response.VideoURL = qwenString(output["videoUrl"])
		}
	}
	if response.TaskID == "" {
		response.TaskID = qwenString(raw["task_id"])
	}
	if response.TaskID == "" {
		response.TaskID = qwenString(raw["id"])
	}
	if response.Status == "" {
		response.Status = qwenString(raw["task_status"])
	}
	if response.Status == "" {
		response.Status = qwenString(raw["status"])
	}
	if response.VideoURL == "" {
		response.VideoURL = qwenString(raw["video_url"])
	}
	if response.TaskID == "" && response.VideoURL == "" {
		return nil, fmt.Errorf("video response did not include output.task_id or video_url")
	}
	if response.VideoURL != "" {
		response.Data = []adapter.VideoItem{{URL: response.VideoURL}}
	}
	return response, nil
}

func qwenString(value any) string {
	text, _ := value.(string)
	return strings.TrimSpace(text)
}

func qwenVideoErrorMessage(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	for _, key := range []string{"message", "error_message", "errorMessage"} {
		if message := qwenString(raw[key]); message != "" {
			if code := qwenString(raw["code"]); code != "" && code != "0" {
				return message
			}
		}
	}
	if code := qwenString(raw["code"]); code != "" && code != "0" && !strings.EqualFold(code, "success") {
		return code
	}
	if errValue, ok := raw["error"].(map[string]any); ok {
		return qwenString(errValue["message"])
	}
	return ""
}
