package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	"github.com/zgiai/zgi/api/internal/modules/workspace/repository"
	"gorm.io/gorm"
)

var (
	ErrDepartmentNotFound       = errors.New("department not found")
	ErrDepartmentNameExists     = errors.New("department name already exists in the same parent")
	ErrMemberAlreadyInDept      = errors.New("member already in a department")
	ErrMemberNotInDept          = errors.New("member not in any department")
	ErrCannotDeleteNonEmptyDept = errors.New("cannot delete department with members or sub-departments")
	ErrCircularReference        = errors.New("circular reference detected in department hierarchy")
)

// MemberListParams re-exports from repository
type MemberListParams = repository.MemberListParams

// DepartmentMemberDetail re-exports from repository
type DepartmentMemberDetail = repository.DepartmentMemberDetail

// MemberListResponse represents paginated member list results
type MemberListResponse struct {
	Data    []*DepartmentMemberDetail `json:"data"`
	Total   int64                     `json:"total"`
	Page    int                       `json:"page"`
	Limit   int                       `json:"limit"`
	HasMore bool                      `json:"has_more"`
}

// DepartmentService interface defines the contract for department operations
type DepartmentService interface {
	// Department operations
	CreateDepartment(ctx context.Context, organizationID, name string, parentID *string, sortOrder int, createdBy string) (*model.Department, error)
	GetDepartment(ctx context.Context, id string) (*model.Department, error)
	UpdateDepartment(ctx context.Context, id, name string, parentID *string, sortOrder *int, status *model.DepartmentStatus) (*model.Department, error)
	DeleteDepartment(ctx context.Context, id string) error
	GetDepartmentTree(ctx context.Context, organizationID string) ([]*DepartmentTreeNode, error)

	// Member operations (single department mode)
	AddMemberToDepartment(ctx context.Context, organizationID, departmentID, accountID string) (*model.DepartmentMember, error)
	RemoveMemberFromDepartment(ctx context.Context, departmentID, accountID string) error
	ChangeMemberDepartment(ctx context.Context, organizationID, accountID, newDepartmentID string) (*model.DepartmentMember, error)
	GetMemberDepartment(ctx context.Context, organizationID, accountID string) (*model.Department, error)
	GetDepartmentMembers(ctx context.Context, departmentID string) ([]*model.DepartmentMember, error)
	GetDepartmentMembersWithSubDepts(ctx context.Context, departmentID string) ([]*model.DepartmentMember, error)
	GetDepartmentMembersDetail(ctx context.Context, organizationID, departmentID string) ([]*DepartmentMemberDetail, error)

	// Member list/search
	ListMembers(ctx context.Context, params *MemberListParams) (*MemberListResponse, error)

	GetMemberDetailByAccountID(ctx context.Context, organizationID, accountID string) (*DepartmentMemberDetail, error)
}

// DepartmentTreeNode represents a department with its children
type DepartmentTreeNode struct {
	*model.Department
	MemberCount int64                 `json:"member_count"`
	Children    []*DepartmentTreeNode `json:"children"`
}

type departmentService struct {
	deptRepo       repository.DepartmentRepository
	enterpriseRepo repository.OrganizationRepository
}

// NewDepartmentService creates a new department service
func NewDepartmentService(deptRepo repository.DepartmentRepository, enterpriseRepo repository.OrganizationRepository) DepartmentService {
	return &departmentService{
		deptRepo:       deptRepo,
		enterpriseRepo: enterpriseRepo,
	}
}

