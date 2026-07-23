package service

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	runtimemodel "github.com/zgiai/zgi/api/internal/capabilities/chatruntime/model"
	"github.com/zgiai/zgi/api/internal/capabilities/chatruntime/repository"
	"github.com/zgiai/zgi/api/internal/modules/skills"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/gorm"
)

const (
	customSkillStorageRoot    = "storage/aichat/skills"
	customSkillMaxPackageSize = 20 * 1024 * 1024
	customSkillMaxFileSize    = 5 * 1024 * 1024
	customSkillMaxFileCount   = 200
)

const customSkillSystemNameConflictMessage = "This skill name is reserved by a built-in system skill. Please rename your custom skill and try again."

const customSkillDeleteBindingOperation = "delete_custom_skill"

type extractedSkillFile struct {
	Path string `json:"path"`
	Size int64  `json:"size"`
}

type extractedSkillPackage struct {
	Root        string
	FileCount   int
	TotalSize   int64
	Files       []string
	FileDetails []extractedSkillFile
}

func (s *service) PreviewImportCustomSkill(ctx context.Context, scope Scope, fileHeader *multipart.FileHeader) (*SkillImportPreview, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	if fileHeader == nil {
		return nil, fmt.Errorf("%w: skill package is required", ErrInvalidInput)
	}
	if s.skillRuntime == nil {
		return nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if s.repos == nil || s.repos.CustomSkill == nil {
		return nil, fmt.Errorf("custom skill repository is not configured")
	}
	if s.customSkillStorage == nil {
		return nil, fmt.Errorf("custom skill storage is not configured")
	}
	data, err := readUploadedSkillPackage(fileHeader)
	if err != nil {
		return nil, err
	}
	importID := uuid.New().String()
	preview, err := s.customSkillStorage.SavePreviewPackage(ctx, scope.OrganizationID, importID, data)
	if err != nil {
		return nil, err
	}
	result := skillImportPreviewFromStored(preview)
	doc, err := s.skillRuntime.LoadCustomSkillDocument(preview.Root)
	if err != nil {
		_ = s.customSkillStorage.DeleteSkill(ctx, preview.Root)
		result.ImportID = ""
		result.ExpiresAt = time.Time{}
		result.ValidationErrors = []string{err.Error()}
		result.CanImport = false
		return result, nil
	}
	if s.customSkillIDConflictsWithSystem(ctx, doc.Metadata.ID) {
		_ = s.customSkillStorage.DeleteSkill(ctx, preview.Root)
		result.ImportID = ""
		result.ExpiresAt = time.Time{}
		result.Skill = skillDiscoveryMetadataPtr(doc)
		result.ValidationErrors = []string{customSkillSystemNameConflictMessage}
		result.CanImport = false
		return result, nil
	}
	result.Skill = skillDiscoveryMetadataPtr(doc)
	result.References = skillReferencePaths(doc)
	result.HasScripts = doc.Metadata.HasScripts
	result.ScriptsSupported = doc.Metadata.ScriptsSupported
	if existing, err := s.existingCustomSkill(ctx, scope.OrganizationID, doc.Metadata.ID); err != nil {
		_ = s.customSkillStorage.DeleteSkill(ctx, preview.Root)
		return nil, err
	} else if existing != nil {
		result.WillOverwrite = true
		result.ExistingSkill = existingSkillPreview(existing)
	}
	if doc.Metadata.HasScripts && !doc.Metadata.ScriptsSupported {
		result.Warnings = append(result.Warnings, "scripts are present but are not supported for custom skills")
	}
	result.CanImport = true
	return result, nil
}

func (s *service) ConfirmCustomSkillImport(ctx context.Context, scope Scope, importID string, overwriteConfirmed bool) (*skills.SkillDiscoveryMetadata, error) {
	return s.confirmCustomSkillImport(ctx, scope, importID, overwriteConfirmed)
}

func (s *service) confirmCustomSkillImport(ctx context.Context, scope Scope, importID string, overwriteConfirmed bool) (*skills.SkillDiscoveryMetadata, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	if s.skillRuntime == nil {
		return nil, fmt.Errorf("%w: skill runtime is not configured", ErrInvalidInput)
	}
	if s.repos == nil || s.repos.CustomSkill == nil {
		return nil, fmt.Errorf("custom skill repository is not configured")
	}
	if s.customSkillStorage == nil {
		return nil, fmt.Errorf("custom skill storage is not configured")
	}
	preview, err := s.customSkillStorage.LoadPreview(ctx, scope.OrganizationID, importID)
	if err != nil {
		return nil, err
	}
	doc, err := s.skillRuntime.LoadCustomSkillDocument(preview.Root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if s.customSkillIDConflictsWithSystem(ctx, doc.Metadata.ID) {
		return nil, fmt.Errorf("%w: %s", ErrInvalidInput, customSkillSystemNameConflictMessage)
	}
	existing, err := s.existingCustomSkill(ctx, scope.OrganizationID, doc.Metadata.ID)
	if err != nil {
		return nil, err
	}
	if existing != nil && !overwriteConfirmed {
		return nil, fmt.Errorf("%w: custom skill already exists; confirm overwrite before importing", ErrInvalidInput)
	}
	published, finalRoot, err := s.customSkillStorage.PublishPreview(ctx, preview, doc.Metadata.ID)
	if err != nil {
		return nil, err
	}
	extracted := extractedSkillPackageFromPreview(preview)
	record := customSkillRecordFromDocument(scope, doc, finalRoot, extracted)
	if err := s.repos.CustomSkill.Upsert(ctx, record); err != nil {
		published.rollback()
		return nil, err
	}
	published.cleanup()
	metadata := skillDiscoveryMetadataPtr(doc)
	metadata.Enabled = s.isOrganizationSkillEnabled(ctx, scope.OrganizationID, metadata.ID)
	return metadata, nil
}

func (s *service) existingCustomSkill(ctx context.Context, organizationID uuid.UUID, skillID string) (*runtimemodel.CustomSkill, error) {
	if s.repos == nil || s.repos.CustomSkill == nil {
		return nil, fmt.Errorf("custom skill repository is not configured")
	}
	existing, err := s.repos.CustomSkill.GetBySkillID(ctx, organizationID, skillID)
	if err == nil {
		return existing, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(mapRepoError(err), ErrNotFound) {
		return nil, nil
	}
	return nil, err
}

func (s *service) CancelCustomSkillImportPreview(ctx context.Context, scope Scope, importID string) error {
	if err := s.ensureMember(ctx, scope); err != nil {
		return err
	}
	if s.customSkillStorage == nil {
		return fmt.Errorf("custom skill storage is not configured")
	}
	return s.customSkillStorage.DeletePreview(ctx, scope.OrganizationID, importID)
}

func (s *service) CleanupExpiredCustomSkillImportPreviews(ctx context.Context) error {
	if s.customSkillStorage == nil {
		return nil
	}
	s.customSkillStorage.CleanupExpiredPreviews(ctx, time.Now())
	return nil
}

func (s *service) customSkillIDConflictsWithSystem(ctx context.Context, skillID string) bool {
	_ = ctx
	if s.skillRuntime == nil {
		return false
	}
	return s.skillRuntime.SystemSkillExists(skillID)
}

func (s *service) DeleteSkill(ctx context.Context, scope Scope, skillID, agentBindingAction, impactToken string) error {
	if err := s.ensureMember(ctx, scope); err != nil {
		return err
	}
	agentBindingAction = strings.ToLower(strings.TrimSpace(agentBindingAction))
	if agentBindingAction != "" && agentBindingAction != "unbind" {
		return fmt.Errorf("%w: invalid agent binding action", ErrInvalidInput)
	}
	id := strings.ToLower(strings.TrimSpace(skillID))
	if id == "" {
		return fmt.Errorf("%w: skill id is required", ErrInvalidInput)
	}
	if s.skillRuntime != nil {
		if metadata, err := s.skillRuntime.GetSkillMetadata(ctx, id); err == nil && metadata.Source == skills.SkillSourceSystem {
			return fmt.Errorf("%w: system skill cannot be deleted", ErrInvalidInput)
		}
	}
	if s.repos == nil || s.repos.CustomSkill == nil || s.repos.SkillConfig == nil || s.repos.DB == nil {
		return fmt.Errorf("custom skill repository is not configured")
	}
	record, err := s.repos.CustomSkill.GetBySkillID(ctx, scope.OrganizationID, id)
	if err != nil {
		return mapRepoError(err)
	}
	bindingRef := agentbindings.ResourceRef{
		OrganizationID: scope.OrganizationID,
		BindingType:    agentbindings.BindingTypeSkill,
		ResourceID:     id,
	}
	bindingRepo := agentbindings.NewRepository(s.repos.DB)
	var affectedAgentIDs []uuid.UUID
	if err := s.repos.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		txBindingRepo := bindingRepo.WithTx(tx)
		if err := txBindingRepo.LockResources(ctx, tx, []agentbindings.ResourceRef{bindingRef}); err != nil {
			return fmt.Errorf("lock custom skill agent binding resource: %w", err)
		}
		impact, err := txBindingRepo.PreviewImpact(ctx, bindingRef, customSkillDeleteBindingOperation, scope.AccountID, time.Now())
		if err != nil {
			return fmt.Errorf("preview custom skill agent binding impact: %w", err)
		}
		if impact != nil {
			if agentBindingAction != "unbind" {
				return &agentbindings.ConflictError{Impact: *impact}
			}
			if err := txBindingRepo.VerifyImpactToken(ctx, bindingRef, customSkillDeleteBindingOperation, scope.AccountID, impactToken, time.Now()); err != nil {
				return &agentbindings.ConflictError{Impact: *impact}
			}
			affectedAgentIDs, err = txBindingRepo.RevokeAndPruneDrafts(ctx, tx, bindingRef, scope.AccountID)
			if err != nil {
				return fmt.Errorf("revoke custom skill agent bindings: %w", err)
			}
		}

		txRepos := repository.NewRepositories(tx)
		if err := txRepos.CustomSkill.DeleteBySkillID(ctx, scope.OrganizationID, id); err != nil {
			return mapRepoError(err)
		}
		if err := txRepos.SkillConfig.DeleteByOrganizationAndSkill(ctx, scope.OrganizationID, id); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return err
	}
	if len(affectedAgentIDs) > 0 {
		logger.InfoContext(ctx, "agent resource bindings revoked for custom skill deletion",
			"log_type", "audit",
			"actor_account_id", scope.AccountID,
			"organization_id", scope.OrganizationID,
			"binding_type", agentbindings.BindingTypeSkill,
			"resource_id", id,
			"affected_agent_ids", affectedAgentIDs,
			"binding_state_before", "bound",
			"binding_state_after", "unbound",
			"published_scope_revoked", true,
			"drafts_pruned", true,
		)
	}
	if strings.TrimSpace(record.StoragePath) != "" {
		if err := s.customSkillStorage.DeleteSkill(ctx, record.StoragePath); err != nil {
			logger.WarnContext(ctx, "failed to remove custom skill directory", "skill_id", id, "path", record.StoragePath, err)
		}
	}
	return nil
}

func (s *service) PreviewSkillDeleteImpact(ctx context.Context, scope Scope, skillID string) (*agentbindings.Impact, error) {
	if err := s.ensureMember(ctx, scope); err != nil {
		return nil, err
	}
	id := strings.ToLower(strings.TrimSpace(skillID))
	if id == "" {
		return nil, fmt.Errorf("%w: skill id is required", ErrInvalidInput)
	}
	if s.skillRuntime != nil {
		if metadata, err := s.skillRuntime.GetSkillMetadata(ctx, id); err == nil && metadata.Source == skills.SkillSourceSystem {
			return nil, fmt.Errorf("%w: system skill cannot be deleted", ErrInvalidInput)
		}
	}
	if s.repos == nil || s.repos.CustomSkill == nil || s.repos.DB == nil {
		return nil, fmt.Errorf("custom skill repository is not configured")
	}
	if _, err := s.repos.CustomSkill.GetBySkillID(ctx, scope.OrganizationID, id); err != nil {
		return nil, mapRepoError(err)
	}
	return agentbindings.NewRepository(s.repos.DB).PreviewImpact(ctx, agentbindings.ResourceRef{
		OrganizationID: scope.OrganizationID,
		BindingType:    agentbindings.BindingTypeSkill,
		ResourceID:     id,
	}, customSkillDeleteBindingOperation, scope.AccountID, time.Now())
}
