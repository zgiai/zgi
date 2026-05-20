package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/zgiai/ginext/config"
	"github.com/zgiai/ginext/internal/dto"
	"github.com/zgiai/ginext/internal/infra/platform/console"

	"strings"
	"time"

	shared_dto "github.com/zgiai/ginext/internal/dto"
	agents_model "github.com/zgiai/ginext/internal/modules/app/agents"
	datasetsvc "github.com/zgiai/ginext/internal/modules/dataset/service"
	datasource_model "github.com/zgiai/ginext/internal/modules/datasource/model"
	file_model "github.com/zgiai/ginext/internal/modules/file_process/model"
	interfaces "github.com/zgiai/ginext/internal/modules/shared/interface"
	"github.com/zgiai/ginext/internal/modules/shared/workspacebootstrap"
	system_service "github.com/zgiai/ginext/internal/modules/system/service"

	"github.com/google/uuid"

	apikeymodel "github.com/zgiai/ginext/internal/modules/llm/apikey/model"
	auth_model "github.com/zgiai/ginext/internal/modules/user/auth/model"
	"github.com/zgiai/ginext/internal/modules/workspace/model"
	workspace_repo "github.com/zgiai/ginext/internal/modules/workspace/repository"
	"github.com/zgiai/ginext/pkg/logger"

	"gorm.io/gorm"
)

var ErrCannotUpdateBuiltinRole = errors.New("cannot update built-in role")
var ErrRoleNameExists = errors.New("role name already exists")
var ErrMemberNameExists = errors.New("member name already exists")

// GetDepartmentInviteLink gets invite link for a department or organization-level when departmentID is empty.
func (s *organizationService) GetDepartmentInviteLink(ctx context.Context, organizationID, departmentID, accountID string) (*model.OrganizationInviteLink, error) {
	if departmentID == "" {
		return s.organizationRepo.GetInviteLinkByOrganization(ctx, organizationID)
	}
	return s.organizationRepo.GetInviteLinkByDepartment(ctx, organizationID, departmentID)
}

// CreateOrResetDepartmentInviteLink creates or resets invite link.
func (s *organizationService) CreateOrResetDepartmentInviteLink(ctx context.Context, organizationID, departmentID, accountID string, requireApproval bool, expiresAt *time.Time) (*model.OrganizationInviteLink, error) {
	token := uuid.New().String()

	var existing *model.OrganizationInviteLink
	var err error

	if departmentID == "" {
		existing, err = s.organizationRepo.GetInviteLinkByOrganization(ctx, organizationID)
	} else {
		existing, err = s.organizationRepo.GetInviteLinkByDepartment(ctx, organizationID, departmentID)
	}

	if err == nil && existing != nil {
		existing.Token = token
		existing.RequireApproval = requireApproval
		existing.ExpiresAt = expiresAt
		existing.Status = "active"
		existing.CreatedBy = accountID
		existing.UpdatedAt = time.Now()
		if err := s.organizationRepo.UpdateInviteLink(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	link := &model.OrganizationInviteLink{
		OrganizationID:          organizationID,
		Token:                   token,
		Status:                  "active",
		RequireApproval:         requireApproval,
		ExpiresAt:               expiresAt,
		CreatedBy:               accountID,
		DefaultOrganizationRole: "normal",
		DefaultWorkspaceRole:    "normal",
	}

	if departmentID != "" {
		link.DepartmentID = &departmentID
	}

	if err := s.organizationRepo.CreateInviteLink(ctx, link); err != nil {
		return nil, err
	}
	return link, nil
}

// UpdateDepartmentInviteLinkStatus updates status.
func (s *organizationService) UpdateDepartmentInviteLinkStatus(ctx context.Context, organizationID, departmentID, accountID, status string) (*model.OrganizationInviteLink, error) {
	var link *model.OrganizationInviteLink
	var err error

	if departmentID == "" {
		link, err = s.organizationRepo.GetInviteLinkByOrganization(ctx, organizationID)
	} else {
		link, err = s.organizationRepo.GetInviteLinkByDepartment(ctx, organizationID, departmentID)
	}

	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, fmt.Errorf("invite link not found")
	}
	link.Status = status
	if err := s.organizationRepo.UpdateInviteLink(ctx, link); err != nil {
		return nil, err
	}
	return link, nil
}

// GetInviteLinkByToken gets link by token
func (s *organizationService) GetInviteLinkByToken(ctx context.Context, token string) (*model.OrganizationInviteLink, error) {
	return s.organizationRepo.GetInviteLinkByToken(ctx, token)
}

// AcceptInviteByToken handles invite acceptance
func (s *organizationService) AcceptInviteByToken(ctx context.Context, token, accountID string, name *string) (*model.OrganizationJoinRequest, error) {
	link, err := s.organizationRepo.GetInviteLinkByToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if link == nil {
		return nil, fmt.Errorf("invalid invite token")
	}

	if link.Status != "active" {
		return nil, fmt.Errorf("invite link is not active")
	}
	if link.ExpiresAt != nil && link.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("invite link expired")
	}

	// Organization membership is handled by the approval flow.
	// Note: We are not checking department membership here because of dependency limits.
	// The caller or subsequent logic should handle "already in department" gracefully.

	// If auto-approve is enabled (RequireApproval=false), we still create a request record but mark it approved
	status := model.OrganizationJoinRequestStatusPending
	if !link.RequireApproval {
		status = model.OrganizationJoinRequestStatusApproved
	}

	req := &model.OrganizationJoinRequest{
		OrganizationID:          link.OrganizationID,
		InviteLinkID:            &link.ID,
		AccountID:               accountID,
		DepartmentID:            link.DepartmentID,
		WorkspaceID:             link.WorkspaceID,
		DefaultOrganizationRole: link.DefaultOrganizationRole,
		DefaultWorkspaceRole:    link.DefaultWorkspaceRole,
		Status:                  status,
		Name:                    name,
	}

	// If not member of organization, we should probably add them implicitly?
	// But let's stick to creating the request. If approved, the approval logic adds them.
	// If auto-approved, the caller should add them.

	if err := s.organizationRepo.CreateJoinRequest(ctx, req); err != nil {
		return nil, err
	}

	return req, nil
}

// ListDepartmentJoinRequests lists requests.
func (s *organizationService) ListDepartmentJoinRequests(ctx context.Context, organizationID, departmentID, accountID string, status *model.OrganizationJoinRequestStatus) ([]*model.OrganizationJoinRequest, error) {
	return s.organizationRepo.ListJoinRequestsByDepartment(ctx, organizationID, departmentID, status)
}

