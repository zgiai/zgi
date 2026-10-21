package executionruntime

import (
	"context"

	"github.com/google/uuid"
	execution "github.com/zgiai/zgi/api/internal/capabilities/agentexecution"
	runtimeservice "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/service"
	"github.com/zgiai/zgi/api/internal/contracts/chatfile"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/pkg/apperror"
	"github.com/zgiai/zgi/api/pkg/apperror/catalog"
)

// AttachmentSource provides metadata and bytes; it does not authorize access.
type AttachmentSource interface {
	GetFileByID(context.Context, string) (*dto.UploadFile, error)
	DownloadFile(context.Context, string) ([]byte, error)
}

// ScopedAttachmentReader freezes one turn's admitted references. Authorize must
// recheck current file permissions and application/entry upload ownership; chat
// preparation alone is not authorization for a later Shell or CLI request.
type ScopedAttachmentReader struct {
	source    AttachmentSource
	scope     execution.Scope
	refs      []runtimeservice.RuntimeAttachmentRef
	authorize func(context.Context, *dto.UploadFile) error
}

// NewScopedAttachmentReader copies the scope and references and requires a live
// authorizer. The source and authorizer must remain valid for the prepared turn.
func NewScopedAttachmentReader(source AttachmentSource, scope execution.Scope, refs []runtimeservice.RuntimeAttachmentRef, authorize func(context.Context, *dto.UploadFile) error) (*ScopedAttachmentReader, error) {
	if source == nil || authorize == nil || scope.OrganizationID == "" || scope.SubjectID == "" {
		return nil, inputAccessDenied()
	}
	if scope.Identity != nil {
		if scope.Identity.Validate() != nil {
			return nil, inputAccessDenied()
		}
		identity := *scope.Identity
		scope.Identity = &identity
	}
	seen := make(map[string]bool, len(refs))
	var total int64
	for _, ref := range refs {
		id, err := uuid.Parse(ref.ID)
		if err != nil || id == uuid.Nil || id.String() != ref.ID || seen[ref.ID] || ref.Size < 0 || ref.Size > chatfile.MaxTurnBytes-total {
			return nil, inputAccessDenied()
		}
		total += ref.Size
		if total > chatfile.MaxTurnBytes {
			return nil, inputAccessDenied()
		}
		seen[ref.ID] = true
	}
	return &ScopedAttachmentReader{source: source, scope: scope, refs: append([]runtimeservice.RuntimeAttachmentRef(nil), refs...), authorize: authorize}, nil
}

func (r *ScopedAttachmentReader) matches(scope execution.Scope, refs []runtimeservice.RuntimeAttachmentRef) bool {
	if r == nil || !sameFactoryScope(r.scope, scope) || len(r.refs) != len(refs) {
		return false
	}
	for i, ref := range refs {
		if r.refs[i] != ref {
			return false
		}
	}
	return true
}

// CheckAttachments also runs for commands reusing already staged inputs. Revoked
// inputs must not become usable merely because staging was cached for this turn.
func (r *ScopedAttachmentReader) CheckAttachments(ctx context.Context) error {
	for _, ref := range r.refs {
		if err := r.check(ctx, ref); err != nil {
			return err
		}
	}
	return nil
}

func (r *ScopedAttachmentReader) check(ctx context.Context, ref runtimeservice.RuntimeAttachmentRef) error {
	file, err := r.source.GetFileByID(ctx, ref.ID)
	if err != nil || file == nil || file.ID != ref.ID || file.Name != ref.Name || file.Size != ref.Size {
		return inputAccessDenied()
	}
	if err := r.authorize(ctx, file); err != nil {
		return inputAccessDenied()
	}
	return nil
}

// DownloadFile authorizes before reading bytes and never accepts an arbitrary
// file ID, even when the underlying generic file service could read that file.
func (r *ScopedAttachmentReader) DownloadFile(ctx context.Context, id string) ([]byte, error) {
	for _, ref := range r.refs {
		if ref.ID != id {
			continue
		}
		if err := r.check(ctx, ref); err != nil {
			return nil, err
		}
		data, err := r.source.DownloadFile(ctx, id)
		if err != nil || int64(len(data)) != ref.Size {
			return nil, inputAccessDenied()
		}
		return data, nil
	}
	return nil, inputAccessDenied()
}

func inputAccessDenied() error {
	return apperror.New(catalog.CodePermissionDenied, apperror.WithOperation("agent_execution.input_access"))
}
