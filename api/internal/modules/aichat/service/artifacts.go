package service

import (
	"strings"

	aichatmodel "github.com/zgiai/ginext/internal/modules/aichat/model"
	tool_file "github.com/zgiai/ginext/internal/modules/app/workflow/tool_file"
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
	files := generatedFilesFromMetadata(message.Metadata["generated_files"])
	if len(files) == 0 {
		return
	}
	hydrated := make([]map[string]interface{}, 0, len(files))
	for _, file := range files {
		hydrated = append(hydrated, hydrateGeneratedFileURL(file))
	}
	metadata := copyStringAnyMap(message.Metadata)
	metadata["generated_files"] = hydrated
	message.Metadata = metadata
}

func hydrateGeneratedFileURL(file map[string]interface{}) map[string]interface{} {
	hydrated := copyStringAnyMap(file)
	fileID := firstNonEmptyString(hydrated["file_id"])
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
