package digitalhuman

import (
	"context"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

// AuthorizeExecutionUpload rechecks the existing immutable external file binding
// when a prepared attachment enters Linux or is read by the platform CLI. It
// never substitutes a credential principal or the application creator for the
// explicit end user. Console attachments retain the separate console validator.
func (s *AccessService) AuthorizeExecutionUpload(ctx context.Context, scope Scope, digitalHumanID uuid.UUID, file *dto.UploadFile) error {
	if s == nil || s.db == nil || file == nil || !file.IsTemporary || scope.UserSubjectID == nil ||
		*scope.UserSubjectID == uuid.Nil || scope.ActorID != *scope.UserSubjectID || file.CreatedBy != scope.UserSubjectID.String() {
		return ErrNotFound
	}
	switch scope.Surface {
	case SurfaceWebApp:
		if scope.UserSubjectKind != "account" || file.CreatedByRole != dto.CreatedByRoleAccount {
			return ErrNotFound
		}
	case SurfaceAPI:
		if scope.UserSubjectKind != "api_end_user" || file.CreatedByRole != dto.CreatedByRoleEndUser {
			return ErrNotFound
		}
	default:
		return ErrNotFound
	}
	return s.ValidateExternalFiles(ctx, scope, digitalHumanID, []string{file.ID}, scope.Surface)
}
