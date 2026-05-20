import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';

export interface ApprovalRuntimeField {
  key: string;
  label: string;
  type: 'text' | 'textarea' | string;
  required: boolean;
}

export interface ApprovalRuntimeAction {
  id: string;
  label: string;
  style?: 'primary' | 'secondary' | 'danger' | string;
}

export type ApprovalEmailRecipient =
  | {
      type: 'external';
      email: string;
    }
  | {
      type: 'member';
      account_id: string;
    };

export interface ApprovalRuntimeForm {
  id: string;
  token: string;
  node_id: string;
  node_title: string;
  content: string;
  fields: ApprovalRuntimeField[];
  actions: ApprovalRuntimeAction[];
  submit_methods?: {
    webapp?: { enabled?: boolean };
    email?: {
      enabled?: boolean;
      subject?: string;
      body?: string;
      recipients?: ApprovalEmailRecipient[];
    };
  };
  resolved_default_values?: Record<string, unknown>;
  expiration_at?: number;
  expires_at?: number;
}

export interface ApprovalSubmitRequest {
  inputs: Record<string, unknown>;
  action: string;
}

export interface ApprovalSubmitResponse {
  form_id: string;
  workflow_run_id: string;
  status: 'submitted' | string;
  action: string;
}

export interface ApprovalEventItem {
  sequence?: number;
  sequence_number?: number;
  event?: string;
  data?: unknown;
  [key: string]: unknown;
}

export interface WorkflowRunApprovalEventsParams {
  after?: number;
  sequence?: number;
  include_snapshot?: boolean;
  continue_on_pause?: boolean;
}

export interface ApprovalEventsResponse {
  data?: ApprovalEventItem[];
  events?: ApprovalEventItem[];
  has_more?: boolean;
  next_after?: number;
}

export const APPROVAL_FORM_ALREADY_SUBMITTED_CODE = '199001';

function getErrorRecord(value: unknown): Record<string, unknown> | null {
  return value && typeof value === 'object' ? (value as Record<string, unknown>) : null;
}

export function isApprovalFormAlreadySubmittedError(error: unknown): boolean {
  const record = getErrorRecord(error);
  const response = getErrorRecord(record?.response);
  const data = getErrorRecord(response?.data);
  const code = data?.code ?? record?.code;
  const message = data?.message ?? record?.message;

  return (
    String(code || '') === APPROVAL_FORM_ALREADY_SUBMITTED_CODE ||
    String(message || '').toLowerCase() === 'approval form already submitted'
  );
}

export class ApprovalService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  async getForm(token: string): Promise<ApprovalRuntimeForm> {
    const response = await this.request<ApiResponseData<ApprovalRuntimeForm>>(
      'get',
      `/approval/forms/${encodeURIComponent(token)}`,
      undefined,
      { skipAuth: true }
    );
    return response.data;
  }

  async submitForm(token: string, payload: ApprovalSubmitRequest): Promise<ApprovalSubmitResponse> {
    const response = await this.request<ApiResponseData<ApprovalSubmitResponse>>(
      'post',
      `/approval/forms/${encodeURIComponent(token)}/submit`,
      payload,
      { skipAuth: true }
    );
    return response.data;
  }

  async getEvents(
    token: string,
    params: { after?: number; limit?: number } = {}
  ): Promise<ApprovalEventsResponse> {
    const response = await this.request<ApiResponseData<ApprovalEventsResponse>>(
      'get',
      `/approval/forms/${encodeURIComponent(token)}/events`,
      undefined,
      {
        skipAuth: true,
        params: {
          after: params.after ?? 0,
          limit: params.limit ?? 100,
        },
      }
    );
    return response.data;
  }
}

export const approvalService = new ApprovalService();
export default approvalService;
