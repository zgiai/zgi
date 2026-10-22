import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';

/** One in-app notification (ADR 0008). */
export interface InboxNotification {
  id: string;
  organization_id: string;
  workspace_id?: string;
  event: string;
  source_type: string;
  source_id: string;
  title: string;
  summary: string;
  link: string;
  context: Record<string, unknown>;
  read_at?: string;
  created_at: string;
}

export interface InboxList {
  items: InboxNotification[];
  total: number;
  unread_count: number;
  page: number;
  page_size: number;
}

class InboxService extends BaseService {
  constructor() {
    super({ basePath: '/console/api', endpoint: 'main' });
  }

  list(params: { unread_only?: boolean; page?: number; page_size?: number } = {}): Promise<ApiResponseData<InboxList>> {
    return this.request('get', '/inbox', undefined, { params });
  }

  unreadCount(): Promise<ApiResponseData<{ unread_count: number }>> {
    return this.request('get', '/inbox/unread-count');
  }

  markRead(request: { ids: string[] } | { source_type: string; source_id: string }): Promise<ApiResponseData<{ unread_count: number }>> {
    return this.request('post', '/inbox/read', request);
  }

  markAllRead(): Promise<ApiResponseData<{ unread_count: number }>> {
    return this.request('post', '/inbox/read-all', {});
  }
}

export const inboxService = new InboxService();
