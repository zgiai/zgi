package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/zgi/api/internal/capabilities/agentbindings"
	"github.com/zgiai/zgi/api/internal/dto"
	"github.com/zgiai/zgi/api/internal/modules/app/runtimeauth"
	interfaces "github.com/zgiai/zgi/api/internal/modules/shared/interface"
	shared_service "github.com/zgiai/zgi/api/internal/modules/shared/service"
	workspace_model "github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/pkg/logger"
	"gorm.io/datatypes"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

type WorkspaceAssetMoveService struct {
	db                   *gorm.DB
	authorizationService interfaces.AuthorizationService
	agentBindings        *agentbindings.Repository
}

func NewWorkspaceAssetMoveService(db *gorm.DB, authorizationService interfaces.AuthorizationService, bindingRepositories ...*agentbindings.Repository) *WorkspaceAssetMoveService {
	bindingRepository := agentbindings.NewRepository(db)
	if len(bindingRepositories) > 0 {
		bindingRepository = bindingRepositories[0]
	}
	return &WorkspaceAssetMoveService{db: db, authorizationService: authorizationService, agentBindings: bindingRepository}
}

func (s *WorkspaceAssetMoveService) Preview(ctx context.Context, organizationID, accountID string, req dto.WorkspaceAssetMoveRequest) (*dto.WorkspaceAssetMovePreviewResponse, error) {
	return s.previewWithDB(ctx, s.db, organizationID, accountID, req)
}

func (s *WorkspaceAssetMoveService) Move(ctx context.Context, organizationID, accountID string, req dto.WorkspaceAssetMoveRequest) (*dto.WorkspaceAssetMoveResponse, error) {
	req.AgentBindingAction = strings.ToLower(strings.TrimSpace(req.AgentBindingAction))
	if req.AgentBindingAction != "" && req.AgentBindingAction != "unbind" {
		return nil, ErrAssetMoveInvalidRequest
	}
	var response dto.WorkspaceAssetMoveResponse
	var affectedAgentIDs []uuid.UUID
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.lockMoveAssets(ctx, tx, organizationID, req.Items); err != nil {
			return err
		}
		preview, err := s.previewWithDB(ctx, tx, organizationID, accountID, req)
		if err != nil {
			return err
		}
		response.Preview = *preview
		if !preview.Movable {
			return ErrAssetMoveBlocked
		}
		if preview.AgentBindingImpact != nil && req.AgentBindingAction != "unbind" {
			return &agentbindings.ConflictError{Impact: *preview.AgentBindingImpact}
		}
		if s.agentBindings != nil {
			moveImpactReq, ok, err := moveImpactRequestFromPreview(organizationID, accountID, *preview)
			if err != nil {
				return err
			}
			if ok {
				affectedAgentIDs, err = s.agentBindings.ApplyMoveImpact(ctx, tx, moveImpactReq, req.ImpactToken, time.Now())
				if err != nil {
					if preview.AgentBindingImpact != nil {
						return &agentbindings.ConflictError{Impact: *preview.AgentBindingImpact}
					}
					return err
				}
			}
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
	if len(affectedAgentIDs) > 0 {
		logger.InfoContext(ctx, "agent resource bindings updated for workspace move",
			"log_type", "audit",
			"actor_account_id", accountID,
			"organization_id", organizationID,
			"target_workspace_id", req.TargetWorkspaceID,
			"affected_agent_ids", affectedAgentIDs,
			"agent_binding_action", req.AgentBindingAction,
			"binding_state_before", "bound",
			"binding_state_after", "unbound_or_relocated",
			"published_scope_revoked", true,
			"drafts_pruned", true,
		)
	}
	return &response, nil
}

type assetMoveLockTarget struct {
	assetType string
	id        string
	table     string
}

// lockMoveAssets makes the source workspace observed by previewWithDB stable
// until the move transaction commits. Binding resource advisory locks must be
// acquired before asset rows to match delete and other lifecycle operations.
func (s *WorkspaceAssetMoveService) lockMoveAssets(ctx context.Context, tx *gorm.DB, organizationID string, items []dto.WorkspaceAssetMoveItem) error {
	targets := make([]assetMoveLockTarget, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		assetType := strings.ToLower(strings.TrimSpace(item.Type))
		id := strings.TrimSpace(item.ID)
		table := assetMoveTable(assetType)
		if table == "" || id == "" {
			continue
		}
		key := assetType + ":" + id
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		targets = append(targets, assetMoveLockTarget{assetType: assetType, id: id, table: table})
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].assetType == targets[j].assetType {
			return targets[i].id < targets[j].id
		}
		return targets[i].assetType < targets[j].assetType
	})
	if err := s.lockMoveBindingResources(ctx, tx, organizationID, targets); err != nil {
		return err
	}

	for _, target := range targets {
		var row struct {
			ID string
		}
		query := tx.WithContext(ctx).
			Table(target.table).
			Select("id").
			Where("id = ?", target.id)
		if target.assetType == AssetMoveTypeAgent {
			query = query.Where("deleted_at IS NULL")
		}
		err := query.Clauses(clause.Locking{Strength: "UPDATE"}).Take(&row).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// The authoritative preview below returns the existing structured
			// not-found blocker. A missing row has nothing to lock.
			continue
		}
		if err != nil {
			return fmt.Errorf("lock %s %s for workspace move: %w", target.assetType, target.id, err)
		}
	}
	return nil
}

