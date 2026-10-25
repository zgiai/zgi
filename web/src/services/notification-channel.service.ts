import { BaseService } from '@/lib/http/services';
import type { ApiResponseData } from './types/common';

export type NotificationChannelType = 'email' | 'webhook' | 'im';
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
  /** The IM Connection an IM channel sends with. */
  connection_id?: string;
  created_at: string;
  /** Returned once, when a webhook is created. */
  secret?: string;
}

/** One recent delivery to a channel. */
export interface NotificationDelivery {
  id: string;
  event: string;
  title: string;
  status: 'pending' | 'held' | 'sent' | 'failed';
  attempts: number;
  last_error?: string;
  sent_at?: string;
  created_at: string;
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
  /** For an IM channel: the Connection; target is then the member's own user ID. */
  connection_id?: string;
}

/** An IM Connection of the member that can send them direct messages. */
export interface IMConnection {
  id: string;
  connector_id: string;
  name: string;
  /** The user ID the tool needs, such as open_id. */
  recipient_kind: string;
}

/** The member's quiet hours, in minutes after midnight of their time zone. */
export interface NotificationPreferences {
  quiet_start: number | null;
  quiet_end: number | null;
  timezone: string;
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

  /** The channel's most recent deliveries, newest first. */
  deliveries(basePath: string, id: string): Promise<ApiResponseData<{ items: NotificationDelivery[] }>> {
    return this.request('get', `${basePath}/${id}/deliveries`);
  }

  /** Sends a failed delivery again. */
  retryDelivery(basePath: string, id: string, deliveryId: string): Promise<ApiResponseData<{ queued: boolean }>> {
    return this.request('post', `${basePath}/${id}/deliveries/${deliveryId}/retry`, {});
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

  imConnections(basePath: string): Promise<ApiResponseData<{ items: IMConnection[] }>> {
    return this.request('get', `${basePath}/im-connections`);
  }

  preferences(): Promise<ApiResponseData<NotificationPreferences>> {
    return this.request('get', '/notification-preferences');
  }

  setQuietHours(quietStart: number | null, quietEnd: number | null): Promise<ApiResponseData<NotificationPreferences>> {
    return this.request('put', '/notification-preferences', { quiet_start: quietStart, quiet_end: quietEnd });
  }

  teamChoices(): Promise<ApiResponseData<{ items: NotificationChannelChoice[] }>> {
    return this.request('get', `${TEAM_NOTIFICATION_CHANNELS_PATH}/options`);
  }

  test(basePath: string, id: string): Promise<ApiResponseData<{ sent: boolean; error?: string }>> {
    return this.request('post', `${basePath}/${id}/test`, {});
  }
}

export const notificationChannelService = new NotificationChannelService();
