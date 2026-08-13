package music

import (
	"context"
	"fmt"
	"net/url"

	"github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

type ToolFileAssetStore struct{}

func NewToolFileAssetStore() *ToolFileAssetStore { return &ToolFileAssetStore{} }

func (*ToolFileAssetStore) Save(ctx context.Context, task *Task, audio []byte) (string, error) {
	if task == nil || len(audio) == 0 {
		return "", ErrInvalidRequest
	}
	filename := fmt.Sprintf("music-%s.mp3", task.ID.String())
	file, err := tool_file.CreateFileByRawGlobal(ctx, tool_file.CreateFileByRawParams{
		UserID:    task.AccountID.String(),
		TenantID:  task.OrganizationID.String(),
		FileData:  audio,
		MimeType:  "audio/mpeg",
		Filename:  &filename,
		Lifecycle: tool_file.ToolFileLifecyclePersistent,
	})
	if err != nil {
		return "", err
	}
	return file.ID, nil
}

func (*ToolFileAssetStore) Delete(ctx context.Context, fileID string) error {
	return tool_file.DeleteToolFileGlobal(ctx, fileID)
}

func (*ToolFileAssetStore) DeleteStoredObject(ctx context.Context, fileID string, scope Scope) error {
	return tool_file.DeleteStoredObjectGlobal(
		ctx,
		fileID,
		scope.OrganizationID.String(),
		scope.AccountID.String(),
	)
}

func (*ToolFileAssetStore) URL(_ context.Context, fileID string) (string, error) {
	rawURL, err := tool_file.SignToolFileGlobal(fileID, ".mp3")
	if err != nil {
		return "", err
	}
	parsedURL, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse signed music URL: %w", err)
	}
	query := parsedURL.Query()
	query.Set("delivery", "direct")
	parsedURL.RawQuery = query.Encode()
	return parsedURL.String(), nil
}
