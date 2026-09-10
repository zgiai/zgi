package digitalhuman

import (
	"context"
	"strings"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

const maxConsoleInvocationFiles = 10

// ConsoleFileLookup is the smallest file-service boundary needed before a
// DigitalHuman console invocation becomes durable.
type ConsoleFileLookup interface {
	GetFileByID(context.Context, string) (*dto.UploadFile, error)
}

// ManualInvocationFileValidator normalizes and validates console attachment
// references before they are copied into the immutable invocation context.
type ManualInvocationFileValidator interface {
	Validate(context.Context, Scope, []string) ([]string, error)
}

type consoleFileValidator struct {
	files ConsoleFileLookup
}

func NewConsoleFileValidator(files ConsoleFileLookup) ManualInvocationFileValidator {
	if files == nil {
		panic("DigitalHuman console file validator requires file lookup")
	}
	return &consoleFileValidator{files: files}
}

func (v *consoleFileValidator) Validate(ctx context.Context, scope Scope, fileIDs []string) ([]string, error) {
	if !validScope(scope) || scope.Surface != SurfaceConsole || len(fileIDs) > maxConsoleInvocationFiles {
		return nil, ErrInvalidRequest
	}
	seen := make(map[uuid.UUID]struct{}, len(fileIDs))
	normalized := make([]string, 0, len(fileIDs))
	for _, raw := range fileIDs {
		fileID, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil || fileID == uuid.Nil {
			return nil, ErrInvalidRequest
		}
		if _, exists := seen[fileID]; exists {
			return nil, ErrInvalidRequest
		}
		seen[fileID] = struct{}{}
		normalized = append(normalized, fileID.String())
	}

	for _, fileID := range normalized {
		file, err := v.files.GetFileByID(ctx, fileID)
		if err != nil || file == nil || !consoleFileBelongsToScope(file, scope) {
			return nil, ErrConsoleFileNotAccessible
		}
	}
	return normalized, nil
}

func consoleFileBelongsToScope(file *dto.UploadFile, scope Scope) bool {
	if file == nil || file.CreatedByRole != dto.CreatedByRoleAccount ||
		strings.TrimSpace(file.CreatedBy) != scope.ActorID.String() {
		return false
	}
	organizationID := strings.TrimSpace(file.OrganizationID)
	if organizationID == "" {
		organizationID = strings.TrimSpace(file.TenantID)
	}
	return organizationID == scope.OrganizationID.String() && file.WorkspaceID != nil &&
		strings.TrimSpace(*file.WorkspaceID) == scope.WorkspaceID.String()
}
