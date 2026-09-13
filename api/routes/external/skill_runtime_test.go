package external

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestExternalSkillInputFileProviderUsesExternalUserForTemporaryFile(t *testing.T) {
	accountID := "api-key-account"
	externalUserID := "derived-external-user"
	fileService := &externalSkillRuntimeFileService{
		file: &dto.UploadFile{
			ID:          "file-1",
			Name:        "dataset.csv",
			Size:        7,
			CreatedBy:   externalUserID,
			IsTemporary: true,
		},
		data: []byte("a,b,c\n"),
	}
	provider := externalSkillInputFileProvider{fileService: fileService}

	file, err := provider.GetSkillScriptInputFile(context.Background(), "file-1", 100, skills.ExecutionContext{
		UserID:         accountID,
		ExternalUserID: externalUserID,
	})
	if err != nil {
		t.Fatalf("GetSkillScriptInputFile() error = %v", err)
	}
	if file.FileID != "file-1" || file.Filename != "dataset.csv" {
		t.Fatalf("file metadata = %#v, want file-1/dataset.csv", file)
	}
	if !bytes.Equal(file.Data, fileService.data) {
		t.Fatalf("file data = %q, want %q", file.Data, fileService.data)
	}
	if fileService.downloads != 1 {
		t.Fatalf("DownloadFile calls = %d, want 1", fileService.downloads)
	}
}

func TestExternalSkillInputFileProviderFallsBackToAccountForInternalTemporaryFile(t *testing.T) {
	fileService := &externalSkillRuntimeFileService{
		file: &dto.UploadFile{
			ID:          "file-1",
			Name:        "dataset.csv",
			Size:        7,
			CreatedBy:   "account-1",
			IsTemporary: true,
		},
		data: []byte("a,b,c\n"),
	}
	provider := externalSkillInputFileProvider{fileService: fileService}

	_, err := provider.GetSkillScriptInputFile(context.Background(), "file-1", 100, skills.ExecutionContext{
		UserID: "account-1",
	})
	if err != nil {
		t.Fatalf("GetSkillScriptInputFile() error = %v", err)
	}
}

func TestExternalSkillInputFileProviderRejectsWrongExternalUser(t *testing.T) {
	fileService := &externalSkillRuntimeFileService{
		file: &dto.UploadFile{
			ID:          "file-1",
			Size:        7,
			CreatedBy:   "derived-user-1",
			IsTemporary: true,
		},
	}
	provider := externalSkillInputFileProvider{fileService: fileService}

	_, err := provider.GetSkillScriptInputFile(context.Background(), "file-1", 100, skills.ExecutionContext{
		UserID:         "api-key-account",
		ExternalUserID: "derived-user-2",
	})
	if err == nil || !strings.Contains(err.Error(), "file is not accessible") {
		t.Fatalf("GetSkillScriptInputFile() error = %v, want inaccessible file", err)
	}
	if fileService.downloads != 0 {
		t.Fatalf("DownloadFile calls = %d, want 0", fileService.downloads)
	}
}

type externalSkillRuntimeFileService struct {
	interfaces.FileService
	file      *dto.UploadFile
	data      []byte
	downloads int
}

func (s *externalSkillRuntimeFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return s.file, nil
}

func (s *externalSkillRuntimeFileService) DownloadFile(context.Context, string) ([]byte, error) {
	s.downloads++
	return append([]byte(nil), s.data...), nil
}
