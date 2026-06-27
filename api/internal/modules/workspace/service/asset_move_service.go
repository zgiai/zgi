package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/dto"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

const (
	AssetMoveTypeAgent    = "agent"
	AssetMoveTypeDataset  = "dataset"
	AssetMoveTypeFile     = "file"
	AssetMoveTypeDatabase = "database"
)

var (
	ErrAssetMovePermissionDenied = errors.New("permission denied")
	ErrAssetMoveInvalidRequest   = errors.New("invalid asset move request")
	ErrAssetMoveBlocked          = errors.New("asset move blocked")
)

type assetMoveOrganizationService interface {
	CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode workspace_model.WorkspacePermissionCode) (bool, error)
}

type WorkspaceAssetMoveService struct {
	db                  *gorm.DB
	organizationService assetMoveOrganizationService
}

func NewWorkspaceAssetMoveService(db *gorm.DB, organizationService assetMoveOrganizationService) *WorkspaceAssetMoveService {
	return &WorkspaceAssetMoveService{db: db, organizationService: organizationService}
}

func (s *WorkspaceAssetMoveService) Preview(ctx context.Context, organizationID, accountID string, req dto.WorkspaceAssetMoveRequest) (*dto.WorkspaceAssetMovePreviewResponse, error) {
	return s.previewWithDB(ctx, s.db, organizationID, accountID, req)
}

func (s *WorkspaceAssetMoveService) Move(ctx context.Context, organizationID, accountID string, req dto.WorkspaceAssetMoveRequest) (*dto.WorkspaceAssetMoveResponse, error) {
	var response dto.WorkspaceAssetMoveResponse
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		preview, err := s.previewWithDB(ctx, tx, organizationID, accountID, req)
		if err != nil {
			return err
		}
		response.Preview = *preview
		if !preview.Movable {
			return ErrAssetMoveBlocked
		}

		for _, item := range preview.Items {
			if err := s.movePreviewItem(ctx, tx, organizationID, accountID, item); err != nil {
				return err
			}
		}
		response.Moved = true
		return nil
	})
	if err != nil {
		if errors.Is(err, ErrAssetMoveBlocked) {
			return &response, err
		}
		return nil, err
	}
	return &response, nil
}

func (s *WorkspaceAssetMoveService) previewWithDB(ctx context.Context, db *gorm.DB, organizationID, accountID string, req dto.WorkspaceAssetMoveRequest) (*dto.WorkspaceAssetMovePreviewResponse, error) {
	if organizationID == "" || accountID == "" || strings.TrimSpace(req.TargetWorkspaceID) == "" || len(req.Items) == 0 {
		return nil, ErrAssetMoveInvalidRequest
	}

	targetWorkspace, targetBlockers, err := s.loadTargetWorkspace(ctx, db, organizationID, req.TargetWorkspaceID)
	if err != nil {
		return nil, err
	}

	preview := &dto.WorkspaceAssetMovePreviewResponse{
		Movable: len(targetBlockers) == 0,
		Items:   make([]dto.WorkspaceAssetMovePreviewItem, 0, len(req.Items)),
	}
	seen := make(map[string]struct{}, len(req.Items))
	for _, requestItem := range req.Items {
		requestItem.Type = strings.ToLower(strings.TrimSpace(requestItem.Type))
		requestItem.ID = strings.TrimSpace(requestItem.ID)
		item := dto.WorkspaceAssetMovePreviewItem{
			Type:              requestItem.Type,
			ID:                requestItem.ID,
			TargetWorkspaceID: req.TargetWorkspaceID,
			TargetFolderID:    strings.TrimSpace(req.TargetFolderID),
			TargetWorkspace:   toMoveWorkspace(targetWorkspace),
			Blockers:          append([]string{}, targetBlockers...),
			Warnings:          []string{},
		}
		if requestItem.Type == "" || requestItem.ID == "" {
			item.Blockers = append(item.Blockers, "asset type and id are required")
		}
		seenKey := requestItem.Type + ":" + requestItem.ID
		if _, exists := seen[seenKey]; exists {
			item.Blockers = append(item.Blockers, "duplicate asset in request")
		}
		seen[seenKey] = struct{}{}

		permission, supportedPermission := assetMovePermissionForType(requestItem.Type)
		if len(targetBlockers) == 0 && supportedPermission {
			if err := s.requireAssetMovePermission(ctx, organizationID, item.TargetWorkspaceID, accountID, permission); err != nil {
				return nil, err
			}
		}

		if len(targetBlockers) == 0 && requestItem.Type != "" && requestItem.ID != "" {
			if err := s.previewAsset(ctx, db, organizationID, requestItem, &item); err != nil {
				return nil, err
			}
		}
		if len(item.Blockers) == 0 && supportedPermission && item.FromWorkspaceID != "" {
			if err := s.requireAssetMovePermission(ctx, organizationID, item.FromWorkspaceID, accountID, permission); err != nil {
				return nil, err
			}
		}
		item.Movable = len(item.Blockers) == 0
		if !item.Movable {
			preview.Movable = false
		}
		preview.Items = append(preview.Items, item)
	}

	return preview, nil
}

