package service

import (
	"context"
	"strings"
	"time"

	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	tool_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/util"
)

const (
	recentGeneratedArtifactLimit         = 8
	maxConversationArtifactRecords       = 50
	conversationArtifactSchemaVersion    = "conversation_artifact.v1"
	conversationArtifactTypeFile         = "file"
	conversationArtifactStatusAvailable  = "available"
	conversationArtifactStatusSaved      = "saved_to_file_management"
	conversationArtifactAvailabilityLive = "available"
	conversationArtifactAvailabilityGone = "expired"
	conversationArtifactLifecycleTemp    = "temporary"
	conversationArtifactLifecycleManaged = "managed"
)

func hydrateMessagesGeneratedFileURLs(messages []*runtimemodel.Message) {
	for _, message := range messages {
		hydrateMessageGeneratedFileURLs(message)
	}
}

func hydrateMessagesGeneratedFileState(ctx context.Context, messages []*runtimemodel.Message) {
	toolFiles, lookupAttempted := generatedToolFilesForHistory(ctx, messages)
	now := time.Now()
	for _, message := range messages {
		hydrateMessageGeneratedFileURLsWithLookup(message, toolFiles, lookupAttempted, now)
	}
}

func hydrateMessageGeneratedFileState(ctx context.Context, message *runtimemodel.Message) {
	hydrateMessagesGeneratedFileState(ctx, []*runtimemodel.Message{message})
}

func hydrateMessageGeneratedFileURLs(message *runtimemodel.Message) {
	hydrateMessageGeneratedFileURLsWithLookup(message, nil, false, time.Now())
}

func hydrateMessageGeneratedFileURLsWithLookup(message *runtimemodel.Message, toolFiles map[string]*tool_file.ToolFile, lookupAttempted bool, now time.Time) {
	if message == nil || len(message.Metadata) == 0 {
		return
	}
	files := generatedFilesFromMetadata(message.Metadata["generated_files"])
	if len(files) == 0 {
		return
	}
	hydrated := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		hydrated = append(hydrated, hydrateGeneratedFileURLWithLookup(file, toolFiles, lookupAttempted, now))
	}
	metadata := copyStringAnyMap(message.Metadata)
	metadata["generated_files"] = hydrated
	message.Metadata = metadata
}

func generatedToolFilesForHistory(ctx context.Context, messages []*runtimemodel.Message) (map[string]*tool_file.ToolFile, bool) {
	if tool_file.GlobalToolFileManager == nil {
		return nil, false
	}
	seen := map[string]struct{}{}
	toolFileIDs := make([]string, 0)
	for _, message := range messages {
		if message == nil || len(message.Metadata) == 0 {
			continue
		}
		for _, file := range generatedFilesFromMetadata(message.Metadata["generated_files"]) {
			if isManagedFileArtifact(file) || !generatedFileNeedsLifecycleLookup(file) {
				continue
			}
			toolFileID := generatedArtifactToolFileID(file)
			if toolFileID == "" {
				continue
			}
			if _, exists := seen[toolFileID]; exists {
				continue
			}
			seen[toolFileID] = struct{}{}
			toolFileIDs = append(toolFileIDs, toolFileID)
		}
	}
	if len(toolFileIDs) == 0 {
		return nil, false
	}
	toolFiles, err := tool_file.GetToolFilesByIDsGlobal(ctx, toolFileIDs)
	if err != nil {
		return nil, false
	}
	return toolFiles, true
}

func generatedFileNeedsLifecycleLookup(file map[string]interface{}) bool {
	return strings.TrimSpace(stringFromAny(file["lifecycle"])) == "" || file["expires_at"] == nil
}

func hydrateGeneratedFileURL(file map[string]interface{}) map[string]interface{} {
	return hydrateGeneratedFileURLWithLookup(file, nil, false, time.Now())
}