func (s *organizationService) ListOrganizationJoinRequests(ctx context.Context, organizationID, accountID string, departmentID *string, status *model.OrganizationJoinRequestStatus, page, limit int) (*dto.OrganizationJoinRequestPaginationResponse, error) {
	if page <= 0 {
		page = 1
	}
	if limit <= 0 {
		limit = 20
	}

	requests, total, err := s.organizationRepo.ListJoinRequestsByOrganization(ctx, organizationID, departmentID, status, page, limit)
	if err != nil {
		return nil, err
	}

	accountIDSet := make(map[string]struct{})
	departmentIDSet := make(map[string]struct{})

	for _, req := range requests {
		if req == nil {
			continue
		}
		if _, exists := accountIDSet[req.AccountID]; !exists && req.AccountID != "" {
			accountIDSet[req.AccountID] = struct{}{}
		}
		if req.DepartmentID != nil && *req.DepartmentID != "" {
			if _, exists := departmentIDSet[*req.DepartmentID]; !exists {
				departmentIDSet[*req.DepartmentID] = struct{}{}
			}
		}
	}

	accountIDs := make([]string, 0, len(accountIDSet))
	for id := range accountIDSet {
		accountIDs = append(accountIDs, id)
	}

	accountsByID := make(map[string]*auth_model.Account)
	if len(accountIDs) > 0 {
		accountsByID, err = s.accountService.GetAccountsByIDs(ctx, accountIDs)
		if err != nil {
			return nil, err
		}
	}

	departmentIDs := make([]string, 0, len(departmentIDSet))
	for id := range departmentIDSet {
		departmentIDs = append(departmentIDs, id)
	}

	departmentNameByID := make(map[string]string)
	if len(departmentIDs) > 0 {
		db := s.organizationRepo.GetDB()
		var departments []model.Department
		if err := db.WithContext(ctx).Where("id IN ?", departmentIDs).Find(&departments).Error; err != nil {
			return nil, err
		}
		for _, dept := range departments {
			departmentNameByID[dept.ID] = dept.Name
		}
	}

	items := make([]*dto.OrganizationJoinRequestItem, 0, len(requests))
	for _, req := range requests {
		if req == nil {
			continue
		}

		account := accountsByID[req.AccountID]
		var accountName, accountEmail, avatar string
		if account != nil {
			accountName = account.Name
			accountEmail = account.Email
			if account.Avatar != nil {
				avatar = *account.Avatar
			}
		}

		var departmentNamePtr *string
		if req.DepartmentID != nil {
			if name, ok := departmentNameByID[*req.DepartmentID]; ok {
				n := name
				departmentNamePtr = &n
			}
		}

		var reviewedAt *int64
		if req.ReviewedAt != nil {
			timestamp := req.ReviewedAt.Unix()
			reviewedAt = &timestamp
		}

		item := &dto.OrganizationJoinRequestItem{
			ID:             req.ID,
			OrganizationID: req.OrganizationID,
			InviteLinkID:   req.InviteLinkID,
			AccountID:      req.AccountID,
			AccountName:    accountName,
			MemberName:     req.Name,
			AccountEmail:   accountEmail,
			Avatar:         avatar,
			DepartmentID:   req.DepartmentID,
			DepartmentName: departmentNamePtr,
			WorkspaceID:    req.WorkspaceID,
			Status:         req.Status,
			Reason:         req.Reason,
			ReviewerID:     req.ReviewerID,
			CreatedAt:      req.CreatedAt.Unix(),
			ReviewedAt:     reviewedAt,
		}

		items = append(items, item)
	}

	hasMore := int64(page*limit) < total

	return &dto.OrganizationJoinRequestPaginationResponse{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

// RejectDepartmentJoinRequest rejects request.
func (s *organizationService) RejectDepartmentJoinRequest(ctx context.Context, organizationID, joinRequestID, reviewerAccountID string, reason *string) error {
	req, err := s.organizationRepo.GetJoinRequestByID(ctx, joinRequestID)
	if err != nil {
		return err
	}
	if req.OrganizationID != organizationID {
		return fmt.Errorf("invalid organization id")
	}
	req.Status = model.OrganizationJoinRequestStatusRejected
	req.ReviewerID = &reviewerAccountID
	now := time.Now()
	req.ReviewedAt = &now
	req.Reason = reason
	return s.organizationRepo.UpdateJoinRequest(ctx, req)
}

// ApproveDepartmentJoinRequest approves request.
func (s *organizationService) ApproveDepartmentJoinRequest(ctx context.Context, organizationID, joinRequestID, reviewerAccountID string) (*model.OrganizationJoinRequest, error) {
	req, err := s.organizationRepo.GetJoinRequestByID(ctx, joinRequestID)
	if err != nil {
		return nil, err
	}
	if req.OrganizationID != organizationID {
		return nil, fmt.Errorf("invalid organization id")
	}
	if req.Status != model.OrganizationJoinRequestStatusPending {
		return nil, fmt.Errorf("request is not pending")
	}

	// Update status
	req.Status = model.OrganizationJoinRequestStatusApproved
	req.ReviewerID = &reviewerAccountID
	now := time.Now()
	req.ReviewedAt = &now

	if err := s.organizationRepo.UpdateJoinRequest(ctx, req); err != nil {
		return nil, err
	}

	return req, nil
}

// organizationService implements the OrganizationService interface
type organizationService struct {
	organizationRepo           workspace_repo.OrganizationRepository
	accountService             interfaces.AccountService
	workspaceRepo              workspace_repo.WorkspaceRepository
	workspaceManagementService interfaces.WorkspaceManagementService
	featureService             interfaces.FeatureService
	systemConfigService        system_service.SystemConfigService
	datasetService             datasetsvc.DatasetService
	db                         *gorm.DB
	consoleProvider            console.ConsoleProvider
	officialRouteBootstrapper  interfaces.OfficialRouteBootstrapper
}

type organizationRoleInfo struct {
	ID          string
	Name        string
	Builtin     bool
	Editable    bool
	Status      model.WorkspaceCustomRoleStatus
	Permissions []string
}

func (s *organizationService) getOrganizationRolesMap(ctx context.Context, organizationID, accountID string) (map[string]organizationRoleInfo, error) {
	resp, err := s.ListWorkspaceRoles(ctx, organizationID, accountID, true)
	if err != nil {
		return nil, err
	}

	result := make(map[string]organizationRoleInfo, len(resp.Roles))
	for _, r := range resp.Roles {
		result[r.ID] = organizationRoleInfo{
			ID:          r.ID,
			Name:        r.Name,
			Builtin:     r.Builtin,
			Editable:    r.Editable,
			Status:      r.Status,
			Permissions: r.Permissions,
		}
	}

	return result, nil
}

// NewOrganizationService creates a new instance of OrganizationService
func NewOrganizationService(
	organizationRepo workspace_repo.OrganizationRepository,
	accountService interfaces.AccountService,
	workspaceRepo workspace_repo.WorkspaceRepository,
	workspaceService interfaces.WorkspaceManagementService,
	featureService interfaces.FeatureService,
	systemConfigService system_service.SystemConfigService,
	datasetService datasetsvc.DatasetService,
	db *gorm.DB,
	consoleProvider console.ConsoleProvider,
	officialRouteBootstrapper interfaces.OfficialRouteBootstrapper,
) interfaces.OrganizationService {
	return &organizationService{
		organizationRepo:           organizationRepo,
		accountService:             accountService,
		workspaceRepo:              workspaceRepo,
		workspaceManagementService: workspaceService,
		featureService:             featureService,
		systemConfigService:        systemConfigService,
		datasetService:             datasetService,
		db:                         db,
		consoleProvider:            consoleProvider,
		officialRouteBootstrapper:  officialRouteBootstrapper,
	}
}

func (s *organizationService) bootstrapOfficialRoute(ctx context.Context, organizationID string) {
	if s.officialRouteBootstrapper == nil || organizationID == "" {
		return
	}

	organizationUUID, err := uuid.Parse(organizationID)
	if err != nil {
		logger.Warn("Failed to parse organization ID for official route bootstrap: %v", err)
		return
	}

	if err := s.officialRouteBootstrapper.InitOfficialChannel(ctx, organizationUUID); err != nil {
		logger.Warn("Failed to bootstrap official route after organization creation: %v", err)
	}
}

func (s *organizationService) CheckWorkspaceAssets(ctx context.Context, workspaceID string) (bool, map[string]int64, error) {
	assetCounts := make(map[string]int64)

	var datasetCount int64
	if s.db != nil {
		if err := s.db.WithContext(ctx).
			Table("datasets").
			Where("workspace_id = ?", workspaceID).
			Count(&datasetCount).Error; err != nil {
			return false, nil, fmt.Errorf("failed to get dataset count: %w", err)
		}
	}
	assetCounts["datasets"] = datasetCount

	agentsCount, err := s.countAgentsByWorkspace(ctx, workspaceID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get agents count: %w", err)
	}
	assetCounts["agents"] = agentsCount

	dataSourceCount, err := s.countDataSourcesByTenant(ctx, workspaceID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get data source count: %w", err)
	}
	assetCounts["data_sources"] = dataSourceCount

	fileCount, err := s.countFilesByTeamTenant(ctx, workspaceID)
	if err != nil {
		return false, nil, fmt.Errorf("failed to get file count: %w", err)
	}
	assetCounts["files"] = fileCount

	hasAssets := datasetCount > 0 || agentsCount > 0 || dataSourceCount > 0 || fileCount > 0
	return hasAssets, assetCounts, nil
}

func (s *organizationService) countAgentsByWorkspace(ctx context.Context, workspaceID string) (int64, error) {
	var count int64
	if s.db == nil {
		return 0, nil
	}

	if err := s.db.WithContext(ctx).
		Model(&agents_model.Agent{}).
		Where("tenant_id = ? AND deleted_at IS NULL", workspaceID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *organizationService) countDataSourcesByTenant(ctx context.Context, workspaceID string) (int64, error) {
	var count int64
	if s.db == nil {
		return 0, nil
	}

	if err := s.db.WithContext(ctx).
		Model(&datasource_model.DataSource{}).
		Where("workspace_id = ?", workspaceID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func (s *organizationService) countFilesByTeamTenant(ctx context.Context, workspaceID string) (int64, error) {
	var count int64
	if s.db == nil {
		return 0, nil
	}

	if err := s.db.WithContext(ctx).
		Model(&file_model.UploadFile{}).
		Where("workspace_id = ?", workspaceID).
		Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// ListWorkspacePermissionDefinitions returns static permission definitions for an organization.
func (s *organizationService) ListWorkspacePermissionDefinitions(ctx context.Context, organizationID, accountID string) ([]string, error) {
	codes := model.AllWorkspacePermissionCodes()
	result := make([]string, len(codes))
	for i, c := range codes {
		result[i] = string(c)
	}
	return result, nil
}

func getBuiltinOrganizationRoleDescription(roleID string) (*string, *shared_dto.LocalizedString) {
	switch roleID {
	case model.WorkspaceBuiltinRoleOwnerID:
		descI18n := &shared_dto.LocalizedString{
			EnUS:   "Has the highest permissions in the team and full control over all resources.",
			ZhHans: "拥有团队内最高权限，全面管理团队和所有资源。",
		}
		desc := descI18n.EnUS
		return &desc, descI18n
	case model.WorkspaceBuiltinRoleAdminID:
		descI18n := &shared_dto.LocalizedString{
			EnUS:   "Helps the owner manage daily operations without permission to delete the team.",
			ZhHans: "协助拥有者进行日常运营，无权删除团队，适用于 Scrum Master。",
		}
		desc := descI18n.EnUS
		return &desc, descI18n
	case model.WorkspaceBuiltinRoleMemberID:
		descI18n := &shared_dto.LocalizedString{
			EnUS:   "Standard executor role with access only to assigned resources, suitable for most members.",
			ZhHans: "标准执行者角色，仅可使用分配的资源，适用于大多数团队成员。",
		}
		desc := descI18n.EnUS
		return &desc, descI18n
	case model.WorkspaceBuiltinRoleViewerID:
		descI18n := &shared_dto.LocalizedString{
			EnUS:   "Read-only role for viewing data without modifying resources, suitable for audit or external observers.",
			ZhHans: "仅查看数据，不可操作资源，适用于审计或外部观察员。",
		}
		desc := descI18n.EnUS
		return &desc, descI18n
	default:
		return nil, nil
	}
}

// ListWorkspaceRoles lists built-in and custom roles for an organization.
func (s *organizationService) ListWorkspaceRoles(ctx context.Context, organizationID, accountID string, includeOwner bool) (*shared_dto.WorkspaceRoleListResponse, error) {
	db := s.organizationRepo.GetDB()

	var customRoles []model.WorkspaceCustomRole
	if err := db.WithContext(ctx).
		Where("group_id = ? AND status != ?", organizationID, model.WorkspaceCustomRoleStatusDeleted).
		Find(&customRoles).Error; err != nil {
		return nil, fmt.Errorf("failed to list custom roles: %w", err)
	}

	roleSummaries := make([]shared_dto.WorkspaceRoleSummary, 0)

	convertPermissions := func(codes []model.WorkspacePermissionCode) []string {
		if len(codes) == 0 {
			return []string{}
		}
		result := make([]string, len(codes))
		for i, c := range codes {
			result[i] = string(c)
		}
		return result
	}

	ownerDesc, ownerDescI18n := getBuiltinOrganizationRoleDescription(model.WorkspaceBuiltinRoleOwnerID)
	adminDesc, adminDescI18n := getBuiltinOrganizationRoleDescription(model.WorkspaceBuiltinRoleAdminID)
	memberDesc, memberDescI18n := getBuiltinOrganizationRoleDescription(model.WorkspaceBuiltinRoleMemberID)
	viewerDesc, viewerDescI18n := getBuiltinOrganizationRoleDescription(model.WorkspaceBuiltinRoleViewerID)

	if includeOwner {
		roleSummaries = append(roleSummaries,
			shared_dto.WorkspaceRoleSummary{
				ID:              model.WorkspaceBuiltinRoleOwnerID,
				Name:            "Owner",
				Description:     ownerDesc,
				DescriptionI18n: ownerDescI18n,
				Builtin:         true,
				Editable:        false,
				Status:          model.WorkspaceCustomRoleStatusActive,
				Permissions:     convertPermissions(model.GetBuiltinGroupRolePermissionsByID(model.WorkspaceBuiltinRoleOwnerID)),
				MemberCount:     0,
			},
		)
	}

	roleSummaries = append(roleSummaries,
		shared_dto.WorkspaceRoleSummary{
			ID:              model.WorkspaceBuiltinRoleAdminID,
			Name:            "Admin",
			Description:     adminDesc,
			DescriptionI18n: adminDescI18n,
			Builtin:         true,
			Editable:        false,
			Status:          model.WorkspaceCustomRoleStatusActive,
			Permissions:     convertPermissions(model.GetBuiltinGroupRolePermissionsByID(model.WorkspaceBuiltinRoleAdminID)),
			MemberCount:     0,
		},
		shared_dto.WorkspaceRoleSummary{
			ID:              model.WorkspaceBuiltinRoleMemberID,
			Name:            "Member",
			Description:     memberDesc,
			DescriptionI18n: memberDescI18n,
			Builtin:         true,
			Editable:        false,
			Status:          model.WorkspaceCustomRoleStatusActive,
			Permissions:     convertPermissions(model.GetBuiltinGroupRolePermissionsByID(model.WorkspaceBuiltinRoleMemberID)),
			MemberCount:     0,
		},
		shared_dto.WorkspaceRoleSummary{
			ID:              model.WorkspaceBuiltinRoleViewerID,
			Name:            "Viewer",
			Description:     viewerDesc,
			DescriptionI18n: viewerDescI18n,
			Builtin:         true,
			Editable:        false,
			Status:          model.WorkspaceCustomRoleStatusActive,
			Permissions:     convertPermissions(model.GetBuiltinGroupRolePermissionsByID(model.WorkspaceBuiltinRoleViewerID)),
			MemberCount:     0,
		},
	)

	if len(customRoles) > 0 {
		for _, r := range customRoles {
			role := shared_dto.WorkspaceRoleSummary{
				ID:          r.ID,
				Name:        r.Name,
				Description: r.Description,
				Builtin:     false,
				Editable:    true,
				Status:      r.Status,
				Permissions: r.Permissions,
				MemberCount: 0,
			}
			roleSummaries = append(roleSummaries, role)
		}
	}

	tenantSubquery := db.WithContext(ctx).Table("workspaces").
		Select("id").
		Where("organization_id = ?", organizationID)

	for i, role := range roleSummaries {
		countQuery := db.WithContext(ctx).Table("accounts").
			Select("DISTINCT accounts.id").
			Joins("JOIN workspace_members ON accounts.id = workspace_members.account_id").
			Where("workspace_members.workspace_id IN (?)", tenantSubquery)

		builtinRole := ""
		if role.ID == model.WorkspaceBuiltinRoleOwnerID {
			builtinRole = string(model.WorkspaceRoleOwner)
		} else if role.ID == model.WorkspaceBuiltinRoleAdminID {
			builtinRole = string(model.WorkspaceRoleAdmin)
		} else if role.ID == model.WorkspaceBuiltinRoleMemberID {
			builtinRole = string(model.WorkspaceRoleNormal)
		}

		if builtinRole != "" {
			countQuery = countQuery.Where(
				"(workspace_members.role_id = ? OR workspace_members.role = ?)",
				role.ID,
				builtinRole,
			)
		} else {
			countQuery = countQuery.Where("workspace_members.role_id = ?", role.ID)
		}

		var total int64
		if err := countQuery.Count(&total).Error; err != nil {
			return nil, fmt.Errorf("failed to count role members: %w", err)
		}

		roleSummaries[i].MemberCount = total
	}

	return &shared_dto.WorkspaceRoleListResponse{
		Roles: roleSummaries,
	}, nil
}

func (s *organizationService) GetWorkspaceRoleDetail(ctx context.Context, organizationID, roleID, accountID string) (*shared_dto.OrganizationRoleDetailResponse, error) {
	db := s.organizationRepo.GetDB()

	switch roleID {
	case model.WorkspaceBuiltinRoleOwnerID,
		model.WorkspaceBuiltinRoleAdminID,
		model.WorkspaceBuiltinRoleMemberID,
		model.WorkspaceBuiltinRoleViewerID:
		desc, descI18n := getBuiltinOrganizationRoleDescription(roleID)
		perms := model.GetBuiltinGroupRolePermissionsByID(roleID)
		permCodes := make([]string, 0, len(perms))
		for _, p := range perms {
			permCodes = append(permCodes, string(p))
		}

		name := ""
		switch roleID {
		case model.WorkspaceBuiltinRoleOwnerID:
			name = "Owner"
		case model.WorkspaceBuiltinRoleAdminID:
			name = "Admin"
		case model.WorkspaceBuiltinRoleMemberID:
			name = "Member"
		case model.WorkspaceBuiltinRoleViewerID:
			name = "Viewer"
		}

		return &shared_dto.OrganizationRoleDetailResponse{
			ID:              roleID,
			OrganizationID:  organizationID,
			Name:            name,
			Description:     desc,
			DescriptionI18n: descI18n,
			Builtin:         true,
			Editable:        false,
			Status:          model.WorkspaceCustomRoleStatusActive,
			Permissions:     permCodes,
		}, nil
	default:
		var role model.WorkspaceCustomRole
		if err := db.WithContext(ctx).
			Where("id = ? AND group_id = ?", roleID, organizationID).
			First(&role).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("role not found")
			}
			return nil, fmt.Errorf("failed to get role: %w", err)
		}

		if role.Status == model.WorkspaceCustomRoleStatusDeleted {
			return nil, fmt.Errorf("role is deleted")
		}

		return &shared_dto.OrganizationRoleDetailResponse{
			ID:             role.ID,
			OrganizationID: role.OrganizationID,
			Name:           role.Name,
			Description:    role.Description,
			Builtin:        false,
			Editable:       true,
			Status:         role.Status,
			Permissions:    role.Permissions,
		}, nil
	}
}

func (s *organizationService) ListWorkspaceRoleMembers(ctx context.Context, organizationID, roleID, accountID, keyword string, page, limit int) (*shared_dto.OrganizationRoleMembersResponse, error) {
	db := s.organizationRepo.GetDB()

	offset := (page - 1) * limit

	tenantSubquery := db.WithContext(ctx).Table("workspaces").
		Select("id").
		Where("organization_id = ?", organizationID)

	var rows []struct {
		AccountID string    `gorm:"column:account_id"`
		Name      string    `gorm:"column:name"`
		Email     string    `gorm:"column:email"`
		Avatar    *string   `gorm:"column:avatar"`
		CreatedAt time.Time `gorm:"column:created_at"`
	}

	countQuery := db.WithContext(ctx).Table("accounts").
		Select("DISTINCT accounts.id").
		Joins("JOIN workspace_members ON accounts.id = workspace_members.account_id").
		Where("workspace_members.workspace_id IN (?)", tenantSubquery)

	baseQuery := db.WithContext(ctx).Table("accounts").
		Select("DISTINCT accounts.id as account_id, accounts.name, accounts.email, accounts.avatar, accounts.created_at").
		Joins("JOIN workspace_members ON accounts.id = workspace_members.account_id").
		Where("workspace_members.workspace_id IN (?)", tenantSubquery)

	builtinRole := ""
	switch roleID {
	case model.WorkspaceBuiltinRoleOwnerID:
		builtinRole = string(model.WorkspaceRoleOwner)
	case model.WorkspaceBuiltinRoleAdminID:
		builtinRole = string(model.WorkspaceRoleAdmin)
	case model.WorkspaceBuiltinRoleMemberID:
		builtinRole = string(model.WorkspaceRoleNormal)
	}

	if builtinRole != "" {
		condition := "(workspace_members.role_id IS NULL OR workspace_members.role_id = ?) AND workspace_members.role = ?"
		countQuery = countQuery.Where(condition, roleID, builtinRole)
		baseQuery = baseQuery.Where(condition, roleID, builtinRole)
	} else {
		countQuery = countQuery.Where("workspace_members.role_id = ?", roleID)
		baseQuery = baseQuery.Where("workspace_members.role_id = ?", roleID)
	}

	if keyword != "" {
		like := "%" + keyword + "%"
		countQuery = countQuery.Where("(accounts.name ILIKE ? OR accounts.email ILIKE ?)", like, like)
		baseQuery = baseQuery.Where("(accounts.name ILIKE ? OR accounts.email ILIKE ?)", like, like)
	}

	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count role members: %w", err)
	}

	if err := baseQuery.Order("accounts.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list role members: %w", err)
	}

	if len(rows) == 0 {
		return &shared_dto.OrganizationRoleMembersResponse{
			RoleID:  roleID,
			Items:   []shared_dto.OrganizationRoleMemberItem{},
			Page:    page,
			Limit:   limit,
			Total:   total,
			HasMore: false,
		}, nil
	}

	accountIDs := make([]string, 0, len(rows))
	accountIDSet := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if _, ok := accountIDSet[row.AccountID]; !ok {
			accountIDSet[row.AccountID] = struct{}{}
			accountIDs = append(accountIDs, row.AccountID)
		}
	}

	roleMap, err := s.getOrganizationRolesMap(ctx, organizationID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization roles: %w", err)
	}

	type workspaceRoleRow struct {
		AccountID   string                    `gorm:"column:account_id"`
		WorkspaceID string                    `gorm:"column:workspace_id"`
		Role        model.WorkspaceMemberRole `gorm:"column:role"`
		RoleID      *string                   `gorm:"column:role_id"`
	}

	var workspaceRoleRows []workspaceRoleRow
	roleQuery := db.WithContext(ctx).
		Table("workspace_members").
		Select("workspace_members.account_id, workspace_members.workspace_id, workspace_members.role, workspace_members.role_id").
		Where("workspace_members.workspace_id IN (?)", tenantSubquery).
		Where("workspace_members.account_id IN (?)", accountIDs)

	if builtinRole != "" {
		roleQuery = roleQuery.Where("(workspace_members.role_id IS NULL OR workspace_members.role_id = ?) AND workspace_members.role = ?", roleID, builtinRole)
	} else {
		roleQuery = roleQuery.Where("workspace_members.role_id = ?", roleID)
	}

	if err := roleQuery.Find(&workspaceRoleRows).Error; err != nil {
		return nil, fmt.Errorf("failed to list role member tenants: %w", err)
	}

	workspacePermsByAccount := make(map[string][]shared_dto.MemberWorkspacePermission)
	if len(workspaceRoleRows) > 0 {
		workspaceIDSet := make(map[string]struct{})
		for _, tr := range workspaceRoleRows {
			if _, ok := workspaceIDSet[tr.WorkspaceID]; !ok {
				workspaceIDSet[tr.WorkspaceID] = struct{}{}
			}
		}

		workspaceIDs := make([]string, 0, len(workspaceIDSet))
		for id := range workspaceIDSet {
			workspaceIDs = append(workspaceIDs, id)
		}

		workspaces, err := s.workspaceManagementService.GetWorkspacesByIDs(ctx, workspaceIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get workspaces by ids: %w", err)
		}

		workspaceNameByID := make(map[string]string, len(workspaces))
		for _, workspace := range workspaces {
			workspaceNameByID[workspace.ID] = workspace.Name
		}

		for _, tr := range workspaceRoleRows {
			roleName := ""
			if tr.RoleID != nil && *tr.RoleID != "" {
				if r, ok := roleMap[*tr.RoleID]; ok {
					roleName = r.Name
				}
			}
			if roleName == "" {
				roleName = string(tr.Role)
			}

			item := shared_dto.MemberWorkspacePermission{
				WorkspaceID:   tr.WorkspaceID,
				WorkspaceName: workspaceNameByID[tr.WorkspaceID],
				Role:          string(tr.Role),
				RoleID:        tr.RoleID,
				RoleName:      roleName,
			}

			workspacePermsByAccount[tr.AccountID] = append(workspacePermsByAccount[tr.AccountID], item)
		}
	}

	items := make([]shared_dto.OrganizationRoleMemberItem, 0, len(rows))
	for _, row := range rows {
		avatar := ""
		if row.Avatar != nil {
			avatar = *row.Avatar
		}

		item := shared_dto.OrganizationRoleMemberItem{
			AccountID:  row.AccountID,
			Name:       row.Name,
			Email:      row.Email,
			Avatar:     avatar,
			AvatarURL:  avatar,
			Workspaces: workspacePermsByAccount[row.AccountID],
		}
		items = append(items, item)
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.OrganizationRoleMembersResponse{
		RoleID:  roleID,
		Items:   items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) IsValidCustomWorkspaceRole(ctx context.Context, organizationID, roleID, accountID string) (bool, error) {
	db := s.organizationRepo.GetDB()

	var role model.WorkspaceCustomRole
	if err := db.WithContext(ctx).
		Where("id = ? AND group_id = ? AND status = ?", roleID, organizationID, model.WorkspaceCustomRoleStatusActive).
		First(&role).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to get custom role: %w", err)
	}

	return true, nil
}

// CreateCustomWorkspaceRole creates a new custom role with permissions
func (s *organizationService) CreateCustomWorkspaceRole(ctx context.Context, req *shared_dto.CreateWorkspaceRoleRequest) (*shared_dto.OrganizationRoleDetailResponse, error) {
	db := s.organizationRepo.GetDB()

	// Check if role name conflicts with built-in roles
	builtinNames := []string{"Owner", "Admin", "Member", "Viewer"}
	for _, name := range builtinNames {
		if strings.EqualFold(req.Name, name) {
			return nil, ErrRoleNameExists
		}
	}

	// Check if role name conflicts with existing custom roles
	var count int64
	if err := db.WithContext(ctx).Model(&model.WorkspaceCustomRole{}).
		Where("group_id = ? AND name = ? AND status != ?", req.OrganizationID, req.Name, model.WorkspaceCustomRoleStatusDeleted).
		Count(&count).Error; err != nil {
		return nil, fmt.Errorf("failed to check role name existence: %w", err)
	}
	if count > 0 {
		return nil, ErrRoleNameExists
	}

	role := &model.WorkspaceCustomRole{
		OrganizationID: req.OrganizationID,
		Name:           req.Name,
		Description:    req.Description,
		Status:         model.WorkspaceCustomRoleStatusActive,
		CreatedBy:      req.CreatedBy,
		Permissions:    req.Permissions,
	}

	if err := db.WithContext(ctx).Create(role).Error; err != nil {
		return nil, fmt.Errorf("failed to create role: %w", err)
	}

	return &shared_dto.OrganizationRoleDetailResponse{
		ID:             role.ID,
		OrganizationID: role.OrganizationID,
		Name:           role.Name,
		Description:    role.Description,
		Builtin:        false,
		Editable:       true,
		Status:         role.Status,
		Permissions:    req.Permissions,
	}, nil
}

// UpdateCustomWorkspaceRole updates role basic info
func (s *organizationService) UpdateCustomWorkspaceRole(ctx context.Context, req *shared_dto.UpdateWorkspaceRoleRequest) (*shared_dto.OrganizationRoleDetailResponse, error) {
	db := s.organizationRepo.GetDB()

	var role model.WorkspaceCustomRole
	if err := db.WithContext(ctx).
		Where("id = ? AND group_id = ?", req.RoleID, req.OrganizationID).
		First(&role).Error; err != nil {
		return nil, fmt.Errorf("role not found: %w", err)
	}

	if role.Status == model.WorkspaceCustomRoleStatusDeleted {
		return nil, fmt.Errorf("role is deleted")
	}

	if req.Name != nil {
		role.Name = *req.Name
	}
	if req.Description != nil {
		role.Description = req.Description
	}

	if err := db.WithContext(ctx).Save(&role).Error; err != nil {
		return nil, fmt.Errorf("failed to update role: %w", err)
	}

	return &shared_dto.OrganizationRoleDetailResponse{
		ID:             role.ID,
		OrganizationID: role.OrganizationID,
		Name:           role.Name,
		Description:    role.Description,
		Builtin:        false,
		Editable:       true,
		Status:         role.Status,
		Permissions:    role.Permissions,
	}, nil
}

// UpdateWorkspaceRolePermissions updates permissions of a role (idempotent)
func (s *organizationService) UpdateWorkspaceRolePermissions(ctx context.Context, req *shared_dto.UpdateWorkspaceRolePermissionsRequest) error {
	if model.IsBuiltinRole(req.RoleID) {
		return ErrCannotUpdateBuiltinRole
	}

	db := s.organizationRepo.GetDB()

	var role model.WorkspaceCustomRole
	if err := db.WithContext(ctx).
		Where("id = ? AND group_id = ?", req.RoleID, req.OrganizationID).
		First(&role).Error; err != nil {
		return fmt.Errorf("role not found: %w", err)
	}

	if role.Status == model.WorkspaceCustomRoleStatusDeleted {
		return fmt.Errorf("role is deleted")
	}

	role.Permissions = req.Permissions
	if err := db.WithContext(ctx).Save(&role).Error; err != nil {
		return fmt.Errorf("failed to update role permissions: %w", err)
	}

	return nil
}

func (s *organizationService) UpdateMemberInfo(ctx context.Context, req *shared_dto.UpdateOrganizationMemberRequest) error {
	// 1. Verify organization exists
	_, err := s.organizationRepo.GetByID(ctx, req.OrganizationID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	// 2. Verify member exists
	member, err := s.organizationRepo.GetAccountJoin(ctx, req.OrganizationID, req.AccountID)
	if err != nil {
		return fmt.Errorf("member not found: %w", err)
	}

	// 3. Update fields
	updates := map[string]interface{}{}
	if req.Role != nil {
		updates["role"] = *req.Role
	}
	if req.Name != nil && *req.Name != "" {
		// Check if name exists
		exists, err := s.organizationRepo.ExistsMemberByName(ctx, req.OrganizationID, *req.Name, req.AccountID)
		if err != nil {
			return fmt.Errorf("failed to check member name: %w", err)
		}
		if exists {
			return ErrMemberNameExists
		}
		updates["name"] = *req.Name
	}

	if len(updates) == 0 {
		return nil
	}

	// 4. Save updates
	if err := s.organizationRepo.GetDB().WithContext(ctx).Model(member).Updates(updates).Error; err != nil {
		return fmt.Errorf("failed to update member info: %w", err)
	}

	return nil
}

// DeleteCustomWorkspaceRole performs soft delete on custom role.
func (s *organizationService) DeleteCustomWorkspaceRole(ctx context.Context, organizationID, roleID, accountID string) error {
	db := s.organizationRepo.GetDB()

	var role model.WorkspaceCustomRole
	if err := db.WithContext(ctx).
		Where("id = ? AND group_id = ?", roleID, organizationID).
		First(&role).Error; err != nil {
		return fmt.Errorf("role not found: %w", err)
	}

	if role.Status == model.WorkspaceCustomRoleStatusDeleted {
		return nil
	}

	role.Status = model.WorkspaceCustomRoleStatusDeleted
	if err := db.WithContext(ctx).Save(&role).Error; err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	return nil
}

type workspaceRoleDetail struct {
	WorkspaceID string
	Role        model.WorkspaceMemberRole
	RoleID      *string
}

func (s *organizationService) getUserWorkspaceRoleDetailsInOrganization(ctx context.Context, organizationID, accountID string) ([]workspaceRoleDetail, error) {
	workspaces, err := s.organizationRepo.GetWorkspacesByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspaces for organization: %w", err)
	}

	details := make([]workspaceRoleDetail, 0)
	for _, workspace := range workspaces {
		join, err := s.workspaceManagementService.GetByWorkspaceAndMember(ctx, workspace.ID, accountID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) || join == nil {
				continue
			}
			return nil, fmt.Errorf("failed to get workspace account join: %w", err)
		}
		if join != nil {
			details = append(details, workspaceRoleDetail{
				WorkspaceID: workspace.ID,
				Role:        join.Role,
				RoleID:      join.RoleID,
			})
		}
	}

	return details, nil
}

// getUserWorkspaceRolesInOrganization returns all workspace roles of a user within an organization.
func (s *organizationService) getUserWorkspaceRolesInOrganization(ctx context.Context, organizationID, accountID string) ([]model.WorkspaceMemberRole, error) {
	details, err := s.getUserWorkspaceRoleDetailsInOrganization(ctx, organizationID, accountID)
	if err != nil {
		return nil, err
	}

	roles := make([]model.WorkspaceMemberRole, 0, len(details))
	for _, d := range details {
		roles = append(roles, d.Role)
	}

	return roles, nil
}

// deriveOrganizationRoleFromWorkspaceRoles maps workspace roles to an organization role.
func deriveOrganizationRoleFromWorkspaceRoles(workspaceRoles []model.WorkspaceMemberRole) model.OrganizationRole {
	groupRole := model.OrganizationRoleNormal
	for _, r := range workspaceRoles {
		switch r {
		case model.WorkspaceRoleOwner:
			return model.OrganizationRoleOwner
		case model.WorkspaceRoleAdmin:
			groupRole = model.OrganizationRoleAdmin
		default:
			// keep existing groupRole
		}
	}
	return groupRole
}

func (s *organizationService) GetMemberEffectivePermissions(ctx context.Context, organizationID, accountID, targetAccountID string) (*shared_dto.MemberPermissionsResponse, error) {
	db := s.organizationRepo.GetDB()

	var organizationRole model.OrganizationRole
	if err := db.WithContext(ctx).
		Table("members").
		Select("role").
		Where("organization_id = ? AND account_id = ?", organizationID, targetAccountID).
		Scan(&organizationRole).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to get organization role: %w", err)
	}
	if organizationRole == "" {
		organizationRole = model.OrganizationRoleNormal
	}

	details, err := s.getUserWorkspaceRoleDetailsInOrganization(ctx, organizationID, targetAccountID)
	if err != nil {
		return nil, err
	}

	if len(details) == 0 {
		return &shared_dto.MemberPermissionsResponse{
			OrganizationID: organizationID,
			AccountID:      targetAccountID,
			Role:           string(organizationRole),
			Workspaces:     []shared_dto.MemberWorkspacePermission{},
		}, nil
	}

	roleMap, err := s.getOrganizationRolesMap(ctx, organizationID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization roles: %w", err)
	}

	workspaceIDs := make([]string, 0, len(details))
	roleIDSet := make(map[string]struct{})
	for _, d := range details {
		workspaceIDs = append(workspaceIDs, d.WorkspaceID)
		if d.RoleID != nil && *d.RoleID != "" {
			if _, ok := roleIDSet[*d.RoleID]; !ok {
				roleIDSet[*d.RoleID] = struct{}{}
			}
		}
	}

	workspaces, err := s.workspaceManagementService.GetWorkspacesByIDs(ctx, workspaceIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get workspaces by ids: %w", err)
	}

	workspaceNameByID := make(map[string]string, len(workspaces))
	for _, workspace := range workspaces {
		workspaceNameByID[workspace.ID] = workspace.Name
	}

	roleNameByID := make(map[string]string)
	if len(roleIDSet) > 0 {
		ids := make([]string, 0, len(roleIDSet))
		for id := range roleIDSet {
			ids = append(ids, id)
		}

		var roles []model.WorkspaceCustomRole
		if err := db.WithContext(ctx).
			Where("group_id = ? AND id IN ?", organizationID, ids).
			Find(&roles).Error; err != nil {
			return nil, fmt.Errorf("failed to get custom roles: %w", err)
		}

		for _, r := range roles {
			roleNameByID[r.ID] = r.Name
		}
	}

	rolePermissionsCache := make(map[string][]string)
	workspacePermissions := make([]shared_dto.MemberWorkspacePermission, 0, len(details))
	for _, d := range details {
		name := workspaceNameByID[d.WorkspaceID]

		roleName := ""
		if d.RoleID != nil && *d.RoleID != "" {
			if r, ok := roleMap[*d.RoleID]; ok {
				roleName = r.Name
			} else if n, ok := roleNameByID[*d.RoleID]; ok {
				roleName = n
			}
		}
		if roleName == "" {
			roleName = string(d.Role)
		}

		var perms []string

		var roleID string
		if d.RoleID != nil && *d.RoleID != "" {
			roleID = *d.RoleID
		} else {
			switch d.Role {
			case model.WorkspaceRoleOwner:
				roleID = model.WorkspaceBuiltinRoleOwnerID
			case model.WorkspaceRoleAdmin:
				roleID = model.WorkspaceBuiltinRoleAdminID
			case model.WorkspaceRoleViewer:
				roleID = model.WorkspaceBuiltinRoleViewerID
			default:
				roleID = model.WorkspaceBuiltinRoleMemberID
			}
		}

		if cachedPerms, ok := rolePermissionsCache[roleID]; ok {
			perms = cachedPerms
		} else {
			roleDetail, err := s.GetWorkspaceRoleDetail(ctx, organizationID, roleID, accountID)
			if err != nil {
				if !strings.Contains(err.Error(), "role not found") {
					return nil, fmt.Errorf("failed to get organization role detail: %w", err)
				}
				perms = []string{}
			} else {
				perms = roleDetail.Permissions
			}
			rolePermissionsCache[roleID] = perms
		}

		item := shared_dto.MemberWorkspacePermission{
			WorkspaceID:   d.WorkspaceID,
			WorkspaceName: name,
			Role:          string(d.Role),
			RoleID:        d.RoleID,
			RoleName:      roleName,
			Permissions:   perms,
		}
		workspacePermissions = append(workspacePermissions, item)
	}

	return &shared_dto.MemberPermissionsResponse{
		OrganizationID: organizationID,
		AccountID:      targetAccountID,
		Role:           string(organizationRole),
		Workspaces:     workspacePermissions,
	}, nil
}

