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

func requestedManagedFileTargetsFromParts(parts *chatRequestParts) []requestedManagedFileTarget {
	if parts == nil {
		return nil
	}
	return explicitRequestedManagedFileTargetsFromQuery(parts.Query)
}

func explicitRequestedManagedFileTargetsFromQuery(query string) []requestedManagedFileTarget {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	seen := map[string]struct{}{}
	targets := []requestedManagedFileTarget{}
	for _, match := range managedFileTargetPattern.FindAllString(query, -1) {
		filename := normalizeManagedFileTargetName(match)
		if filename == "" {
			continue
		}
		key := "name:" + filename
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, requestedManagedFileTarget{
			Filename:  filename,
			Extension: managedFileTargetExtension(filename),
		})
	}
	return targets
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

func normalizeManagedFileTargetName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, " \t\r\n\"'`.,，。;；:：!！?？)）]】}》>“”‘’")
	name = strings.ReplaceAll(name, "\\", "/")
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}
	return strings.ToLower(strings.TrimSpace(name))
}

func managedFileTargetExtension(filename string) string {
	filename = normalizeManagedFileTargetName(filename)
	if filename == "" {
		return ""
	}
	idx := strings.LastIndex(filename, ".")
	if idx < 0 || idx == len(filename)-1 {
		return ""
	}
	ext := strings.TrimPrefix(filename[idx+1:], ".")
	if ext == "markdown" {
		return "md"
	}
	return ext
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

func shouldReuseRecentGeneratedArtifactForManagedCreate(parts *chatRequestParts) bool {
	if parts == nil || len(parts.RecentGeneratedArtifacts) == 0 ||
		!turnTaskContractRequestsManagedFileCreate(parts, nil, "") {
		return false
	}
	return !modelTurnIntentRequestsTemporaryFileArtifact(parts.ModelTurnIntent)
}
