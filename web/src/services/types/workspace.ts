export interface WorkspaceEntity {
  id: string;
  name: string;
  created_at: string;
  updated_at: string;
}

export interface WorkspaceManagement extends WorkspaceEntity {
  description: string;
  avatar?: string;
  member_count?: number;
  leader_id?: string;
  leader_name?: string;
  api_key_id?: string;
  api_key_name?: string;
  quota_configured?: boolean;
  used_quota?: number;
  remain_quota?: number;
  quota_limit?: number | null;
}

export interface WorkspaceManagementList {
  data: WorkspaceManagement[];
  total: number;
  has_more: boolean;
  page: number;
  limit: number;
}

export interface WorkspaceStatistics {
  admins_count: number;
  members_count: number;
  datasets_count: number;
  agents_count: number;
  name: string;
}

export interface WorkspaceMemberAccount {
  id: string;
  name: string;
  avatar: string | null;
  avatar_url: string | null;
  email: string;
  last_login_at: number;
  last_active_at: number;
  created_at: number;
  role: string;
  role_id: string;
  role_name: string;
  permissions?: string[];
  permission_source?: 'owner' | 'role_template' | 'direct' | 'legacy_role';
  permission_template_role_id?: string;
  status: string;
  has_mobile?: boolean;
  department_id?: string;
  department_name?: string;
  member_name?: string;
}

export interface AvailableWorkspaceMember {
  id: string;
  account_id: string;
  account_name: string;
  member_name?: string;
  account_email: string;
  department_id: string;
  department_name: string;
  organization_status: 'active';
  joined_workspaces: Array<{
    workspace_id: string;
    workspace_name: string;
  }> | null;
}

export interface AvailableWorkspaceMemberList {
  data: AvailableWorkspaceMember[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

export interface GetAvailableWorkspaceMembersParams {
  department_id?: string;
  include_sub_depts?: string;
  keyword?: string;
  page?: number;
  limit?: number;
}

export interface WorkspaceMemberRole {
  role: string;
  workspace_name: string;
  workspace_id: string;
  position?: string;
  permissions: string[];
  permission_source?: 'owner' | 'role_template' | 'direct' | 'legacy_role';
  permission_template_role_id?: string;
}

export interface UpdateWorkspaceRequest {
  name?: string;
  leader_id?: string;
  api_key_id?: string;
}

export interface CreateWorkspaceRequest {
  name: string;
  leader_id?: string;
  api_key_id?: string;
}

export interface CreateWorkspaceResponse {
  id: string;
  name: string;
  created_at: string;
}

export interface WorkspaceList {
  data: WorkspaceEntity[];
  total: number;
  has_more: boolean;
  page: number;
  limit: number;
}

export interface AccountPermissions {
  organization_id: string;
  workspace_id: string;
  workspace_name: string;
  account_id: string;
  /** User's role in the organization */
  organization_role: 'owner' | 'admin' | 'normal';
  /** User's role in the current workspace */
  workspace_role: 'owner' | 'admin' | 'normal';
  /** Human-readable workspace role name */
  workspace_role_name: string;
  /** List of permission strings */
  permissions: string[];
  permission_source?: 'owner' | 'role_template' | 'direct' | 'legacy_role';
  permission_template_role_id?: string;
}

export interface BatchAddMemberRequest {
  account_ids: string[];
  role_id?: string;
  role?: string;
  permissions?: string[];
}

export type BatchAddMemberResultStatus = 'success' | 'skipped' | 'failed' | string;

export interface BatchAddMemberResultItem {
  account_id: string;
  status: BatchAddMemberResultStatus;
  reason?: string;
  message?: string;
}

export interface BatchAddMembersResponse {
  success: boolean;
  result: string;
  added_count: number;
  skipped_count: number;
  failed_count: number;
  invitation_results: BatchAddMemberResultItem[];
}
