package digitalhuman

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/invocation"
	"github.com/zgiai/zgi/api/internal/capabilities/usersubject"
)

// AuthorizeLinuxExecution rechecks live application opt-in and external access.
// The caller separately checks console workspace permission. Credential IDs are
// references only; raw API keys and credential owners never enter this path.
func (s *AccessService) AuthorizeLinuxExecution(ctx context.Context, scope Scope, application, publication uuid.UUID) error {
	return s.authorizeInteractiveAttachmentAccess(ctx, scope, application, publication, true)
}

// AuthorizeAttachmentAccess checks the published interactive application and
// credential without requiring the optional Linux execution feature.
func (s *AccessService) AuthorizeAttachmentAccess(ctx context.Context, scope Scope, application, publication uuid.UUID) error {
	return s.authorizeInteractiveAttachmentAccess(ctx, scope, application, publication, false)
}

func (s *AccessService) authorizeInteractiveAttachmentAccess(ctx context.Context, scope Scope, application, publication uuid.UUID, requireLinux bool) error {
	if s == nil || s.db == nil || application == uuid.Nil || publication == uuid.Nil || scope.OrganizationID == uuid.Nil || scope.WorkspaceID == uuid.Nil ||
		scope.UserSubjectID == nil || *scope.UserSubjectID == uuid.Nil || scope.ActorID != *scope.UserSubjectID || scope.PrincipalID == uuid.Nil {
		return ErrNotFound
	}
	switch scope.Surface {
	case SurfaceConsole:
		if scope.WebAppID != nil {
			return ErrNotFound
		}
		fallthrough
	case SurfaceWebApp:
		if scope.UserSubjectKind != string(usersubject.Account) || scope.PrincipalType != string(invocation.PrincipalAccount) || scope.PrincipalID != scope.ActorID {
			return ErrNotFound
		}
	case SurfaceAPI:
		if scope.UserSubjectKind != string(usersubject.APIEndUser) || scope.PrincipalType != string(invocation.PrincipalService) || scope.WebAppID != nil {
			return ErrNotFound
		}
	default:
		return ErrNotFound
	}
	access, err := s.publishedAccess(ctx, application, scope.OrganizationID, scope.WorkspaceID)
	if err != nil {
		return err
	}
	if access.Version.ID != publication || access.Version.OrganizationID != scope.OrganizationID || access.Version.WorkspaceID != scope.WorkspaceID ||
		(requireLinux && (access.Version.ExecutionRuntime == nil || !access.Version.ExecutionRuntime.Enabled)) {
		return ErrPublishInvalid
	}
	switch scope.Surface {
	case SurfaceWebApp:
		if scope.WebAppID == nil || *scope.WebAppID == uuid.Nil || access.Definition.WebAppID != *scope.WebAppID || !access.Settings.WebAppEnabled {
			return ErrNotFound
		}
	case SurfaceAPI:
		if !access.Settings.APIEnabled {
			return ErrNotFound
		}
		var count int64
		if err := s.db.WithContext(ctx).Model(&APIKey{}).
			Where("id = ? AND digital_human_id = ? AND organization_id = ? AND workspace_id = ? AND status = ? AND (expires_at IS NULL OR expires_at > ?)",
				scope.PrincipalID, application, scope.OrganizationID, scope.WorkspaceID, APIKeyStatusActive, s.now()).Count(&count).Error; err != nil {
			return err
		}
		if count != 1 {
			return ErrNotFound
		}
	}
	return nil
}
