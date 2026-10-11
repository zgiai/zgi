package executionruntime

import (
	"context"
	"fmt"
	"mime"
	"path"
	"time"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

type agentExecutionArtifactPersister interface {
	Persist(context.Context, agentExecutionScope, string, string, []byte) (map[string]interface{}, error)
}

type defaultAgentExecutionArtifactPersister struct{}

func (defaultAgentExecutionArtifactPersister) Persist(ctx context.Context, scope agentExecutionScope, conversationID, filePath string, data []byte) (map[string]interface{}, error) {
	if scope.Identity != nil {
		return nil, fmt.Errorf("identity-scoped execution requires protected artifact persistence")
	}
	if scope.OrganizationID == "" || scope.SubjectID == "" || conversationID == "" || len(data) > agentExecutionFileLimit {
		return nil, fmt.Errorf("agent execution artifact is invalid or exceeds its limit")
	}
	filename := path.Base(filePath)
	if filename == "" || filename == "." || filename == "/" {
		return nil, fmt.Errorf("agent execution artifact name is invalid")
	}
	extension := path.Ext(filename)
	if extension == "" {
		extension = ".bin"
	}
	mimeType := mime.TypeByExtension(extension)
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	file, err := tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
		UserID: scope.SubjectID, TenantID: scope.OrganizationID, ConversationID: &conversationID,
		FileData: data, MimeType: mimeType, Filename: &filename, Lifecycle: tool_file.ToolFileLifecyclePersistent,
	})
	if err != nil {
		return nil, fmt.Errorf("persist agent execution artifact: %w", err)
	}
	url, err := tool_file.SignToolFileGlobal(file.ID, extension)
	if err != nil {
		return nil, fmt.Errorf("sign agent execution artifact: %w", err)
	}
	now := time.Now()
	return map[string]interface{}{
		"artifact_id": "tool_file:" + file.ID, "artifact_type": "file",
		"file_id": file.ID, "tool_file_id": file.ID, "filename": file.Name,
		"extension": extension, "mime_type": file.MimeType, "size": file.Size,
		"url": url, "download_url": url + "&download=1", "transfer_method": "tool_file",
		"lifecycle": "persistent", "created_at": now.Unix(), "created_at_ms": now.UnixMilli(),
	}, nil
}