func assetMovePermissionForType(assetType string) (workspace_model.WorkspacePermissionCode, bool) {
	switch assetType {
	case AssetMoveTypeAgent:
		return workspace_model.WorkspacePermissionAgentMove, true
	case AssetMoveTypeDataset:
		return workspace_model.WorkspacePermissionKnowledgeBaseMove, true
	case AssetMoveTypeFile:
		return workspace_model.WorkspacePermissionFileMove, true
	case AssetMoveTypeDatabase:
		return workspace_model.WorkspacePermissionDatabaseMove, true
	default:
		return "", false
	}
}

func (s *WorkspaceAssetMoveService) requireAssetMovePermission(ctx context.Context, organizationID, workspaceID, accountID string, permission workspace_model.WorkspacePermissionCode) error {
	if s.organizationService == nil {
		return ErrAssetMovePermissionDenied
	}
	allowed, err := s.organizationService.CheckWorkspacePermission(ctx, organizationID, workspaceID, accountID, permission)
	if err != nil {
		return err
	}
	if !allowed {
		return ErrAssetMovePermissionDenied
	}
	return nil
}

func (s *WorkspaceAssetMoveService) loadTargetWorkspace(ctx context.Context, db *gorm.DB, organizationID, workspaceID string) (*workspace_model.Workspace, []string, error) {
	var workspace workspace_model.Workspace
	err := db.WithContext(ctx).
		Where("id = ?", workspaceID).
		First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, []string{"target workspace not found"}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	blockers := []string{}
	if workspace.OrganizationID == nil || *workspace.OrganizationID != organizationID {
		blockers = append(blockers, "target workspace is outside current organization")
	}
	if workspace.Status != workspace_model.WorkspaceStatusNormal {
		blockers = append(blockers, "target workspace is not active")
	}
	return &workspace, blockers, nil
}

func (s *WorkspaceAssetMoveService) previewAsset(ctx context.Context, db *gorm.DB, organizationID string, requestItem dto.WorkspaceAssetMoveItem, item *dto.WorkspaceAssetMovePreviewItem) error {
	switch requestItem.Type {
	case AssetMoveTypeAgent:
		return s.previewAgent(ctx, db, organizationID, requestItem.ID, item)
	case AssetMoveTypeDataset:
		return s.previewDataset(ctx, db, organizationID, requestItem.ID, item)
	case AssetMoveTypeFile:
		return s.previewFile(ctx, db, organizationID, requestItem.ID, item)
	case AssetMoveTypeDatabase:
		return s.previewDatabase(ctx, db, organizationID, requestItem.ID, item)
	default:
		item.Blockers = append(item.Blockers, "unsupported asset type")
		return nil
	}
}

func (s *WorkspaceAssetMoveService) previewAgent(ctx context.Context, db *gorm.DB, organizationID, agentID string, item *dto.WorkspaceAssetMovePreviewItem) error {
	var agent struct {
		ID       string
		TenantID string
	}
	err := db.WithContext(ctx).
		Table("agents").
		Select("id, tenant_id").
		Where("id = ? AND deleted_at IS NULL", agentID).
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item.Blockers = append(item.Blockers, "agent not found")
		return nil
	}
	if err != nil {
		return err
	}
	item.ResolvedAgentID = agent.ID
	if err := s.attachAndCheckSourceWorkspace(ctx, db, organizationID, agent.TenantID, item); err != nil {
		return err
	}
	if blockIfAlreadyInTargetWorkspace(item) {
		return nil
	}
	if len(item.Blockers) == 0 {
		warnings, err := s.workflowReferenceWarningsForAgent(ctx, db, organizationID, agent.ID, item.TargetWorkspaceID)
		if err != nil {
			return err
		}
		item.Warnings = append(item.Warnings, warnings...)
	}
	return nil
}

