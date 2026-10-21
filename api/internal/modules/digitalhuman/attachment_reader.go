package digitalhuman

import (
	"context"

	"github.com/google/uuid"
	chatruntime "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/dto"
)

// AttachmentReadAuthorizer reuses interactive identity and upload boundaries,
// but does not create a Worker binding or require Linux to be enabled.
func (r *LinuxExecutionRuntime) AttachmentReadAuthorizer(run *Run, resolved *ResolvedExecution, event invocation.Event, current *Scope) func(context.Context, *chatruntime.PreparedChat, *dto.UploadFile) error {
	access, err := executionAccessFor(run, resolved, event)
	if err != nil || r == nil || r.options.Access == nil {
		return nil
	}
	authorizer, ok := r.options.Access.(interface {
		AuthorizeAttachmentAccess(context.Context, Scope, uuid.UUID, uuid.UUID) error
	})
	if !ok {
		return nil
	}
	authorizationScope := access.scope()
	if current != nil {
		if !continuationRunOwnedByScope(run, *current, access.application) || current.ActorID != access.user ||
			current.PrincipalID == uuid.Nil || current.PrincipalType != authorizationScope.PrincipalType {
			return nil
		}
		if access.surface == SurfaceAPI {
			authorizationScope.PrincipalID = current.PrincipalID
		}
	}
	return func(ctx context.Context, prepared *chatruntime.PreparedChat, file *dto.UploadFile) error {
		if _, err := access.resolve(prepared); err != nil {
			return err
		}
		if access.surface == SurfaceConsole {
			if r.options.AuthorizeConsole == nil {
				return ErrConsoleFileNotAccessible
			}
			if err := r.options.AuthorizeConsole(ctx, access.scope()); err != nil {
				return err
			}
		}
		if err := authorizer.AuthorizeAttachmentAccess(ctx, authorizationScope, access.application, access.publication); err != nil {
			return err
		}
		if access.surface == SurfaceConsole {
			if !consoleFileBelongsToScope(file, access.scope()) {
				return ErrConsoleFileNotAccessible
			}
			return nil
		}
		return r.options.Access.AuthorizeExecutionUpload(ctx, access.scope(), access.application, file)
	}
}

func (s *RuntimeService) attachAttachmentReader(config *chatruntime.RunConfig, run *Run, resolved *ResolvedExecution, event invocation.Event, current *Scope) {
	if provider, ok := s.linux.(interface {
		AttachmentReadAuthorizer(*Run, *ResolvedExecution, invocation.Event, *Scope) func(context.Context, *chatruntime.PreparedChat, *dto.UploadFile) error
	}); ok {
		config.AttachmentReadAuthorizer = provider.AttachmentReadAuthorizer(run, resolved, event, current)
	}
}
