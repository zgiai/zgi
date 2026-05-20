import { BaseService } from '@/lib/http';
import type { ApiResponseData } from './types/common';
import type {
  Organization,
  OrganizationList,
  Role,
  RoleMemberList,
  CreateRoleRequest,
  UpdateRolePermissionsRequest,
  UpdateRoleInfoRequest,
  DepartmentList,
  AllDepartmentMemberList,
  DirectAddMemberRequest,
  AdminRegisterMemberRequest,
  AdminRegisterMemberResponse,
  JoinRequestList,
  ResetCurrentOrgMemberPasswordRequest,
  ResetCurrentOrgMemberPasswordResponse,
  CreateDepartmentRequest,
  UpdateDepartmentRequest,
  CheckMemberNameResponse,
} from './types/organization';

export type * from './types/organization';
export type { ApiResponseData };

// Organization service (Focused on organization management: Roles, Departments, Join Requests)
class OrganizationService extends BaseService {
  constructor() {
    super({
      basePath: '/console/api',
      endpoint: 'main',
    });
  }

  // --- Organization Management ---

  // Get all organizations
  async getOrganizationList(params?: { page?: number; limit?: number; status?: string }) {
    const response = await this.request<ApiResponseData<OrganizationList>>(
      'get',
      '/organizations/',
      undefined,
      { params }
    );
    return response.data;
  }

  // Remove member from current organization
  async removeMemberFromOrganization(memberId: string) {
    const response = await this.request<ApiResponseData<{ result: 'success' }>>(
      'delete',
      `/organizations/current/members/${memberId}`
    );
    return response.data;
  }

  // Update member status in organization
  async updateMemberStatus(
    organizationId: string,
    memberId: string,
    status: 'active' | 'inactive'
  ) {
    const response = await this.request<ApiResponseData<void>>(
      'put',
      `/organizations/${organizationId}/members/${memberId}/status`,
      { status }
    );
    return response.data;
  }

  // Get current organization
  async getCurrentOrganization() {
    const response = await this.request<ApiResponseData<Organization>>(
      'get',
      '/organizations/current'
    );
    return response.data;
  }

  // Check if member name (nickname) already exists
  async checkMemberName(organizationId: string, name: string) {
    const response = await this.request<ApiResponseData<CheckMemberNameResponse>>(
      'get',
      `/organizations/${organizationId}/check-member-name`,
      undefined,
      { params: { name } }
    );
    return response.data;
  }

  // Update member info (nickname)
  async updateMember(organizationId: string, memberId: string, data: { name: string }) {
    const response = await this.request<ApiResponseData<void>>(
      'put',
      `/organizations/${organizationId}/members/${memberId}`,
      data
    );
    return response.data;
  }

  // --- Role Management ---

  // Get roles for organization
  async getRoles(organizationId: string) {
    const response = await this.request<ApiResponseData<{ roles: Role[] }>>(
      'get',
      `/organizations/${organizationId}/roles`
    );
    return response.data;
  }

  // Get members for a specific role with pagination
  async getRoleMembers(
    organizationId: string,
    roleId: string,
    params?: { page?: number; limit?: number }
  ) {
    const response = await this.request<ApiResponseData<RoleMemberList>>(
      'get',
      `/organizations/${organizationId}/roles/${roleId}/members`,
      undefined,
      { params }
    );
    return response.data;
  }

  // Create a new role
  async createRole(organizationId: string, data: CreateRoleRequest) {
    const response = await this.request<ApiResponseData<{ role: Role }>>(
      'post',
      `/organizations/${organizationId}/roles`,
      data
    );
    return response.data;
  }

  // Update role permissions
  async updateRolePermissions(
    organizationId: string,
    roleId: string,
    data: UpdateRolePermissionsRequest
  ) {
    const response = await this.request<ApiResponseData<void>>(
      'put',
      `/organizations/${organizationId}/roles/${roleId}/permissions`,
      data
    );
    return response.data;
  }

  // Get role detail
  async getRoleDetail(organizationId: string, roleId: string) {
    const response = await this.request<ApiResponseData<Role>>(
      'get',
      `/organizations/${organizationId}/roles/${roleId}`
    );
    return response.data;
  }

  // Update role basic info (name, description)
  async updateRoleInfo(organizationId: string, roleId: string, data: UpdateRoleInfoRequest) {
    const response = await this.request<ApiResponseData<Role>>(
      'patch',
      `/organizations/${organizationId}/roles/${roleId}`,
      data
    );
    return response.data;
  }

