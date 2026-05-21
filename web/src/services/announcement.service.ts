import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';

export interface AnnouncementRuntimePayload {
  id: string;
  token: string;
  node_id: string;
  title?: string;
  node_title?: string;
  content: string;
  expiration_at: number;
  expired: boolean;
  url?: string;
}

export class AnnouncementService extends BaseService {
  constructor() {
    super({
      endpoint: 'main',
      basePath: '/console/api',
    });
  }

  async getAnnouncement(token: string): Promise<AnnouncementRuntimePayload> {
    const response = await this.request<ApiResponseData<AnnouncementRuntimePayload>>(
      'get',
      `/announcements/${encodeURIComponent(token)}`,
      undefined,
      { skipAuth: true }
    );
    return response.data;
  }
}

export const announcementService = new AnnouncementService();
export default announcementService;
