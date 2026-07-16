import type { AgentResourceBoundImpact, AgentResourceImpactAgent } from './common';

export interface Organization {
  id: string;
  name: string;
  short_name: string | null;
  status: 'active' | 'inactive';
  billing_display_currency?: 'USD' | 'CNY';
  usd_to_cny_rate?: number | string | null;
  created_at: number;
  organization_role?: 'owner' | 'admin' | 'normal';
}

export interface OrganizationCreateRequest {
  name: string;
  short_name?: string;
}

export interface OrganizationUpdateRequest {
  name: string;
  short_name?: string;
  status?: 'active' | 'inactive';
  billing_display_currency?: 'USD' | 'CNY';
  usd_to_cny_rate?: number;
}

export type OrganizationMemberRole = 'owner' | 'admin' | 'normal';

export interface OrganizationMemberRoleUpdateRequest {
  role: Exclude<OrganizationMemberRole, 'owner'>;
}

export interface OrganizationList {
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
  data: Organization[];
}

export interface Member {
  id: string;
  name: string;
  avatar: string | null;
  avatar_url: string | null;
  email: string;
  is_password_set: boolean;
  interface_language: string;
  interface_theme: string;
  timezone: string;
  last_login_at: number | null;
  last_login_ip: string | null;
  created_at: number;
  status: 'active' | 'inactive' | 'pending' | 'banned';
  organization_role?: OrganizationMemberRole;
  account_role: {
    role_type: 'system_admin' | 'normal';
  } | null;
  extension: {
    mobile: string | null;
    wechat: string | null;
    address: string | null;
  } | null;
  member_name?: string;
}

export interface MemberList {
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
  data: Member[];
}

export interface MemberListResponse {
  data: Member[];
  total?: number;
}

export interface Role {
  id: string;
  name: string;
  name_i18n?: {
    en_US?: string;
    zh_Hans?: string;
  };
  description?: string;
  description_i18n?: {
    en_US?: string;
    zh_Hans?: string;
  };
  builtin: boolean;
  editable: boolean;
  deletable?: boolean;
  applicable?: boolean;
  fixed_governance?: boolean;
  role_kind?: 'governance' | 'permission_template' | 'legacy_builtin' | string;
  system_key?: string;
  template_origin?: 'custom' | 'system_default' | string;
  status: 'active' | 'inactive';
  permissions: string[];
  member_count?: number;
}

export interface RoleMember {
  account_id: string;
  name: string;
  email: string;
  avatar: string;
  avatar_url: string;
  member_name?: string;
  workspaces?: MemberWorkspacePermission[];
}

