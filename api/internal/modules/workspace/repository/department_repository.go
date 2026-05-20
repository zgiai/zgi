package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/zgiai/ginext/internal/modules/workspace/model"
	"gorm.io/gorm"
)

// MemberListParams represents list/search parameters
type MemberListParams struct {
	OrganizationID        string
	Keyword               string
	DepartmentID          *string
	IncludeSubDept        bool
	Page                  int
	Limit                 int
	OnlyWithoutDepartment bool
	OnlyActive            bool
	ExcludeWorkspaceID    *string
}

// JoinedWorkspaceInfo represents tenant info that a member has joined
type JoinedWorkspaceInfo struct {
	WorkspaceID   string `json:"workspace_id"`
	WorkspaceName string `json:"workspace_name"`
}

type DepartmentMemberDetail struct {
	ID                 string                   `json:"id"`
	DepartmentID       string                   `json:"department_id"`
	DepartmentName     *string                  `json:"department_name,omitempty"`
	AccountID          string                   `json:"account_id"`
	AccountName        string                   `json:"account_name"`
	MemberName         *string                  `json:"member_name"`
	AccountEmail       string                   `json:"account_email"`
	Avatar             *string                  `json:"avatar"`
	GroupStatus        model.OrganizationStatus `json:"group_status"`
	OrganizationStatus model.OrganizationStatus `json:"organization_status"`
	JoinedWorkspace    []JoinedWorkspaceInfo    `json:"joined_workspaces"`
	CreatedAt          string                   `json:"created_at"`
}

// DepartmentRepository interface defines the contract for department operations
type DepartmentRepository interface {
	// Department CRUD
	Create(ctx context.Context, dept *model.Department) error
	CreateWithTx(ctx context.Context, tx *gorm.DB, dept *model.Department) error
	GetByID(ctx context.Context, id string) (*model.Department, error)
	Update(ctx context.Context, dept *model.Department) error
	Delete(ctx context.Context, id string) error

	// Department queries
	GetByOrganizationID(ctx context.Context, organizationID string) ([]*model.Department, error)
	GetByParentID(ctx context.Context, organizationID string, parentID *string) ([]*model.Department, error)
	GetRootDepartments(ctx context.Context, organizationID string) ([]*model.Department, error)
	ExistsByNameInParent(ctx context.Context, organizationID string, parentID *string, name string, excludeID string) (bool, error)

	// Department member operations
	CreateMember(ctx context.Context, member *model.DepartmentMember) error
	CreateMemberWithTx(ctx context.Context, tx *gorm.DB, member *model.DepartmentMember) error
	GetMember(ctx context.Context, departmentID, accountID string) (*model.DepartmentMember, error)
	DeleteMember(ctx context.Context, departmentID, accountID string) error
	DeleteMemberByID(ctx context.Context, id string) error

	// Member queries
	GetMembersByDepartmentID(ctx context.Context, departmentID string) ([]*model.DepartmentMember, error)
	GetMembersByAccountID(ctx context.Context, accountID string) ([]*model.DepartmentMember, error)
	GetMemberByAccountIDInOrganization(ctx context.Context, organizationID, accountID string) (*model.DepartmentMember, error)
	CountMembersByDepartmentID(ctx context.Context, departmentID string) (int64, error)
	GetMembersDetailByDepartmentID(ctx context.Context, organizationID, departmentID string) ([]*DepartmentMemberDetail, error)

	// Member list/search
	ListMembers(ctx context.Context, params *MemberListParams) ([]*DepartmentMemberDetail, int64, error)

	GetMemberDetailByAccountIDInOrganization(ctx context.Context, organizationID, accountID string) (*DepartmentMemberDetail, error)

	// Transaction support
	GetDB() *gorm.DB
	WithTx(tx *gorm.DB) DepartmentRepository
}

type departmentRepository struct {
	db *gorm.DB
}

// NewDepartmentRepository creates a new department repository
func NewDepartmentRepository(db *gorm.DB) DepartmentRepository {
	return &departmentRepository{db: db}
}

// Create creates a new department
func (r *departmentRepository) Create(ctx context.Context, dept *model.Department) error {
	if dept.ID == "" {
		dept.ID = uuid.New().String()
	}
	now := time.Now()
	dept.CreatedAt = now
	dept.UpdatedAt = now
	return r.db.WithContext(ctx).Create(dept).Error
}

