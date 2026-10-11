package executionruntime

import (
	"context"
	"fmt"
	"mime"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

type protectedArtifactPersister struct {
	manager   *tool_file.ToolFileManager
	scope     execution.Scope
	messageID string
}

// NewProtectedArtifactPersister freezes the verified owner and prepared message.
// Both workspace_export_file and platform CLI exports use this same persister.
func NewProtectedArtifactPersister(manager *tool_file.ToolFileManager, scope execution.Scope, messageID string) (ArtifactPersister, error) {
	id, err := uuid.Parse(messageID)
	if manager == nil || scope.Identity == nil || scope.Identity.Validate() != nil || err != nil || id == uuid.Nil || id.String() != messageID {
		return nil, fmt.Errorf("protected artifact message and identity are required")
	}
	identity := *scope.Identity
	scope.Identity = &identity
	return &protectedArtifactPersister{manager: manager, scope: scope, messageID: messageID}, nil
}

func (p *protectedArtifactPersister) Persist(ctx context.Context, scope execution.Scope, conversationID, filePath string, data []byte) (map[string]interface{}, error) {
	if !sameFactoryScope(p.scope, scope) || conversationID != p.scope.Identity.ConversationID || len(data) > agentExecutionFileLimit ||
		!validPlatformFilePath(filePath) {
		return nil, fmt.Errorf("protected artifact owner or path is invalid")
	}
	filename := path.Base(filePath)
	extension := path.Ext(filename)
	if extension == "" {
		extension = ".bin"
	}
	mimeType := mime.TypeByExtension(strings.ToLower(extension))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}
	file, err := p.manager.CreateFileByRaw(ctx, tool_file.CreateFileByRawParams{
		UserID: scope.SubjectID, TenantID: scope.OrganizationID, ConversationID: &conversationID,
		ExecutionIdentity: p.scope.Identity, ExecutionMessageID: &p.messageID,
		FileData: data, MimeType: mimeType, Filename: &filename, Lifecycle: tool_file.ToolFileLifecyclePersistent,
	})
	if err != nil {
		return nil, fmt.Errorf("persist protected execution artifact: %w", err)
	}
	now := time.Now()
	return map[string]interface{}{
		"artifact_id": "tool_file:" + file.ID, "artifact_type": "file",
		"file_id": file.ID, "tool_file_id": file.ID, "filename": file.Name,
		"extension": extension, "mime_type": file.MimeType, "size": file.Size,
		"access_mode": "authenticated", "transfer_method": "tool_file", "lifecycle": "persistent",
		"conversation_id": conversationID, "message_id": p.messageID,
		"created_at": now.Unix(), "created_at_ms": now.UnixMilli(),
	}, nil
}