export interface RoleMemberList {
  role_id: string;
  items: RoleMember[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

export interface CreateRoleRequest {
  name: string;
  description?: string;
  permissions?: string[];
}

export interface UpdateRolePermissionsRequest {
  permissions: string[];
}

export interface UpdateRoleInfoRequest {
  name?: string;
  description?: string;
}

export interface MemberWorkspacePermission {
  workspace_id: string;
  workspace_name: string;
  role: string;
  role_id?: string;
  role_name: string;
  permissions?: string[];
  permission_source?: string;
  permission_template_role_id?: string;
}

export interface ApplyRoleTemplateTarget {
  workspace_id: string;
  account_id: string;
}

export interface ApplyRoleTemplateRequest {
  members: ApplyRoleTemplateTarget[];
}

export interface ApplyRoleTemplateResult {
  workspace_id: string;
  account_id: string;
  status: 'applied' | 'failed';
  message?: string;
}

export interface ApplyRoleTemplateResponse {
  applied_count: number;
  failed_count: number;
  results: ApplyRoleTemplateResult[];
}

export interface ReplaceAndDeleteRoleRequest {
  replacement_role_id: string;
}

export interface ReplaceAndDeleteRoleResponse {
  replaced_count: number;
  failed_count: number;
  deleted: boolean;
  results: ApplyRoleTemplateResult[];
}

// Direct add member request
export interface DirectAddMemberRequest {
  name: string;
  email: string;
  workspace_id?: string;
  department_id: string;
  send_email: boolean;
}

export interface AdminRegisterMemberRequest {
  name: string;
  email: string;
  workspace_id?: string;
  password?: string;
  department_id?: string;
}

export interface AdminRegisterMemberResponse {
  account_id: string;
  email: string;
  name: string;
  organization_id: string;
  role: 'normal' | string;
  created_account: boolean;
  already_member: boolean;
  password_applied: boolean;
  department?: {
    id: string;
    name: string;
  };
  workspace?: {
    id: string;
    name: string;
  };
}

export interface ResetCurrentOrgMemberPasswordRequest {
  email: string;
  password?: string;
}

export interface ResetCurrentOrgMemberPasswordResponse {
  account_id: string;
  email: string;
  password_reset: boolean;
}

// Join request
export interface JoinRequest {
  id: string;
  account_id: string;
  account_name: string;
  account_email: string;
  department_id?: string;
  department_name?: string;
  status: 'pending' | 'approved' | 'rejected' | 'expired';
  created_at: string;
}

// Join request list
export interface JoinRequestList {
  data: JoinRequest[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

// Department types
export interface Department {
  id: string;
  organization_id?: string;
  parent_id: string | null;
  name: string;
  sort_order: number;
  status: 'active' | 'inactive';
  member_count: number;
  children?: Department[];
}

export interface DepartmentList {
  departments: Department[];
}

export interface JoinedWorkspace {
  workspace_id: string;
  workspace_name: string;
}

export interface DepartmentMember {
  id: string;
  department_id: string;
  department_name: string;
  account_id: string;
  account_name: string;
  account_email: string;
  avatar: string | null;
  organization_status: 'active' | 'inactive';
  organization_role?: OrganizationMemberRole;
  joined_workspaces: JoinedWorkspace[] | null;
  group_status?: 'active' | 'inactive';
  created_at: string;
  member_name?: string;
}

export interface AllDepartmentMemberList {
  data: DepartmentMember[];
  total: number;
  page: number;
  limit: number;
  has_more: boolean;
}

export interface CreateDepartmentRequest {
  name: string;
  parent_id: string;
}

export interface UpdateDepartmentRequest {
  name: string;
  parent_id?: string;
}

/**
 * Account permissions response
 * From: /organizations/{organization_id}/workspaces/{workspace_id}/accounts/{account_id}/permissions
 */
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
  department_id: string;
  department_name: string;
  member_name?: string;
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

export interface UpdateAccountRequest {
  status?: 'active' | 'banned';
  organization_role?: 'admin' | 'normal';
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

export interface CheckEmailResponse {
  id: string;
  name: string;
  email: string;
  avatar_url?: string;
  status: string;
  organization_role?: string;
  extension?: {
    mobile?: string;
  };
}

export interface CreateMemberRequest {
  name: string;
  email: string;
  mobile?: string;
  role: 'admin' | 'normal';
  position: string;
}

export interface CheckMemberNameResponse {
  is_exist: boolean;
}

export type WorkspaceAssetMoveType = 'agent' | 'dataset' | 'file' | 'database';

export interface WorkspaceAssetMoveItem {
  type: WorkspaceAssetMoveType;
  id: string;
}

export interface WorkspaceAssetMoveRequest {
  target_workspace_id: string;
  target_folder_id?: string;
  items: WorkspaceAssetMoveItem[];
  agent_binding_action?: 'unbind';
  impact_token?: string;
}

export interface WorkspaceAssetMoveEligibleTargetsRequest {
  items: WorkspaceAssetMoveItem[];
  keyword?: string;
  page?: number;
  limit?: number;
}

export interface WorkspaceAssetMoveDependencyPreviewRequest {
  items: WorkspaceAssetMoveItem[];
}

export interface WorkspaceAssetMoveDependencyPreviewResponse {
  agent_binding_impact?: {
    agents: AgentResourceImpactAgent[];
  };
}

export interface WorkspaceAssetMoveWorkspace {
  id: string;
  name?: string;
}

export interface WorkspaceAssetMoveEligibleTarget {
  id: string;
  name: string;
}

export interface WorkspaceAssetMoveEligibleTargetsResponse {
  data: WorkspaceAssetMoveEligibleTarget[];
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
}

export interface WorkspaceAssetMovePreviewItem {
  type: WorkspaceAssetMoveType;
  id: string;
  from_workspace?: WorkspaceAssetMoveWorkspace;
  target_workspace?: WorkspaceAssetMoveWorkspace;
  movable: boolean;
  blockers: string[];
  warnings: string[];
}

export interface WorkspaceAssetMovePreviewResponse {
  movable: boolean;
  items: WorkspaceAssetMovePreviewItem[];
  agent_binding_impact?: AgentResourceBoundImpact;
}

export interface WorkspaceAssetMoveResponse {
  moved: boolean;
  preview: WorkspaceAssetMovePreviewResponse;
}
