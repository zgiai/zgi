package digitalhuman

import (
	"context"
	"time"

	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	"github.com/zgiai/zgi/api/internal/capabilities/channelruntime"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	toolfile "github.com/zgiai/zgi/api/internal/modules/app/workflow/tool_file"
)

// AuthorizeBackgroundExecutionArtifact admits a protected result only for its
// exact live run, publication and message. Callers must use the execution file
// reader with the returned identity; account-based file reads are insufficient.
func (s *AccessService) AuthorizeBackgroundExecutionArtifact(ctx context.Context, run *Run, event invocation.Event, file *toolfile.ToolFile) (toolfile.ExecutionFileAccess, error) {
	if err := s.AuthorizeBackgroundLinuxExecution(ctx, run, event); err != nil {
		return toolfile.ExecutionFileAccess{}, err
	}
	return backgroundExecutionFileAccess(run, file)
}

// backgroundExecutionFileAccess binds an artifact to the exact frozen run and
// message, not the legacy billing account or a model-supplied path.
func backgroundExecutionFileAccess(run *Run, file *toolfile.ToolFile) (toolfile.ExecutionFileAccess, error) {
	resolved, event, err := decodeAndVerifyRun(run)
	if err != nil {
		return toolfile.ExecutionFileAccess{}, toolfile.ErrExecutionFileAccess
	}
	a, err := backgroundExecutionAccessFor(run, resolved, event)
	if err != nil || run.MessageID == nil || file == nil || file.ExecutionMessageID == nil || *file.ExecutionMessageID != run.MessageID.String() || file.ExecutionIdentity == nil ||
		file.ExecutionIdentity.Version != execution.BackgroundIdentityVersion || file.ExecutionIdentity.PublicationRef != a.publication.String() || file.DeletedAt != nil || file.IsExpired(time.Now()) {
		return toolfile.ExecutionFileAccess{}, toolfile.ErrExecutionFileAccess
	}
	access := toolfile.ExecutionFileAccess{OrganizationID: a.organization.String(), WorkspaceID: a.workspace.String(),
		ApplicationType: "digital_human", ApplicationID: a.application.String(), Entry: a.entry, SubjectKind: string(a.kind), SubjectID: a.user.String(), ConversationID: a.conversation.String()}
	i := file.ExecutionIdentity
	if i.Validate() != nil || file.TenantID != access.OrganizationID || file.UserID != access.SubjectID || file.ConversationID == nil || *file.ConversationID != access.ConversationID ||
		i.WorkspaceID != access.WorkspaceID || i.ApplicationType != access.ApplicationType || i.ApplicationID != access.ApplicationID || i.Entry != access.Entry || i.SubjectKind != access.SubjectKind || i.ConversationID != access.ConversationID {
		return toolfile.ExecutionFileAccess{}, toolfile.ErrExecutionFileAccess
	}
	return access, nil
}

func (s *RuntimeService) DescribeChannelExecutionArtifact(ctx context.Context, delivery *channelruntime.Delivery, id string) (*toolfile.ToolFile, error) {
	_, file, err := s.channelExecutionFile(ctx, delivery, id)
	return file, err
}

func (s *RuntimeService) ReadChannelExecutionArtifact(ctx context.Context, delivery *channelruntime.Delivery, id string) ([]byte, error) {
	access, _, err := s.channelExecutionFile(ctx, delivery, id)
	if err != nil {
		return nil, err
	}
	data, _, err := toolfile.GlobalToolFileManager.GetExecutionFileBinary(ctx, access, id)
	return data, err
}

func (s *RuntimeService) channelExecutionFile(ctx context.Context, delivery *channelruntime.Delivery, id string) (toolfile.ExecutionFileAccess, *toolfile.ToolFile, error) {
	denied := toolfile.ErrExecutionFileAccess
	if s == nil || delivery == nil || toolfile.GlobalToolFileManager == nil {
		return toolfile.ExecutionFileAccess{}, nil, denied
	}
	store, ok := s.store.(ChannelStore)
	if !ok {
		return toolfile.ExecutionFileAccess{}, nil, denied
	}
	route, err := store.ResolveChannelRoute(ctx, delivery.InstallationID, delivery.ExternalConversationID)
	if err != nil || route == nil || route.Binding == nil || route.Installation == nil || !route.Binding.Enabled || route.Installation.Status != channelruntime.InstallationStatusActive ||
		route.Binding.ID != delivery.BindingID || route.Binding.SubjectID != delivery.SubjectID || route.Binding.SubjectType != delivery.SubjectType ||
		route.Binding.OrganizationID != delivery.OrganizationID || route.Binding.WorkspaceID != delivery.WorkspaceID || route.Binding.InstallationID != delivery.InstallationID ||
		route.Installation.ID != delivery.InstallationID || route.Installation.OrganizationID != delivery.OrganizationID || route.Installation.WorkspaceID != delivery.WorkspaceID || route.Installation.Provider != delivery.Provider || route.Installation.ConnectionID != delivery.ConnectionID {
		return toolfile.ExecutionFileAccess{}, nil, denied
	}
	run, err := s.store.RunByID(ctx, Scope{OrganizationID: delivery.OrganizationID, WorkspaceID: delivery.WorkspaceID}, delivery.SubjectID, delivery.RunID)
	if err != nil || run == nil || run.ConversationID == nil {
		return toolfile.ExecutionFileAccess{}, nil, denied
	}
	_, event, err := decodeAndVerifyRun(run)
	if err != nil || event.ReplyTo == nil || event.ReplyTo.Provider != delivery.Provider || event.ReplyTo.InstallationID != delivery.InstallationID.String() || event.ReplyTo.ConversationID != delivery.ExternalConversationID || event.ReplyTo.ThreadID != delivery.ExternalThreadID {
		return toolfile.ExecutionFileAccess{}, nil, denied
	}
	if run.Surface == SurfaceChannel && (route.Mapping == nil || route.Mapping.LocalConversationID == nil || *route.Mapping.LocalConversationID != *run.ConversationID) {
		return toolfile.ExecutionFileAccess{}, nil, denied
	}
	file, err := toolfile.GlobalToolFileManager.GetToolFileByID(ctx, id)
	if err != nil {
		return toolfile.ExecutionFileAccess{}, nil, denied
	}
	access, err := backgroundExecutionFileAccess(run, file)
	if err != nil {
		return toolfile.ExecutionFileAccess{}, nil, err
	}
	return access, file, nil
}

// Keep the internal reader identity contract independent from user account IDs.
var _ channelExecutionArtifactReader = (*RuntimeService)(nil)