func hydrateGeneratedFileURLWithLookup(file map[string]interface{}, toolFiles map[string]*tool_file.ToolFile, lookupAttempted bool, now time.Time) map[string]interface{} {
	hydrated := copyStringAnyMap(file)
	transferMethod := strings.TrimSpace(stringFromAny(hydrated["transfer_method"]))
	if isManagedFileArtifact(hydrated) || (transferMethod != "" && transferMethod != "tool_file") {
		if fileID := strings.TrimSpace(firstNonEmptyString(hydrated["upload_file_id"], hydrated["file_id"], hydrated["id"])); fileID != "" && strings.TrimSpace(stringFromAny(hydrated["artifact_id"])) == "" {
			hydrated["artifact_id"] = "managed_file:" + fileID
		}
		hydrateManagedGeneratedFileURL(hydrated)
		return hydrated
	}
	fileID := generatedArtifactToolFileID(hydrated)
	if fileID != "" && strings.TrimSpace(stringFromAny(hydrated["artifact_id"])) == "" {
		hydrated["artifact_id"] = "tool_file:" + fileID
	}
	if toolFile := toolFiles[fileID]; toolFile != nil {
		hydrated["lifecycle"] = string(toolFile.LifecycleValue())
		if toolFile.ExpiresAt != nil {
			hydrated["expires_at"] = toolFile.ExpiresAt.Unix()
		}
	} else if lookupAttempted && generatedFileNeedsLifecycleLookup(hydrated) {
		hydrated["availability"] = conversationArtifactAvailabilityGone
		delete(hydrated, "url")
		delete(hydrated, "download_url")
		return hydrated
	}
	if generatedArtifactExpired(hydrated, now) {
		hydrated["availability"] = conversationArtifactAvailabilityGone
		delete(hydrated, "url")
		delete(hydrated, "download_url")
		return hydrated
	}
	hydrated["availability"] = conversationArtifactAvailabilityLive
	extension := normalizedFileExtension(hydrated["extension"])
	if fileID == "" || extension == "" {
		return hydrated
	}
	url, err := tool_file.SignToolFileGlobal(fileID, extension)
	if err != nil {
		return hydrated
	}
	hydrated["url"] = url
	hydrated["download_url"] = appendDownloadQuery(url)
	return hydrated
}

func generatedArtifactToolFileID(file map[string]interface{}) string {
	return strings.TrimSpace(firstNonEmptyString(file["tool_file_id"], file["file_id"], file["id"]))
}

func generatedArtifactExpired(file map[string]interface{}, now time.Time) bool {
	if strings.EqualFold(strings.TrimSpace(stringFromAny(file["availability"])), conversationArtifactAvailabilityGone) {
		return true
	}
	expiresAt, ok := unixSecondsFromAny(file["expires_at"])
	return ok && expiresAt > 0 && expiresAt <= now.Unix()
}

func hydrateManagedGeneratedFileURL(file map[string]interface{}) {
	if !isManagedFileArtifact(file) {
		return
	}
	fileID := firstNonEmptyString(file["upload_file_id"], file["file_id"], file["id"])
	if fileID == "" {
		return
	}
	if url, err := util.GetSignedFileURL(fileID); err == nil && strings.TrimSpace(url) != "" {
		file["url"] = url
		file["download_url"] = appendAttachmentQuery(url)
		return
	}
	if url := strings.TrimSpace(stringFromAny(file["url"])); url != "" && strings.TrimSpace(stringFromAny(file["download_url"])) == "" {
		file["download_url"] = appendAttachmentQuery(url)
	}
}

func appendAttachmentQuery(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" || strings.Contains(rawURL, "as_attachment=") {
		return rawURL
	}
	if strings.Contains(rawURL, "?") {
		return rawURL + "&as_attachment=true"
	}
	return rawURL + "?as_attachment=true"
}

func hydrateStreamEventGeneratedFileURL(event StreamEvent) StreamEvent {
	if event.EventType != streamEventSkillArtifactCreated || len(event.Payload) == 0 {
		return event
	}
	event.Payload = hydrateGeneratedFileURL(event.Payload)
	return event
}