  // Delete role
  async deleteRole(organizationId: string, roleId: string) {
    const response = await this.request<ApiResponseData<void>>(
      'delete',
      `/organizations/${organizationId}/roles/${roleId}`
    );
    return response.data;
  }

  // --- Department Management ---

  // Get departments tree
  async getDepartments(organizationId: string) {
    const response = await this.request<ApiResponseData<DepartmentList>>(
      'get',
      `/organizations/${organizationId}/departments`
    );
    return response.data;
  }

  // Create department
  async createDepartment(organizationId: string, data: CreateDepartmentRequest) {
    const response = await this.request<ApiResponseData<{ id: string }>>(
      'post',
      `/organizations/${organizationId}/departments`,
      data
    );
    return response.data;
  }

  // Update department
  async updateDepartment(organizationId: string, deptId: string, data: UpdateDepartmentRequest) {
    const response = await this.request<ApiResponseData<{ id: string }>>(
      'put',
      `/organizations/${organizationId}/departments/${deptId}`,
      data
    );
    return response.data;
  }

  // Delete department
  async deleteDepartment(organizationId: string, deptId: string) {
    const response = await this.request<ApiResponseData<{ success: boolean }>>(
      'delete',
      `/organizations/${organizationId}/departments/${deptId}`
    );
    return response.data;
  }

  // Get department members with query parameters
  async getDepartmentMembersWithParams(
    organizationId: string,
    params?: {
      department_id?: string;
      include_sub_depts?: string;
      keyword?: string;
      limit?: string;
      page?: string;
    }
  ) {
    const response = await this.request<ApiResponseData<AllDepartmentMemberList>>(
      'get',
      `/organizations/${organizationId}/departments/members`,
      undefined,
      { params }
    );
    return response.data;
  }

  // Direct add member
  async directAddMember(organizationId: string, data: DirectAddMemberRequest) {
    const response = await this.request<ApiResponseData<{ success: boolean }>>(
      'post',
      `/organizations/${organizationId}/members/direct-add`,
      data
    );
    return response.data;
  }

  // Register or add a member to the current organization in non-cloud deployments
  async adminRegisterCurrentOrganizationMember(data: AdminRegisterMemberRequest) {
    const response = await this.request<ApiResponseData<AdminRegisterMemberResponse>>(
      'post',
      '/organizations/current/members/invite',
      data
    );
    return response.data;
  }

  // Reset password for an existing member in the current organization
  async resetCurrentOrganizationMemberPassword(data: ResetCurrentOrgMemberPasswordRequest) {
    const response = await this.request<ApiResponseData<ResetCurrentOrgMemberPasswordResponse>>(
      'post',
      '/organizations/current/members/reset-password',
      data
    );
    return response.data;
  }

  // Get join requests
  async getJoinRequests(organizationId: string, params?: { limit?: string; page?: string }) {
    const response = await this.request<ApiResponseData<JoinRequestList>>(
      'get',
      `/organizations/${organizationId}/join-requests`,
      undefined,
      { params }
    );
    return response.data;
  }

  // Approve join request
  async approveJoinRequest(organizationId: string, requestId: string) {
    const response = await this.request<ApiResponseData<{ success: boolean }>>(
      'post',
      `/organizations/${organizationId}/join-requests/${requestId}/approve`
    );
    return response.data;
  }

  // Reject join request
  async rejectJoinRequest(organizationId: string, requestId: string) {
    const response = await this.request<ApiResponseData<{ success: boolean }>>(
      'post',
      `/organizations/${organizationId}/join-requests/${requestId}/reject`
    );
    return response.data;
  }

  // Remove member from department
  async removeDepartmentMember(organizationId: string, deptId: string, accountId: string) {
    const response = await this.request<ApiResponseData<void>>(
      'delete',
      `/organizations/${organizationId}/departments/${deptId}/members/${accountId}`
    );
    return response.data;
  }

  // Update member department
  async updateMemberDepartment(
    organizationId: string,
    deptId: string,
    accountId: string,
    newDeptId: string
  ) {
    const response = await this.request<ApiResponseData<void>>(
      'put',
      `/organizations/${organizationId}/departments/member/${accountId}`,
      { department_id: newDeptId }
    );
    return response.data;
  }

  // Get invite link
  async getInviteLink(organizationId: string, departmentId?: string) {
    const response = await this.request<ApiResponseData<{ url: string }>>(
      'get',
      `/organizations/${organizationId}/invite-link`,
      undefined,
      { params: { department_id: departmentId } }
    );
    return response.data;
  }
}

// Export singleton instance
export const organizationService = new OrganizationService();

// Backward compatibility alias
export const workspaceManagementService = organizationService;