func (s *organizationService) GetWorkspaceMemberPermissions(ctx context.Context, organizationID, workspaceID, accountID, targetAccountID string) (*shared_dto.WorkspaceMemberPermissionsResponse, error) {
	if organizationID == "" || workspaceID == "" || targetAccountID == "" {
		return nil, fmt.Errorf("invalid parameters")
	}

	_, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	if err := s.ensureWorkspaceInOrganization(ctx, organizationID, workspaceID); err != nil {
		return nil, err
	}

	db := s.organizationRepo.GetDB()

	organizationRole := model.OrganizationRoleNormal
	if err := db.WithContext(ctx).
		Table("members").
		Select("role").
		Where("organization_id = ? AND account_id = ?", organizationID, targetAccountID).
		Scan(&organizationRole).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to get organization role: %w", err)
	}
	if organizationRole == "" {
		organizationRole = model.OrganizationRoleNormal
	}

	isOrgAdmin := organizationRole == model.OrganizationRoleOwner || organizationRole == model.OrganizationRoleAdmin

	var workspaceRoleID *string

	join, err := s.workspaceManagementService.GetByWorkspaceAndMember(ctx, workspaceID, targetAccountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if !isOrgAdmin {
				return nil, fmt.Errorf("workspace member not found")
			}
		} else {
			return nil, fmt.Errorf("failed to get workspace account join: %w", err)
		}
	} else if join == nil {
		if !isOrgAdmin {
			return nil, fmt.Errorf("workspace member not found")
		}
	}

	if join != nil {
		workspaceRoleID = join.RoleID
		if workspaceRoleID == nil || *workspaceRoleID == "" {
			if join.Role != "" {
				// Map built-in roles when RoleID is not available
				var roleID string
				switch join.Role {
				case model.WorkspaceRoleOwner:
					roleID = model.WorkspaceBuiltinRoleOwnerID
				case model.WorkspaceRoleAdmin:
					roleID = model.WorkspaceBuiltinRoleAdminID
				case model.WorkspaceRoleViewer:
					roleID = model.WorkspaceBuiltinRoleViewerID
				case model.WorkspaceRoleNormal: // Map 'normal' to 'member' role
					roleID = model.WorkspaceBuiltinRoleMemberID
				default:
					// Fallback or handle other legacy roles if needed
					roleID = model.WorkspaceBuiltinRoleMemberID
				}
				workspaceRoleID = &roleID
			}
		}
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("workspace not found")
		}
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}
	if workspace == nil {
		return nil, fmt.Errorf("workspace not found")
	}

	roleName := ""
	if workspaceRoleID != nil && *workspaceRoleID != "" {
		roleMap, err := s.getOrganizationRolesMap(ctx, organizationID, accountID)
		if err != nil {
			return nil, fmt.Errorf("failed to get organization roles: %w", err)
		}
		if r, ok := roleMap[*workspaceRoleID]; ok {
			roleName = r.Name
		}
	}

	var rolePermissions []string

	var roleID string
	if isOrgAdmin {
		roleID = model.WorkspaceBuiltinRoleOwnerID
	} else if workspaceRoleID != nil && *workspaceRoleID != "" {
		roleID = *workspaceRoleID
	}

	roleDetail, err := s.GetWorkspaceRoleDetail(ctx, organizationID, roleID, accountID)
	if err != nil {
		if !strings.Contains(err.Error(), "role not found") {
			return nil, fmt.Errorf("failed to get workspace role detail: %w", err)
		}
		rolePermissions = []string{}
	} else {
		rolePermissions = roleDetail.Permissions
	}

	return &shared_dto.WorkspaceMemberPermissionsResponse{
		OrganizationID:    organizationID,
		WorkspaceID:       workspaceID,
		WorkspaceName:     workspace.Name,
		AccountID:         targetAccountID,
		OrganizationRole:  string(organizationRole),
		WorkspaceRole:     roleName,
		WorkspaceRoleID:   workspaceRoleID,
		WorkspaceRoleName: roleName,
		Permissions:       rolePermissions,
	}, nil
}

