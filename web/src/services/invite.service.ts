import { BaseService } from '@/lib/http';
import type { ApiResponseData } from './types/common';

export interface InviteInfo {
  department: {
    id: string;
    name: string;
  };
  expires_at: string | null;
  group: {
    id: string;
    name: string;
  };
  require_approval: boolean;
  valid: boolean;
}

export type InviteAcceptStatus = 'pending' | 'approved' | 'rejected' | 'expired';

export interface InviteAcceptResult {
  status: InviteAcceptStatus;
}

class InviteService extends BaseService {
  constructor() {
    super({
      basePath: '/console/api',
      endpoint: 'auth',
    });
  }

  // Get invite info
  async getInviteInfo(token: string) {
    const response = await this.request<ApiResponseData<InviteInfo>>(
      'get',
      `/public/invites/${token}`
    );
    return response.data;
  }

  // Accept invite
  async acceptInvite(token: string, member_name?: string) {
    const response = await this.request<ApiResponseData<InviteAcceptResult>>(
      'post',
      `/public/invites/${token}/accept`,
      { member_name }
    );
    return response.data;
  }
}

export const inviteService = new InviteService();