func (s *WorkspaceAssetMoveService) lockMoveBindingResources(ctx context.Context, tx *gorm.DB, organizationID string, targets []assetMoveLockTarget) error {
	if s.agentBindings == nil {
		return nil
	}
	refs := make([]agentbindings.ResourceRef, 0, len(targets))
	for _, target := range targets {
		var bindingType agentbindings.BindingType
		switch target.assetType {
		case AssetMoveTypeAgent:
			// Workflow assets share the Agent row and use the Agent ID as their
			// binding resource ID. Taking this lock for a regular Agent is safe
			// and keeps type resolution after the row lock.
			bindingType = agentbindings.BindingTypeWorkflow
		case AssetMoveTypeDataset:
			bindingType = agentbindings.BindingTypeKnowledgeDataset
		case AssetMoveTypeDatabase:
			bindingType = agentbindings.BindingTypeDatabase
		default:
			continue
		}
		refs = append(refs, agentbindings.ResourceRef{
			BindingType: bindingType,
			ResourceID:  target.id,
		})
	}
	if len(refs) == 0 {
		return nil
	}
	organizationUUID, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil {
		return ErrAssetMoveInvalidRequest
	}
	for idx := range refs {
		refs[idx].OrganizationID = organizationUUID
	}
	if err := s.agentBindings.WithTx(tx).LockResources(ctx, tx, refs); err != nil {
		return fmt.Errorf("lock workspace move binding resources: %w", err)
	}
	return nil
}