func persistentGeneratedArtifact(artifact map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	copyStringField(out, artifact, "artifact_id")
	copyStringField(out, artifact, "file_id")
	copyStringField(out, artifact, "tool_file_id")
	copyStringField(out, artifact, "source_file_id")
	copyStringField(out, artifact, "source_tool_file_id")
	copyStringField(out, artifact, "filename")
	copyStringField(out, artifact, "extension")
	copyStringField(out, artifact, "mime_type")
	copyStringField(out, artifact, "transfer_method")
	copyStringField(out, artifact, "file_type")
	copyStringField(out, artifact, "target")
	copyStringField(out, artifact, "lifecycle")
	copyStringField(out, artifact, "workspace_id")
	copyStringField(out, artifact, "folder_id")
	copyStringField(out, artifact, "upload_file_id")
	if transferMethod := strings.TrimSpace(stringFromAny(artifact["transfer_method"])); transferMethod != "" && transferMethod != "tool_file" {
		copyStringField(out, artifact, "url")
		copyStringField(out, artifact, "download_url")
	}
	copyStringField(out, artifact, "skill_id")
	copyStringField(out, artifact, "tool_name")
	copyStringField(out, artifact, "operation_id")
	copyStringField(out, artifact, "correlation_id")
	copyScalarField(out, artifact, "size")
	copyScalarField(out, artifact, "created_at")
	copyScalarField(out, artifact, "expires_at")
	if strings.TrimSpace(stringFromAny(out["artifact_id"])) == "" {
		if isManagedFileArtifact(out) {
			if fileID := strings.TrimSpace(firstNonEmptyString(out["upload_file_id"], out["file_id"])); fileID != "" {
				out["artifact_id"] = "managed_file:" + fileID
			}
		} else if toolFileID := generatedArtifactToolFileID(out); toolFileID != "" {
			out["artifact_id"] = "tool_file:" + toolFileID
			out["tool_file_id"] = toolFileID
		}
	}
	if audit := governanceMapFromAny(artifact["asset_operation_audit"]); len(audit) > 0 {
		out["asset_operation_audit"] = audit
	}
	return out
}

func persistentConversationArtifact(artifact map[string]interface{}) map[string]interface{} {
	if len(artifact) == 0 {
		return nil
	}
	isManaged := isManagedFileArtifact(artifact)
	toolFileID := strings.TrimSpace(firstNonEmptyString(artifact["tool_file_id"], artifact["source_tool_file_id"], artifact["source_file_id"], artifact["file_id"], artifact["id"]))
	managedFileID := strings.TrimSpace(firstNonEmptyString(artifact["upload_file_id"], artifact["file_id"], artifact["id"]))
	if isManaged && managedFileID == "" {
		return nil
	}
	if !isManaged && toolFileID == "" {
		return nil
	}

	out := map[string]interface{}{
		"schema_version": conversationArtifactSchemaVersion,
		"artifact_type":  conversationArtifactTypeFile,
	}
	if isManaged {
		out["artifact_id"] = "managed_file:" + managedFileID
		out["status"] = conversationArtifactStatusSaved
		out["lifecycle"] = conversationArtifactLifecycleManaged
		out["file_id"] = managedFileID
		out["upload_file_id"] = managedFileID
		out["target"] = "managed_file"
		if toolFileID != "" && toolFileID != managedFileID {
			out["source_tool_file_id"] = toolFileID
		}
	} else {
		out["artifact_id"] = "tool_file:" + toolFileID
		out["status"] = conversationArtifactStatusAvailable
		if lifecycle := strings.TrimSpace(stringFromAny(artifact["lifecycle"])); lifecycle != "" {
			out["lifecycle"] = lifecycle
		} else {
			out["lifecycle"] = conversationArtifactLifecycleTemp
		}
		out["file_id"] = toolFileID
		out["tool_file_id"] = toolFileID
		if target := strings.TrimSpace(stringFromAny(artifact["target"])); target != "" {
			out["target"] = target
		} else {
			out["target"] = "temporary_artifact"
		}
	}

	for _, key := range []string{
		"filename",
		"name",
		"extension",
		"mime_type",
		"file_type",
		"transfer_method",
		"workspace_id",
		"folder_id",
		"skill_id",
		"tool_name",
		"operation_id",
		"correlation_id",
		"source_type",
		"source_url",
		"source_message_id",
	} {
		if value := strings.TrimSpace(stringFromAny(artifact[key])); value != "" {
			out[key] = value
		}
	}
	for _, key := range []string{"size", "created_at", "expires_at"} {
		if value, ok := artifact[key]; ok && value != nil {
			out[key] = value
		}
	}
	if strings.TrimSpace(stringFromAny(out["filename"])) == "" {
		if filename := strings.TrimSpace(firstNonEmptyString(artifact["filename"], artifact["name"], artifact["file_name"], artifact["title"])); filename != "" {
			out["filename"] = filename
		}
	}
	if audit := governanceMapFromAny(artifact["asset_operation_audit"]); len(audit) > 0 {
		out["asset_operation_audit"] = audit
	}
	return out
}

