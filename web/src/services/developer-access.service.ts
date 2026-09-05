import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from '@/services/types/common';

export type DeveloperAccessMode = 'self_service' | 'approval_required' | 'disabled';
export type AccessRequestStatus = 'pending' | 'approved' | 'rejected' | 'cancelled';

export interface DeveloperAccessPolicy {
  id?: string;
  workspace_id: string;
  organization_id: string;
  mode: DeveloperAccessMode;
  default_quota?: number | null;
  max_quota?: number | null;
  max_keys: number;
  default_ttl_seconds?: number | null;
  max_ttl_seconds?: number | null;
  allowed_models: string[];
  version: number;
}

export interface DeveloperAccessGrant {
  id: string;
  status: 'active' | 'disabled' | 'revoked';
  quota_limit?: number | null;
  used_quota: number;
  remain_quota: number;
  max_keys: number;
  allowed_models: string[];
  expires_at?: string | null;
}

export interface DeveloperAccessRequest {
  id: string;
  requester_account_id: string;
  requester_name?: string;
  requester_email?: string;
  purpose: string;
  environment: 'development' | 'production';
  requested_quota?: number | null;
  requested_models: string[];
  requested_ttl_seconds?: number | null;
  status: AccessRequestStatus;
  reviewer_account_id?: string | null;
  review_reason?: string | null;
  reviewed_at?: string | null;
  created_at: string;
}

export interface DeveloperAccessMe {
  workspace_id: string;
  organization_id: string;
  principal_type: 'user';
  principal_id: string;
  role: string;
  can_manage: boolean;
  can_create_key: boolean;
  can_request_access: boolean;
  mode: DeveloperAccessMode;
  policy: DeveloperAccessPolicy;
  grant?: DeveloperAccessGrant | null;
  pending_request?: DeveloperAccessRequest | null;
  active_key_count: number;
}

export interface PersonalApiKey {
  id: string;
  name: string;
  status: 'active' | 'inactive' | 'revoked';
  key_masked: string;
  principal_id: string;
  principal_name?: string;
  principal_email?: string;
  environment: 'development' | 'production';
  model_names: string[];
  created_at: string;
  accessed_at?: string | null;
  expires_at?: string | null;
  revoked_at?: string | null;
  can_activate: boolean;
  can_rotate: boolean;
}

export interface PersonalApiKeyPage {
  items: PersonalApiKey[];
  total: number;
  page: number;
  page_size: number;
}

export interface PersonalApiKeyParams {
  scope?: 'all' | 'mine' | 'members';
  page?: number;
  page_size?: number;
}

export interface DeveloperAccessAuditItem {
  attempt_id: string;
  request_id: string;
  principal_id: string;
  principal_name?: string;
  principal_email?: string;
  api_key_id: string;
  api_key_name?: string;
  api_key_masked?: string;
  model_name: string;
  provider_name: string;
  status: 'success' | 'failed' | 'partial';
  prompt_tokens: number;
  completion_tokens: number;
  total_tokens: number;
  total_points: number;
  quota_charged_points: number;
  quota_overage_points: number;
  response_time_ms: number;
  error_code?: string;
  created_at: string;
}

export interface DeveloperAccessAuditPage {
  items: DeveloperAccessAuditItem[];
  total: number;
  page: number;
  page_size: number;
}

export interface DeveloperAccessAuditParams {
  principal_id?: string;
  api_key_id?: string;
  model_name?: string;
  status?: 'success' | 'failed' | 'partial';
  start_time?: number;
  end_time?: number;
  page?: number;
  page_size?: number;
}

export interface CreatedPersonalApiKey extends PersonalApiKey {
  secret: string;
}

export interface CreateAccessRequestInput {
  purpose: string;
  environment: 'development' | 'production';
  requested_quota?: number;
  requested_models?: string[];
  requested_ttl_seconds?: number;
}

export interface ReviewAccessRequestInput {
  quota_limit?: number;
  max_keys?: number;
  allowed_models?: string[];
  expires_at?: string;
  reason?: string;
}

export interface CreatePersonalApiKeyInput {
  name: string;
  environment: 'development' | 'production';
  expires_at?: string;
  model_names?: string[];
}

