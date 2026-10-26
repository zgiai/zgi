package container

import (
	"context"
	"time"

	"github.com/google/uuid"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	socialtools "github.com/zgiai/zgi/api/internal/modules/tools/builtin/socialoperations"
)

func (s *socialOperationsRuntime) ImportSocialImage(ctx context.Context, identity socialtools.InvocationIdentity, id uuid.UUID) ([]byte, error) {
	scope, restored, err := s.restoreSocialOperationsRun(ctx, identity)
	if err != nil {
		return nil, err
	}
	if id == uuid.Nil || toolfile.GlobalToolFileManager == nil {
		return nil, backgroundDenied(nil)
	}
	file, err := toolfile.GlobalToolFileManager.GetToolFileByID(ctx, id.String())
	if err != nil || file == nil || file.ID != id.String() || file.Size < 0 || file.Size > 8<<20 {
		return nil, backgroundDenied(nil)
	}
	if file.ExecutionIdentity != nil {
		access, err := digitalhuman.NewAccessService(s.container.db).AuthorizeBackgroundExecutionArtifact(ctx, &restored.Run, restored.Event, file)
		if err != nil {
			return nil, backgroundDenied(err)
		}
		data, _, err := toolfile.GlobalToolFileManager.GetExecutionFileBinary(ctx, access, id.String())
		if err != nil || len(data) > 8<<20 {
			return nil, backgroundDenied(err)
		}
		return data, nil
	}
	return importSocialImageFromStore(ctx, scope, identity.ConversationID, id, toolfile.GlobalToolFileManager)
}

type socialImageStore interface {
	GetToolFileByID(context.Context, string) (*toolfile.ToolFile, error)
	GetFileBinary(context.Context, string) ([]byte, string, error)
}

func importSocialImageFromStore(ctx context.Context, scope socialops.Scope, conversation string, id uuid.UUID, store socialImageStore) ([]byte, error) {
	if store == nil || id == uuid.Nil || scope.OrganizationID == uuid.Nil || scope.OwnerID == uuid.Nil || conversation == "" {
		return nil, backgroundDenied(nil)
	}
	file, err := store.GetToolFileByID(ctx, id.String())
	if err != nil {
		return nil, backgroundDenied(err)
	}
	// Ordinary image generation artifacts belong to the billing account and
	// conversation. Execution artifacts use a separate verified-subject reader;
	// never fall back to billing identity for those files.
	if file == nil || file.ID != id.String() || file.DeletedAt != nil || file.IsExpired(time.Now()) || file.ExecutionIdentity != nil || file.TenantID != scope.OrganizationID.String() || file.UserID != scope.OwnerID.String() || file.ConversationID == nil || *file.ConversationID != conversation || file.Size < 0 || file.Size > 8<<20 {
		return nil, backgroundDenied(nil)
	}
	data, _, err := store.GetFileBinary(ctx, id.String())
	if err != nil {
		return nil, backgroundDenied(err)
	}
	if len(data) > 8<<20 {
		return nil, backgroundDenied(nil)
	}
	return data, nil
}
