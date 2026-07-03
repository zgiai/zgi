package v1

import (
	"context"
	"strings"
	"testing"

	"github.com/zgiai/zgi/api/internal/dto"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
)

func TestSkillInputFileProviderRejectsOversizeBeforeDownload(t *testing.T) {
	fileService := &skillRuntimeFileService{
		file: &dto.UploadFile{
			ID:             "file-big",
			Name:           "big.xlsx",
			Size:           11,
			OrganizationID: "org-1",
			CreatedBy:      "user-1",
		},
		data: []byte("too large"),
	}
	provider := skillInputFileProvider{fileService: fileService}

	_, err := provider.GetSkillScriptInputFile(context.Background(), "file-big", 10, skills.ExecutionContext{
		OrganizationID: "org-1",
		UserID:         "user-1",
	})
	if err == nil || !strings.Contains(err.Error(), "max_bytes") {
		t.Fatalf("expected max_bytes rejection, got %v", err)
	}
	if fileService.downloads != 0 {
		t.Fatalf("DownloadFile calls = %d, want 0", fileService.downloads)
	}
}

type skillRuntimeFileService struct {
	interfaces.FileService
	file      *dto.UploadFile
	data      []byte
	downloads int
}

func (s *skillRuntimeFileService) GetFileByID(context.Context, string) (*dto.UploadFile, error) {
	return s.file, nil
}

func (s *skillRuntimeFileService) DownloadFile(context.Context, string) ([]byte, error) {
	s.downloads++
	return s.data, nil
}