class DeveloperAccessService extends BaseService {
  constructor() {
    super({ basePath: '/console/api/llm' });
  }

  private workspacePath(workspaceId: string, path: string): string {
    return `/workspaces/${encodeURIComponent(workspaceId)}${path}`;
  }

  getMe(workspaceId: string): Promise<ApiResponseData<DeveloperAccessMe>> {
    return this.request('get', this.workspacePath(workspaceId, '/developer-access/me'));
  }

  getPolicy(workspaceId: string): Promise<ApiResponseData<DeveloperAccessPolicy>> {
    return this.request('get', this.workspacePath(workspaceId, '/developer-access/policy'));
  }

  updatePolicy(
    workspaceId: string,
    policy: Omit<DeveloperAccessPolicy, 'id' | 'workspace_id' | 'organization_id' | 'version'>
  ): Promise<ApiResponseData<DeveloperAccessPolicy>> {
    return this.request('put', this.workspacePath(workspaceId, '/developer-access/policy'), policy);
  }

  listRequests(
    workspaceId: string,
    status?: AccessRequestStatus
  ): Promise<ApiResponseData<DeveloperAccessRequest[]>> {
    return this.request('get', this.workspacePath(workspaceId, '/access-requests'), undefined, {
      params: status ? { status } : undefined,
    });
  }

  createRequest(
    workspaceId: string,
    input: CreateAccessRequestInput
  ): Promise<ApiResponseData<DeveloperAccessRequest>> {
    return this.request('post', this.workspacePath(workspaceId, '/access-requests'), input);
  }

  cancelRequest(
    workspaceId: string,
    requestId: string
  ): Promise<ApiResponseData<DeveloperAccessRequest>> {
    return this.request(
      'post',
      this.workspacePath(workspaceId, `/access-requests/${requestId}/cancel`),
      {}
    );
  }

  reviewRequest(
    workspaceId: string,
    requestId: string,
    decision: 'approve' | 'reject',
    input: ReviewAccessRequestInput
  ): Promise<ApiResponseData<DeveloperAccessRequest>> {
    return this.request(
      'post',
      this.workspacePath(workspaceId, `/access-requests/${requestId}/${decision}`),
      input
    );
  }

  listKeys(
    workspaceId: string,
    params?: PersonalApiKeyParams
  ): Promise<ApiResponseData<PersonalApiKeyPage>> {
    return this.request('get', this.workspacePath(workspaceId, '/api-keys'), undefined, { params });
  }

  listAudit(
    workspaceId: string,
    params?: DeveloperAccessAuditParams
  ): Promise<ApiResponseData<DeveloperAccessAuditPage>> {
    return this.request(
      'get',
      this.workspacePath(workspaceId, '/developer-access/audit'),
      undefined,
      { params }
    );
  }

  createKey(
    workspaceId: string,
    input: CreatePersonalApiKeyInput
  ): Promise<ApiResponseData<CreatedPersonalApiKey>> {
    return this.request('post', this.workspacePath(workspaceId, '/api-keys'), input);
  }

  updateKey(
    workspaceId: string,
    keyId: string,
    name: string
  ): Promise<ApiResponseData<PersonalApiKey>> {
    return this.request('patch', this.workspacePath(workspaceId, `/api-keys/${keyId}`), { name });
  }

  setKeyStatus(
    workspaceId: string,
    keyId: string,
    action: 'enable' | 'disable' | 'revoke',
    reason?: string
  ): Promise<ApiResponseData<PersonalApiKey>> {
    return this.request(
      'post',
      this.workspacePath(workspaceId, `/api-keys/${keyId}/${action}`),
      action === 'revoke' ? { reason: reason ?? '' } : {}
    );
  }

  rotateKey(
    workspaceId: string,
    keyId: string,
    name?: string
  ): Promise<ApiResponseData<CreatedPersonalApiKey>> {
    return this.request('post', this.workspacePath(workspaceId, `/api-keys/${keyId}/rotate`), {
      name,
    });
  }
}

export const developerAccessService = new DeveloperAccessService();
