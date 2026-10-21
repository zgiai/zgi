package tool_file

import (
	"context"
	"fmt"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/capabilities/toolgovernance"
)

type executionFileContextKey struct{}

// WithExecutionOrigin protects files created by a CLI business tool, including
// preview files. The origin is server-owned and overrides tool-selected owners.
func WithExecutionOrigin(ctx context.Context, origin toolgovernance.ExecutionOrigin) (context.Context, error) {
	if err := origin.Validate(); err != nil {
		return nil, fmt.Errorf("protect execution file: %w", err)
	}
	return context.WithValue(ctx, executionFileContextKey{}, origin), nil
}

func executionFileParams(ctx context.Context, params CreateFileByRawParams) CreateFileByRawParams {
	o, ok := ctx.Value(executionFileContextKey{}).(toolgovernance.ExecutionOrigin)
	if !ok {
		return params
	}
	params.TenantID, params.UserID = o.OrganizationID, o.SubjectID
	params.ConversationID, params.ExecutionMessageID = &o.ConversationID, &o.MessageID
	params.ExecutionIdentity = &execution.Identity{Version: 1, WorkspaceID: o.WorkspaceID, ApplicationType: o.ApplicationType,
		ApplicationID: o.ApplicationID, Entry: o.Entry, SubjectKind: o.SubjectKind, ConversationID: o.ConversationID, PublicationRef: o.PublicationRef}
	return params
}

// PublicGeneratedFileURL preserves legacy links, but protected files are
// addressed by ID through the authenticated conversation file transport.
func PublicGeneratedFileURL(file *ToolFile, extension string) (string, error) {
	return PublicGeneratedFileURLWithMode(file, extension, ToolFileURLModeSigned)
}

// PublicGeneratedFileURLWithMode keeps the requested legacy URL lifecycle.
func PublicGeneratedFileURLWithMode(file *ToolFile, extension string, mode ToolFileURLMode) (string, error) {
	if file == nil {
		return "", fmt.Errorf("generated file is required")
	}
	if file.ExecutionIdentity != nil {
		return "", nil
	}
	return SignToolFileGlobalWithMode(file.ID, extension, mode)
}

// ProtectGeneratedFileMetadata marks both file events and JSON tool results so
// history hydration cannot accidentally restore a legacy public URL.
func ProtectGeneratedFileMetadata(file *ToolFile, metadata ...map[string]interface{}) {
	if file == nil || file.ExecutionIdentity == nil {
		return
	}
	for _, value := range metadata {
		value["access_mode"] = "authenticated"
		delete(value, "url")
		delete(value, "download_url")
	}
}