// CreateWithTx creates a new department within a transaction
func (r *departmentRepository) CreateWithTx(ctx context.Context, tx *gorm.DB, dept *model.Department) error {
	if dept.ID == "" {
		dept.ID = uuid.New().String()
	}
	now := time.Now()
	dept.CreatedAt = now
	dept.UpdatedAt = now
	return tx.WithContext(ctx).Create(dept).Error
}

// GetByID retrieves a department by ID
func (r *departmentRepository) GetByID(ctx context.Context, id string) (*model.Department, error) {
	var dept model.Department
	err := r.db.WithContext(ctx).Where("id = ?", id).First(&dept).Error
	if err != nil {
		return nil, err
	}
	return &dept, nil
}

// Update updates a department
func (r *departmentRepository) Update(ctx context.Context, dept *model.Department) error {
	dept.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Save(dept).Error
}

// Delete deletes a department by ID
func (r *departmentRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.Department{}, "id = ?", id).Error
}

// GetByOrganizationID retrieves all departments for an organization.
func (r *departmentRepository) GetByOrganizationID(ctx context.Context, organizationID string) ([]*model.Department, error) {
	var depts []*model.Department
	err := r.db.WithContext(ctx).
		Where("group_id = ? AND status = ?", organizationID, model.DepartmentStatusActive).
		Order("sort_order ASC, created_at ASC").
		Find(&depts).Error
	return depts, err
}

// GetByParentID retrieves departments by parent ID.
func (r *departmentRepository) GetByParentID(ctx context.Context, organizationID string, parentID *string) ([]*model.Department, error) {
	var depts []*model.Department
	query := r.db.WithContext(ctx).Where("group_id = ? AND status = ?", organizationID, model.DepartmentStatusActive)
	if parentID == nil || *parentID == "" {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	err := query.Order("sort_order ASC, created_at ASC").Find(&depts).Error
	return depts, err
}

// GetRootDepartments retrieves root departments (no parent) for an organization.
func (r *departmentRepository) GetRootDepartments(ctx context.Context, organizationID string) ([]*model.Department, error) {
	return r.GetByParentID(ctx, organizationID, nil)
}

// ExistsByNameInParent checks if a department with the same name exists under the same parent
func (r *departmentRepository) ExistsByNameInParent(ctx context.Context, organizationID string, parentID *string, name string, excludeID string) (bool, error) {
	var count int64
	query := r.db.WithContext(ctx).Model(&model.Department{}).
		Where("group_id = ? AND name = ?", organizationID, name)
	if parentID == nil || *parentID == "" {
		query = query.Where("parent_id IS NULL")
	} else {
		query = query.Where("parent_id = ?", *parentID)
	}
	if excludeID != "" {
		query = query.Where("id != ?", excludeID)
	}
	err := query.Count(&count).Error
	return count > 0, err
}

// CreateMember creates a new department member
func (r *departmentRepository) CreateMember(ctx context.Context, member *model.DepartmentMember) error {
	if member.ID == "" {
		member.ID = uuid.New().String()
	}
	member.CreatedAt = time.Now()
	return r.db.WithContext(ctx).Create(member).Error
}

// CreateMemberWithTx creates a new department member within a transaction
func (r *departmentRepository) CreateMemberWithTx(ctx context.Context, tx *gorm.DB, member *model.DepartmentMember) error {
	if member.ID == "" {
		member.ID = uuid.New().String()
	}
	member.CreatedAt = time.Now()
	return tx.WithContext(ctx).Create(member).Error
}

// GetMember retrieves a department member by department ID and account ID
func (r *departmentRepository) GetMember(ctx context.Context, departmentID, accountID string) (*model.DepartmentMember, error) {
	var member model.DepartmentMember
	err := r.db.WithContext(ctx).
		Where("department_id = ? AND account_id = ?", departmentID, accountID).
		First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// DeleteMember deletes a department member by department ID and account ID
func (r *departmentRepository) DeleteMember(ctx context.Context, departmentID, accountID string) error {
	return r.db.WithContext(ctx).
		Delete(&model.DepartmentMember{}, "department_id = ? AND account_id = ?", departmentID, accountID).Error
}

// DeleteMemberByID deletes a department member by ID
func (r *departmentRepository) DeleteMemberByID(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&model.DepartmentMember{}, "id = ?", id).Error
}

// GetMembersByDepartmentID retrieves all members of a department
func (r *departmentRepository) GetMembersByDepartmentID(ctx context.Context, departmentID string) ([]*model.DepartmentMember, error) {
	var members []*model.DepartmentMember
	err := r.db.WithContext(ctx).
		Where("department_id = ?", departmentID).
		Order("created_at ASC").
		Find(&members).Error
	return members, err
}

// GetMembersByAccountID retrieves all department memberships for an account
func (r *departmentRepository) GetMembersByAccountID(ctx context.Context, accountID string) ([]*model.DepartmentMember, error) {
	var members []*model.DepartmentMember
	err := r.db.WithContext(ctx).
		Where("account_id = ?", accountID).
		Find(&members).Error
	return members, err
}

// GetMemberByAccountIDInOrganization retrieves the department membership for an account in a specific organization.
func (r *departmentRepository) GetMemberByAccountIDInOrganization(ctx context.Context, organizationID, accountID string) (*model.DepartmentMember, error) {
	var member model.DepartmentMember
	err := r.db.WithContext(ctx).
		Joins("JOIN departments ON departments.id = department_members.department_id").
		Where("departments.group_id = ? AND department_members.account_id = ?", organizationID, accountID).
		First(&member).Error
	if err != nil {
		return nil, err
	}
	return &member, nil
}

// CountMembersByDepartmentID counts members in a department
func (r *departmentRepository) CountMembersByDepartmentID(ctx context.Context, departmentID string) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).
		Model(&model.DepartmentMember{}).
		Where("department_id = ?", departmentID).
		Count(&count).Error
	return count, err
}

