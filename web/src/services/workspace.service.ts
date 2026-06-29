import { BaseService } from '@/lib/http';
import type { ApiResponseData } from './types/common';
import type {
  WorkspaceManagement,
  WorkspaceManagementList,
  WorkspaceStatistics,
  WorkspaceMemberAccount,
  WorkspaceMemberOption,
  WorkspaceMemberOptionList,
  WorkspaceMemberRole,
  BatchAddMemberRequest,
  BatchAddMembersResponse,
  UpdateWorkspaceRequest,
  CreateWorkspaceRequest,
  CreateWorkspaceResponse,
  WorkspaceList,
  AccountPermissions,
  AvailableWorkspaceMemberList,
  GetAvailableWorkspaceMembersParams,
} from './types/workspace';

class WorkspaceService extends BaseService {
  constructor() {
    super({
      basePath: '/console/api',
      endpoint: 'main',
    });
  }

  // --- Workspace Management ---

  // Get workspaces list with search and pagination
  async getWorkspaces(
    organizationId: string,
    params?: {
      keyword?: string;
      page?: number;
      limit?: number;
    }
  ): Promise<WorkspaceManagementList> {
    const response = await this.request<ApiResponseData<WorkspaceManagementList>>(
      'get',
      `/organizations/${organizationId}/workspaces`,
      undefined,
      { params }
    );
    return response.data;
  }

  // Get workspaces joined by a specific organization member
  async getJoinedWorkspaces(
    organizationId: string,
    accountId: string,
    params?: {
      page?: number;
      limit?: number;
    }
  ): Promise<WorkspaceManagementList> {
    const response = await this.request<ApiResponseData<WorkspaceManagementList>>(
      'get',
      `/organizations/${organizationId}/joined-workspaces/${accountId}`,
      undefined,
      { params }
    );
    return response.data;
  }

  // Get managed workspaces list for an organization
  async getManagedWorkspaces(organizationId: string, params: { limit?: number; page?: number }) {
    const response = await this.request<ApiResponseData<WorkspaceList>>(
      'get',
      `/organizations/${organizationId}/managed-workspaces`,
      undefined,
      { params }
    );
    return response.data;
  }

  // Get workspace detail
  async getWorkspaceDetail(organizationId: string, workspaceId: string) {
    const response = await this.request<ApiResponseData<WorkspaceManagement>>(
      'get',
      `/organizations/${organizationId}/workspaces/${workspaceId}`
    );
    return response.data;
  }

  // Get workspace statistics/details
  async getWorkspaceStatistics(workspaceId: string) {
    const response = await this.request<ApiResponseData<WorkspaceStatistics>>(
      'get',
      `/workspaces/${workspaceId}/statistics`
    );
    return response.data;
  }

  // Create new workspace in organization
  async createWorkspace(organizationId: string, data: CreateWorkspaceRequest | { name: string }) {
    const response = await this.request<ApiResponseData<CreateWorkspaceResponse>>(
      'post',
      `/organizations/${organizationId}/workspaces`,
      data
    );
    return response.data;
  }

  // Update workspace
  async updateWorkspace(organizationId: string, workspaceId: string, data: UpdateWorkspaceRequest) {
    const response = await this.request<ApiResponseData<{ result: 'success' }>>(
      'put',
      `/organizations/${organizationId}/workspaces/${workspaceId}`,
      data
    );
    return response.data;
  }

  // Delete workspace
  async deleteWorkspace(organizationId: string, workspaceId: string) {
    const response = await this.request<ApiResponseData<{ success: boolean; result?: string }>>(
      'delete',
      `/organizations/${organizationId}/workspaces/${workspaceId}`
    );
    return response.data;
  }

  // Transfer workspace ownership
  async transferOwnership(
    organizationId: string,
    workspaceId: string,
    data: { new_owner_id: string }
  ) {
    const response = await this.request<ApiResponseData<{ result: 'success' }>>(
      'post',
      `/organizations/${organizationId}/workspaces/${workspaceId}/transfer-ownership`,
      data
    );
    return response.data;
  }

  // Leave workspace
  async leaveWorkspace(organizationId: string, workspaceId: string) {
    const response = await this.request<ApiResponseData<{ result: 'success' }>>(
      'post',
      `/organizations/${organizationId}/workspaces/${workspaceId}/leave`
    );
    return response.data;
  }

  // --- Workspace Member Management ---

  // Get workspace members list
  async getWorkspaceMembers(
    organizationId: string,
    workspaceId: string,
    params?: { page?: number; limit?: number; keyword?: string }
  ) {
    const response = await this.request<
      ApiResponseData<{
        data: WorkspaceMemberAccount[];
        total: number;
        has_more: boolean;
        page: number;
        limit: number;
      }>
    >('get', `/organizations/${organizationId}/workspaces/${workspaceId}/members`, undefined, {
      params,
    });
    return response.data;
  }

