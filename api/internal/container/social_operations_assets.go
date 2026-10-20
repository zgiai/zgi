package container

import (
	"context"
	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/modules/connectors/core"
	"github.com/zgiai/zgi/api/internal/modules/connectors/providers/socialmcp"
	"github.com/zgiai/zgi/api/internal/modules/digitalhuman"
	"github.com/zgiai/zgi/api/internal/modules/socialops"
	"net/http"
)

// UploadBrandImage stores only verified image bytes in the account-scoped SOP
// journal. Filenames, paths and provider credentials are never accepted.
func (s *SocialOperationsSetupService) UploadBrandImage(ctx context.Context, scope digitalhuman.Scope, employeeID uuid.UUID, data []byte) (socialops.Artifact, error) {
	restored, _, err := s.restoreManagementScope(ctx, scope, employeeID)
	if err != nil {
		return socialops.Artifact{}, err
	}
	if _, _, err = socialops.ImageDimensions(data); err != nil {
		return socialops.Artifact{}, err
	}
	if _, err = socialmcp.PrepareImage(data); err != nil {
		return socialops.Artifact{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	mime := http.DetectContentType(data)
	switch mime {
	case "image/png", "image/jpeg", "image/webp":
	default:
		return socialops.Artifact{}, core.NewError(core.ErrorCodeInvalidInput, false, nil)
	}
	return s.journal.PutArtifact(ctx, restored, mime, data)
}