// GetDB returns the underlying database connection
func (r *departmentRepository) GetDB() *gorm.DB {
	return r.db
}

// WithTx returns a new repository with the given transaction
func (r *departmentRepository) WithTx(tx *gorm.DB) DepartmentRepository {
	return NewDepartmentRepository(tx)
}

func joinCurrentOrganizationDepartmentMember(query *gorm.DB, organizationID string) *gorm.DB {
	return query.
		Joins(`
			LEFT JOIN department_members AS dm
				ON dm.account_id = egaj.account_id
				AND EXISTS (
					SELECT 1 FROM departments AS d_scope
					WHERE d_scope.id = dm.department_id
						AND d_scope.group_id = ?
				)
		`, organizationID).
		Joins("LEFT JOIN departments AS d ON d.id = dm.department_id")
}

// ListMembers lists/searches members in an organization with detailed info.
func (r *departmentRepository) ListMembers(ctx context.Context, params *MemberListParams) ([]*DepartmentMemberDetail, int64, error) {
	// Build base conditions
	type memberBasic struct {
		ID             string
		DepartmentID   *string
		DepartmentName *string
		AccountID      string
		AccountName    string
		MemberName     *string
		AccountEmail   string
		Avatar         *string
		GroupStatus    model.OrganizationStatus
		CreatedAt      string
	}

	// Base query
	baseQuery := r.db.WithContext(ctx).
		Table("members AS egaj").
		Select(`
			dm.id,
			dm.department_id,
			d.name AS department_name,
			egaj.account_id,
			a.name AS account_name,
			egaj.name AS member_name,
			a.email AS account_email,
			a.avatar,
			egaj.status AS group_status,
			to_char(egaj.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at
		`).
		Joins("JOIN accounts AS a ON a.id = egaj.account_id").
		Where("egaj.organization_id = ?", params.OrganizationID)
	baseQuery = joinCurrentOrganizationDepartmentMember(baseQuery, params.OrganizationID)

	// Count query (same conditions)
	countQuery := r.db.WithContext(ctx).
		Table("members AS egaj").
		Joins("JOIN accounts AS a ON a.id = egaj.account_id").
		Where("egaj.organization_id = ?", params.OrganizationID)
	countQuery = joinCurrentOrganizationDepartmentMember(countQuery, params.OrganizationID)

	if params.OnlyActive {
		baseQuery = baseQuery.Where("egaj.status = ?", model.OrganizationStatusActive)
		countQuery = countQuery.Where("egaj.status = ?", model.OrganizationStatusActive)
	}

	if params.ExcludeWorkspaceID != nil && *params.ExcludeWorkspaceID != "" {
		baseQuery = baseQuery.Where(`
			NOT EXISTS (
				SELECT 1 FROM workspace_members wm
				WHERE wm.workspace_id = ? AND wm.account_id = egaj.account_id
			)
		`, *params.ExcludeWorkspaceID)
		countQuery = countQuery.Where(`
			NOT EXISTS (
				SELECT 1 FROM workspace_members wm
				WHERE wm.workspace_id = ? AND wm.account_id = egaj.account_id
			)
		`, *params.ExcludeWorkspaceID)
	}

	// Apply keyword filter
	if params.Keyword != "" {
		keyword := "%" + params.Keyword + "%"
		baseQuery = baseQuery.Where("(a.name ILIKE ? OR egaj.name ILIKE ? OR a.email ILIKE ?)", keyword, keyword, keyword)
		countQuery = countQuery.Where("(a.name ILIKE ? OR egaj.name ILIKE ? OR a.email ILIKE ?)", keyword, keyword, keyword)
	}

	// Apply department filter
	if params.DepartmentID != nil && *params.DepartmentID != "" {
		if params.IncludeSubDept {
			var deptIDs []string
			r.db.WithContext(ctx).Raw(`
				WITH RECURSIVE dept_tree AS (
					SELECT id FROM departments WHERE id = ?
					UNION ALL
					SELECT d.id FROM departments d
					INNER JOIN dept_tree dt ON d.parent_id = dt.id
				)
				SELECT id FROM dept_tree
			`, *params.DepartmentID).Scan(&deptIDs)
			if len(deptIDs) > 0 {
				baseQuery = baseQuery.Where("dm.department_id IN ?", deptIDs)
				countQuery = countQuery.Where("dm.department_id IN ?", deptIDs)
			}
		} else {
			baseQuery = baseQuery.Where("dm.department_id = ?", *params.DepartmentID)
			countQuery = countQuery.Where("dm.department_id = ?", *params.DepartmentID)
		}
	} else if params.OnlyWithoutDepartment {
		baseQuery = baseQuery.Where("dm.department_id IS NULL")
		countQuery = countQuery.Where("dm.department_id IS NULL")
	}

	// Count total
	var total int64
	if err := countQuery.Distinct("egaj.account_id").Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Apply pagination and execute
	offset := (params.Page - 1) * params.Limit
	var basics []memberBasic
	if err := baseQuery.Order("a.name ASC").Offset(offset).Limit(params.Limit).Scan(&basics).Error; err != nil {
		return nil, 0, err
	}

	// Build results with joined workspaces for each member
	results := make([]*DepartmentMemberDetail, len(basics))
	for i, basic := range basics {
		// Get joined JoinedWorkspace for this account in this group
		var JoinedWorkspace []JoinedWorkspaceInfo
		r.db.WithContext(ctx).
			Table("workspaces AS t").
			Select("t.id AS workspace_id, t.name AS workspace_name").
			Joins("JOIN workspace_members AS taj ON taj.workspace_id = t.id").
			Where("taj.account_id = ? AND t.organization_id = ? AND t.status = 'normal'", basic.AccountID, params.OrganizationID).
			Order("t.created_at DESC").
			Scan(&JoinedWorkspace)

		deptID := ""
		var deptName *string
		if basic.DepartmentID != nil {
			deptID = *basic.DepartmentID
			deptName = basic.DepartmentName
		}

		// Set department_member id if the member belongs to a department, otherwise leave empty
		memberID := basic.ID
		if memberID == "" && basic.DepartmentID != nil {
			// If basic.ID is empty but there's a department association,
			// it means this member is linked to a department but doesn't have a proper department_member record
			memberID = ""
		}

		results[i] = &DepartmentMemberDetail{
			ID:                 memberID,
			DepartmentID:       deptID,
			DepartmentName:     deptName,
			AccountID:          basic.AccountID,
			AccountName:        basic.AccountName,
			MemberName:         basic.MemberName,
			AccountEmail:       basic.AccountEmail,
			Avatar:             basic.Avatar,
			GroupStatus:        basic.GroupStatus,
			OrganizationStatus: basic.GroupStatus,
			JoinedWorkspace:    JoinedWorkspace,
			CreatedAt:          basic.CreatedAt,
		}
	}

	return results, total, nil
}