// CreateOrganization creates a new organization and adds the creator as admin
func (s *organizationService) CreateOrganization(ctx context.Context, req *shared_dto.CreateOrganizationRequest) (*model.Organization, error) {
	// Begin transaction
	tx := s.organizationRepo.GetDB().Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	var committed bool
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
		if !committed {
			tx.Rollback()
		}
	}()

	account, err := s.accountService.GetAccountByID(ctx, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("creator account not found: %w", err)
	}

	organization := &model.Organization{
		ID:        uuid.New().String(),
		Name:      req.Name,
		Status:    model.OrganizationStatusActive,
		CreatedAt: time.Now(),
	}

	if err := s.organizationRepo.CreateWithTx(ctx, tx, organization); err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	join := &model.OrganizationMember{
		OrganizationID: organization.ID,
		AccountID:      req.CreatedBy,
		Role:           model.OrganizationRoleAdmin,
		Name:           &account.Name,
		CreatedAt:      time.Now(),
	}

	if err := s.organizationRepo.CreateAccountJoinWithTx(ctx, tx, join); err != nil {
		return nil, fmt.Errorf("failed to add creator as admin: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}
	committed = true
	s.bootstrapOfficialRoute(ctx, organization.ID)

	return organization, nil
}

// GetOrganizationByID retrieves an organization by its ID
func (s *organizationService) GetOrganizationByID(ctx context.Context, id string) (*model.Organization, error) {
	organization, err := s.organizationRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	return organization, nil
}

// UpdateOrganization updates the name of an existing organization
func (s *organizationService) UpdateOrganization(ctx context.Context, id string, req *shared_dto.UpdateOrganizationRequest) error {
	organization, err := s.organizationRepo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	organization.Name = req.Name
	organization.UpdatedAt = time.Now()

	if err := s.organizationRepo.Update(ctx, organization); err != nil {
		return fmt.Errorf("failed to update organization: %w", err)
	}

	return nil
}

// DeleteOrganization performs a soft delete of an organization (archives it)
func (s *organizationService) DeleteOrganization(ctx context.Context, id string, accountID string) error {
	organization, err := s.organizationRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("organization not found")
		}
		return fmt.Errorf("failed to get organization: %w", err)
	}

	organization.Status = model.OrganizationStatusArchived
	organization.UpdatedAt = time.Now()

	if err := s.organizationRepo.Update(ctx, organization); err != nil {
		return fmt.Errorf("failed to delete organization: %w", err)
	}

	return nil
}

// AddWorkspace adds a tenant to an organization
func (s *organizationService) AddWorkspace(ctx context.Context, req *shared_dto.AddWorkspaceToOrganizationRequest) error {
	_, err := s.organizationRepo.GetByID(ctx, req.OrganizationID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	workspaceID := req.WorkspaceID
	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	if workspace.OrganizationID != nil && *workspace.OrganizationID == req.OrganizationID {
		return errors.New("workspace already exists in organization")
	}

	if workspace.OrganizationID != nil && *workspace.OrganizationID != "" {
		return errors.New("workspace already belongs to an organization")
	}

	db := s.organizationRepo.GetDB()

	// Update tenant organization info
	workspace.OrganizationID = &req.OrganizationID

	if req.DepartmentID != nil && *req.DepartmentID != "" {
		var dept struct {
			ID             string
			OrganizationID string `gorm:"column:group_id"`
		}

		if err := db.WithContext(ctx).
			Table("departments").
			Select("id, group_id").
			Where("id = ?", *req.DepartmentID).
			First(&dept).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("department not found in organization")
			}
			return fmt.Errorf("failed to get department: %w", err)
		}

		if dept.OrganizationID != req.OrganizationID {
			return fmt.Errorf("department does not belong to the specified organization")
		}

		workspace.DepartmentID = req.DepartmentID
	}

	if req.APIKeyID != nil && *req.APIKeyID != "" {
		workspaceIDs := []string{req.OrganizationID}

		var existingTenantIDs []string
		if err := db.Model(&model.Workspace{}).Where("organization_id = ?", req.OrganizationID).Pluck("id", &existingTenantIDs).Error; err != nil {
			return fmt.Errorf("failed to get workspaces for organization: %w", err)
		}
		workspaceIDs = append(workspaceIDs, existingTenantIDs...)

		var apiKey apikeymodel.TenantAPIKey

		if err := db.WithContext(ctx).
			Where("id = ? AND tenant_id IN ? AND is_internal = ?", *req.APIKeyID, workspaceIDs, false).
			First(&apiKey).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("api key not found in organization")
			}
			return fmt.Errorf("failed to get api key: %w", err)
		}

		if !apiKey.IsActive() {
			return fmt.Errorf("api key is not active")
		}

		workspace.ApiKeyID = req.APIKeyID
	}

	if err := s.workspaceRepo.Update(ctx, workspace); err != nil {
		return fmt.Errorf("failed to add workspace to organization: %w", err)
	}

	return nil
}

func (s *organizationService) UpdateWorkspaceJoinMeta(ctx context.Context, organizationID, workspaceID string, departmentID *string, apiKeyID *string) error {
	if departmentID == nil && apiKeyID == nil {
		return nil
	}

	db := s.organizationRepo.GetDB()

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace: %w", err)
	}

	if workspace.OrganizationID == nil || *workspace.OrganizationID != organizationID {
		return fmt.Errorf("workspace does not exist in organization")
	}

	if departmentID != nil {
		if *departmentID == "" {
			workspace.DepartmentID = nil
		} else {
			var dept struct {
				ID             string
				OrganizationID string `gorm:"column:group_id"`
			}

			if err := db.WithContext(ctx).
				Table("departments").
				Select("id, group_id").
				Where("id = ?", *departmentID).
				First(&dept).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("department not found in organization")
				}
				return fmt.Errorf("failed to get department: %w", err)
			}

			if dept.OrganizationID != organizationID {
				return fmt.Errorf("department does not belong to the specified organization")
			}

			workspace.DepartmentID = departmentID
		}
	}

	if apiKeyID != nil {
		if *apiKeyID == "" {
			workspace.ApiKeyID = nil
		} else {
			workspaceIDs := []string{organizationID}

			var existingTenantIDs []string
			if err := db.Model(&model.Workspace{}).Where("organization_id = ?", organizationID).Pluck("id", &existingTenantIDs).Error; err != nil {
				return fmt.Errorf("failed to get workspaces for organization: %w", err)
			}
			workspaceIDs = append(workspaceIDs, existingTenantIDs...)

			var apiKey apikeymodel.TenantAPIKey

			if err := db.WithContext(ctx).
				Where("id = ? AND tenant_id IN ? AND is_internal = ?", *apiKeyID, workspaceIDs, false).
				First(&apiKey).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("api key not found in organization")
				}
				return fmt.Errorf("failed to get api key: %w", err)
			}

			if !apiKey.IsActive() {
				return fmt.Errorf("api key is not active")
			}

			workspace.ApiKeyID = apiKeyID
		}
	}

	if err := s.workspaceRepo.Update(ctx, workspace); err != nil {
		return fmt.Errorf("failed to update workspace join: %w", err)
	}

	return nil
}

// RemoveWorkspace removes a workspace from an organization.
func (s *organizationService) RemoveWorkspace(ctx context.Context, organizationID, workspaceID string) error {
	_, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	if workspace.OrganizationID == nil || *workspace.OrganizationID != organizationID {
		return errors.New("workspace does not exist in organization")
	}

	workspace.OrganizationID = nil
	workspace.DepartmentID = nil
	workspace.ApiKeyID = nil

	if err := s.workspaceRepo.Update(ctx, workspace); err != nil {
		return fmt.Errorf("failed to remove workspace from organization: %w", err)
	}

	return nil
}

// AddMember adds a new member to an organization with specified role
func (s *organizationService) AddMember(ctx context.Context, req *shared_dto.AddOrganizationMemberRequest) error {
	_, err := s.organizationRepo.GetByID(ctx, req.OrganizationID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	_, err = s.accountService.GetAccountByID(ctx, req.AccountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	join, err := s.organizationRepo.GetAccountJoin(ctx, req.OrganizationID, req.AccountID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check account existence: %w", err)
	}
	if join != nil {
		return errors.New("account already exists in organization")
	}

	join = &model.OrganizationMember{
		OrganizationID: req.OrganizationID,
		AccountID:      req.AccountID,
		Role:           req.Role,
		Name:           req.Name, // Map nickname from request
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.organizationRepo.CreateAccountJoin(ctx, join); err != nil {
		return fmt.Errorf("failed to add member to organization: %w", err)
	}

	return nil
}

// RemoveMember removes a member from an organization and all related tenants
func (s *organizationService) RemoveMember(ctx context.Context, organizationID, accountID string) error {
	_, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	_, err = s.accountService.GetAccountByID(ctx, accountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	join, err := s.organizationRepo.GetAccountJoin(ctx, organizationID, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("account does not exist in organization")
		}
		return fmt.Errorf("failed to check account existence: %w", err)
	}
	if join == nil {
		return errors.New("account does not exist in organization")
	}

	// Cannot remove owner
	if join.Role == model.OrganizationRoleOwner {
		return errors.New("cannot remove the owner of the organization")
	}

	db := s.organizationRepo.GetDB()

	// Use transaction to ensure atomicity
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. Get all tenant IDs where the member belongs under this organization
		var workspaceIDs []string
		groupTenantsSubquery := tx.Table("workspaces").
			Select("id").
			Where("organization_id = ?", organizationID)

		err := tx.Table("workspace_members").
			Select("workspace_id").
			Where("workspace_id IN (?) AND account_id = ?", groupTenantsSubquery, accountID).
			Pluck("workspace_id", &workspaceIDs).Error
		if err != nil {
			return fmt.Errorf("failed to get user workspaces: %w", err)
		}

		// 2. Delete member associations in all workspaces under this organization (including the shadow workspace).
		allWorkspaceIDs := append(workspaceIDs, organizationID)

		if len(allWorkspaceIDs) > 0 {
			// First get the workspace member IDs to be deleted.
			var joinIDs []string
			if err := tx.Table("workspace_members").
				Select("id").
				Where("workspace_id IN (?) AND account_id = ?", allWorkspaceIDs, accountID).
				Pluck("id", &joinIDs).Error; err != nil {
				return fmt.Errorf("failed to get workspace account joins: %w", err)
			}

			if len(joinIDs) > 0 {
				// Delete workspace memberships.
				if err := tx.Where("id IN (?)", joinIDs).
					Delete(&model.WorkspaceMember{}).Error; err != nil {
					return fmt.Errorf("failed to remove workspace memberships: %w", err)
				}
			}
		}

		// 3. Delete department memberships
		deptSubQuery := tx.Table("departments").Select("id").Where("group_id = ?", organizationID)
		if err := tx.Where("account_id = ? AND department_id IN (?)", accountID, deptSubQuery).
			Delete(&model.DepartmentMember{}).Error; err != nil {
			return fmt.Errorf("failed to remove department memberships: %w", err)
		}

		// 4. Delete organization member role
		if err := tx.Where("organization_id = ? AND account_id = ?", organizationID, accountID).
			Delete(&model.OrganizationMember{}).Error; err != nil {
			return fmt.Errorf("failed to remove organization membership: %w", err)
		}

		return nil
	})
}

// UpdateMemberRole updates the role of a member in an organization
func (s *organizationService) UpdateMemberRole(ctx context.Context, req *shared_dto.UpdateOrganizationMemberRoleRequest) error {
	_, err := s.organizationRepo.GetByID(ctx, req.OrganizationID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	_, err = s.accountService.GetAccountByID(ctx, req.AccountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	join, err := s.organizationRepo.GetAccountJoin(ctx, req.OrganizationID, req.AccountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("account does not exist in organization")
		}
		return fmt.Errorf("failed to check account existence: %w", err)
	}
	if join == nil {
		return errors.New("account does not exist in organization")
	}

	if join.Role == model.OrganizationRoleAdmin {
		return errors.New("cannot change the role of the admin")
	}

	join.Role = req.Role
	join.UpdatedAt = time.Now()
	if err := s.organizationRepo.UpdateAccountJoin(ctx, join); err != nil {
		return fmt.Errorf("failed to update member role: %w", err)
	}

	return nil
}

// UpdateMemberStatus updates the status of a member in an organization (active/inactive)
func (s *organizationService) UpdateMemberStatus(ctx context.Context, req *shared_dto.UpdateOrganizationMemberStatusRequest) error {
	_, err := s.organizationRepo.GetByID(ctx, req.OrganizationID)
	if err != nil {
		return fmt.Errorf("organization not found: %w", err)
	}

	_, err = s.accountService.GetAccountByID(ctx, req.AccountID)
	if err != nil {
		return fmt.Errorf("account not found: %w", err)
	}

	join, err := s.organizationRepo.GetAccountJoin(ctx, req.OrganizationID, req.AccountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return errors.New("account does not exist in organization")
		}
		return fmt.Errorf("failed to check account existence: %w", err)
	}
	if join == nil {
		return errors.New("account does not exist in organization")
	}

	// Cannot disable the admin/owner
	if join.Role == model.OrganizationRoleAdmin && req.Status == model.OrganizationMemberStatusInactive {
		return errors.New("cannot disable the admin/owner of the organization")
	}

	join.Status = req.Status
	join.UpdatedAt = time.Now()
	if err := s.organizationRepo.UpdateAccountJoin(ctx, join); err != nil {
		return fmt.Errorf("failed to update member status: %w", err)
	}

	return nil
}

// TransferOwnership transfers the ownership of the organization.
func (s *organizationService) TransferOwnership(ctx context.Context, organizationID, currentOwnerID, newOwnerID string) error {
	return s.organizationRepo.GetDB().Transaction(func(tx *gorm.DB) error {
		repoTx := s.organizationRepo.WithTx(tx)

		// 1. Verify organization exists.
		organization, err := repoTx.GetByID(ctx, organizationID)
		if err != nil {
			return fmt.Errorf("organization not found: %w", err)
		}
		if organization == nil {
			return errors.New("organization not found")
		}

		// 2. Verify current owner.
		currentOwnerJoin, err := repoTx.GetAccountJoin(ctx, organizationID, currentOwnerID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("current owner is not a member of the organization")
			}
			return fmt.Errorf("failed to check current owner: %w", err)
		}
		if currentOwnerJoin.Role != model.OrganizationRoleOwner {
			return errors.New("current user is not the owner of the organization")
		}

		// 3. Verify new owner.
		newOwnerJoin, err := repoTx.GetAccountJoin(ctx, organizationID, newOwnerID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("new owner is not a member of the organization")
			}
			return fmt.Errorf("failed to check new owner: %w", err)
		}
		if newOwnerJoin == nil {
			return errors.New("new owner is not a member of the organization")
		}

		if newOwnerJoin.Status != model.OrganizationMemberStatusActive {
			return errors.New("new owner is not active")
		}

		// 4. Update roles.
		// Demote current owner to admin.
		currentOwnerJoin.Role = model.OrganizationRoleAdmin
		if err := repoTx.UpdateAccountJoin(ctx, currentOwnerJoin); err != nil {
			return fmt.Errorf("failed to demote current owner: %w", err)
		}

		// Promote new owner to owner.
		newOwnerJoin.Role = model.OrganizationRoleOwner
		if err := repoTx.UpdateAccountJoin(ctx, newOwnerJoin); err != nil {
			return fmt.Errorf("failed to promote new owner: %w", err)
		}

		return nil
	})
}

// GetOrganizationMembers retrieves all members of an organization
func (s *organizationService) GetOrganizationMembers(ctx context.Context, organizationID string) ([]*shared_dto.OrganizationMemberResponse, error) {
	_, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	joins, err := s.organizationRepo.GetAccountsByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization members: %w", err)
	}

	responses := make([]*shared_dto.OrganizationMemberResponse, 0, len(joins))
	for _, join := range joins {
		account, err := s.accountService.GetAccountByID(ctx, join.AccountID)
		if err != nil {
			continue
		}

		responses = append(responses, &shared_dto.OrganizationMemberResponse{
			AccountID: account.ID,
			Email:     account.Email,
			Username:  account.Name,
			Role:      join.Role,
			JoinedAt:  join.CreatedAt,
		})
	}

	return responses, nil
}

