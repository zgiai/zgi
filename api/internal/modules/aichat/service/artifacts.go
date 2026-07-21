//go:build legacy_aichat_service
// +build legacy_aichat_service

package service

import (
	"path/filepath"
	"strings"

	aichatmodel "github.com/zgiai/zgi/api/internal/modules/aichat/model"
	tool_file "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

func hydrateMessagesGeneratedFileURLs(messages []*aichatmodel.Message) {
	for _, message := range messages {
		hydrateMessageGeneratedFileURLs(message)
	}
}

func hydrateMessageGeneratedFileURLs(message *aichatmodel.Message) {
	if message == nil || len(message.Metadata) == 0 {
		return
	}
	metadata := copyStringAnyMap(message.Metadata)
	changed := false

	files := generatedFilesFromMetadata(message.Metadata["generated_files"])
	if len(files) > 0 {
		metadata["generated_files"] = hydrateGeneratedFileURLs(files)
		changed = true
	}

	if hydrateImageGenerationFileURLs(metadata) {
		changed = true
	}
	if changed {
		message.Metadata = metadata
	}
}

func hydrateImageGenerationFileURLs(metadata map[string]interface{}) bool {
	imageGeneration, ok := metadata["image_generation"].(map[string]interface{})
	if !ok {
		return false
	}
	files := generatedFilesFromMetadata(imageGeneration["files"])
	if len(files) == 0 {
		return false
	}
	hydrated := copyStringAnyMap(imageGeneration)
	hydrated["files"] = hydrateGeneratedFileURLs(files)
	metadata["image_generation"] = hydrated
	return true
}

func hydrateGeneratedFileURLs(files []map[string]interface{}) []map[string]interface{} {
	hydrated := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		hydrated = append(hydrated, hydrateGeneratedFileURL(file))
	}
	return hydrated
}

func hydrateGeneratedFileURL(file map[string]interface{}) map[string]interface{} {
	hydrated := copyStringAnyMap(file)
	transferMethod := strings.TrimSpace(stringFromAny(hydrated["transfer_method"]))
	if transferMethod != "" && transferMethod != "tool_file" {
		return hydrated
	}
	fileID := firstNonEmptyString(hydrated["file_id"])
	extension := generatedFileExtension(hydrated)
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

func generatedFileExtension(file map[string]interface{}) string {
	if file == nil {
		return ""
	}
	if extension := normalizedFileExtension(file["extension"]); extension != "" {
		return extension
	}
	if extension := normalizedFileExtension(filepath.Ext(stringFromAny(file["filename"]))); extension != "" {
		return extension
	}
	return extensionFromMIMEType(file["mime_type"])
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
	copyStringField(out, artifact, "file_id")
	copyStringField(out, artifact, "filename")
	copyStringField(out, artifact, "extension")
	copyStringField(out, artifact, "mime_type")
	copyStringField(out, artifact, "transfer_method")
	copyStringField(out, artifact, "file_type")
	copyStringField(out, artifact, "target")
	copyStringField(out, artifact, "workspace_id")
	copyStringField(out, artifact, "folder_id")
	copyStringField(out, artifact, "upload_file_id")
	if transferMethod := strings.TrimSpace(stringFromAny(artifact["transfer_method"])); transferMethod != "" && transferMethod != "tool_file" {
		copyStringField(out, artifact, "url")
		copyStringField(out, artifact, "download_url")
	}
	copyStringField(out, artifact, "skill_id")
	copyStringField(out, artifact, "tool_name")
	copyScalarField(out, artifact, "size")
	copyScalarField(out, artifact, "created_at")
	return out
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

func extensionFromMIMEType(value interface{}) string {
	switch strings.ToLower(strings.TrimSpace(stringFromAny(value))) {
	case "image/png":
		return ".png"
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/webp":
		return ".webp"
	default:
		return ""
	}
}