func (r *departmentRepository) GetMemberDetailByAccountIDInOrganization(ctx context.Context, organizationID, accountID string) (*DepartmentMemberDetail, error) {
	type memberBasic struct {
		ID             string
		DepartmentID   *string
		DepartmentName *string
		AccountID      string
		AccountName    string
		MemberName     *string
		AccountEmail   string
		Avatar         *string
		GroupStatus    model.OrganizationStatus
		CreatedAt      string
	}

	var basic memberBasic

	query := r.db.WithContext(ctx).
		Table("members AS egaj").
		Select(`
			dm.id,
			dm.department_id,
			d.name AS department_name,
			egaj.account_id,
			a.name AS account_name,
			egaj.name AS member_name,
			a.email AS account_email,
			a.avatar,
			egaj.status AS group_status,
			to_char(egaj.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at
		`).
		Joins("JOIN accounts AS a ON a.id = egaj.account_id").
		Where("egaj.organization_id = ? AND egaj.account_id = ?", organizationID, accountID)
	query = joinCurrentOrganizationDepartmentMember(query, organizationID)

	err := query.
		Order("a.name ASC").
		Limit(1).
		Scan(&basic).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	var JoinedWorkspace []JoinedWorkspaceInfo
	err = r.db.WithContext(ctx).
		Table("workspaces AS t").
		Select("t.id AS workspace_id, t.name AS workspace_name").
		Joins("JOIN workspace_members AS taj ON taj.workspace_id = t.id").
		Where("taj.account_id = ? AND t.organization_id = ? AND t.status = 'normal'", basic.AccountID, organizationID).
		Order("t.created_at DESC").
		Scan(&JoinedWorkspace).Error
	if err != nil {
		return nil, err
	}

	deptID := ""
	var deptName *string
	if basic.DepartmentID != nil {
		deptID = *basic.DepartmentID
		deptName = basic.DepartmentName
	}

	// Set department_member id if the member belongs to a department, otherwise leave empty
	memberID := basic.ID
	if memberID == "" && basic.DepartmentID != nil {
		// If basic.ID is empty but there's a department association,
		// it means this member is linked to a department but doesn't have a proper department_member record
		memberID = ""
	}

	result := &DepartmentMemberDetail{
		ID:                 memberID,
		DepartmentID:       deptID,
		DepartmentName:     deptName,
		AccountID:          basic.AccountID,
		AccountName:        basic.AccountName,
		MemberName:         basic.MemberName,
		AccountEmail:       basic.AccountEmail,
		Avatar:             basic.Avatar,
		GroupStatus:        basic.GroupStatus,
		OrganizationStatus: basic.GroupStatus,
		JoinedWorkspace:    JoinedWorkspace,
		CreatedAt:          basic.CreatedAt,
	}

	return result, nil
}