// GetOrganizationWorkspaces retrieves all tenants in an organization with pagination
func (s *organizationService) GetOrganizationWorkspaces(ctx context.Context, organizationID string, page, limit int, accountID string) (*shared_dto.WorkspacePaginationResponse, error) {
	if accountID == "" {
		return nil, fmt.Errorf("user not found")
	}

	_, err := s.accountService.GetAccountByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	_, err = s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)

	var tenants []*model.Workspace
	var total int64

	if isOrganizationAdmin {
		tenants, total, err = s.getWorkspacesByOrganizationIDWithPagination(ctx, organizationID, page, limit, "", "")
		if err != nil {
			return nil, fmt.Errorf("failed to get organization tenants: %w", err)
		}
	} else {
		tenants, total, err = s.getUserWorkspacesByOrganizationIDWithPagination(ctx, organizationID, accountID, page, limit, "", "")
		if err != nil {
			return nil, fmt.Errorf("failed to get user organization workspaces: %w", err)
		}
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.WorkspacePaginationResponse{
		Data:    tenants,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetOrganizationWorkspacesWithDetails(ctx context.Context, organizationID string, page, limit int, accountID string, status string, keyword string) (*shared_dto.OrganizationWorkspacePaginationResponse, error) {
	if accountID == "" {
		return nil, fmt.Errorf("user not found")
	}

	_, err := s.accountService.GetAccountByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	_, err = s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)

	var tenants []*model.Workspace
	var total int64

	if isOrganizationAdmin {
		tenants, total, err = s.getWorkspacesByOrganizationIDWithPagination(ctx, organizationID, page, limit, status, keyword)
		if err != nil {
			return nil, fmt.Errorf("failed to get organization tenants: %w", err)
		}
	} else {
		tenants, total, err = s.getUserWorkspacesByOrganizationIDWithPagination(ctx, organizationID, accountID, page, limit, status, keyword)
		if err != nil {
			return nil, fmt.Errorf("failed to get user organization workspaces: %w", err)
		}
	}

	hasMore := int64(page*limit) < total

	items, err := s.buildOrganizationWorkspaceResponses(ctx, organizationID, tenants)
	if err != nil {
		return nil, err
	}

	return &shared_dto.OrganizationWorkspacePaginationResponse{
		Data:    items,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetOrganizationWorkspaceDetail(ctx context.Context, organizationID, workspaceID, accountID string) (*shared_dto.OrganizationWorkspaceResponse, error) {
	if accountID == "" {
		return nil, fmt.Errorf("user not found")
	}

	_, err := s.accountService.GetAccountByID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	_, err = s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	if err := s.ensureWorkspaceInOrganization(ctx, organizationID, workspaceID); err != nil {
		return nil, err
	}

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)

	if !isOrganizationAdmin {
		db := s.organizationRepo.GetDB()
		var count int64
		if err := db.WithContext(ctx).
			Table("workspace_members").
			Where("workspace_id = ? AND account_id = ?", workspaceID, accountID).
			Count(&count).Error; err != nil {
			return nil, fmt.Errorf("failed to check workspace membership: %w", err)
		}
		if count == 0 {
			return nil, fmt.Errorf("permission denied")
		}
	}

	workspace, err := s.workspaceRepo.GetByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("workspace not found")
		}
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}
	if workspace == nil {
		return nil, fmt.Errorf("workspace not found")
	}

	items, err := s.buildOrganizationWorkspaceResponses(ctx, organizationID, []*model.Workspace{workspace})
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("workspace not found")
	}

	return items[0], nil
}

func (s *organizationService) getWorkspacesByOrganizationIDWithPagination(ctx context.Context, organizationID string, page, limit int, status string, keyword string) ([]*model.Workspace, int64, error) {
	offset := (page - 1) * limit

	workspaces, err := s.organizationRepo.GetWorkspacesByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, 0, err
	}

	workspaceIDs := make([]string, len(workspaces))
	for i, workspace := range workspaces {
		workspaceIDs[i] = workspace.ID
	}

	if len(workspaceIDs) == 0 {
		return []*model.Workspace{}, 0, nil
	}

	var total int64

	db := s.organizationRepo.GetDB()
	query := db.WithContext(ctx).Model(&model.Workspace{}).Where("id IN (?)", workspaceIDs)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		query = query.Where("name ILIKE ?", "%"+keyword+"%")
	}

	err = query.Count(&total).Error
	if err != nil {
		return nil, 0, err
	}

	query = db.WithContext(ctx).Where("id IN (?)", workspaceIDs)
	if status != "" {
		query = query.Where("status = ?", status)
	}
	if keyword != "" {
		query = query.Where("name ILIKE ?", "%"+keyword+"%")
	}

	err = query.
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&workspaces).Error

	return workspaces, total, err
}

func (s *organizationService) buildOrganizationWorkspaceResponses(ctx context.Context, organizationID string, workspaces []*model.Workspace) ([]*shared_dto.OrganizationWorkspaceResponse, error) {
	if len(workspaces) == 0 {
		return []*shared_dto.OrganizationWorkspaceResponse{}, nil
	}

	workspaceIDs := make([]string, 0, len(workspaces))
	for _, workspace := range workspaces {
		workspaceIDs = append(workspaceIDs, workspace.ID)
	}

	db := s.organizationRepo.GetDB()

	type workspaceJoinInfo struct {
		WorkspaceID  string  `gorm:"column:id"`
		DepartmentID *string `gorm:"column:department_id"`
		APIKeyID     *string `gorm:"column:api_key_id"`
	}

	var joins []workspaceJoinInfo
	if err := db.WithContext(ctx).
		Table("workspaces").
		Select("id, department_id, api_key_id").
		Where("organization_id = ? AND id IN ?", organizationID, workspaceIDs).
		Scan(&joins).Error; err != nil {
		return nil, fmt.Errorf("failed to load workspace joins: %w", err)
	}

	joinByWorkspace := make(map[string]workspaceJoinInfo, len(joins))
	departmentIDs := make(map[string]struct{})
	apiKeyIDs := make(map[string]struct{})

	for _, j := range joins {
		joinByWorkspace[j.WorkspaceID] = j
		if j.DepartmentID != nil && *j.DepartmentID != "" {
			departmentIDs[*j.DepartmentID] = struct{}{}
		}
		if j.APIKeyID != nil && *j.APIKeyID != "" {
			apiKeyIDs[*j.APIKeyID] = struct{}{}
		}
	}

	type ownerInfo struct {
		WorkspaceID string `gorm:"column:workspace_id"`
		AccountID   string `gorm:"column:account_id"`
	}

	var owners []ownerInfo
	if err := db.WithContext(ctx).
		Table("workspace_members").
		Select("workspace_id, account_id").
		Where("workspace_id IN ? AND role = ?", workspaceIDs, model.WorkspaceRoleOwner).
		Scan(&owners).Error; err != nil {
		return nil, fmt.Errorf("failed to load workspace owners: %w", err)
	}

	ownerByWorkspace := make(map[string]string, len(owners))
	accountIDs := make(map[string]struct{})
	for _, o := range owners {
		ownerByWorkspace[o.WorkspaceID] = o.AccountID
		if o.AccountID != "" {
			accountIDs[o.AccountID] = struct{}{}
		}
	}

	type memberCountResult struct {
		WorkspaceID string `gorm:"column:workspace_id"`
		MemberCount int64  `gorm:"column:member_count"`
	}

	var memberCounts []memberCountResult
	if err := db.WithContext(ctx).
		Table("workspace_members").
		Select("workspace_id, COUNT(*) AS member_count").
		Where("workspace_id IN ?", workspaceIDs).
		Group("workspace_id").
		Scan(&memberCounts).Error; err != nil {
		return nil, fmt.Errorf("failed to load workspace member counts: %w", err)
	}

	memberCountByWorkspace := make(map[string]int64, len(memberCounts))
	for _, mc := range memberCounts {
		memberCountByWorkspace[mc.WorkspaceID] = mc.MemberCount
	}

	accountNameByID := make(map[string]string)
	if len(accountIDs) > 0 {
		var accounts []auth_model.Account
		ids := make([]string, 0, len(accountIDs))
		for id := range accountIDs {
			ids = append(ids, id)
		}

		if err := db.WithContext(ctx).
			Model(&auth_model.Account{}).
			Where("id IN ?", ids).
			Find(&accounts).Error; err != nil {
			return nil, fmt.Errorf("failed to load accounts: %w", err)
		}

		for _, a := range accounts {
			accountNameByID[a.ID] = a.Name
		}
	}

	departmentNameByID := make(map[string]string)
	if len(departmentIDs) > 0 {
		var departments []model.Department
		ids := make([]string, 0, len(departmentIDs))
		for id := range departmentIDs {
			ids = append(ids, id)
		}

		if err := db.WithContext(ctx).
			Model(&model.Department{}).
			Where("id IN ?", ids).
			Find(&departments).Error; err != nil {
			return nil, fmt.Errorf("failed to load departments: %w", err)
		}

		for _, d := range departments {
			departmentNameByID[d.ID] = d.Name
		}
	}

	apiKeyNameByID := make(map[string]string)
	if len(apiKeyIDs) > 0 {
		var apiKeys []apikeymodel.TenantAPIKey
		ids := make([]string, 0, len(apiKeyIDs))
		for id := range apiKeyIDs {
			ids = append(ids, id)
		}

		if err := db.WithContext(ctx).
			Model(&apikeymodel.TenantAPIKey{}).
			Where("id IN ? AND is_internal = ?", ids, false).
			Find(&apiKeys).Error; err != nil {
			return nil, fmt.Errorf("failed to load api keys: %w", err)
		}

		for _, k := range apiKeys {
			apiKeyNameByID[k.ID] = k.Name
		}
	}

	items := make([]*shared_dto.OrganizationWorkspaceResponse, 0, len(workspaces))
	for _, workspace := range workspaces {
		join, ok := joinByWorkspace[workspace.ID]

		var departmentIDPtr *string
		var departmentNamePtr *string
		var apiKeyIDPtr *string
		var apiKeyNamePtr *string

		if ok {
			if join.DepartmentID != nil && *join.DepartmentID != "" {
				deptID := *join.DepartmentID
				departmentIDPtr = &deptID
				if name, exists := departmentNameByID[deptID]; exists {
					n := name
					departmentNamePtr = &n
				}
			}

			if join.APIKeyID != nil && *join.APIKeyID != "" {
				keyID := *join.APIKeyID
				apiKeyIDPtr = &keyID
				if name, exists := apiKeyNameByID[keyID]; exists {
					n := name
					apiKeyNamePtr = &n
				}
			}
		}

		var leaderIDPtr *string
		var leaderNamePtr *string
		if ownerID, exists := ownerByWorkspace[workspace.ID]; exists && ownerID != "" {
			id := ownerID
			leaderIDPtr = &id
			if name, ok := accountNameByID[ownerID]; ok {
				n := name
				leaderNamePtr = &n
			}
		}

		item := &shared_dto.OrganizationWorkspaceResponse{
			ID:             workspace.ID,
			Name:           workspace.Name,
			Status:         string(workspace.Status),
			CreatedAt:      workspace.CreatedAt.Unix(),
			LeaderID:       leaderIDPtr,
			LeaderName:     leaderNamePtr,
			DepartmentID:   departmentIDPtr,
			DepartmentName: departmentNamePtr,
			APIKeyID:       apiKeyIDPtr,
			APIKeyName:     apiKeyNamePtr,
			MemberCount:    memberCountByWorkspace[workspace.ID],
		}

		items = append(items, item)
	}

	return items, nil
}

func (s *organizationService) getUserWorkspacesByOrganizationIDWithPagination(ctx context.Context, organizationID, accountID string, page, limit int, status string, keyword string) ([]*model.Workspace, int64, error) {
	offset := (page - 1) * limit

	db := s.organizationRepo.GetDB()
	query := db.WithContext(ctx).Model(&model.Workspace{}).
		Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
		Where("workspaces.organization_id = ? AND workspace_members.account_id = ?", organizationID, accountID)

	if status != "" {
		query = query.Where("workspaces.status = ?", status)
	}
	if keyword != "" {
		query = query.Where("workspaces.name ILIKE ?", "%"+keyword+"%")
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	var tenants []*model.Workspace
	err := query.Order("workspaces.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tenants).Error

	return tenants, total, err
}

func (s *organizationService) checkOrganizationAdmin(ctx context.Context, accountID, organizationID string) bool {
	join, err := s.organizationRepo.GetAccountJoin(ctx, organizationID, accountID)
	if err != nil {
		return false
	}
	return join.Role == model.OrganizationRoleOwner || join.Role == model.OrganizationRoleAdmin
}

func (s *organizationService) ensureWorkspaceInOrganization(ctx context.Context, organizationID, workspaceID string) error {
	targetOrganizationID, err := s.organizationRepo.GetOrganizationIDByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("failed to get workspace organization: %w", err)
	}
	if targetOrganizationID != organizationID {
		return fmt.Errorf("workspace not in organization")
	}
	return nil
}

func (s *organizationService) IsOrganizationMember(ctx context.Context, organizationID, accountID string) (bool, error) {
	join, err := s.organizationRepo.GetAccountJoin(ctx, organizationID, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check organization membership: %w", err)
	}
	if join == nil {
		return false, nil
	}
	return true, nil
}

func (s *organizationService) ExistsMemberByName(ctx context.Context, organizationID string, name string, excludeAccountID string) (bool, error) {
	return s.organizationRepo.ExistsMemberByName(ctx, organizationID, name, excludeAccountID)
}

func (s *organizationService) GetPendingJoinRequest(ctx context.Context, organizationID, accountID string) (*model.OrganizationJoinRequest, error) {
	return s.organizationRepo.GetPendingJoinRequest(ctx, organizationID, accountID)
}

func (s *organizationService) GetOrganizationByWorkspaceID(ctx context.Context, workspaceID string) (*model.Organization, error) {
	organizationID, err := s.organizationRepo.GetOrganizationIDByWorkspaceID(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	if organizationID == "" {
		return nil, nil
	}
	return s.organizationRepo.GetByID(ctx, organizationID)
}

// GetUserOrganizationRole retrieves the role of a user in an organization
func (s *organizationService) GetUserOrganizationRole(ctx context.Context, organizationID, accountID string) (model.OrganizationRole, error) {
	db := s.organizationRepo.GetDB()

	var organizationRole model.OrganizationRole
	err := db.WithContext(ctx).
		Table("members").
		Select("role").
		Where("organization_id = ? AND account_id = ?", organizationID, accountID).
		Scan(&organizationRole).Error

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", fmt.Errorf("failed to get organization role: %w", err)
	}

	if organizationRole != "" {
		return organizationRole, nil
	}

	// Fallback: derive from workspace roles (for users who only exist in department workspaces).
	tenantRoles, err := s.getUserWorkspaceRolesInOrganization(ctx, organizationID, accountID)
	if err != nil {
		return model.OrganizationRoleNormal, nil
	}

	if len(tenantRoles) == 0 {
		return model.OrganizationRoleNormal, nil
	}

	return deriveOrganizationRoleFromWorkspaceRoles(tenantRoles), nil
}

func (s *organizationService) CheckOrganizationNameExists(ctx context.Context, name string) (bool, error) {
	return s.organizationRepo.ExistsByName(ctx, name)
}

// CreateOrganizationWithWorkspace creates an organization
func (s *organizationService) CreateOrganizationWithWorkspace(ctx context.Context, req *shared_dto.CreateOrganizationWithWorkspaceRequest) (*model.Organization, error) {
	// Start database transaction
	db := s.organizationRepo.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}

	var committed bool
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
		if !committed {
			tx.Rollback()
		}
	}()

	currentAccount, err := s.accountService.GetAccountByID(ctx, req.CreatedBy)
	if err != nil {
		return nil, fmt.Errorf("failed to get current account for configuration: %w", err)
	}

	organization := &model.Organization{
		ID:        uuid.New().String(),
		Name:      req.Name,
		ShortName: req.ShortName,
		Status:    model.OrganizationStatusActive,
		CreatedAt: time.Now(),
	}

	if err := s.organizationRepo.CreateWithTx(ctx, tx, organization); err != nil {
		return nil, fmt.Errorf("failed to create organization: %w", err)
	}

	join := &model.OrganizationMember{
		OrganizationID: organization.ID,
		AccountID:      req.CreatedBy,
		Role:           model.OrganizationRoleOwner,
		Name:           &currentAccount.Name,
		CreatedAt:      time.Now(),
		UpdatedAt:      time.Now(),
	}

	if err := s.organizationRepo.CreateAccountJoinWithTx(ctx, tx, join); err != nil {
		return nil, fmt.Errorf("failed to add creator as owner: %w", err)
	}

	// Commit transaction
	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	committed = true
	s.bootstrapOfficialRoute(ctx, organization.ID)

	// Sync to Console (async, non-blocking)
	if s.consoleProvider != nil && s.consoleProvider.IsAvailable() {
		_ = s.consoleProvider.RegisterOrganization(ctx, &console.RegisterOrganizationRequest{
			OrganizationID: organization.ID,
			Name:           organization.Name,
			OwnerEmail:     currentAccount.Email,
			CreatedAt:      organization.CreatedAt,
		})
	}

	return organization, nil
}

