import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';

export type NotificationChannelType = 'email' | 'webhook';
export type NotificationChannelEvent = 'task_report' | 'task_failed' | 'task_approval';
export type NotificationChannelStatus = 'pending' | 'active' | 'disabled';

/** A place notifications are delivered outside the console (ADR 0008). */
export interface NotificationChannel {
  id: string;
  owner_kind: 'account' | 'workspace' | 'digital_human';
  type: NotificationChannelType;
  name: string;
  target: string;
  events: NotificationChannelEvent[];
  status: NotificationChannelStatus;
  verified_at?: string;
  created_at: string;
  /** Returned once, when a webhook is created. */
  secret?: string;
}

/** A team channel as someone choosing where a task reports sees it. */
export interface NotificationChannelChoice {
  id: string;
  type: NotificationChannelType;
  name: string;
  events: NotificationChannelEvent[];
}

/** Team channels of the current workspace, kept by task center managers. */
export const TEAM_NOTIFICATION_CHANNELS_PATH = '/team-notification-channels';

export interface NotificationChannelInput {
  type: NotificationChannelType;
  name?: string;
  target: string;
  events: NotificationChannelEvent[];
}

export interface NotificationChannelUpdate {
  name?: string;
  events?: NotificationChannelEvent[];
  enabled?: boolean;
}

// basePath is /notification-channels for the signed-in account, or
// /digital-humans/{id}/notification-channels for a DigitalHuman's webhook.
class NotificationChannelService extends BaseService {
  constructor() {
    super({ basePath: '/console/api', endpoint: 'main' });
  }

  list(basePath: string): Promise<ApiResponseData<{ items: NotificationChannel[] }>> {
    return this.request('get', basePath);
  }

  create(basePath: string, input: NotificationChannelInput): Promise<ApiResponseData<NotificationChannel>> {
    return this.request('post', basePath, input);
  }

  update(basePath: string, id: string, update: NotificationChannelUpdate): Promise<ApiResponseData<NotificationChannel>> {
    return this.request('patch', `${basePath}/${id}`, update);
  }

  remove(basePath: string, id: string): Promise<ApiResponseData<{ deleted: boolean }>> {
    return this.request('delete', `${basePath}/${id}`);
  }

  verify(basePath: string, id: string, code: string): Promise<ApiResponseData<NotificationChannel>> {
    return this.request('post', `${basePath}/${id}/verify`, { code });
  }

  resendCode(basePath: string, id: string): Promise<ApiResponseData<{ sent: boolean }>> {
    return this.request('post', `${basePath}/${id}/resend-code`, {});
  }

  rotateSecret(basePath: string, id: string): Promise<ApiResponseData<{ secret: string }>> {
    return this.request('post', `${basePath}/${id}/rotate-secret`, {});
  }

  teamChoices(): Promise<ApiResponseData<{ items: NotificationChannelChoice[] }>> {
    return this.request('get', `${TEAM_NOTIFICATION_CHANNELS_PATH}/options`);
  }

  test(basePath: string, id: string): Promise<ApiResponseData<{ sent: boolean; error?: string }>> {
    return this.request('post', `${basePath}/${id}/test`, {});
  }
}

export const notificationChannelService = new NotificationChannelService();