// GetMembersDetailByDepartmentID retrieves detailed member info including status and joined workspaces.
func (r *departmentRepository) GetMembersDetailByDepartmentID(ctx context.Context, organizationID, departmentID string) ([]*DepartmentMemberDetail, error) {
	// First get basic member info with account details and organization status.
	type memberBasic struct {
		ID             string
		DepartmentID   string
		DepartmentName *string
		AccountID      string
		AccountName    string
		MemberName     *string
		AccountEmail   string
		Avatar         *string
		GroupStatus    model.OrganizationStatus
		CreatedAt      string
	}

	var basics []memberBasic
	err := r.db.WithContext(ctx).
		Table("department_members AS dm").
		Select(`
			dm.id,
			dm.department_id,
			d.name AS department_name,
			dm.account_id,
			a.name AS account_name,
			egaj.name AS member_name,
			a.email AS account_email,
			a.avatar,
			egaj.status AS group_status,
			to_char(dm.created_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS created_at
		`).
		Joins("JOIN accounts AS a ON a.id = dm.account_id").
		Joins("JOIN members AS egaj ON egaj.account_id = dm.account_id AND egaj.organization_id = ?", organizationID).
		Where("dm.department_id = ?", departmentID).
		Order("dm.created_at ASC").
		Scan(&basics).Error
	if err != nil {
		return nil, err
	}

	// Build results with joined workspaces for each member.
	results := make([]*DepartmentMemberDetail, len(basics))
	for i, basic := range basics {
		// Get joined joinedWorkspaces for this account in this group
		var joinedWorkspaces []JoinedWorkspaceInfo
		err := r.db.WithContext(ctx).
			Table("workspaces AS t").
			Select("t.id AS workspace_id, t.name AS workspaces_name").
			Joins("JOIN workspace_members AS taj ON taj.workspace_id = t.id").
			Where("taj.account_id = ? AND t.organization_id = ? AND t.status = 'normal'", basic.AccountID, organizationID).
			Order("t.created_at DESC").
			Scan(&joinedWorkspaces).Error
		if err != nil {
			return nil, err
		}

		results[i] = &DepartmentMemberDetail{
			ID:                 basic.ID,
			DepartmentID:       basic.DepartmentID,
			DepartmentName:     basic.DepartmentName,
			AccountID:          basic.AccountID,
			AccountName:        basic.AccountName,
			MemberName:         basic.MemberName,
			AccountEmail:       basic.AccountEmail,
			Avatar:             basic.Avatar,
			GroupStatus:        basic.GroupStatus,
			OrganizationStatus: basic.GroupStatus,
			JoinedWorkspace:    joinedWorkspaces,
			CreatedAt:          basic.CreatedAt,
		}
	}

	return results, nil
}