func (s *WorkspaceAssetMoveService) previewDataset(ctx context.Context, db *gorm.DB, organizationID, datasetID string, item *dto.WorkspaceAssetMovePreviewItem) error {
	var dataset struct {
		ID             string
		OrganizationID string
		WorkspaceID    string
	}
	err := db.WithContext(ctx).
		Table("datasets").
		Select("id, organization_id, workspace_id").
		Where("id = ?", datasetID).
		First(&dataset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item.Blockers = append(item.Blockers, "dataset not found")
		return nil
	}
	if err != nil {
		return err
	}
	if dataset.OrganizationID != organizationID {
		item.Blockers = append(item.Blockers, "dataset is outside current organization")
	}
	if err := s.attachSourceWorkspace(ctx, db, dataset.WorkspaceID, item); err != nil {
		return err
	}
	if blockIfAlreadyInTargetWorkspace(item) {
		return nil
	}
	return s.validateTargetFolder(ctx, db, "dataset_folders", "workspace_id", organizationID, item)
}

func (s *WorkspaceAssetMoveService) previewFile(ctx context.Context, db *gorm.DB, organizationID, fileID string, item *dto.WorkspaceAssetMovePreviewItem) error {
	var file struct {
		ID             string
		OrganizationID string
		WorkspaceID    *string
	}
	err := db.WithContext(ctx).
		Table("upload_files").
		Select("id, organization_id, workspace_id").
		Where("id = ?", fileID).
		First(&file).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item.Blockers = append(item.Blockers, "file not found")
		return nil
	}
	if err != nil {
		return err
	}
	if file.OrganizationID != organizationID {
		item.Blockers = append(item.Blockers, "file is outside current organization")
	}
	if file.WorkspaceID != nil {
		if err := s.attachSourceWorkspace(ctx, db, *file.WorkspaceID, item); err != nil {
			return err
		}
		if blockIfAlreadyInTargetWorkspace(item) {
			return nil
		}
	}
	return s.validateTargetFolder(ctx, db, "file_folders", "workspace_id", organizationID, item)
}

func (s *WorkspaceAssetMoveService) previewDatabase(ctx context.Context, db *gorm.DB, organizationID, databaseID string, item *dto.WorkspaceAssetMovePreviewItem) error {
	var dataSource struct {
		ID             string
		OrganizationID string
		WorkspaceID    *string
	}
	err := db.WithContext(ctx).
		Table("data_sources").
		Select("id, organization_id, workspace_id").
		Where("id = ?", databaseID).
		First(&dataSource).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item.Blockers = append(item.Blockers, "database not found")
		return nil
	}
	if err != nil {
		return err
	}
	if dataSource.OrganizationID != organizationID {
		item.Blockers = append(item.Blockers, "database is outside current organization")
	}
	if dataSource.WorkspaceID != nil {
		if err := s.attachSourceWorkspace(ctx, db, *dataSource.WorkspaceID, item); err != nil {
			return err
		}
		blockIfAlreadyInTargetWorkspace(item)
	}
	return nil
}

func blockIfAlreadyInTargetWorkspace(item *dto.WorkspaceAssetMovePreviewItem) bool {
	if item.FromWorkspaceID == "" || item.FromWorkspaceID != item.TargetWorkspaceID {
		return false
	}
	item.Blockers = append(item.Blockers, "asset is already in target workspace")
	return true
}

func (s *WorkspaceAssetMoveService) attachAndCheckSourceWorkspace(ctx context.Context, db *gorm.DB, organizationID, workspaceID string, item *dto.WorkspaceAssetMovePreviewItem) error {
	if err := s.attachSourceWorkspace(ctx, db, workspaceID, item); err != nil {
		return err
	}
	if item.FromWorkspace == nil {
		return nil
	}
	var workspace workspace_model.Workspace
	if err := db.WithContext(ctx).Where("id = ?", workspaceID).First(&workspace).Error; err != nil {
		return err
	}
	if workspace.OrganizationID == nil || *workspace.OrganizationID != organizationID {
		item.Blockers = append(item.Blockers, fmt.Sprintf("%s is outside current organization", item.Type))
	}
	return nil
}

