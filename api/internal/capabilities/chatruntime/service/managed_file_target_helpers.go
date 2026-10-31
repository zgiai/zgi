package service

import (
	"regexp"
	"strings"

	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/skillloop"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func hasFileManagerSaveCall(calls []skillloop.SkillToolCallRef) bool {
	for _, call := range calls {
		if strings.EqualFold(strings.TrimSpace(call.SkillID), skills.SkillFileManager) &&
			strings.EqualFold(strings.TrimSpace(call.ToolName), "save_file_to_management") {
			return true
		}
	}
	return false
}

var managedFileTargetPattern = regexp.MustCompile(`(?i)([^\s，,。；;、：:（）()【】\[\]{}"'“”‘’<>]+?\.(?:txt|md|markdown|html|json|csv|svg|pdf|docx|xlsx|pptx))`)

type requestedManagedFileTarget struct {
	Filename  string
	Extension string
}

func fileManagerSaveToolFileID(call skillloop.SkillToolCallRef) string {
	if !hasFileManagerSaveCall([]skillloop.SkillToolCallRef{call}) {
		return ""
	}
	return strings.TrimSpace(firstNonEmptyString(
		call.Arguments["tool_file_id"],
		call.Arguments["file_id"],
		call.Result["source_tool_file_id"],
		call.Result["source_file_id"],
		call.Result["tool_file_id"],
	))
}

func generatedArtifactMapSaveArguments(artifact map[string]interface{}) map[string]interface{} {
	if len(artifact) == 0 {
		return nil
	}
	if strings.TrimSpace(stringFromAny(artifact["upload_file_id"])) != "" ||
		strings.EqualFold(strings.TrimSpace(stringFromAny(artifact["target"])), "managed_file") {
		return nil
	}
	toolFileID := strings.TrimSpace(firstNonEmptyString(artifact["tool_file_id"], artifact["file_id"]))
	if toolFileID == "" {
		return nil
	}
	args := map[string]interface{}{
		"source_type":  "tool_file",
		"tool_file_id": toolFileID,
	}
	if filename := strings.TrimSpace(firstNonEmptyString(artifact["filename"], artifact["name"])); filename != "" {
		args["filename"] = filename
	}
	return args
}
