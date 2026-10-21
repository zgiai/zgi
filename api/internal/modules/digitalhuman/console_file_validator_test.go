package digitalhuman

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
)

func TestConsoleFileValidatorEnforcesAccountAndWorkspaceOwnership(t *testing.T) {
	scope := testRunScope()
	workspaceID := scope.WorkspaceID.String()
	validID := uuid.New()
	lookup := consoleFileLookupStub{files: map[string]*dto.UploadFile{
		validID.String(): {
			ID: validID.String(), OrganizationID: uuid.Nil.String(), WorkspaceID: &workspaceID,
			CreatedByRole: dto.CreatedByRoleAccount, CreatedBy: scope.ActorID.String(), IsTemporary: true,
		},
	}}
	validator := NewConsoleFileValidator(lookup)
	scope.Surface = SurfaceConsole

	got, err := validator.Validate(t.Context(), scope, []string{"  " + validID.String() + "  "})
	if err != nil || len(got) != 1 || got[0] != validID.String() {
		t.Fatalf("Validate(valid) = %v, %v", got, err)
	}

	tests := map[string]func(*dto.UploadFile){
		"other account":   func(file *dto.UploadFile) { file.CreatedBy = uuid.NewString() },
		"other workspace": func(file *dto.UploadFile) { value := uuid.NewString(); file.WorkspaceID = &value },
		"end user":        func(file *dto.UploadFile) { file.CreatedByRole = dto.CreatedByRoleEndUser },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			clonedFile := *lookup.files[validID.String()]
			mutate(&clonedFile)
			candidate := NewConsoleFileValidator(consoleFileLookupStub{files: map[string]*dto.UploadFile{validID.String(): &clonedFile}})
			if _, err := candidate.Validate(t.Context(), scope, []string{validID.String()}); !errors.Is(err, ErrConsoleFileNotAccessible) {
				t.Fatalf("Validate() error = %v, want ErrConsoleFileNotAccessible", err)
			}
		})
	}

	t.Run("temporary organization is not an ownership boundary", func(t *testing.T) {
		clonedFile := *lookup.files[validID.String()]
		clonedFile.OrganizationID = uuid.NewString()
		candidate := NewConsoleFileValidator(consoleFileLookupStub{files: map[string]*dto.UploadFile{validID.String(): &clonedFile}})
		if _, err := candidate.Validate(t.Context(), scope, []string{validID.String()}); err != nil {
			t.Fatalf("Validate() error = %v, want temporary file to ignore organization ownership", err)
		}
	})

	t.Run("persistent file still requires organization ownership", func(t *testing.T) {
		clonedFile := *lookup.files[validID.String()]
		clonedFile.IsTemporary = false
		clonedFile.OrganizationID = uuid.NewString()
		candidate := NewConsoleFileValidator(consoleFileLookupStub{files: map[string]*dto.UploadFile{validID.String(): &clonedFile}})
		if _, err := candidate.Validate(t.Context(), scope, []string{validID.String()}); !errors.Is(err, ErrConsoleFileNotAccessible) {
			t.Fatalf("Validate() error = %v, want ErrConsoleFileNotAccessible", err)
		}
	})
}

func TestConsoleFileValidatorRejectsInvalidAndDuplicateReferences(t *testing.T) {
	scope := testRunScope()
	scope.Surface = SurfaceConsole
	validator := NewConsoleFileValidator(consoleFileLookupStub{})
	fileID := uuid.NewString()
	for name, fileIDs := range map[string][]string{
		"invalid":   {"not-a-file-id"},
		"duplicate": {fileID, fileID},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validator.Validate(t.Context(), scope, fileIDs); !errors.Is(err, ErrInvalidRequest) {
				t.Fatalf("Validate() error = %v, want ErrInvalidRequest", err)
			}
		})
	}
}

type consoleFileLookupStub struct {
	files map[string]*dto.UploadFile
}

func (s consoleFileLookupStub) GetFileByID(_ context.Context, fileID string) (*dto.UploadFile, error) {
	file := s.files[fileID]
	if file == nil {
		return nil, errors.New("file not found")
	}
	return file, nil
}