// ListUserOrganizations retrieves all organizations a user has access to
func (s *organizationService) ListUserOrganizations(ctx context.Context, page, limit int, status string, accountID string) (*shared_dto.OrganizationPaginationResponse, error) {
	var organizations []*model.Organization
	var total int64

	organizations, total, err := s.organizationRepo.ListUserOrganizationsForAccount(ctx, page, limit, status, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to list user organizations: %w", err)
	}

	var organizationsWithRole []*shared_dto.OrganizationWithRoleResponse
	for _, organization := range organizations {
		role, err := s.GetUserOrganizationRole(ctx, organization.ID, accountID)
		if err != nil {
			role = model.OrganizationRoleNormal
		}

		organizationWithRole := &shared_dto.OrganizationWithRoleResponse{
			ID:               organization.ID,
			Name:             organization.Name,
			ShortName:        organization.ShortName,
			Status:           organization.Status,
			CreatedAt:        organization.CreatedAt.Unix(),
			OrganizationRole: role,
		}
		organizationsWithRole = append(organizationsWithRole, organizationWithRole)
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.OrganizationPaginationResponse{
		Data:    organizationsWithRole,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetOrganizationWorkspacesList(ctx context.Context, organizationID string) ([]*model.Workspace, error) {
	_, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("organization not found: %w", err)
	}

	var tenants []*model.Workspace
	db := s.organizationRepo.GetDB()
	if err := db.Where("organization_id = ?", organizationID).Find(&tenants).Error; err != nil {
		return nil, fmt.Errorf("failed to get organization tenants: %w", err)
	}

	return tenants, nil
}

func (s *organizationService) CheckWorkspaceNameExistsInOrganization(ctx context.Context, organizationID, workspaceName string) (bool, error) {
	db := s.organizationRepo.GetDB()
	var count int64
	err := db.Model(&model.Workspace{}).
		Where("organization_id = ? AND name = ?", organizationID, workspaceName).
		Count(&count).Error
	return count > 0, err
}

func (s *organizationService) GetUnjoinedWorkspacesForUser(ctx context.Context, organizationID, userID string, page, limit int) (*shared_dto.WorkspacePaginationResponse, error) {
	offset := (page - 1) * limit

	db := s.organizationRepo.GetDB()

	joinedTenantIDsSubquery := db.WithContext(ctx).Table("workspace_members").
		Select("workspace_id").
		Where("account_id = ?", userID)

	organizationWorkspacesSubquery := db.WithContext(ctx).Table("workspaces").
		Select("id").
		Joins("JOIN organizations ON organizations.id = workspaces.organization_id").
		Where("workspaces.organization_id = ? AND organizations.status = ?", organizationID, model.OrganizationStatusActive)

	var tenants []*model.Workspace
	var total int64

	err := db.WithContext(ctx).Model(&model.Workspace{}).
		Where("id IN (?) AND id NOT IN (?) AND status = ?",
			organizationWorkspacesSubquery,
			joinedTenantIDsSubquery,
			model.WorkspaceStatusNormal).
		Count(&total).Error
	if err != nil {
		return nil, err
	}

	err = db.WithContext(ctx).
		Where("id IN (?) AND id NOT IN (?) AND status = ?",
			organizationWorkspacesSubquery,
			joinedTenantIDsSubquery,
			model.WorkspaceStatusNormal).
		Order("created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tenants).Error
	if err != nil {
		return nil, err
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.WorkspacePaginationResponse{
		Data:    tenants,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetUserOrganizationsByAccount(ctx context.Context, accountID string, page, limit int) (*shared_dto.OrganizationPaginationResponse, error) {
	offset := (page - 1) * limit

	db := s.organizationRepo.GetDB()

	joinedTenantIDsSubquery := db.WithContext(ctx).Table("workspace_members").
		Select("workspace_id").
		Where("account_id = ?", accountID)

	var organizations []*model.Organization
	var total int64

	err := db.WithContext(ctx).Model(&model.Organization{}).
		Joins("JOIN workspaces ON organizations.id = workspaces.organization_id").
		Where("workspaces.id IN (?)", joinedTenantIDsSubquery).
		Group("organizations.id").
		Count(&total).Error
	if err != nil {
		return nil, err
	}

	err = db.WithContext(ctx).
		Joins("JOIN workspaces ON organizations.id = workspaces.organization_id").
		Where("workspaces.id IN (?)", joinedTenantIDsSubquery).
		Group("organizations.id").
		Order("organizations.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&organizations).Error
	if err != nil {
		return nil, err
	}

	var organizationsWithRole []*shared_dto.OrganizationWithRoleResponse
	for _, organization := range organizations {
		role, err := s.GetUserOrganizationRole(ctx, organization.ID, accountID)
		if err != nil {
			role = model.OrganizationRoleNormal
		}

		organizationWithRole := &shared_dto.OrganizationWithRoleResponse{
			ID:               organization.ID,
			Name:             organization.Name,
			ShortName:        organization.ShortName,
			Status:           organization.Status,
			CreatedAt:        organization.CreatedAt.Unix(),
			OrganizationRole: role,
		}
		organizationsWithRole = append(organizationsWithRole, organizationWithRole)
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.OrganizationPaginationResponse{
		Data:    organizationsWithRole,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetUserWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*shared_dto.WorkspacePaginationResponse, error) {
	offset := (page - 1) * limit

	db := s.organizationRepo.GetDB()

	organizationWorkspacesSubquery := db.WithContext(ctx).Table("workspaces").
		Select("id").
		Joins("JOIN organizations ON organizations.id = workspaces.organization_id").
		Where("workspaces.organization_id = ? AND organizations.status = ?", organizationID, model.OrganizationStatusActive)

	var tenants []*model.Workspace
	var total int64

	err := db.WithContext(ctx).Model(&model.Workspace{}).
		Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
		Where("workspaces.id IN (?) AND workspace_members.account_id = ? AND workspaces.status = ?",
			organizationWorkspacesSubquery,
			accountID,
			model.WorkspaceStatusNormal).
		Count(&total).Error
	if err != nil {
		return nil, err
	}

	err = db.WithContext(ctx).
		Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
		Where("workspaces.id IN (?) AND workspace_members.account_id = ? AND workspaces.status = ?",
			organizationWorkspacesSubquery,
			accountID,
			model.WorkspaceStatusNormal).
		Order("workspaces.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&tenants).Error
	if err != nil {
		return nil, err
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.WorkspacePaginationResponse{
		Data:    tenants,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetUserWorkspacesRolesInOrganization(ctx context.Context, organizationID, accountID string) ([]*shared_dto.WorkspaceRoleResponse, error) {
	db := s.organizationRepo.GetDB()

	organizationWorkspacesSubquery := db.WithContext(ctx).Table("workspaces").
		Select("id").
		Joins("JOIN organizations ON organizations.id = workspaces.organization_id").
		Where("workspaces.organization_id = ? AND organizations.status = ?", organizationID, model.OrganizationStatusActive)

	type TenantJoinResult struct {
		TenantID    string `json:"tenant_id"`
		TenantName  string `json:"tenant_name"`
		Role        string `json:"role"`
		Position    string `json:"position"`
		Permissions string `json:"permissions"`
	}

	var results []TenantJoinResult
	err := db.WithContext(ctx).Table("workspaces").
		Select("workspaces.id as tenant_id, workspaces.name as tenant_name, workspace_members.role as role, "+
			"COALESCE(workspace_members.extensions->>'position', '') as position, "+
			"COALESCE(workspace_members.extensions->>'permissions', '') as permissions").
		Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
		Where("workspaces.id IN (?) AND workspace_members.account_id = ? AND workspaces.status = ?",
			organizationWorkspacesSubquery,
			accountID,
			model.WorkspaceStatusNormal).
		Order("workspaces.created_at DESC").
		Find(&results).Error
	if err != nil {
		return nil, err
	}

	responses := make([]*shared_dto.WorkspaceRoleResponse, 0, len(results))
	for _, result := range results {
		var permissions []string
		if result.Permissions != "" && result.Permissions != "null" {
			trimmed := strings.TrimSpace(result.Permissions)

			if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
				if trimmed == "{}" {
					permissions = []string{}
				} else {
					var permObj map[string]interface{}
					if err := json.Unmarshal([]byte(trimmed), &permObj); err == nil {
						permissions = []string{}
					} else {
						permissions = []string{}
					}
				}
			} else {
				if err := json.Unmarshal([]byte(trimmed), &permissions); err != nil {
					if trimmed != "" {
						parts := strings.Split(trimmed, ",")
						permissions = make([]string, 0, len(parts))
						for _, part := range parts {
							if cleaned := strings.TrimSpace(part); cleaned != "" {
								permissions = append(permissions, cleaned)
							}
						}
					} else {
						permissions = []string{}
					}
				}
			}
		} else {
			permissions = []string{}
		}

		responses = append(responses, &shared_dto.WorkspaceRoleResponse{
			WorkspaceID:   result.TenantID,
			WorkspaceName: result.TenantName,
			Role:          result.Role,
			Position:      result.Position,
			Permissions:   permissions,
		})
	}

	return responses, nil
}

func (s *organizationService) CheckAnyManagedWorkspacePermission(ctx context.Context, organizationID, accountID string) (bool, error) {
	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Errorf("organization not found")
		}
		return false, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return false, fmt.Errorf("organization not found")
	}

	organizationRole, err := s.GetUserOrganizationRole(ctx, organizationID, accountID)
	if err == nil && (organizationRole == model.OrganizationRoleOwner || organizationRole == model.OrganizationRoleAdmin) {
		return true, nil
	}

	managedWorkspaceCount, err := s.getManagedWorkspaceCountInOrganization(ctx, organizationID, accountID)
	if err != nil {
		return false, fmt.Errorf("failed to check managed workspaces: %w", err)
	}

	return managedWorkspaceCount > 0, nil
}

func (s *organizationService) getManagedWorkspaceCountInOrganization(ctx context.Context, organizationID, accountID string) (int64, error) {
	db := s.organizationRepo.GetDB()

	var workspaceIDs []string
	err := db.WithContext(ctx).Table("workspaces").
		Where("organization_id = ?", organizationID).
		Pluck("id", &workspaceIDs).Error
	if err != nil {
		return 0, fmt.Errorf("failed to get organization workspace IDs: %w", err)
	}

	if len(workspaceIDs) == 0 {
		return 0, nil
	}

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)
	if isOrganizationAdmin {
		return int64(len(workspaceIDs)), nil
	}

	var count int64
	err = db.WithContext(ctx).Table("workspace_members").
		Where("workspace_members.workspace_id IN (?) AND workspace_members.account_id = ?", workspaceIDs, accountID).
		Where("workspace_members.role IN (?)",
			[]string{string(model.WorkspaceRoleOwner), string(model.WorkspaceRoleAdmin)}).
		Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count managed workspaces: %w", err)
	}

	return count, nil
}

func (s *organizationService) GetShadowWorkspaceByID(ctx context.Context, organizationID string) (*model.Workspace, error) {
	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return nil, fmt.Errorf("organization not found")
	}

	shadowWorkspace, err := s.workspaceRepo.GetByID(ctx, organization.ID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to get shadow workspace: %w", err)
	}

	if shadowWorkspace == nil {
		shadowWorkspace = &model.Workspace{
			ID:        organization.ID,
			Name:      organization.Name,
			Status:    model.WorkspaceStatusNormal,
			Plan:      "basic",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}

		if err := s.workspaceRepo.Create(ctx, shadowWorkspace); err != nil {
			if isDuplicateShadowWorkspaceCreateError(err) {
				shadowWorkspace, retryErr := s.workspaceRepo.GetByID(ctx, organization.ID)
				if retryErr == nil && shadowWorkspace != nil {
					return shadowWorkspace, nil
				}
				if retryErr != nil {
					return nil, fmt.Errorf("failed to get shadow workspace after duplicate create: %w", retryErr)
				}
			}
			return nil, fmt.Errorf("failed to create shadow workspace: %w", err)
		}

	}

	return shadowWorkspace, nil
}

func isDuplicateShadowWorkspaceCreateError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate key") ||
		strings.Contains(message, "violates unique constraint") ||
		strings.Contains(message, "unique constraint failed")
}

func (s *organizationService) IsShadowWorkspace(ctx context.Context, workspaceID string) (bool, error) {
	organization, err := s.organizationRepo.GetByID(ctx, workspaceID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check organization: %w", err)
	}

	return organization != nil, nil
}

func (s *organizationService) GetByID(ctx context.Context, organizationID string) (*model.Organization, error) {
	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	return organization, nil
}

func (s *organizationService) GetFirstOwnedOrganization(ctx context.Context, accountID string) (*model.Organization, error) {
	return s.organizationRepo.GetFirstOwnedOrganization(ctx, accountID)
}

func (s *organizationService) GetFirstJoinedOrganization(ctx context.Context, accountID string) (*model.Organization, error) {
	return s.organizationRepo.GetFirstJoinedOrganization(ctx, accountID)
}

func (s *organizationService) GetCurrentOrganization(ctx context.Context, accountID string) (*shared_dto.CurrentOrganizationResponse, error) {
	// Try to get current organization ID from account context
	orgID, err := s.accountService.EnsureCurrentOrganizationID(ctx, accountID)

	// If failed to get current organization (likely means no organization exists for this user),
	// only cloud mode may create a default one automatically.
	if err != nil {
		if !strings.EqualFold(strings.TrimSpace(config.Current().Platform.Edition), "CLOUD") {
			return nil, fmt.Errorf("failed to get current organization: %w", err)
		}

		account, err := s.accountService.GetAccountByID(ctx, accountID)
		if err != nil {
			return nil, fmt.Errorf("failed to get account: %w", err)
		}

		organizationName := fmt.Sprintf("%s's organization %s", account.Name, uuid.New().String()[:8])
		organization, err := s.CreateOrganizationWithWorkspace(ctx, &shared_dto.CreateOrganizationWithWorkspaceRequest{
			Name:      organizationName,
			CreatedBy: accountID,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create organization: %w", err)
		}

		// Create tenant member join for shadow tenant
		// Not needed as CreateWorkspaceMember, current user is organization admin, has permission for all team members.
		// Kept commented out for potential future use if logic changes.
		// if err := s.tenantService.CreateWorkspaceMember(ctx, organization.ID, accountID, "owner"); err != nil {
		// 	return nil, fmt.Errorf("failed to create tenant member: %w", err)
		// }

		// Set as current tenant
		if _, err := s.accountService.UpdateAccountContext(ctx, accountID, &organization.ID, nil); err != nil {
			return nil, fmt.Errorf("failed to update account context: %w", err)
		}

		// Create default workspace (non-shadow tenant)
		defaultTenant, err := s.workspaceManagementService.CreateWorkspace(ctx, fmt.Sprintf("%s's Workspace", account.Name), true)
		if err != nil {
			return nil, fmt.Errorf("failed to create default workspace: %w", err)
		}

		// Add default workspace to organization.
		if err := s.AddWorkspace(ctx, &shared_dto.AddWorkspaceToOrganizationRequest{
			OrganizationID: organization.ID,
			WorkspaceID:    defaultTenant.ID,
		}); err != nil {
			return nil, fmt.Errorf("failed to add workspace to organization: %w", err)
		}

		if err := workspacebootstrap.EnsureOwnerWorkspaceReady(ctx, s.workspaceManagementService, accountID, defaultTenant.ID); err != nil {
			return nil, fmt.Errorf("failed to initialize default workspace state: %w", err)
		}

		// Return the newly created organization
		return &shared_dto.CurrentOrganizationResponse{
			ID:               organization.ID,
			Name:             organization.Name,
			ShortName:        organization.ShortName,
			Status:           organization.Status,
			CreatedAt:        organization.CreatedAt.Unix(),
			OrganizationRole: model.OrganizationRoleOwner,
		}, nil
	}

	// Organization found, fetch details
	organization, err := s.organizationRepo.GetByID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization by ID: %w", err)
	}
	if organization == nil {
		return nil, fmt.Errorf("organization not found for ID: %s", orgID)
	}

	groupRole, err := s.GetUserOrganizationRole(ctx, organization.ID, accountID)
	if err != nil {
		groupRole = model.OrganizationRoleNormal
	}

	response := &shared_dto.CurrentOrganizationResponse{
		ID:               organization.ID,
		Name:             organization.Name,
		ShortName:        organization.ShortName,
		Status:           organization.Status,
		CreatedAt:        organization.CreatedAt.Unix(),
		OrganizationRole: groupRole,
	}

	return response, nil
}

func (s *organizationService) GetCurrentOrganizationDetail(ctx context.Context, accountID string) (*shared_dto.CurrentOrganizationDetailResponse, error) {
	orgID, err := s.accountService.EnsureCurrentOrganizationID(ctx, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get current organization: %w", err)
	}

	organization, err := s.organizationRepo.GetByID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization by ID: %w", err)
	}
	if organization == nil {
		return nil, fmt.Errorf("organization not found")
	}

	shadowWorkspace, err := s.workspaceRepo.GetByID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get shadow workspace: %w", err)
	}

	groupRole, err := s.GetUserOrganizationRole(ctx, organization.ID, accountID)
	if err != nil {
		groupRole = model.OrganizationRoleNormal
	}

	organizationResponse := &shared_dto.CurrentOrganizationResponse{
		ID:               organization.ID,
		Name:             organization.Name,
		ShortName:        organization.ShortName,
		Status:           organization.Status,
		CreatedAt:        organization.CreatedAt.Unix(),
		OrganizationRole: groupRole,
	}

	response := &shared_dto.CurrentOrganizationDetailResponse{
		EnterpriseGroup: organizationResponse,
		ShadowTenant:    shadowWorkspace,
	}

	return response, nil
}

func (s *organizationService) GetManagedWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*shared_dto.WorkspacePaginationResponse, error) {
	offset := (page - 1) * limit

	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return nil, fmt.Errorf("organization not found")
	}

	db := s.organizationRepo.GetDB()

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)

	var tenants []*model.Workspace
	var total int64

	if isOrganizationAdmin {
		// Organization owner/admin can see all workspaces in the organization.
		err = db.WithContext(ctx).Model(&model.Workspace{}).
			Where("organization_id = ?", organizationID).
			Count(&total).Error
		if err != nil {
			return nil, fmt.Errorf("failed to count admin tenants: %w", err)
		}

		err = db.WithContext(ctx).
			Where("organization_id = ?", organizationID).
			Order("created_at DESC").
			Offset(offset).
			Limit(limit).
			Find(&tenants).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get admin tenants: %w", err)
		}
	} else {
		// Non-admin users can only see tenants where they have owner/admin role
		err = db.WithContext(ctx).Table("workspaces").
			Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
			Where("workspaces.organization_id = ? AND workspace_members.account_id = ? AND workspace_members.role IN (?)",
				organizationID, accountID, []string{string(model.WorkspaceRoleOwner), string(model.WorkspaceRoleAdmin)}).
			Group("workspaces.id").
			Count(&total).Error
		if err != nil {
			return nil, fmt.Errorf("failed to count user managed tenants: %w", err)
		}

		err = db.WithContext(ctx).Table("workspaces").
			Select("DISTINCT workspaces.*").
			Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
			Where("workspaces.organization_id = ? AND workspace_members.account_id = ? AND workspace_members.role IN (?)",
				organizationID, accountID, []string{string(model.WorkspaceRoleOwner), string(model.WorkspaceRoleAdmin)}).
			Order("workspaces.created_at DESC").
			Offset(offset).
			Limit(limit).
			Find(&tenants).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get user managed tenants: %w", err)
		}
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.WorkspacePaginationResponse{
		Data:    tenants,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetManagedAppWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*shared_dto.WorkspacePaginationResponse, error) {
	offset := (page - 1) * limit

	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return nil, fmt.Errorf("organization not found")
	}

	db := s.organizationRepo.GetDB()

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)

	var tenants []*model.Workspace
	var total int64

	if isOrganizationAdmin {
		err = db.WithContext(ctx).Table("workspaces").
			Joins("JOIN organizations ON organizations.id = workspaces.organization_id").
			Where("workspaces.organization_id = ? AND organizations.status = ?",
				organizationID, model.OrganizationStatusActive).
			Count(&total).Error
		if err != nil {
			return nil, fmt.Errorf("failed to count admin tenants: %w", err)
		}

		err = db.WithContext(ctx).Table("workspaces").
			Joins("JOIN organizations ON organizations.id = workspaces.organization_id").
			Where("workspaces.organization_id = ? AND organizations.status = ?",
				organizationID, model.OrganizationStatusActive).
			Order("workspaces.created_at DESC").
			Offset(offset).
			Limit(limit).
			Find(&tenants).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get admin tenants: %w", err)
		}
	} else {

		ownerAdminQuery := `
			SELECT DISTINCT workspaces.* FROM workspaces
			JOIN workspace_members ON workspaces.id = workspace_members.workspace_id
			WHERE workspaces.organization_id = ?
			  AND workspace_members.account_id = ?
			  AND workspace_members.role IN (?, ?)
		`

		normalWithPermissionQuery := `
			SELECT DISTINCT workspaces.* FROM workspaces
			JOIN workspace_members ON workspaces.id = workspace_members.workspace_id
			LEFT JOIN tenant_account_extensions ON workspace_members.id = tenant_account_extensions.tenant_account_join_id
			WHERE workspaces.organization_id = ?
			  AND workspace_members.account_id = ?
			  AND workspace_members.role = ?
			  AND (tenant_account_extensions.permissions IS NOT NULL
			       AND JSON_CONTAINS(tenant_account_extensions.permissions, ?))
		`

		combinedQuery := fmt.Sprintf(`
			SELECT * FROM (
				(%s) UNION (%s)
			) AS combined_tenants
			ORDER BY created_at DESC
		`, ownerAdminQuery, normalWithPermissionQuery)

		countQuery := fmt.Sprintf(`
			SELECT COUNT(*) FROM (
				(%s) UNION (%s)
			) AS combined_tenants
		`, ownerAdminQuery, normalWithPermissionQuery)

		err = db.WithContext(ctx).Raw(countQuery,
			organizationID, accountID, string(model.WorkspaceRoleOwner), string(model.WorkspaceRoleAdmin),
			organizationID, accountID, string(model.WorkspaceRoleNormal)).
			Count(&total).Error
		if err != nil {
			return nil, fmt.Errorf("failed to count user app tenants: %w", err)
		}

		paginatedQuery := combinedQuery + " LIMIT ? OFFSET ?"
		err = db.WithContext(ctx).Raw(paginatedQuery,
			organizationID, accountID, string(model.WorkspaceRoleOwner), string(model.WorkspaceRoleAdmin),
			organizationID, accountID, string(model.WorkspaceRoleNormal),
			limit, offset).
			Scan(&tenants).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get user app tenants: %w", err)
		}
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.WorkspacePaginationResponse{
		Data:    tenants,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetManagedDatasetWorkspacesInOrganization(ctx context.Context, organizationID, accountID string, page, limit int) (*shared_dto.WorkspacePaginationResponse, error) {
	offset := (page - 1) * limit

	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return nil, fmt.Errorf("organization not found")
	}

	db := s.organizationRepo.GetDB()

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)

	var tenants []*model.Workspace
	var total int64

	if isOrganizationAdmin {
		err = db.WithContext(ctx).Table("workspaces").
			Joins("JOIN organizations ON organizations.id = workspaces.organization_id").
			Where("workspaces.organization_id = ? AND organizations.status = ?",
				organizationID, model.OrganizationStatusActive).
			Count(&total).Error
		if err != nil {
			return nil, fmt.Errorf("failed to count admin tenants: %w", err)
		}

		err = db.WithContext(ctx).Table("workspaces").
			Joins("JOIN organizations ON organizations.id = workspaces.organization_id").
			Where("workspaces.organization_id = ? AND organizations.status = ?",
				organizationID, model.OrganizationStatusActive).
			Order("workspaces.created_at DESC").
			Offset(offset).
			Limit(limit).
			Find(&tenants).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get admin tenants: %w", err)
		}
	} else {

		ownerAdminQuery := `
			SELECT DISTINCT workspaces.* FROM workspaces
			JOIN workspace_members ON workspaces.id = workspace_members.workspace_id
			WHERE workspaces.organization_id = ?
			  AND workspace_members.account_id = ?
			  AND workspace_members.role IN (?, ?)
		`

		normalWithPermissionQuery := `
			SELECT DISTINCT workspaces.* FROM workspaces
			JOIN workspace_members ON workspaces.id = workspace_members.workspace_id
			LEFT JOIN tenant_account_extensions ON workspace_members.id = tenant_account_extensions.tenant_account_join_id
			WHERE workspaces.organization_id = ?
			  AND workspace_members.account_id = ?
			  AND workspace_members.role = ?
			  AND (tenant_account_extensions.permissions IS NOT NULL
			       AND JSON_CONTAINS(tenant_account_extensions.permissions, ?))
		`

		combinedQuery := fmt.Sprintf(`
			SELECT * FROM (
				(%s) UNION (%s)
			) AS combined_tenants
			ORDER BY created_at DESC
		`, ownerAdminQuery, normalWithPermissionQuery)

		countQuery := fmt.Sprintf(`
			SELECT COUNT(*) FROM (
				(%s) UNION (%s)
			) AS combined_tenants
		`, ownerAdminQuery, normalWithPermissionQuery)

		err = db.WithContext(ctx).Raw(countQuery,
			organizationID, accountID, string(model.WorkspaceRoleOwner), string(model.WorkspaceRoleAdmin),
			organizationID, accountID, string(model.WorkspaceRoleNormal)).
			Count(&total).Error
		if err != nil {
			return nil, fmt.Errorf("failed to count user dataset tenants: %w", err)
		}

		paginatedQuery := combinedQuery + " LIMIT ? OFFSET ?"
		err = db.WithContext(ctx).Raw(paginatedQuery,
			organizationID, accountID, string(model.WorkspaceRoleOwner), string(model.WorkspaceRoleAdmin),
			organizationID, accountID, string(model.WorkspaceRoleNormal),
			limit, offset).
			Scan(&tenants).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get user dataset tenants: %w", err)
		}
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.WorkspacePaginationResponse{
		Data:    tenants,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetOrganizationMembersPaginated(ctx context.Context, organizationID string, page, limit int, keyword string) (*shared_dto.OrganizationMemberPaginationResponse, error) {
	offset := (page - 1) * limit

	db := s.organizationRepo.GetDB()

	var accounts []struct {
		Account          auth_model.Account     `gorm:"embedded"`
		OrganizationRole model.OrganizationRole `gorm:"column:organization_role"`
		MemberName       *string                `gorm:"column:member_name"`
	}
	var total int64

	baseQuery := db.WithContext(ctx).Table("members").
		Select("accounts.*, members.role as organization_role, members.name as member_name").
		Joins("JOIN accounts ON members.account_id = accounts.id").
		Where("members.organization_id = ?", organizationID)

	if keyword != "" {
		searchPattern := "%" + keyword + "%"
		baseQuery = baseQuery.Where(
			"accounts.name ILIKE ? OR accounts.email ILIKE ? OR members.name ILIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	err := baseQuery.Count(&total).Error
	if err != nil {
		return nil, fmt.Errorf("failed to count organization members: %w", err)
	}

	err = baseQuery.Order("members.created_at DESC").
		Offset(offset).
		Limit(limit).
		Find(&accounts).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get organization members: %w", err)
	}

	responses := make([]*shared_dto.OrganizationMemberWithExtensionResponse, 0, len(accounts))
	for _, accountData := range accounts {
		account := accountData.Account

		var lastLoginAt *int64
		if account.LastLoginAt != nil {
			timestamp := account.LastLoginAt.Unix()
			lastLoginAt = &timestamp
		}

		var lastActiveAt *int64
		if account.LastActiveAt != nil {
			timestamp := account.LastActiveAt.Unix()
			lastActiveAt = &timestamp
		}

		var accountRole *shared_dto.AccountSystemRole

		extension := map[string]interface{}{}

		var avatar, avatarURL string
		if account.Avatar != nil {
			avatar = *account.Avatar
			avatarURL = *account.Avatar
		}

		// Use member name (nickname) if available, otherwise fallback to account name
		displayName := account.Name
		if accountData.MemberName != nil && *accountData.MemberName != "" {
			displayName = *accountData.MemberName
		}

		response := &shared_dto.OrganizationMemberWithExtensionResponse{
			ID:          account.ID,
			Name:        displayName,
			AccountName: account.Name,
			MemberName:  accountData.MemberName,

			Avatar:           avatar,
			AvatarURL:        avatarURL,
			Email:            account.Email,
			LastLoginAt:      lastLoginAt,
			LastActiveAt:     lastActiveAt,
			CreatedAt:        account.CreatedAt.Unix(),
			Status:           string(account.Status),
			OrganizationRole: accountData.OrganizationRole,
			AccountRole:      accountRole,
			Extension:        extension,
		}
		responses = append(responses, response)
	}

	hasMore := int64(page*limit) < total

	return &shared_dto.OrganizationMemberPaginationResponse{
		Data:    responses,
		Page:    page,
		Limit:   limit,
		Total:   total,
		HasMore: hasMore,
	}, nil
}

func (s *organizationService) GetOrganizationMemberByAccountID(ctx context.Context, organizationID, accountID string) (*shared_dto.OrganizationMemberWithExtensionResponse, error) {
	db := s.organizationRepo.GetDB()

	var accountData struct {
		Account          auth_model.Account     `gorm:"embedded"`
		OrganizationRole model.OrganizationRole `gorm:"column:organization_role"`
		MemberName       *string                `gorm:"column:member_name"`
	}

	err := db.WithContext(ctx).Table("members").
		Select("accounts.*, members.role as organization_role, members.name as member_name").
		Joins("JOIN accounts ON members.account_id = accounts.id").
		Where("members.organization_id = ?", organizationID).
		Where("members.account_id = ?", accountID).
		Take(&accountData).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("member not found")
		}
		return nil, fmt.Errorf("failed to get organization member: %w", err)
	}

	account := accountData.Account

	var lastLoginAt *int64
	if account.LastLoginAt != nil {
		timestamp := account.LastLoginAt.Unix()
		lastLoginAt = &timestamp
	}

	var lastActiveAt *int64
	if account.LastActiveAt != nil {
		timestamp := account.LastActiveAt.Unix()
		lastActiveAt = &timestamp
	}

	var accountRole *shared_dto.AccountSystemRole

	extension := map[string]interface{}{}
	if account.Extensions != nil {
		extension = account.Extensions
	}

	var avatar, avatarURL string
	if account.Avatar != nil {
		avatar = *account.Avatar
		avatarURL = *account.Avatar
	}

	displayName := account.Name
	if accountData.MemberName != nil && *accountData.MemberName != "" {
		displayName = *accountData.MemberName
	}

	response := &shared_dto.OrganizationMemberWithExtensionResponse{
		ID:               account.ID,
		Name:             displayName,
		AccountName:      account.Name,
		MemberName:       accountData.MemberName,
		Avatar:           avatar,
		AvatarURL:        avatarURL,
		Email:            account.Email,
		LastLoginAt:      lastLoginAt,
		LastActiveAt:     lastActiveAt,
		CreatedAt:        account.CreatedAt.Unix(),
		Status:           string(account.Status),
		OrganizationRole: accountData.OrganizationRole,
		AccountRole:      accountRole,
		Extension:        extension,
	}

	return response, nil
}