func mergeConversationArtifactMetadata(metadata map[string]interface{}, artifact map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		metadata = map[string]interface{}{}
	}
	storedArtifact := persistentConversationArtifact(artifact)
	if len(storedArtifact) == 0 {
		return metadata
	}
	artifacts := conversationArtifactsFromMetadata(metadata["conversation_artifacts"])
	artifactID := strings.TrimSpace(stringFromAny(storedArtifact["artifact_id"]))
	replaced := false
	for idx, item := range artifacts {
		if artifactID != "" && strings.TrimSpace(stringFromAny(item["artifact_id"])) == artifactID {
			artifacts[idx] = mergeConversationArtifact(item, storedArtifact)
			replaced = true
			break
		}
	}
	if !replaced {
		artifacts = append(artifacts, storedArtifact)
	}
	if len(artifacts) > maxConversationArtifactRecords {
		artifacts = artifacts[len(artifacts)-maxConversationArtifactRecords:]
	}
	metadata["conversation_artifacts"] = mapsToInterfaceSlice(artifacts)
	metadata["conversation_artifact_count"] = len(artifacts)
	return metadata
}

func mergeConversationArtifact(existing map[string]interface{}, incoming map[string]interface{}) map[string]interface{} {
	merged := copyStringAnyMap(existing)
	if merged == nil {
		merged = map[string]interface{}{}
	}
	for key, value := range incoming {
		if value == nil {
			continue
		}
		if text, ok := value.(string); ok && strings.TrimSpace(text) == "" {
			continue
		}
		merged[key] = value
	}
	return merged
}

func copyStringField(out map[string]interface{}, source map[string]interface{}, key string) {
	if out == nil || source == nil {
		return
	}
	if value := strings.TrimSpace(stringFromAny(source[key])); value != "" {
		out[key] = value
	}
}

func copyScalarField(out map[string]interface{}, source map[string]interface{}, key string) {
	if out == nil || source == nil {
		return
	}
	switch value := source[key].(type) {
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64, bool:
		out[key] = value
	}
}

func normalizedFileExtension(value interface{}) string {
	extension := strings.TrimSpace(stringFromAny(value))
	if extension == "" {
		return ""
	}
	if strings.HasPrefix(extension, ".") {
		return extension
	}
	return "." + extension
}

func applyRecentGeneratedArtifactsFromBranch(parts *chatRequestParts, branch []*runtimemodel.Message) {
	if parts == nil || len(parts.RecentGeneratedArtifacts) > 0 {
		return
	}
	parts.RecentGeneratedArtifacts = recentGeneratedArtifactsFromBranch(branch)
}