  // Get workspace member options for business pickers without role/permission management fields
  async getWorkspaceMemberOptions(
    organizationId: string,
    workspaceId: string,
    params?: { page?: number; limit?: number; keyword?: string }
  ): Promise<WorkspaceMemberOptionList> {
    const response = await this.request<ApiResponseData<WorkspaceMemberOptionList>>(
      'get',
      `/organizations/${organizationId}/workspaces/${workspaceId}/member-options`,
      undefined,
      { params }
    );
    return response.data;
  }

  // Get a single workspace member option for business picker display hydration
  async getWorkspaceMemberOption(
    organizationId: string,
    workspaceId: string,
    memberId: string
  ): Promise<WorkspaceMemberOption> {
    const response = await this.request<ApiResponseData<WorkspaceMemberOption>>(
      'get',
      `/organizations/${organizationId}/workspaces/${workspaceId}/member-options/${memberId}`
    );
    return response.data;
  }

  // Get workspace member detail
  async getWorkspaceMember(
    organizationId: string,
    workspaceId: string,
    memberId: string
  ): Promise<WorkspaceMemberAccount> {
    const response = await this.request<ApiResponseData<WorkspaceMemberAccount>>(
      'get',
      `/organizations/${organizationId}/workspaces/${workspaceId}/members/${memberId}`
    );
    return response.data;
  }

  // Get members that can be added to a workspace
  async getAvailableMembers(
    organizationId: string,
    workspaceId: string,
    params?: GetAvailableWorkspaceMembersParams
  ): Promise<AvailableWorkspaceMemberList> {
    const response = await this.request<ApiResponseData<AvailableWorkspaceMemberList>>(
      'get',
      `/organizations/${organizationId}/workspaces/${workspaceId}/available-members`,
      undefined,
      { params }
    );
    return response.data;
  }

  // Get member roles in organization
  async getMemberRoles(organizationId: string, accountId: string) {
    const response = await this.request<ApiResponseData<WorkspaceMemberRole[]>>(
      'get',
      `/organizations/${organizationId}/joined-workspaces-roles/${accountId}`
    );
    return response.data;
  }

  // Batch add members to workspace
  async batchAddWorkspaceMembers(
    organizationId: string,
    workspaceId: string,
    data: BatchAddMemberRequest
  ) {
    const response = await this.request<ApiResponseData<BatchAddMembersResponse>>(
      'post',
      `/organizations/${organizationId}/workspaces/${workspaceId}/members/batch-add`,
      data
    );
    return response.data;
  }

  // Remove member from workspace
  async removeWorkspaceMember(organizationId: string, workspaceId: string, memberId: string) {
    const response = await this.request<ApiResponseData<void>>(
      'delete',
      `/organizations/${organizationId}/workspaces/${workspaceId}/members/${memberId}`
    );
    return response.data;
  }

  // Update workspace member role
  async updateWorkspaceMemberRole(
    organizationId: string,
    workspaceId: string,
    memberId: string,
    role_id: string
  ) {
    const response = await this.request<ApiResponseData<void>>(
      'put',
      `/organizations/${organizationId}/workspaces/${workspaceId}/members/${memberId}/update-role`,
      { role_id }
    );
    return response.data;
  }

  // Update workspace member direct permissions
  async updateWorkspaceMemberPermissions(
    organizationId: string,
    workspaceId: string,
    memberId: string,
    permissions: string[]
  ) {
    const response = await this.request<ApiResponseData<{ result: 'success' }>>(
      'put',
      `/organizations/${organizationId}/workspaces/${workspaceId}/members/${memberId}/permissions`,
      { permissions }
    );
    return response.data;
  }

  // Check workspace assets
  async getWorkspaceAssets(organizationId: string, workspaceId: string) {
    const response = await this.request<ApiResponseData<{ has_assets: boolean }>>(
      'get',
      `/organizations/${organizationId}/workspaces/${workspaceId}/assets`
    );
    return response.data;
  }

  // Get account permissions in a specific workspace
  async getAccountPermissions(
    organizationId: string = 'current',
    workspaceId: string = 'current',
    accountId: string = 'current'
  ): Promise<AccountPermissions> {
    const response = await this.request<ApiResponseData<AccountPermissions>>(
      'get',
      `/organizations/${organizationId}/workspaces/${workspaceId}/accounts/${accountId}/permissions`
    );
    return response.data;
  }
}

// Export singleton instance
export const workspaceService = new WorkspaceService();