func (s *organizationService) CheckAnyWorkspaceCreateAppPermission(ctx context.Context, organizationID, accountID string) (bool, error) {
	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Errorf("organization not found")
		}
		return false, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return false, fmt.Errorf("organization not found")
	}

	organizationRole, err := s.GetUserOrganizationRole(ctx, organizationID, accountID)
	if err == nil && (organizationRole == model.OrganizationRoleOwner || organizationRole == model.OrganizationRoleAdmin) {
		return true, nil
	}

	managedWorkspaceCount, err := s.getManagedWorkspaceCountInOrganization(ctx, organizationID, accountID)
	if err != nil {
		return false, fmt.Errorf("failed to check managed workspaces: %w", err)
	}

	return managedWorkspaceCount > 0, nil
}

func (s *organizationService) CheckAnyWorkspaceCreateDatasetPermission(ctx context.Context, organizationID, accountID string) (bool, error) {

	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, fmt.Errorf("organization not found")
		}
		return false, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return false, fmt.Errorf("organization not found")
	}

	organizationWorkspaces, err := s.GetUserAllWorkspacesInOrganization(ctx, organizationID, accountID)
	if err != nil {
		return false, fmt.Errorf("failed to get user workspaces in organization: %w", err)
	}

	if len(organizationWorkspaces) == 0 {
		return false, nil
	}

	workspaceIDs := make([]string, len(organizationWorkspaces))
	for i, workspace := range organizationWorkspaces {
		workspaceIDs[i] = workspace.ID
	}

	db := s.organizationRepo.GetDB()

	type WorkspacePermissionResult struct {
		WorkspaceID string `json:"workspace_id"`
		Role        string `json:"role"`
		// Permissions *string `json:"permissions"` // Permissions are now handled via Role
		RoleID *string `json:"role_id"`
	}

	var results []WorkspacePermissionResult
	err = db.WithContext(ctx).
		Table("workspace_members").
		Select("workspace_members.workspace_id, workspace_members.role, workspace_members.role_id").
		Where("workspace_members.account_id = ? AND workspace_members.workspace_id IN (?)", accountID, workspaceIDs).
		Find(&results).Error

	if err != nil {
		return false, fmt.Errorf("failed to query workspace permissions: %w", err)
	}

	for _, result := range results {
		if result.Role == string(model.WorkspaceRoleOwner) || result.Role == string(model.WorkspaceRoleAdmin) {
			return true, nil
		}

		// Check permission via Role + Permission logic
		var roleID string
		if result.RoleID != nil && *result.RoleID != "" {
			roleID = *result.RoleID
		} else {
			// Map built-in role to role ID
			switch model.WorkspaceMemberRole(result.Role) {
			case model.WorkspaceRoleViewer:
				roleID = model.WorkspaceBuiltinRoleViewerID
			case model.WorkspaceRoleNormal:
				roleID = model.WorkspaceBuiltinRoleMemberID
			default:
				roleID = model.WorkspaceBuiltinRoleMemberID
			}
		}

		// Get role details to check permissions
		roleDetail, err := s.GetWorkspaceRoleDetail(ctx, organizationID, roleID, accountID)
		if err == nil {
			for _, codeStr := range roleDetail.Permissions {
				if codeStr == string(model.WorkspacePermissionKnowledgeBaseManage) {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

func (s *organizationService) CheckWorkspacePermission(ctx context.Context, organizationID, workspaceID, accountID string, permissionCode model.WorkspacePermissionCode) (bool, error) {
	if organizationID == "" {
		return true, nil
	}

	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return false, fmt.Errorf("failed to get organization: %w", err)
	}

	belongs, err := s.workspaceBelongsToOrganization(ctx, organization.ID, workspaceID)
	if err != nil {
		return false, err
	}
	if !belongs {
		return false, nil
	}

	organizationRole, err := s.GetUserOrganizationRole(ctx, organization.ID, accountID)
	if err == nil && (organizationRole == model.OrganizationRoleOwner || organizationRole == model.OrganizationRoleAdmin) {
		return true, nil
	}

	permSet := make(map[model.WorkspacePermissionCode]struct{})

	workspaceJoin, err := s.workspaceManagementService.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("failed to get workspace account join: %w", err)
	}

	if workspaceJoin != nil {
		var roleID string
		// Use RoleID if available (custom role), otherwise map built-in roles
		if workspaceJoin.RoleID != nil && *workspaceJoin.RoleID != "" {
			roleID = *workspaceJoin.RoleID
		} else {
			switch workspaceJoin.Role {
			case model.WorkspaceRoleOwner:
				roleID = model.WorkspaceBuiltinRoleOwnerID
			case model.WorkspaceRoleAdmin:
				roleID = model.WorkspaceBuiltinRoleAdminID
			case model.WorkspaceRoleViewer:
				roleID = model.WorkspaceBuiltinRoleViewerID
			case model.WorkspaceRoleNormal: // Map 'normal' to 'member' role
				roleID = model.WorkspaceBuiltinRoleMemberID
			default:
				// Fallback or handle other legacy roles if needed
				roleID = model.WorkspaceBuiltinRoleMemberID
			}
		}

		roleDetail, err := s.GetWorkspaceRoleDetail(ctx, organization.ID, roleID, accountID)
		if err != nil {
			if !strings.Contains(err.Error(), "role not found") {
				return false, fmt.Errorf("failed to get organization role detail: %w", err)
			}
		} else {
			for _, codeStr := range roleDetail.Permissions {
				permSet[model.WorkspacePermissionCode(codeStr)] = struct{}{}
			}
		}
	}

	_, ok := permSet[permissionCode]
	return ok, nil
}

func (s *organizationService) CheckWorkspaceOrganizationAnyPermission(
	ctx context.Context,
	organizationID, workspaceID, accountID string,
	permissionCodes ...model.WorkspacePermissionCode,
) (bool, error) {
	if len(permissionCodes) == 0 {
		return false, nil
	}

	if organizationID == "" {
		return true, nil
	}

	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return true, nil
		}
		return false, fmt.Errorf("failed to get organization: %w", err)
	}

	belongs, err := s.workspaceBelongsToOrganization(ctx, organization.ID, workspaceID)
	if err != nil {
		return false, err
	}
	if !belongs {
		return false, nil
	}

	organizationRole, err := s.GetUserOrganizationRole(ctx, organization.ID, accountID)
	if err == nil && (organizationRole == model.OrganizationRoleOwner || organizationRole == model.OrganizationRoleAdmin) {
		return true, nil
	}

	permSet := make(map[model.WorkspacePermissionCode]struct{})

	workspaceJoin, err := s.workspaceManagementService.GetByWorkspaceAndMember(ctx, workspaceID, accountID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, fmt.Errorf("failed to get workspace account join: %w", err)
	}

	if workspaceJoin != nil {
		var roleID string
		// Use RoleID if available (custom role), otherwise map built-in roles
		if workspaceJoin.RoleID != nil && *workspaceJoin.RoleID != "" {
			roleID = *workspaceJoin.RoleID
		} else {
			switch workspaceJoin.Role {
			case model.WorkspaceRoleOwner:
				roleID = model.WorkspaceBuiltinRoleOwnerID
			case model.WorkspaceRoleAdmin:
				roleID = model.WorkspaceBuiltinRoleAdminID
			case model.WorkspaceRoleViewer:
				roleID = model.WorkspaceBuiltinRoleViewerID
			case model.WorkspaceRoleNormal: // Map 'normal' to 'member' role
				roleID = model.WorkspaceBuiltinRoleMemberID
			default:
				// Fallback or handle other legacy roles if needed
				roleID = model.WorkspaceBuiltinRoleMemberID
			}
		}

		roleDetail, err := s.GetWorkspaceRoleDetail(ctx, organization.ID, roleID, accountID)
		if err != nil {
			if !strings.Contains(err.Error(), "role not found") {
				return false, fmt.Errorf("failed to get organization role detail: %w", err)
			}
		} else {
			for _, codeStr := range roleDetail.Permissions {
				permSet[model.WorkspacePermissionCode(codeStr)] = struct{}{}
			}
		}
	}

	for _, code := range permissionCodes {
		if _, ok := permSet[code]; ok {
			return true, nil
		}
	}

	return false, nil
}