// CreateDepartment creates a new department
func (s *departmentService) CreateDepartment(ctx context.Context, organizationID, name string, parentID *string, sortOrder int, createdBy string) (*model.Department, error) {
	// Normalize parentID: treat empty string as nil (root department)
	if parentID != nil && *parentID == "" {
		parentID = nil
	}

	// Check if name already exists in the same parent
	exists, err := s.deptRepo.ExistsByNameInParent(ctx, organizationID, parentID, name, "")
	if err != nil {
		return nil, fmt.Errorf("failed to check department name: %w", err)
	}
	if exists {
		return nil, ErrDepartmentNameExists
	}

	// If parentID is provided, validate it exists and belongs to the same organization.
	if parentID != nil && *parentID != "" {
		parent, err := s.deptRepo.GetByID(ctx, *parentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrDepartmentNotFound
			}
			return nil, fmt.Errorf("failed to get parent department: %w", err)
		}
		if parent.OrganizationID != organizationID {
			return nil, errors.New("parent department does not belong to the same organization")
		}
	}

	dept := &model.Department{
		OrganizationID: organizationID,
		ParentID:       parentID,
		Name:           name,
		SortOrder:      sortOrder,
		Status:         model.DepartmentStatusActive,
		CreatedBy:      &createdBy,
	}

	if err := s.deptRepo.Create(ctx, dept); err != nil {
		return nil, fmt.Errorf("failed to create department: %w", err)
	}

	return dept, nil
}

// GetDepartment retrieves a department by ID
func (s *departmentService) GetDepartment(ctx context.Context, id string) (*model.Department, error) {
	dept, err := s.deptRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDepartmentNotFound
		}
		return nil, fmt.Errorf("failed to get department: %w", err)
	}
	return dept, nil
}

// UpdateDepartment updates a department
// parentID semantics:
//   - nil: do not update parent
//   - pointer to empty string "": update to root (top-level department)
//   - pointer to valid UUID: update to that parent
func (s *departmentService) UpdateDepartment(ctx context.Context, id, name string, parentID *string, sortOrder *int, status *model.DepartmentStatus) (*model.Department, error) {
	dept, err := s.deptRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDepartmentNotFound
		}
		return nil, fmt.Errorf("failed to get department: %w", err)
	}

	// Update name if provided
	if name != "" && name != dept.Name {
		// Check if new name already exists in the same parent
		checkParentID := dept.ParentID
		if parentID != nil {
			checkParentID = parentID
		}
		exists, err := s.deptRepo.ExistsByNameInParent(ctx, dept.OrganizationID, checkParentID, name, id)
		if err != nil {
			return nil, fmt.Errorf("failed to check department name: %w", err)
		}
		if exists {
			return nil, ErrDepartmentNameExists
		}
		dept.Name = name
	}

	// Update parent if provided
	if parentID != nil {
		if *parentID == "" {
			// Empty string means update to root (top-level department)
			dept.ParentID = nil
		} else {
			// Check for circular reference
			if err := s.checkCircularReference(ctx, id, *parentID); err != nil {
				return nil, err
			}
			dept.ParentID = parentID
		}
	}

	if sortOrder != nil {
		dept.SortOrder = *sortOrder
	}

	if status != nil {
		dept.Status = *status
	}

	if err := s.deptRepo.Update(ctx, dept); err != nil {
		return nil, fmt.Errorf("failed to update department: %w", err)
	}

	return dept, nil
}

// checkCircularReference checks if setting newParentID as parent of deptID would create a cycle
func (s *departmentService) checkCircularReference(ctx context.Context, deptID, newParentID string) error {
	if newParentID == "" {
		return nil
	}
	if deptID == newParentID {
		return ErrCircularReference
	}

	// Walk up the tree from newParentID to check if we reach deptID
	currentID := newParentID
	visited := make(map[string]bool)
	for currentID != "" {
		if visited[currentID] {
			return ErrCircularReference
		}
		visited[currentID] = true

		if currentID == deptID {
			return ErrCircularReference
		}

		parent, err := s.deptRepo.GetByID(ctx, currentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				break
			}
			return fmt.Errorf("failed to check circular reference: %w", err)
		}

		if parent.ParentID == nil {
			break
		}
		currentID = *parent.ParentID
	}

	return nil
}

