package skills

import (
	"encoding/json"
	"fmt"
	"strings"

	llmadapter "github.com/zgiai/zgi/api/internal/modules/llm/protocol/adapters"
	"github.com/zgiai/zgi/api/internal/modules/tools"
)

func skillScriptMessages(command *sandboxCommandResult, artifacts []skillScriptArtifact, artifactErr error) ([]tools.ToolInvokeMessage, string, error) {
	if command == nil {
		return nil, "", fmt.Errorf("sandbox command result is empty")
	}
	stdout := strings.TrimSpace(command.Stdout)
	messages := []tools.ToolInvokeMessage{}
	if stdout != "" {
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(stdout), &data); err == nil {
			messages = append(messages, tools.ToolInvokeMessage{Type: tools.ToolInvokeMessageTypeJSON, Data: data})
		} else {
			messages = append(messages, tools.ToolInvokeMessage{Type: tools.ToolInvokeMessageTypeText, Text: command.Stdout})
		}
	}
	if strings.TrimSpace(command.Error) != "" {
		messages = append(messages, tools.ToolInvokeMessage{
			Type: tools.ToolInvokeMessageTypeLog,
			Text: command.Error,
			Meta: map[string]interface{}{"stream": "stderr"},
		})
	}
	if command.Truncated {
		messages = append(messages, tools.ToolInvokeMessage{
			Type: tools.ToolInvokeMessageTypeLog,
			Text: "skill script output was truncated",
		})
	}
	if len(artifacts) > 0 {
		items := make([]map[string]interface{}, 0, len(artifacts))
		for _, artifact := range artifacts {
			item := map[string]interface{}{
				"path":      artifact.Path,
				"name":      artifact.Name,
				"size":      artifact.Size,
				"persisted": artifact.Persisted,
			}
			if artifact.ContentType != "" {
				item["content_type"] = artifact.ContentType
			}
			if artifact.Encoding != "" {
				item["encoding"] = artifact.Encoding
			}
			if artifact.Content != "" {
				item["content"] = artifact.Content
			}
			if artifact.Reason != "" {
				item["reason"] = artifact.Reason
			}
			if artifact.File != nil {
				messages = append(messages, tools.ToolInvokeMessage{
					Type: tools.ToolInvokeMessageTypeFile,
					Text: stringFromMap(artifact.File, "download_url"),
					Meta: map[string]interface{}{
						"file": artifact.File,
					},
				})
				item["file_id"] = stringFromMap(artifact.File, "id")
				item["download_url"] = stringFromMap(artifact.File, "download_url")
			}
			if artifact.Error != "" {
				item["error"] = artifact.Error
			}
			items = append(items, item)
		}
		messages = append(messages, tools.ToolInvokeMessage{
			Type: tools.ToolInvokeMessageTypeJSON,
			Data: map[string]interface{}{"artifacts": items},
		})
	}
	if artifactErr != nil {
		messages = append(messages, tools.ToolInvokeMessage{
			Type: tools.ToolInvokeMessageTypeLog,
			Text: "failed to collect skill script artifacts: " + artifactErr.Error(),
		})
	}
	contentBytes, err := json.Marshal(messages)
	if err != nil {
		return messages, "", err
	}
	return messages, string(contentBytes), nil
}

func stringFromMap(values map[string]interface{}, key string) string {
	if values == nil {
		return ""
	}
	value, ok := values[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func skillScriptToolMessage(callID string, content string) llmadapter.Message {
	callID = strings.TrimSpace(callID)
	if callID == "" {
		callID = "call_" + SkillScriptToolRun
	}
	return llmadapter.Message{
		Role:       "tool",
		ToolCallID: callID,
		Content:    content,
	}
}

func summarizeSkillScriptResult(command *sandboxCommandResult, messages []tools.ToolInvokeMessage, artifacts []skillScriptArtifact) map[string]interface{} {
	if command == nil {
		return nil
	}
	persisted := 0
	skipped := 0
	for _, artifact := range artifacts {
		if artifact.Persisted {
			persisted++
		} else {
			skipped++
		}
	}
	return map[string]interface{}{
		"exit_code":       command.ExitCode,
		"duration_ms":     command.DurationMS,
		"truncated":       command.Truncated,
		"messages":        len(messages),
		"artifact_count":  len(artifacts),
		"persisted_count": persisted,
		"skipped_count":   skipped,
	}
}