func (s *WorkspaceAssetMoveService) attachSourceWorkspace(ctx context.Context, db *gorm.DB, workspaceID string, item *dto.WorkspaceAssetMovePreviewItem) error {
	if workspaceID == "" {
		return nil
	}
	item.FromWorkspaceID = workspaceID
	var workspace workspace_model.Workspace
	err := db.WithContext(ctx).Where("id = ?", workspaceID).First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		item.Blockers = append(item.Blockers, "source workspace not found")
		return nil
	}
	if err != nil {
		return err
	}
	item.FromWorkspace = toMoveWorkspace(&workspace)
	return nil
}

func (s *WorkspaceAssetMoveService) validateTargetFolder(ctx context.Context, db *gorm.DB, tableName, workspaceColumn, organizationID string, item *dto.WorkspaceAssetMovePreviewItem) error {
	if item.TargetFolderID == "" {
		return nil
	}
	var count int64
	err := db.WithContext(ctx).
		Table(tableName).
		Where("id = ? AND organization_id = ? AND "+workspaceColumn+" = ?", item.TargetFolderID, organizationID, item.TargetWorkspaceID).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		item.Blockers = append(item.Blockers, "target folder is not in target workspace")
	}
	return nil
}

func (s *WorkspaceAssetMoveService) workflowReferenceWarningsForAgent(ctx context.Context, db *gorm.DB, organizationID, agentID, targetWorkspaceID string) ([]string, error) {
	var workflows []struct {
		ID    string
		Graph string
	}
	if err := db.WithContext(ctx).
		Table("workflows").
		Select("id, graph").
		Where("agent_id = ? OR app_id = ?", agentID, agentID).
		Find(&workflows).Error; err != nil {
		return nil, err
	}

	uuidSet := map[string]struct{}{}
	for _, workflow := range workflows {
		for _, id := range collectUUIDs(workflow.Graph) {
			uuidSet[id] = struct{}{}
		}
	}
	if len(uuidSet) == 0 {
		return nil, nil
	}
	ids := make([]string, 0, len(uuidSet))
	for id := range uuidSet {
		ids = append(ids, id)
	}

	warnings := []string{}
	type dependency struct {
		ID          string
		WorkspaceID *string
	}
	appendWarnings := func(assetType, table string) error {
		var deps []dependency
		query := db.WithContext(ctx).
			Table(table).
			Select("id, workspace_id").
			Where("id IN ? AND organization_id = ?", ids, organizationID)
		err := query.Find(&deps).Error
		if err != nil {
			return err
		}
		for _, dep := range deps {
			if dep.WorkspaceID == nil || *dep.WorkspaceID != targetWorkspaceID {
				warnings = append(warnings, fmt.Sprintf("workflow references %s %s outside target workspace", assetType, dep.ID))
			}
		}
		return nil
	}
	if err := appendWarnings(AssetMoveTypeDataset, "datasets"); err != nil {
		return nil, err
	}
	if err := appendWarnings(AssetMoveTypeFile, "upload_files"); err != nil {
		return nil, err
	}
	if err := appendWarnings(AssetMoveTypeDatabase, "data_sources"); err != nil {
		return nil, err
	}
	return warnings, nil
}