// DeleteDepartment deletes a department
func (s *departmentService) DeleteDepartment(ctx context.Context, id string) error {
	dept, err := s.deptRepo.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrDepartmentNotFound
		}
		return fmt.Errorf("failed to get department: %w", err)
	}

	// Check if department has members
	memberCount, err := s.deptRepo.CountMembersByDepartmentID(ctx, id)
	if err != nil {
		return fmt.Errorf("failed to count members: %w", err)
	}
	if memberCount > 0 {
		return ErrCannotDeleteNonEmptyDept
	}

	// Check if department has sub-departments
	children, err := s.deptRepo.GetByParentID(ctx, dept.OrganizationID, &id)
	if err != nil {
		return fmt.Errorf("failed to get sub-departments: %w", err)
	}
	if len(children) > 0 {
		return ErrCannotDeleteNonEmptyDept
	}

	if err := s.deptRepo.Delete(ctx, id); err != nil {
		return fmt.Errorf("failed to delete department: %w", err)
	}

	return nil
}

// GetDepartmentTree retrieves the department tree for an organization.
func (s *departmentService) GetDepartmentTree(ctx context.Context, organizationID string) ([]*DepartmentTreeNode, error) {
	depts, err := s.deptRepo.GetByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get departments: %w", err)
	}

	// Build a map for quick lookup
	deptMap := make(map[string]*DepartmentTreeNode)
	for _, dept := range depts {
		count, _ := s.deptRepo.CountMembersByDepartmentID(ctx, dept.ID)
		deptMap[dept.ID] = &DepartmentTreeNode{
			Department:  dept,
			MemberCount: count,
			Children:    []*DepartmentTreeNode{},
		}
	}

	// Build tree structure
	var roots []*DepartmentTreeNode
	for _, dept := range depts {
		node := deptMap[dept.ID]
		if node.ParentID == nil || *node.ParentID == "" {
			roots = append(roots, node)
		} else {
			if parent, ok := deptMap[*node.ParentID]; ok {
				parent.Children = append(parent.Children, node)
			}
		}
	}

	return roots, nil
}

// AddMemberToDepartment adds a member to a department (single department mode)
func (s *departmentService) AddMemberToDepartment(ctx context.Context, organizationID, departmentID, accountID string) (*model.DepartmentMember, error) {
	// Verify department exists and belongs to the organization.
	dept, err := s.deptRepo.GetByID(ctx, departmentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDepartmentNotFound
		}
		return nil, fmt.Errorf("failed to get department: %w", err)
	}
	if dept.OrganizationID != organizationID {
		return nil, errors.New("department does not belong to the specified organization")
	}

	// Check if member is already in a department in this organization (single department mode).
	existing, err := s.deptRepo.GetMemberByAccountIDInOrganization(ctx, organizationID, accountID)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing membership: %w", err)
	}
	if existing != nil {
		return nil, ErrMemberAlreadyInDept
	}

	member := &model.DepartmentMember{
		DepartmentID: departmentID,
		AccountID:    accountID,
	}

	if err := s.deptRepo.CreateMember(ctx, member); err != nil {
		return nil, fmt.Errorf("failed to add member: %w", err)
	}

	return member, nil
}

// RemoveMemberFromDepartment removes a member from a department
func (s *departmentService) RemoveMemberFromDepartment(ctx context.Context, departmentID, accountID string) error {
	if err := s.deptRepo.DeleteMember(ctx, departmentID, accountID); err != nil {
		return fmt.Errorf("failed to remove member: %w", err)
	}
	return nil
}

