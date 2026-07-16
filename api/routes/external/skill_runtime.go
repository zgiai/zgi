package external

import (
	"context"
	"fmt"
	"strings"
	"time"

	appconfig "github.com/zgiai/zgi/api/config"
	"github.com/zgiai/zgi/api/internal/modules/shared/interface"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/internal/modules/tools"
	workspacemodel "github.com/zgiai/zgi/api/internal/modules/workspace/model"
)

func newExternalSkillRuntimeWithSandbox(toolEngine *tools.ToolEngine, toolManager *tools.ToolManager, fileService interfaces.FileService, organizationService interfaces.OrganizationService) *skills.Runtime {
	runtime := skills.NewRuntime(toolEngine, toolManager)
	if appconfig.GlobalConfig == nil {
		return runtime
	}
	return runtime.WithScriptRunner(skills.NewSandboxScriptRunner(skills.SandboxScriptRunnerConfig{
		Endpoint:              appconfig.GlobalConfig.CodeExec.Endpoint,
		APIKey:                appconfig.GlobalConfig.CodeExec.APIKey,
		ConnectTimeout:        externalSecondsDuration(appconfig.GlobalConfig.CodeExec.ConnectTimeoutSeconds),
		CreateTimeout:         externalSecondsDuration(appconfig.GlobalConfig.CodeExec.CreateTimeoutSeconds),
		UploadTimeout:         externalSecondsDuration(appconfig.GlobalConfig.CodeExec.UploadTimeoutSeconds),
		CommandTimeoutPadding: externalSecondsDuration(appconfig.GlobalConfig.CodeExec.CommandTimeoutPaddingSeconds),
		ArtifactTimeout:       externalSecondsDuration(appconfig.GlobalConfig.CodeExec.ArtifactTimeoutSeconds),
		CleanupTimeout:        externalSecondsDuration(appconfig.GlobalConfig.CodeExec.CleanupTimeoutSeconds),
		InputFileProvider:     externalSkillInputFileProvider{fileService: fileService, organizationService: organizationService},
	}))
}

func externalSecondsDuration(seconds int) time.Duration {
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

type externalSkillInputFileProvider struct {
	fileService         interfaces.FileService
	organizationService interfaces.OrganizationService
}

func (p externalSkillInputFileProvider) GetSkillScriptInputFile(ctx context.Context, fileID string, maxBytes int64, execCtx skills.ExecutionContext) (skills.SkillScriptInputFile, error) {
	if p.fileService == nil {
		return skills.SkillScriptInputFile{}, fmt.Errorf("file service is unavailable")
	}
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return skills.SkillScriptInputFile{}, fmt.Errorf("file_id is required")
	}
	file, err := p.fileService.GetFileByID(ctx, fileID)
	if err != nil {
		return skills.SkillScriptInputFile{}, err
	}
	if file == nil {
		return skills.SkillScriptInputFile{}, fmt.Errorf("file not found")
	}
	if maxBytes > 0 && file.Size > maxBytes {
		return skills.SkillScriptInputFile{}, fmt.Errorf("file exceeds max_bytes %d", maxBytes)
	}
	organizationID := externalNonZeroUUIDString(file.OrganizationID)
	if organizationID == "" {
		organizationID = externalNonZeroUUIDString(file.TenantID)
	}
	expectedOrganizationID := externalNonZeroUUIDString(execCtx.OrganizationID)
	if expectedOrganizationID != "" && organizationID != "" && organizationID != expectedOrganizationID {
		return skills.SkillScriptInputFile{}, fmt.Errorf("file is not accessible")
	}
	if file.IsTemporary {
		expectedUser := strings.TrimSpace(execCtx.UserID)
		if expectedUser == "" || strings.TrimSpace(file.CreatedBy) != expectedUser {
			return skills.SkillScriptInputFile{}, fmt.Errorf("file is not accessible")
		}
	}
	if organizationID == "" {
		organizationID = expectedOrganizationID
	}
	if file.WorkspaceID != nil && externalNonZeroUUIDString(*file.WorkspaceID) != "" {
		if p.organizationService == nil {
			return skills.SkillScriptInputFile{}, fmt.Errorf("workspace permission service is unavailable")
		}
		accountID := strings.TrimSpace(execCtx.UserID)
		if accountID == "" {
			return skills.SkillScriptInputFile{}, fmt.Errorf("user id is required to access workspace file")
		}
		allowed, err := p.organizationService.CheckWorkspacePermission(ctx, organizationID, externalNonZeroUUIDString(*file.WorkspaceID), accountID, workspacemodel.WorkspacePermissionFilePreview)
		if err != nil {
			return skills.SkillScriptInputFile{}, fmt.Errorf("failed to check workspace file permission: %w", err)
		}
		if !allowed {
			return skills.SkillScriptInputFile{}, fmt.Errorf("file is not accessible")
		}
	}
	data, err := p.fileService.DownloadFile(ctx, fileID)
	if err != nil {
		return skills.SkillScriptInputFile{}, err
	}
	return skills.SkillScriptInputFile{
		FileID:         file.ID,
		Filename:       file.Name,
		Extension:      file.Extension,
		MimeType:       file.MimeType,
		Size:           file.Size,
		Data:           data,
		OrganizationID: organizationID,
		TenantID:       file.TenantID,
		CreatedBy:      file.CreatedBy,
		IsTemporary:    file.IsTemporary,
	}, nil
}

func externalNonZeroUUIDString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "00000000-0000-0000-0000-000000000000" {
		return ""
	}
	return value
}
