// Authentication related types

export interface User {
  id: string;
  name: string;
  email: string;
  is_super_admin?: boolean;
  avatar?: string | null;
  avatar_url?: string | null;
  is_password_set?: boolean;
  interface_language?: string;
  interface_theme?: 'light' | 'dark';
  timezone: string;
  last_login_at?: string | number | null;
  last_login_ip?: string | null;
  created_at?: string | number;
  status?: 'pending' | 'uninitialized' | 'active' | 'banned' | 'closed';
  organization_role?: 'owner' | 'admin' | 'normal' | '';
  account_role?: AccountRole | null;
  extension?: UserExtension | null;
  // Additional fields from API response
  language?: string;
  // Current selected workspace ID; null means organization view mode
  current_workspace_id?: string | null;
}

export interface AccountRole {
  account_id: string;
  role_type: string;
  assigned_by: string;
  created_at: string;
}

export interface UserExtension {
  account_id: string;
  mobile?: string | null;
  wechat?: string | null;
  address?: string | null;
  birthdate?: string | null;
  gender?: string | null;
  created_at: string;
  updated_at: string;
}

export interface Account extends User {
  role?: string;
  created_by?: string;
  updated_at?: string;
  // Workspace related
  workspace_id?: string;
  workspace_name?: string;
  workspace_role?: string;
}

export interface LoginRequest {
  email: string;
  password: string;
  remember?: boolean;
}

export interface LoginResponse {
  data: {
    access_token: string;
    refresh_token: string;
    account: Account;
  };
  result: string;
}

export interface CasdoorConsumeTicketRequest {
  ticket: string;
}

export interface CasdoorConsumeTicketResponse {
  data: {
    access_token: string;
    refresh_token: string;
    account: Account;
    sso?: {
      provider?: string;
      id_token?: string;
    };
  };
  result: string;
}

export interface AuthResponse {
  access_token: string;
  refresh_token?: string;
  user: User;
}

export interface RegisterVerifyRequest {
  email: string;
  code: string;
  token: string;
}

export interface RegisterFinishRequest {
  token: string;
  name: string;
  password: string;
  password_confirm: string;
  email: string;
  interface_language?: string;
  timezone?: string;
}

export interface VerifyRequest {
  code: string;
  email?: string;
  token?: string;
}

export interface ForgotPasswordRequest {
  email: string;
  language?: string;
}

export interface ResetPasswordRequest {
  email: string;
  password: string;
  password_confirm: string;
  token: string;
  language?: string;
}

export interface SetupRequest {
  name: string;
  email: string;
  password: string;
  interface_language?: string;
  timezone?: string;
}

export interface SetupStatus {
  step: string;
  setup_at?: string;
}

export interface SystemFeatures {
  sso_enforced_for_signin: boolean;
  sso_enforced_for_signin_protocol: string;
  sso_enforced_for_web: boolean;
  sso_enforced_for_web_protocol: string;
  enable_web_sso_switch_component: boolean;
  enable_marketplace: boolean;
  max_plugin_package_size: number;
  enable_email_code_login: boolean;
  enable_email_password_login: boolean;
  enable_social_oauth_login: boolean;
  is_allow_register: boolean;
  is_allow_create_workspace: boolean;
  is_email_setup: boolean;
  license: {
    status: string;
    expired_at: string;
  };
  is_public_deployment: boolean;
  notification_sms?: {
    enabled?: boolean;
    preview_template?: string;
  };
  workflow_nodes?: Record<string, { enabled?: boolean }>;
  automation_channels?: Record<string, { enabled?: boolean }>;
}

export interface ActivationCheckData {
  workspace_name?: string;
  workspace_id?: string;
  user_name?: string;
  is_setup?: boolean;
}

export interface ActivationCheckResponse {
  is_valid: boolean;
  data?: ActivationCheckData;
}

export interface CompleteRegistrationResponse {
  result: string;
  message?: string;
}

export interface UserList {
  page: number;
  limit: number;
  total: number;
  has_more: boolean;
  data: User[];
}

export interface ApiError {
  code: string;
  message: string;
  status: number;
}

// API错误响应结构
export interface ErrorResponse {
  code: string;
  message: string;
}

export interface RegisterInitResponse {
  result: string;
  token: string;
  server_time?: number;
  resend_available_at?: number;
  resend_after_seconds?: number;
}

export interface ForgotPasswordInitResponse {
  result: string;
  token: string;
  server_time?: number;
  resend_available_at?: number;
  resend_after_seconds?: number;
}

export interface ResetPasswordResponse {
  result: string;
  data?: {
    access_token?: string;
    refresh_token?: string;
  };
}

// Payload for updating own account profile
export interface UpdateProfileRequest {
  // Name to display; omit or set null if not changing
  name?: string | null;
  // Avatar data in base64 data URL; omit or null to keep/remove
  avatar?: string | null;
  // Preferred language; optional
  language?: string | null;
  // IANA timezone name
  timezone?: string | null;
  // Mobile phone number; optional
  mobile?: string | null;
  // Current selected workspace ID
  current_workspace_id?: string | null;
}