func (s *WorkspaceAssetMoveService) movePreviewItem(ctx context.Context, tx *gorm.DB, organizationID, accountID string, item dto.WorkspaceAssetMovePreviewItem) error {
	now := time.Now()
	switch item.Type {
	case AssetMoveTypeAgent:
		agentID := firstNonEmpty(item.ResolvedAgentID, item.ID)
		if err := tx.WithContext(ctx).Table("agents").
			Where("id = ? AND deleted_at IS NULL", agentID).
			Updates(map[string]interface{}{"tenant_id": item.TargetWorkspaceID, "updated_by": accountID, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Table("workflows").
			Where("agent_id = ? OR app_id = ?", agentID, agentID).
			Updates(map[string]interface{}{"tenant_id": item.TargetWorkspaceID, "updated_by": accountID, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Table("installed_agents").
			Where("agent_id = ?", agentID).
			Update("agent_owner_tenant_id", item.TargetWorkspaceID).Error; err != nil {
			return err
		}
	case AssetMoveTypeDataset:
		if err := tx.WithContext(ctx).Table("datasets").
			Where("id = ?", item.ID).
			Updates(map[string]interface{}{"workspace_id": item.TargetWorkspaceID, "updated_by": accountID, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Exec("DELETE FROM dataset_folder_joins WHERE dataset_id = ?", item.ID).Error; err != nil {
			return err
		}
		if item.TargetFolderID != "" {
			if err := tx.WithContext(ctx).Table("dataset_folder_joins").Create(map[string]interface{}{
				"id":         uuid.NewString(),
				"dataset_id": item.ID,
				"folder_id":  item.TargetFolderID,
				"created_by": accountID,
				"created_at": now,
			}).Error; err != nil {
				return err
			}
		}
	case AssetMoveTypeFile:
		if err := tx.WithContext(ctx).Table("upload_files").
			Where("id = ?", item.ID).
			Update("workspace_id", item.TargetWorkspaceID).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Exec("DELETE FROM file_folder_joins WHERE file_id = ?", item.ID).Error; err != nil {
			return err
		}
		if item.TargetFolderID != "" {
			if err := tx.WithContext(ctx).Table("file_folder_joins").Create(map[string]interface{}{
				"id":         uuid.NewString(),
				"file_id":    item.ID,
				"folder_id":  item.TargetFolderID,
				"created_by": accountID,
				"created_at": now,
			}).Error; err != nil {
				return err
			}
		}
	case AssetMoveTypeDatabase:
		if err := tx.WithContext(ctx).Table("data_sources").
			Where("id = ?", item.ID).
			Updates(map[string]interface{}{"workspace_id": item.TargetWorkspaceID, "updated_by": accountID, "updated_at": now}).Error; err != nil {
			return err
		}
		if err := tx.WithContext(ctx).Table("data_library_database_asset_refs").
			Where("organization_id = ? AND data_source_id = ? AND deleted_at IS NULL", organizationID, item.ID).
			Updates(map[string]interface{}{"workspace_id": item.TargetWorkspaceID, "updated_at": now}).Error; err != nil {
			return err
		}
	}
	return s.createAuditEvent(ctx, tx, organizationID, accountID, item, now)
}

func (s *WorkspaceAssetMoveService) createAuditEvent(ctx context.Context, tx *gorm.DB, organizationID, accountID string, item dto.WorkspaceAssetMovePreviewItem, now time.Time) error {
	warnings, err := json.Marshal(item.Warnings)
	if err != nil {
		return err
	}
	return tx.WithContext(ctx).Table("workspace_asset_move_events").Create(map[string]interface{}{
		"id":                uuid.NewString(),
		"organization_id":   organizationID,
		"actor_account_id":  accountID,
		"asset_type":        item.Type,
		"asset_id":          item.ID,
		"from_workspace_id": nullableString(item.FromWorkspaceID),
		"to_workspace_id":   item.TargetWorkspaceID,
		"warnings":          datatypes.JSON(warnings),
		"created_at":        now,
	}).Error
}

func toMoveWorkspace(workspace *workspace_model.Workspace) *dto.WorkspaceAssetMoveWorkspace {
	if workspace == nil {
		return nil
	}
	return &dto.WorkspaceAssetMoveWorkspace{ID: workspace.ID, Name: workspace.Name}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" && value != "00000000-0000-0000-0000-000000000000" {
			return value
		}
	}
	return ""
}

func nullableString(value string) interface{} {
	if value == "" {
		return nil
	}
	return value
}

var uuidPattern = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)

func collectUUIDs(value string) []string {
	if value == "" {
		return nil
	}
	matches := uuidPattern.FindAllString(value, -1)
	if len(matches) == 0 {
		return nil
	}
	ids := make([]string, 0, len(matches))
	seen := map[string]struct{}{}
	for _, match := range matches {
		normalized := strings.ToLower(match)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		ids = append(ids, normalized)
	}
	return ids
}