func assetMoveTable(assetType string) string {
	switch assetType {
	case AssetMoveTypeAgent:
		return "agents"
	case AssetMoveTypeDataset:
		return "datasets"
	case AssetMoveTypeFile:
		return "upload_files"
	case AssetMoveTypeDatabase:
		return "data_sources"
	default:
		return ""
	}
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
		var targetPrecheckPermission workspace_model.WorkspacePermissionCode
		if len(targetBlockers) == 0 && supportedPermission {
			if requestItem.Type == AssetMoveTypeAgent {
				var err error
				targetPrecheckPermission, err = s.requireAnyAssetMovePermission(
					ctx,
					organizationID,
					item.TargetWorkspaceID,
					accountID,
					workspace_model.WorkspacePermissionAgentMove,
					workspace_model.WorkspacePermissionWorkflowMove,
				)
				if err != nil {
					return nil, err
				}
			} else if err := s.requireAssetMovePermission(ctx, organizationID, item.TargetWorkspaceID, accountID, permission); err != nil {
				return nil, err
			}
		}

		if len(targetBlockers) == 0 && requestItem.Type != "" && requestItem.ID != "" {
			if err := s.previewAsset(ctx, db, organizationID, requestItem, &item); err != nil {
				return nil, err
			}
		}

		if requestItem.Type == AssetMoveTypeAgent {
			permission = agentMovePermissionForType(item.ResolvedAgentType)
		}
		if len(targetBlockers) == 0 && supportedPermission && requestItem.Type == AssetMoveTypeAgent && item.ResolvedAgentID != "" && targetPrecheckPermission != permission {
			if err := s.requireAssetMovePermission(ctx, organizationID, item.TargetWorkspaceID, accountID, permission); err != nil {
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
	if preview.Movable && s.agentBindings != nil {
		moveImpactReq, ok, err := moveImpactRequestFromPreview(organizationID, accountID, *preview)
		if err != nil {
			return nil, err
		}
		if ok {
			impact, err := s.agentBindings.WithTx(db).PreviewMoveImpact(ctx, moveImpactReq, time.Now())
			if err != nil {
				return nil, fmt.Errorf("preview workspace move agent binding impact: %w", err)
			}
			preview.AgentBindingImpact = impact
		}
	}

	return preview, nil
}

func moveImpactRequestFromPreview(organizationID, accountID string, preview dto.WorkspaceAssetMovePreviewResponse) (agentbindings.MoveImpactRequest, bool, error) {
	organizationUUID, err := uuid.Parse(strings.TrimSpace(organizationID))
	if err != nil {
		return agentbindings.MoveImpactRequest{}, false, ErrAssetMoveInvalidRequest
	}
	actorUUID, err := uuid.Parse(strings.TrimSpace(accountID))
	if err != nil {
		return agentbindings.MoveImpactRequest{}, false, ErrAssetMoveInvalidRequest
	}
	var targetWorkspaceID string
	request := agentbindings.MoveImpactRequest{OrganizationID: organizationUUID, ActorID: actorUUID}
	for _, item := range preview.Items {
		if !item.Movable {
			continue
		}
		if targetWorkspaceID == "" {
			targetWorkspaceID = item.TargetWorkspaceID
		}
		var sourceWorkspaceID *uuid.UUID
		if item.FromWorkspaceID != "" {
			parsed, err := uuid.Parse(item.FromWorkspaceID)
			if err != nil {
				return agentbindings.MoveImpactRequest{}, false, ErrAssetMoveInvalidRequest
			}
			sourceWorkspaceID = &parsed
		}
		switch item.Type {
		case AssetMoveTypeAgent:
			agentID, err := uuid.Parse(firstNonEmpty(item.ResolvedAgentID, item.ID))
			if err != nil {
				return agentbindings.MoveImpactRequest{}, false, ErrAssetMoveInvalidRequest
			}
			request.MovingAgentIDs = append(request.MovingAgentIDs, agentID)
			if isWorkflowRuntimeAssetType(item.ResolvedAgentType) {
				request.ResourceRefs = append(request.ResourceRefs, agentbindings.ResourceRef{
					OrganizationID: organizationUUID,
					WorkspaceID:    sourceWorkspaceID,
					BindingType:    agentbindings.BindingTypeWorkflow,
					ResourceID:     agentID.String(),
				})
			}
		case AssetMoveTypeDataset:
			request.ResourceRefs = append(request.ResourceRefs, agentbindings.ResourceRef{
				OrganizationID: organizationUUID,
				WorkspaceID:    sourceWorkspaceID,
				BindingType:    agentbindings.BindingTypeKnowledgeDataset,
				ResourceID:     item.ID,
			})
		case AssetMoveTypeDatabase:
			request.ResourceRefs = append(request.ResourceRefs, agentbindings.ResourceRef{
				OrganizationID: organizationUUID,
				WorkspaceID:    sourceWorkspaceID,
				BindingType:    agentbindings.BindingTypeDatabase,
				ResourceID:     item.ID,
			})
		}
	}
	if len(request.ResourceRefs) == 0 && len(request.MovingAgentIDs) == 0 {
		return agentbindings.MoveImpactRequest{}, false, nil
	}
	targetUUID, err := uuid.Parse(targetWorkspaceID)
	if err != nil {
		return agentbindings.MoveImpactRequest{}, false, ErrAssetMoveInvalidRequest
	}
	request.TargetWorkspaceID = targetUUID
	return request, true, nil
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

func agentMovePermissionForType(agentType string) workspace_model.WorkspacePermissionCode {
	if isWorkflowRuntimeAssetType(agentType) {
		return workspace_model.WorkspacePermissionWorkflowMove
	}
	return workspace_model.WorkspacePermissionAgentMove
}

func isWorkflowRuntimeAssetType(agentType string) bool {
	switch strings.ToUpper(strings.TrimSpace(agentType)) {
	case "WORKFLOW", "CONVERSATIONAL_WORKFLOW", "CONVERSATIONAL_AGENT":
		return true
	default:
		return false
	}
}

func (s *WorkspaceAssetMoveService) requireAssetMovePermission(ctx context.Context, organizationID, workspaceID, accountID string, permission workspace_model.WorkspacePermissionCode) error {
	if s.authorizationService == nil {
		return ErrAssetMovePermissionDenied
	}
	_, err := s.authorizationService.RequireWorkspacePermission(ctx, interfaces.WorkspaceScopeRequest{
		OrganizationID:  organizationID,
		WorkspaceID:     workspaceID,
		AccountID:       accountID,
		PermissionCodes: []workspace_model.WorkspacePermissionCode{permission},
	})
	return assetMoveAuthorizationError(err)
}

func (s *WorkspaceAssetMoveService) requireAnyAssetMovePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissions ...workspace_model.WorkspacePermissionCode) (workspace_model.WorkspacePermissionCode, error) {
	if s.authorizationService == nil {
		return "", ErrAssetMovePermissionDenied
	}
	for _, permission := range permissions {
		_, err := s.authorizationService.RequireWorkspacePermission(ctx, interfaces.WorkspaceScopeRequest{
			OrganizationID:  organizationID,
			WorkspaceID:     workspaceID,
			AccountID:       accountID,
			PermissionCodes: []workspace_model.WorkspacePermissionCode{permission},
		})
		if err == nil {
			return permission, nil
		}
		if !errors.Is(err, shared_service.ErrAuthorizationDenied) {
			return "", err
		}
	}
	return "", ErrAssetMovePermissionDenied
}

func assetMoveAuthorizationError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, shared_service.ErrAuthorizationDenied) {
		return ErrAssetMovePermissionDenied
	}
	return err
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
		ID        string
		TenantID  string
		AgentType string
	}
	err := db.WithContext(ctx).
		Table("agents").
		Select("id, tenant_id, agent_type").
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
	item.ResolvedAgentType = agent.AgentType
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
		agentUUID, err := uuid.Parse(agentID)
		if err != nil {
			return fmt.Errorf("invalid agent id for runtime authorization relocation: %w", err)
		}
		sourceWorkspaceUUID, err := uuid.Parse(item.FromWorkspaceID)
		if err != nil {
			return fmt.Errorf("invalid source workspace id for runtime authorization relocation: %w", err)
		}
		targetWorkspaceUUID, err := uuid.Parse(item.TargetWorkspaceID)
		if err != nil {
			return fmt.Errorf("invalid target workspace id for runtime authorization relocation: %w", err)
		}
		if err := runtimeauth.NewStore(tx).RelocateResourceWorkspace(
			ctx,
			runtimeauth.PublishedRuntimeResourceAgent,
			agentUUID,
			sourceWorkspaceUUID,
			targetWorkspaceUUID,
		); err != nil {
			return fmt.Errorf("failed to relocate agent runtime authorization: %w", err)
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