func (s *organizationService) workspaceBelongsToOrganization(ctx context.Context, organizationID, workspaceID string) (bool, error) {
	if strings.TrimSpace(workspaceID) == "" {
		return false, nil
	}
	if s.workspaceRepo == nil {
		return false, fmt.Errorf("workspace repository is not initialized")
	}

	workspaceOrganizationID, err := s.workspaceRepo.GetWorkspaceOrganizationID(ctx, workspaceID)
	if err != nil {
		return false, fmt.Errorf("failed to get workspace organization: %w", err)
	}

	return workspaceOrganizationID == organizationID, nil
}

func (s *organizationService) GetUserAllWorkspacesInOrganization(ctx context.Context, organizationID, accountID string) ([]*model.Workspace, error) {
	db := s.organizationRepo.GetDB()

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)
	if isOrganizationAdmin {
		var tenants []*model.Workspace
		err := db.WithContext(ctx).Table("workspaces").
			Where("organization_id = ? AND status = 'normal'", organizationID).
			Find(&tenants).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get organization workspaces: %w", err)
		}
		return tenants, nil
	}

	var tenants []*model.Workspace
	err := db.WithContext(ctx).Table("workspaces").
		Joins("JOIN workspace_members ON workspaces.id = workspace_members.workspace_id").
		Where("workspaces.organization_id = ? AND workspace_members.account_id = ? AND workspace_members.role IN (?)",
			organizationID, accountID, []string{string(model.WorkspaceRoleOwner), string(model.WorkspaceRoleAdmin)}).
		Find(&tenants).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get user managed workspaces: %w", err)
	}

	return tenants, nil
}

func (s *organizationService) CheckOrganizationOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	organization, err := s.organizationRepo.GetByID(ctx, organizationID)
	if err != nil {
		return false, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return false, fmt.Errorf("organization not found")
	}

	isOrganizationAdmin := s.checkOrganizationAdmin(ctx, accountID, organizationID)
	if !isOrganizationAdmin {
		return false, nil
	}

	join, err := s.organizationRepo.GetAccountJoin(ctx, organizationID, accountID)
	if err != nil {
		return false, fmt.Errorf("failed to get account join: %w", err)
	}
	if join == nil {
		return false, nil
	}

	return join.Role == model.OrganizationRoleOwner, nil
}

func (s *organizationService) GetOrganizationDatasetsPaginated(ctx context.Context, req *shared_dto.GetOrganizationDatasetsPaginatedRequest) (*shared_dto.DatasetPaginationResponse, error) {
	organization, err := s.organizationRepo.GetByID(ctx, req.OrganizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}
	if organization == nil {
		return nil, fmt.Errorf("organization not found")
	}

	account, err := s.accountService.GetAccountByID(ctx, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user account: %w", err)
	}
	if account == nil {
		return nil, fmt.Errorf("user account not found")
	}

	var workspaceIDs []string
	var datasetAdmin bool

	isSystemAdmin := false
	isOrganizationOwner, err := s.CheckOrganizationOwner(ctx, req.OrganizationID, req.UserID)
	if err != nil {
		return nil, fmt.Errorf("failed to check organization owner: %w", err)
	}

	if isSystemAdmin || isOrganizationOwner {
		workspaces, err := s.GetUserAllWorkspacesInOrganization(ctx, req.OrganizationID, req.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get all workspaces: %w", err)
		}

		for _, workspace := range workspaces {
			workspaceIDs = append(workspaceIDs, workspace.ID)
		}

		workspaceIDs = append(workspaceIDs, req.OrganizationID)
		datasetAdmin = true
	} else {
		workspaces, err := s.GetUserAllWorkspacesInOrganization(ctx, req.OrganizationID, req.UserID)
		if err != nil {
			return nil, fmt.Errorf("failed to get user workspaces: %w", err)
		}

		for _, workspace := range workspaces {
			workspaceIDs = append(workspaceIDs, workspace.ID)
		}
		datasetAdmin = false
	}

	if len(workspaceIDs) == 0 {
		return &shared_dto.DatasetPaginationResponse{
			Data:    []*shared_dto.DatasetSimple{},
			Page:    req.Page,
			PerPage: req.PerPage,
			Total:   0,
			HasMore: false,
		}, nil
	}

	datasetReq := &datasetsvc.GetPaginateDatasetsByTenantIDsRequest{
		TenantIDs:    workspaceIDs,
		Page:         req.Page,
		Limit:        req.PerPage,
		Search:       req.Search,
		DatasetAdmin: datasetAdmin,
		AccountID:    req.UserID,
	}

	datasetPagination, err := s.datasetService.GetPaginateDatasetsByTenantIDs(ctx, datasetReq)
	if err != nil {
		return nil, fmt.Errorf("failed to get datasets from dataset service: %w", err)
	}

	var datasetSimples []*shared_dto.DatasetSimple
	for _, dataset := range datasetPagination.Data {
		datasetSimple := &shared_dto.DatasetSimple{
			ID:                     dataset.ID,
			Name:                   dataset.Name,
			Description:            dataset.Description,
			Provider:               dataset.Provider,
			AppCount:               dataset.AppCount,
			DocumentCount:          dataset.DocumentCount,
			WordCount:              dataset.WordCount,
			CreatedBy:              dataset.CreatedBy,
			CreatedAt:              dataset.CreatedAt.Unix(),
			UpdatedBy:              dataset.UpdatedBy,
			UpdatedAt:              getInt64Pointer(dataset.UpdatedAt.Unix()),
			EmbeddingModel:         dataset.EmbeddingModel,
			EmbeddingModelProvider: dataset.EmbeddingModelProvider,
			EmbeddingAvailable:     true,
			Tags:                   dataset.Tags,
			DocForm:                dataset.DocForm,
			Icon:                   dataset.Icon,
			IconBackground:         dataset.IconBackground,
			Owner:                  dataset.Owner,
		}

		// Get tenant information separately through workspace service
		if dataset.WorkspaceID != "" {
			tenant, err := s.workspaceManagementService.GetWorkspaceByID(ctx, dataset.WorkspaceID)
			if err == nil && tenant != nil {
				datasetSimple.Tenant = map[string]interface{}{
					"id":   tenant.ID,
					"name": tenant.Name,
				}
			}
		}

		// Get owner account information if available
		if dataset.Owner != nil {
			ownerAccount, err := s.accountService.GetAccountByID(ctx, *dataset.Owner)
			if err == nil && ownerAccount != nil {
				datasetSimple.OwnerAccount = map[string]interface{}{
					"id":   ownerAccount.ID,
					"name": ownerAccount.Name,
				}
			}
		}

		datasetSimples = append(datasetSimples, datasetSimple)
	}

	return &shared_dto.DatasetPaginationResponse{
		Data:    datasetSimples,
		Page:    datasetPagination.Page,
		PerPage: datasetPagination.Limit,
		Total:   datasetPagination.Total,
		HasMore: datasetPagination.HasMore,
	}, nil
}

func (s *organizationService) IsOrganizationAdminOrOwner(ctx context.Context, organizationID, accountID string) (bool, error) {
	userRole, err := s.GetUserOrganizationRole(ctx, organizationID, accountID)
	if err != nil {
		return false, nil
	}
	return userRole == model.OrganizationRoleAdmin || userRole == model.OrganizationRoleOwner, nil
}

func (s *organizationService) ListWorkspaceIDsByPermission(
	ctx context.Context,
	organizationID, accountID string,
	permissionCode model.WorkspacePermissionCode,
) ([]string, error) {
	if organizationID == "" || accountID == "" {
		return nil, fmt.Errorf("invalid parameters")
	}

	if _, err := s.organizationRepo.GetByID(ctx, organizationID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("organization not found")
		}
		return nil, fmt.Errorf("failed to get organization: %w", err)
	}

	groupRole, err := s.GetUserOrganizationRole(ctx, organizationID, accountID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization role: %w", err)
	}
	if groupRole == model.OrganizationRoleOwner || groupRole == model.OrganizationRoleAdmin {
		return s.listNormalWorkspaceIDsByOrganization(ctx, organizationID)
	}

	type workspacePermissionRow struct {
		WorkspaceID string
		Role        model.WorkspaceMemberRole
		RoleID      *string
	}

	var rows []workspacePermissionRow
	if err := s.organizationRepo.GetDB().WithContext(ctx).
		Table("workspace_members").
		Select("workspace_members.workspace_id, workspace_members.role, workspace_members.role_id").
		Joins("JOIN workspaces ON workspaces.id = workspace_members.workspace_id").
		Where("workspace_members.account_id = ?", accountID).
		Where("workspaces.organization_id = ?", organizationID).
		Where("workspaces.status = ?", model.WorkspaceStatusNormal).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to query workspace memberships: %w", err)
	}

	customRoleIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.RoleID == nil || *row.RoleID == "" || model.IsBuiltinRole(*row.RoleID) {
			continue
		}
		customRoleIDs = append(customRoleIDs, *row.RoleID)
	}

	rolePermissions, err := s.listActiveRolePermissionsByID(ctx, organizationID, customRoleIDs)
	if err != nil {
		return nil, err
	}

	workspaceIDs := make([]string, 0, len(rows))
	seen := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if !workspaceRoleAllowsPermission(row.Role, row.RoleID, rolePermissions, permissionCode) {
			continue
		}
		if _, exists := seen[row.WorkspaceID]; exists {
			continue
		}
		seen[row.WorkspaceID] = struct{}{}
		workspaceIDs = append(workspaceIDs, row.WorkspaceID)
	}

	slices.Sort(workspaceIDs)
	return workspaceIDs, nil
}

func (s *organizationService) listNormalWorkspaceIDsByOrganization(ctx context.Context, organizationID string) ([]string, error) {
	var workspaceIDs []string
	if err := s.organizationRepo.GetDB().WithContext(ctx).
		Table("workspaces").
		Where("organization_id = ? AND status = ?", organizationID, model.WorkspaceStatusNormal).
		Order("id ASC").
		Pluck("id", &workspaceIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to list organization workspaces: %w", err)
	}
	return workspaceIDs, nil
}

func (s *organizationService) listActiveRolePermissionsByID(
	ctx context.Context,
	organizationID string,
	roleIDs []string,
) (map[string][]string, error) {
	permissionsByRoleID := make(map[string][]string)
	if len(roleIDs) == 0 {
		return permissionsByRoleID, nil
	}

	var roles []model.WorkspaceCustomRole
	if err := s.organizationRepo.GetDB().WithContext(ctx).
		Where("group_id = ? AND status = ? AND id IN ?", organizationID, model.WorkspaceCustomRoleStatusActive, roleIDs).
		Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("failed to list workspace roles: %w", err)
	}

	for _, role := range roles {
		permissionsByRoleID[role.ID] = role.Permissions
	}

	return permissionsByRoleID, nil
}

func workspaceRoleAllowsPermission(
	role model.WorkspaceMemberRole,
	roleID *string,
	rolePermissions map[string][]string,
	permissionCode model.WorkspacePermissionCode,
) bool {
	effectiveRoleID := workspaceRoleIDFromJoin(role, roleID)
	if model.IsBuiltinRole(effectiveRoleID) {
		return slices.Contains(model.GetBuiltinGroupRolePermissionsByID(effectiveRoleID), permissionCode)
	}

	return slices.Contains(rolePermissions[effectiveRoleID], string(permissionCode))
}

func workspaceRoleIDFromJoin(role model.WorkspaceMemberRole, roleID *string) string {
	if roleID != nil && *roleID != "" {
		return *roleID
	}

	switch role {
	case model.WorkspaceRoleOwner:
		return model.WorkspaceBuiltinRoleOwnerID
	case model.WorkspaceRoleAdmin:
		return model.WorkspaceBuiltinRoleAdminID
	case model.WorkspaceRoleViewer:
		return model.WorkspaceBuiltinRoleViewerID
	default:
		return model.WorkspaceBuiltinRoleMemberID
	}
}

func getStringValue(s *string) string {
	if s != nil {
		return *s
	}
	return ""
}

func getInt64Pointer(i int64) *int64 {
	return &i
}
