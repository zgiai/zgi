package handler

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/zgiai/zgi/api/internal/modules/workspace/model"
	workspace_service "github.com/zgiai/zgi/api/internal/modules/workspace/service"
)

func TestGetDepartmentRejectsDepartmentFromAnotherOrganization(t *testing.T) {
	handler := &DepartmentHandler{
		departmentService: fakeDepartmentService{
			getDepartmentFn: func(ctx context.Context, id string) (*model.Department, error) {
				return &model.Department{
					ID:             id,
					OrganizationID: "org-2",
					Name:           "Other Org Department",
					Status:         model.DepartmentStatusActive,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}, nil
			},
		},
		enterpriseService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				return organizationID == "org-1" && accountID == "account-1", nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/departments/dept-1")
	c.Set("account_id", "account-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "dept_id", Value: "dept-1"},
	}

	handler.GetDepartment(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}

func TestGetMemberDepartmentReturnsEmptySuccessWhenMemberHasNoDepartment(t *testing.T) {
	handler := &DepartmentHandler{
		departmentService: fakeDepartmentService{
			getMemberDepartmentFn: func(ctx context.Context, organizationID, accountID string) (*model.Department, error) {
				return nil, workspace_service.ErrMemberNotInDept
			},
		},
		enterpriseService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				return organizationID == "org-1" && accountID == "account-1", nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/departments/member/account-1")
	c.Set("account_id", "account-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "account_id", Value: "account-1"},
	}

	handler.GetMemberDepartment(c)

	require.Equal(t, http.StatusOK, recorder.Code)
	require.JSONEq(t, `{"code":"0","message":"success"}`, recorder.Body.String())
}

func TestDepartmentMembersRejectsInvisibleDepartmentForNormalMember(t *testing.T) {
	handler := &DepartmentHandler{
		departmentService: fakeDepartmentService{
			getDepartmentFn: func(ctx context.Context, id string) (*model.Department, error) {
				return &model.Department{
					ID:             id,
					OrganizationID: "org-1",
					Name:           "Target Department",
					Status:         model.DepartmentStatusActive,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}, nil
			},
			getMemberDepartmentFn: func(ctx context.Context, organizationID, accountID string) (*model.Department, error) {
				return &model.Department{
					ID:             "dept-self",
					OrganizationID: organizationID,
					Name:           "Self Department",
					Status:         model.DepartmentStatusActive,
					CreatedAt:      time.Now(),
					UpdatedAt:      time.Now(),
				}, nil
			},
			getDepartmentTreeFn: func(ctx context.Context, organizationID string) ([]*workspace_service.DepartmentTreeNode, error) {
				return []*workspace_service.DepartmentTreeNode{
					{
						Department: &model.Department{
							ID:             "dept-self",
							OrganizationID: organizationID,
							Name:           "Self Department",
							Status:         model.DepartmentStatusActive,
							CreatedAt:      time.Now(),
							UpdatedAt:      time.Now(),
						},
					},
				}, nil
			},
		},
		enterpriseService: fakeOrganizationService{
			isOrganizationAdminOrOwnerFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				return false, nil
			},
			isOrganizationMemberFn: func(ctx context.Context, organizationID, accountID string) (bool, error) {
				return organizationID == "org-1" && accountID == "account-1", nil
			},
		},
	}

	c, recorder := newOrganizationHandlerTestContext(http.MethodGet, "/organizations/org-1/departments/dept-other/members")
	c.Set("account_id", "account-1")
	c.Params = gin.Params{
		{Key: "organization_id", Value: "org-1"},
		{Key: "dept_id", Value: "dept-other"},
	}

	handler.GetDepartmentMembers(c)

	require.Equal(t, http.StatusForbidden, recorder.Code)
}
