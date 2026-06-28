package service

import (
	"strings"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

const recentAssetCandidateLimit = 8

func applyRecentAssetCandidatesFromBranch(parts *chatRequestParts, branch []*runtimemodel.Message) {
	if parts == nil || len(parts.RecentAssetCandidates) > 0 {
		return
	}
	parts.RecentAssetCandidates = recentAssetCandidatesFromBranch(branch)
}

func recentAssetCandidatesFromBranch(branch []*runtimemodel.Message) []ResourceCandidate {
	if len(branch) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]ResourceCandidate, 0, recentAssetCandidateLimit)
	for i := len(branch) - 1; i >= 0 && len(out) < recentAssetCandidateLimit; i-- {
		message := branch[i]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		invocations := skillInvocationMaps(message.Metadata)
		for j := len(invocations) - 1; j >= 0 && len(out) < recentAssetCandidateLimit; j-- {
			for _, candidate := range recentAssetCandidatesFromInvocation(invocations[j], message.ID.String()) {
				if candidate.ID == "" {
					continue
				}
				if _, exists := seen[candidate.ID]; exists {
					continue
				}
				seen[candidate.ID] = struct{}{}
				out = append(out, candidate)
				if len(out) >= recentAssetCandidateLimit {
					break
				}
			}
		}
	}
	return out
}

func recentAssetCandidatesFromInvocation(invocation map[string]interface{}, messageID string) []ResourceCandidate {
	if strings.TrimSpace(stringFromAny(invocation["kind"])) != "tool_call" {
		return nil
	}
	skillID := strings.TrimSpace(stringFromAny(invocation["skill_id"]))
	if skillID != skills.SkillFileReader && skillID != skills.SkillFileManager {
		return nil
	}
	status := strings.TrimSpace(stringFromAny(invocation["status"]))
	if status != "" && status != "success" && status != "completed" {
		return nil
	}
	toolName := strings.TrimSpace(stringFromAny(invocation["tool_name"]))
	result, ok := invocation["result"].(map[string]interface{})
	if !ok || len(result) == 0 {
		return nil
	}
	source := "recent_execution." + toolName
	switch toolName {
	case "read_file":
		if skillID != skills.SkillFileReader {
			return nil
		}
		if file := resourceCandidateFromRecentFileMap(mapFromOperationContext(result["file"]), source, messageID, toolName); file.ID != "" {
			return []ResourceCandidate{file}
		}
		if file := resourceCandidateFromRecentFileMap(result, source, messageID, toolName); file.ID != "" {
			return []ResourceCandidate{file}
		}
	case "save_file_to_management":
		if skillID != skills.SkillFileManager {
			return nil
		}
		if file := resourceCandidateFromRecentFileMap(mapFromOperationContext(result["file"]), source, messageID, toolName); file.ID != "" {
			file.Recent = true
			return []ResourceCandidate{file}
		}
		if file := resourceCandidateFromRecentFileMap(result, source, messageID, toolName); file.ID != "" {
			file.Recent = true
			return []ResourceCandidate{file}
		}
	case "delete_file":
		if skillID != skills.SkillFileManager && skillID != skills.SkillFileReader {
			return nil
		}
		if file := resourceCandidateFromRecentFileMap(mapFromOperationContext(result["file"]), source, messageID, toolName); file.ID != "" {
			return []ResourceCandidate{file}
		}
		if file := resourceCandidateFromRecentFileMap(result, source, messageID, toolName); file.ID != "" {
			return []ResourceCandidate{file}
		}
	case "list_visible_files":
		files := modelInvocationFileItemsFromAny(result["files"])
		out := make([]ResourceCandidate, 0, minInt(len(files), recentAssetCandidateLimit))
		for _, item := range files {
			file := resourceCandidateFromRecentFileMap(mapFromOperationContext(item), source, messageID, toolName)
			if file.ID != "" {
				out = append(out, file)
			}
			if len(out) >= recentAssetCandidateLimit {
				break
			}
		}
		return out
	}
	return nil
}

func resourceCandidateFromRecentFileMap(file map[string]interface{}, source string, messageID string, toolName string) ResourceCandidate {
	if len(file) == 0 {
		return ResourceCandidate{}
	}
	id := firstNonEmptyString(
		file["file_id"],
		file["id"],
		file["upload_file_id"],
		file["resource_id"],
		file["file_id_id"],
	)
	if id == "" {
		return ResourceCandidate{}
	}
	name := firstNonEmptyString(
		file["name"],
		file["title"],
		file["filename"],
		file["file_name"],
		file["file_name_name"],
	)
	extension := firstNonEmptyString(
		file["extension"],
		file["file_extension"],
	)
	mimeType := firstNonEmptyString(
		file["mime_type"],
		file["file_mime_type"],
		file["content_type"],
	)
	fileType := firstNonEmptyString(
		file["file_type"],
		file["format"],
	)
	workspaceID := firstNonEmptyString(
		file["workspace_id"],
		file["file_workspace_id"],
	)
	metadata := map[string]interface{}{
		"recent":              true,
		"recent_message_id":   strings.TrimSpace(messageID),
		"recent_tool_name":    strings.TrimSpace(toolName),
		"recent_asset_source": strings.TrimSpace(source),
	}
	if workspaceID != "" {
		metadata["workspace_id"] = workspaceID
	}
	if status := strings.TrimSpace(stringFromAny(file["content_status"])); status != "" {
		metadata["content_status"] = status
	}
	if visibleIndex := intValueFromAny(file["visible_index"]); visibleIndex > 0 {
		metadata["visible_index"] = visibleIndex
	}
	return ResourceCandidate{
		Type:           resourceTypeFile,
		ID:             id,
		Name:           name,
		Title:          name,
		Source:         source,
		Extension:      extension,
		MimeType:       mimeType,
		FileType:       fileType,
		Selected:       boolMetadataValue(file["selected"]),
		Recent:         true,
		Visible:        intValueFromAny(file["visible_index"]) > 0,
		VisibleOrdinal: intValueFromAny(file["visible_index"]),
		Metadata:       metadata,
	}
}
