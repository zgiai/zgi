import { http } from '@/lib/http';
import type { User, UserList, UpdateProfileRequest } from './types/auth';
import type { ApiResponseData } from './types/common';

const BASE_URL = '/console/api/account';
const ACCOUNT_EX_BASE_URL = '/console/api/account';
const WORKSPACE_URL = '/console/api/workspaces';

export type AccountContextMode = 'none' | 'organization' | 'workspace';
export type RuntimeSurface = 'webapp' | 'api' | 'builtin_app' | 'internal';
export type RuntimeResourceList = 'app_center' | 'built_in_workflows';
export type RuntimeGrantSubject = 'public' | 'organization' | 'department' | 'account' | 'internal';

export interface AccountContextResponse {
  account_id: string;
  mode: AccountContextMode;
  current_organization_id: string | null;
  current_workspace_id: string | null;
  created_at: string;
  updated_at: string;
}

export interface AccountCapabilitiesResponse {
  account_id: string;
  context: {
    mode: AccountContextMode;
    current_organization_id: string | null;
    current_workspace_id: string | null;
  };
  organization: {
    id: string | null;
    role: string;
    is_member: boolean;
    is_admin: boolean;
    product_surfaces: {
      chat: boolean;
      image: boolean;
      app: boolean;
      settings: boolean;
    };
  };
  workspace: {
    id: string | null;
    available: boolean;
    requires_workspace: boolean;
    can_view: boolean;
    role: string;
    role_name: string;
    permissions: string[];
  };
  routes: {
    organization_scope_allowed: boolean;
    workspace_scope_allowed: boolean;
    workspace_required: boolean;
  };
  runtime_audience: {
    account_id: string;
    organization_id: string | null;
    subject_types: RuntimeGrantSubject[];
    department_ids?: string[];
  };
  runtime_surfaces: Record<RuntimeSurface, {
    enabled: boolean;
    mode: string;
    grant_subject_types: RuntimeGrantSubject[];
  }>;
  runtime_resource_lists: Record<RuntimeResourceList, {
    enabled: boolean;
    resource_type: string;
    surface: RuntimeSurface;
    mode: string;
    endpoint: string;
  }>;
}

// Account service with enhanced error handling
export const accountService = {
  // Get current user profile
  getProfile: async () => {
    const res = await http.get<ApiResponseData<User>>(`${BASE_URL}/profile`);
    return res.data;
  },

  // Update user profile
  updateProfile: (data: UpdateProfileRequest) =>
    http.put<ApiResponseData<{ message: string }>>(`${BASE_URL}/profile`, data),

  // Update current workspace context
  updateContext: (data: {
    mode?: AccountContextMode;
    current_workspace_id?: string | null;
    current_organization_id?: string | null;
  }) =>
    http.put<ApiResponseData<AccountContextResponse>>(`${BASE_URL}/context`, data),

  getCapabilities: async () => {
    const res = await http.get<ApiResponseData<AccountCapabilitiesResponse>>(
      `${BASE_URL}/capabilities`
    );
    return res.data;
  },

  // Change password
  changePassword: (data: {
    current_password: string;
    new_password: string;
    new_password_confirm: string;
  }) => http.post(`${BASE_URL}/password`, data),

  // Update user avatar
  updateAvatar: (formData: FormData) =>
    http.post(`${BASE_URL}/avatar`, formData, {
      headers: { 'Content-Type': 'multipart/form-data' },
    }),

  // Delete user avatar
  deleteAvatar: () => http.delete(`${BASE_URL}/avatar`),

  // Update interface preferences
  updateInterfaceLanguage: (language: string) =>
    http.post(`${BASE_URL}/interface-language`, { interface_language: language }),

  updateInterfaceTheme: (theme: string) =>
    http.post(`${BASE_URL}/interface-theme`, { interface_theme: theme }),

  updateTimezone: (timezone: string) => http.post(`${BASE_URL}/timezone`, { timezone }),

  // Get user statistics
  getStatistics: () => http.get(`${BASE_URL}/statistics`),

  // Get user activity log
  getActivityLog: (params: { page?: number; limit?: number }) =>
    http.get(`${BASE_URL}/activity-log`, { params }),

  // Deactivate account
  deactivateAccount: () => http.post(`${BASE_URL}/deactivate`),

  // Get user list - Fixed endpoint
  getUserList: (params: { page: number; limit: number }) =>
    http.get<UserList>(`${ACCOUNT_EX_BASE_URL}/list`, { params }),

  // Get user by email - Fixed endpoint
  getUserByEmail: (email: string) =>
    http.get<User>(`${ACCOUNT_EX_BASE_URL}/email`, { params: { email } }),

  // Get user by ID - Fixed endpoint
  getUserById: (id: string) => http.get<User>(`${ACCOUNT_EX_BASE_URL}/id`, { params: { id } }),

  // Update user information - Fixed endpoint
  updateUser: (
    id: string,
    data: {
      name?: string;
      avatar?: string;
      status?: 'active' | 'pending' | 'banned';
      mobile?: string;
      role_type?: 'system_admin' | 'normal';
      organization_role?: 'admin' | 'normal';
    }
  ) => http.put<User>(`${ACCOUNT_EX_BASE_URL}/id`, data, { params: { id } }),

  // Update user role - Fixed endpoint
  updateUserRole: (id: string, role_type: 'system_admin' | 'normal') =>
    http.put<User>(`${ACCOUNT_EX_BASE_URL}/role`, { role_type }, { params: { id } }),

  // Create user - using invitation system
  createUser: (data: {
    email: string;
    name?: string;
    role?: 'admin' | 'normal';
    language?: string;
  }) =>
    http.post<{
      result: string;
      invitation_results: Array<{
        status: string;
        email: string;
        url?: string;
        message?: string;
      }>;
    }>(`${WORKSPACE_URL}/current/members/invite-email`, {
      emails: [data.email],
      role: data.role || 'normal',
      language: data.language || 'zh',
    }),

  // Delete user - using cancel invitation
  deleteUser: (memberId: string) => http.delete(`${WORKSPACE_URL}/current/members/${memberId}`),
};
