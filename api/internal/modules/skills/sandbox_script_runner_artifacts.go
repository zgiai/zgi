package skills

import (
	"context"
	"encoding/base64"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	workflowfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/file"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

func (r *SandboxScriptRunner) collectArtifacts(ctx context.Context, sandboxID string, execCtx ExecutionContext, manifest skillScriptManifest) ([]skillScriptArtifact, error) {
	var tree struct {
		Items []sandboxFileInfo `json:"items"`
	}
	path := withOrganizationQuery("/v1/files/tree?sandbox_id="+url.QueryEscape(sandboxID), execCtx)
	if err := r.doIdempotentJSON(ctx, http.MethodGet, path, nil, &tree, r.timeouts.Artifact); err != nil {
		return nil, err
	}

	artifacts := make([]skillScriptArtifact, 0)
	for _, item := range tree.Items {
		if item.IsDirectory || !skillManifestAllowsArtifactPath(manifest, item.Path) {
			continue
		}
		if len(artifacts) >= manifest.MaxArtifactCount {
			break
		}
		artifacts = append(artifacts, artifactFromFileInfo(item.Path, item.Size, ""))
	}
	return artifacts, nil
}

func (r *SandboxScriptRunner) artifactsFromManifests(manifests []sandboxFileManifest, manifest skillScriptManifest) ([]skillScriptArtifact, error) {
	artifacts := make([]skillScriptArtifact, 0)
	for _, fileManifest := range manifests {
		for _, item := range fileManifest.Items {
			if !skillManifestAllowsArtifactPath(manifest, item.Path) {
				continue
			}
			if len(artifacts) >= manifest.MaxArtifactCount {
				return artifacts, nil
			}
			artifacts = append(artifacts, artifactFromFileInfo(item.Path, item.Size, item.ContentType))
		}
	}
	return artifacts, nil
}

func artifactFromFileInfo(artifactPath string, size int64, contentType string) skillScriptArtifact {
	return skillScriptArtifact{
		Path:        artifactPath,
		Name:        path.Base(filepath.ToSlash(artifactPath)),
		Size:        size,
		ContentType: strings.TrimSpace(contentType),
	}
}

func (r *SandboxScriptRunner) prepareArtifacts(ctx context.Context, sandboxID string, artifacts []skillScriptArtifact, execCtx ExecutionContext, maxArtifactBytes int64) {
	if maxArtifactBytes <= 0 || maxArtifactBytes > maxSkillScriptArtifactBytes {
		maxArtifactBytes = maxSkillScriptArtifactBytes
	}
	for index := range artifacts {
		r.prepareArtifact(ctx, sandboxID, &artifacts[index], execCtx, maxArtifactBytes)
	}
}

func (r *SandboxScriptRunner) prepareArtifact(ctx context.Context, sandboxID string, artifact *skillScriptArtifact, execCtx ExecutionContext, maxArtifactBytes int64) {
	if artifact == nil {
		return
	}
	if artifact.Size > maxArtifactBytes {
		artifact.Persisted = false
		artifact.Reason = "size_limit_exceeded"
		return
	}
	content, err := r.downloadArtifact(ctx, sandboxID, artifact.Path, execCtx)
	if err != nil {
		artifact.Error = err.Error()
		artifact.Persisted = false
		artifact.Reason = "download_failed"
		return
	}
	if content.Encoding != "" && !strings.EqualFold(content.Encoding, "base64") {
		artifact.Error = "unsupported artifact encoding: " + content.Encoding
		artifact.Persisted = false
		artifact.Reason = "unsupported_encoding"
		return
	}
	data, err := base64.StdEncoding.DecodeString(content.Content)
	if err != nil {
		artifact.Error = err.Error()
		artifact.Persisted = false
		artifact.Reason = "decode_failed"
		return
	}
	if int64(len(data)) > maxArtifactBytes {
		artifact.Persisted = false
		artifact.Reason = "size_limit_exceeded"
		return
	}
	artifact.Size = int64(len(data))
	artifact.ContentType = skillArtifactMimeType(artifact.Name, artifact.ContentType, data)
	if artifact.Size <= inlineSkillArtifactMaxBytes {
		artifact.Content = content.Content
		artifact.Encoding = "base64"
	}
	persister := r.artifactPersister
	if persister == nil {
		persister = defaultSkillScriptArtifactPersister{}
	}
	fileMeta, err := persister.PersistSkillScriptArtifact(ctx, SkillScriptArtifactPersistRequest{
		ExecContext: execCtx,
		Path:        artifact.Path,
		Name:        artifact.Name,
		Size:        artifact.Size,
		ContentType: artifact.ContentType,
		Data:        data,
	})
	if err != nil {
		artifact.Error = err.Error()
		artifact.Persisted = false
		artifact.Reason = "persist_failed"
		return
	}
	artifact.Persisted = true
	artifact.File = fileMeta
}

type skillScriptArtifact struct {
	Path        string                 `json:"path"`
	Name        string                 `json:"name"`
	Size        int64                  `json:"size"`
	ContentType string                 `json:"content_type,omitempty"`
	Content     string                 `json:"content,omitempty"`
	Encoding    string                 `json:"encoding,omitempty"`
	Persisted   bool                   `json:"persisted"`
	Reason      string                 `json:"reason,omitempty"`
	File        map[string]interface{} `json:"file,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

type defaultSkillScriptArtifactPersister struct{}

func (defaultSkillScriptArtifactPersister) PersistSkillScriptArtifact(ctx context.Context, request SkillScriptArtifactPersistRequest) (map[string]interface{}, error) {
	organizationID := strings.TrimSpace(request.ExecContext.OrganizationID)
	userID := strings.TrimSpace(request.ExecContext.UserID)
	if organizationID == "" {
		return nil, fmt.Errorf("organization id is required to persist skill artifact")
	}
	if userID == "" {
		return nil, fmt.Errorf("user id is required to persist skill artifact")
	}
	if len(request.Data) == 0 {
		return nil, fmt.Errorf("skill artifact is empty")
	}

	filename := skillArtifactFilename(request.Name, request.Path)
	mimeType := skillArtifactMimeType(filename, request.ContentType, request.Data)
	extension := normalizedSkillArtifactExtension(filename, mimeType)
	conversationID := strings.TrimSpace(request.ExecContext.ConversationID)
	var conversationIDPtr *string
	if conversationID != "" {
		conversationIDPtr = &conversationID
	}

	toolFile, err := tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
		UserID:         userID,
		TenantID:       organizationID,
		ConversationID: conversationIDPtr,
		FileData:       request.Data,
		MimeType:       mimeType,
		Filename:       &filename,
		Lifecycle:      tool_file.ToolFileLifecyclePersistent,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create skill artifact file: %w", err)
	}
	if extension == "" {
		extension = normalizedSkillArtifactExtension(toolFile.Name, toolFile.MimeType)
	}
	url, err := tool_file.SignToolFileGlobal(toolFile.ID, extension)
	if err != nil {
		return nil, fmt.Errorf("failed to sign skill artifact file: %w", err)
	}
	downloadURL := appendSkillArtifactDownloadQuery(url)
	fileType := workflowfile.InferFileType(extension, mimeType)
	fileObj := workflowfile.NewFile(
		organizationID,
		fileType,
		workflowfile.FileTransferMethodToolFile,
		workflowfile.WithID(toolFile.ID),
		workflowfile.WithRelatedID(toolFile.ID),
		workflowfile.WithFilename(toolFile.Name),
		workflowfile.WithExtension(extension),
		workflowfile.WithMimeType(mimeType),
		workflowfile.WithSize(int(toolFile.Size)),
		workflowfile.WithURL(url),
	)
	fileMeta := fileObj.ToDict()
	fileMeta["url"] = url
	fileMeta["download_url"] = downloadURL
	return fileMeta, nil
}

func skillArtifactFilename(name string, artifactPath string) string {
	filename := strings.TrimSpace(name)
	if filename == "" || filename == "." || filename == "/" {
		filename = path.Base(filepath.ToSlash(artifactPath))
	}
	filename = strings.TrimSpace(strings.ReplaceAll(filename, "\\", "_"))
	filename = strings.Trim(filename, "/")
	if filename == "" || filename == "." {
		return "artifact.bin"
	}
	return filename
}

func skillArtifactMimeType(filename string, contentType string, data []byte) string {
	mimeType := strings.TrimSpace(strings.Split(contentType, ";")[0])
	extensionMimeType := ""
	if extension := path.Ext(filename); extension != "" {
		if byExtension := skillArtifactMimeTypeByExtension(extension); byExtension != "" {
			extensionMimeType = byExtension
		} else if byExtension := mime.TypeByExtension(extension); byExtension != "" {
			extensionMimeType = strings.Split(byExtension, ";")[0]
		}
	}
	if mimeType != "" {
		if extensionMimeType != "" && isGenericSkillArtifactMimeType(mimeType) {
			return extensionMimeType
		}
		return mimeType
	}
	if extensionMimeType != "" {
		return extensionMimeType
	}
	if len(data) > 0 {
		if detected := http.DetectContentType(data); detected != "" {
			return detected
		}
	}
	return "application/octet-stream"
}

func isGenericSkillArtifactMimeType(mimeType string) bool {
	switch strings.ToLower(strings.TrimSpace(mimeType)) {
	case "text/plain", "application/octet-stream", "application/zip", "application/x-zip-compressed":
		return true
	default:
		return false
	}
}

func skillArtifactMimeTypeByExtension(extension string) string {
	switch strings.ToLower(strings.TrimPrefix(strings.TrimSpace(extension), ".")) {
	case "json":
		return "application/json"
	case "html", "htm":
		return "text/html"
	case "csv":
		return "text/csv"
	case "txt":
		return "text/plain"
	case "md", "markdown":
		return "text/markdown"
	case "pdf":
		return "application/pdf"
	case "png":
		return "image/png"
	case "jpg", "jpeg":
		return "image/jpeg"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "zip":
		return "application/zip"
	case "doc":
		return "application/msword"
	case "docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	case "xls":
		return "application/vnd.ms-excel"
	case "xlsx":
		return "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
	case "ppt":
		return "application/vnd.ms-powerpoint"
	case "pptx":
		return "application/vnd.openxmlformats-officedocument.presentationml.presentation"
	default:
		return ""
	}
}

func normalizedSkillArtifactExtension(filename string, mimeType string) string {
	if extension := path.Ext(filename); extension != "" {
		return extension
	}
	extensions, err := mime.ExtensionsByType(mimeType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	return ".bin"
}

func appendSkillArtifactDownloadQuery(rawURL string) string {
	if strings.Contains(rawURL, "?") {
		return rawURL + "&download=1"
	}
	return rawURL + "?download=1"
}