func recentGeneratedArtifactsFromBranch(branch []*runtimemodel.Message) []map[string]interface{} {
	if len(branch) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]map[string]interface{}, 0, recentGeneratedArtifactLimit)
	for i := len(branch) - 1; i >= 0 && len(out) < recentGeneratedArtifactLimit; i-- {
		message := branch[i]
		if message == nil || !isUsableAssistantHistoryStatus(message.Status) {
			continue
		}
		artifacts := recentConversationArtifactCandidates(message)
		for j := len(artifacts) - 1; j >= 0 && len(out) < recentGeneratedArtifactLimit; j-- {
			artifact := recentGeneratedArtifactCandidate(artifacts[j], message.ID.String())
			toolFileID := strings.TrimSpace(stringFromAny(artifact["tool_file_id"]))
			if toolFileID == "" {
				continue
			}
			if _, exists := seen[toolFileID]; exists {
				continue
			}
			seen[toolFileID] = struct{}{}
			out = append(out, artifact)
		}
	}
	return out
}

func recentConversationArtifactCandidates(message *runtimemodel.Message) []map[string]interface{} {
	if message == nil || len(message.Metadata) == 0 {
		return nil
	}
	artifacts := conversationArtifactsFromMetadata(message.Metadata["conversation_artifacts"])
	if len(artifacts) > 0 {
		return artifacts
	}
	return generatedFilesFromMetadata(message.Metadata["generated_files"])
}

func recentGeneratedArtifactCandidate(file map[string]interface{}, messageID string) map[string]interface{} {
	if len(file) == 0 {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(file["lifecycle"])), conversationArtifactLifecycleManaged) {
		return nil
	}
	transferMethod := strings.TrimSpace(stringFromAny(file["transfer_method"]))
	if transferMethod != "" && transferMethod != "tool_file" {
		return nil
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(file["target"])), "managed_file") {
		return nil
	}
	if strings.TrimSpace(stringFromAny(file["upload_file_id"])) != "" {
		return nil
	}
	if generatedArtifactExpired(file, time.Now()) {
		return nil
	}
	toolFileID := firstNonEmptyString(file["tool_file_id"], file["file_id"], file["id"])
	if toolFileID == "" {
		return nil
	}
	artifact := map[string]interface{}{
		"tool_file_id":      toolFileID,
		"file_id":           toolFileID,
		"source_message_id": strings.TrimSpace(messageID),
	}
	for _, key := range []string{
		"artifact_id",
		"status",
		"lifecycle",
		"filename",
		"name",
		"extension",
		"mime_type",
		"file_type",
		"transfer_method",
		"target",
		"skill_id",
		"tool_name",
		"operation_id",
		"correlation_id",
		"availability",
	} {
		if value, ok := file[key]; ok && value != nil && strings.TrimSpace(stringFromAny(value)) != "" {
			artifact[key] = value
		}
	}
	for _, key := range []string{"size", "created_at", "expires_at"} {
		if value, ok := file[key]; ok && value != nil {
			artifact[key] = value
		}
	}
	if strings.TrimSpace(stringFromAny(artifact["filename"])) == "" {
		if name := strings.TrimSpace(firstNonEmptyString(file["name"], file["file_name"], file["title"])); name != "" {
			artifact["filename"] = name
		}
	}
	return artifact
}

func conversationArtifactsFromMetadata(value interface{}) []map[string]interface{} {
	switch typed := value.(type) {
	case []map[string]interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, copyStringAnyMap(item))
		}
		return out
	case []interface{}:
		out := make([]map[string]interface{}, 0, len(typed))
		for _, item := range typed {
			if artifact, ok := item.(map[string]interface{}); ok {
				out = append(out, copyStringAnyMap(artifact))
			}
		}
		return out
	default:
		return nil
	}
}

func isManagedFileArtifact(artifact map[string]interface{}) bool {
	if len(artifact) == 0 {
		return false
	}
	if strings.TrimSpace(stringFromAny(artifact["upload_file_id"])) != "" {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(stringFromAny(artifact["target"])), "managed_file") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(stringFromAny(artifact["lifecycle"])), conversationArtifactLifecycleManaged)
}