// ChangeMemberDepartment changes a member's department (single department mode)
func (s *departmentService) ChangeMemberDepartment(ctx context.Context, organizationID, accountID, newDepartmentID string) (*model.DepartmentMember, error) {
	// Verify new department exists and belongs to the organization if newDepartmentID is provided.
	if newDepartmentID != "" {
		newDept, err := s.deptRepo.GetByID(ctx, newDepartmentID)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, ErrDepartmentNotFound
			}
			return nil, fmt.Errorf("failed to get department: %w", err)
		}
		if newDept.OrganizationID != organizationID {
			return nil, errors.New("department does not belong to the specified organization")
		}
	}

	// Get existing membership
	existing, err := s.deptRepo.GetMemberByAccountIDInOrganization(ctx, organizationID, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			existing = nil
		} else {
			return nil, fmt.Errorf("failed to get existing membership: %w", err)
		}
	}

	// Use transaction to ensure atomicity
	db := s.deptRepo.GetDB()
	err = db.Transaction(func(tx *gorm.DB) error {
		txRepo := s.deptRepo.WithTx(tx)

		// Delete old membership if exists
		if existing != nil {
			if err := txRepo.DeleteMemberByID(ctx, existing.ID); err != nil {
				return err
			}
		}

		// Create new membership only if newDepartmentID is provided
		if newDepartmentID != "" {
			newMember := &model.DepartmentMember{
				DepartmentID: newDepartmentID,
				AccountID:    accountID,
			}
			return txRepo.CreateMember(ctx, newMember)
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to change department: %w", err)
	}

	// Return the new membership
	if newDepartmentID == "" {
		return nil, nil
	}
	return s.deptRepo.GetMember(ctx, newDepartmentID, accountID)
}

// GetMemberDepartment retrieves the department a member belongs to
func (s *departmentService) GetMemberDepartment(ctx context.Context, organizationID, accountID string) (*model.Department, error) {
	member, err := s.deptRepo.GetMemberByAccountIDInOrganization(ctx, organizationID, accountID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMemberNotInDept
		}
		return nil, fmt.Errorf("failed to get membership: %w", err)
	}

	return s.deptRepo.GetByID(ctx, member.DepartmentID)
}

// GetDepartmentMembers retrieves all members of a department
func (s *departmentService) GetDepartmentMembers(ctx context.Context, departmentID string) ([]*model.DepartmentMember, error) {
	return s.deptRepo.GetMembersByDepartmentID(ctx, departmentID)
}

// GetDepartmentMembersWithSubDepts retrieves all members of a department and its sub-departments
func (s *departmentService) GetDepartmentMembersWithSubDepts(ctx context.Context, departmentID string) ([]*model.DepartmentMember, error) {
	dept, err := s.deptRepo.GetByID(ctx, departmentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrDepartmentNotFound
		}
		return nil, fmt.Errorf("failed to get department: %w", err)
	}

	// Collect all department IDs (current + all descendants)
	deptIDs := []string{departmentID}
	if err := s.collectDescendantIDs(ctx, dept.OrganizationID, departmentID, &deptIDs); err != nil {
		return nil, err
	}

	// Get all members from all departments
	var allMembers []*model.DepartmentMember
	for _, deptID := range deptIDs {
		members, err := s.deptRepo.GetMembersByDepartmentID(ctx, deptID)
		if err != nil {
			return nil, fmt.Errorf("failed to get members: %w", err)
		}
		allMembers = append(allMembers, members...)
	}

	return allMembers, nil
}

// collectDescendantIDs recursively collects all descendant department IDs.
func (s *departmentService) collectDescendantIDs(ctx context.Context, organizationID, parentID string, ids *[]string) error {
	children, err := s.deptRepo.GetByParentID(ctx, organizationID, &parentID)
	if err != nil {
		return err
	}

	for _, child := range children {
		*ids = append(*ids, child.ID)
		if err := s.collectDescendantIDs(ctx, organizationID, child.ID, ids); err != nil {
			return err
		}
	}

	return nil
}

// ListMembers lists/searches members in an organization.
func (s *departmentService) ListMembers(ctx context.Context, params *MemberListParams) (*MemberListResponse, error) {
	results, total, err := s.deptRepo.ListMembers(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("failed to list members: %w", err)
	}

	hasMore := int64(params.Page*params.Limit) < total

	return &MemberListResponse{
		Data:    results,
		Total:   total,
		Page:    params.Page,
		Limit:   params.Limit,
		HasMore: hasMore,
	}, nil
}

func (s *departmentService) GetMemberDetailByAccountID(ctx context.Context, organizationID, accountID string) (*DepartmentMemberDetail, error) {
	member, err := s.deptRepo.GetMemberDetailByAccountIDInOrganization(ctx, organizationID, accountID)
	if err != nil {
		return nil, err
	}
	return member, nil
}

// GetDepartmentMembersDetail retrieves detailed member info including status and joined workspaces.
func (s *departmentService) GetDepartmentMembersDetail(ctx context.Context, organizationID, departmentID string) ([]*DepartmentMemberDetail, error) {
	return s.deptRepo.GetMembersDetailByDepartmentID(ctx, organizationID, departmentID)
}
