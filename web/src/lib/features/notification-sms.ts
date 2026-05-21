import type { SystemFeatures } from '@/services/types/auth';

export const NOTIFICATION_SMS_NODE_TYPE = 'notification-sms' as const;
export const NOTIFICATION_SMS_CHANNEL_TYPE = 'sms' as const;
export const NOTIFICATION_SMS_TEMPLATE = 'pending_action_notification' as const;

export interface NotificationSMSTemplateParam {
  key: string;
  label?: string;
  required?: boolean;
  max_length?: number;
  pattern?: string;
}

export interface NotificationSMSTemplate {
  key: string;
  name?: string;
  preview_template?: string;
  params?: NotificationSMSTemplateParam[];
}

export function isNotificationSMSEnabled(features?: SystemFeatures | null): boolean {
  if (!features?.notification_sms?.enabled) {
    return false;
  }

  return true;
}

export function isNotificationSMSWorkflowNodeEnabled(features?: SystemFeatures | null): boolean {
  if (!isNotificationSMSEnabled(features)) {
    return false;
  }

  const nodeGate = features?.workflow_nodes?.[NOTIFICATION_SMS_NODE_TYPE];
  return nodeGate?.enabled !== false;
}

export function isNotificationSMSAutomationChannelEnabled(
  features?: SystemFeatures | null
): boolean {
  if (!isNotificationSMSEnabled(features)) {
    return false;
  }

  const channelGate = features?.automation_channels?.[NOTIFICATION_SMS_CHANNEL_TYPE];
  return channelGate?.enabled !== false;
}

export function getNotificationSMSTemplates(
  features?: SystemFeatures | null
): NotificationSMSTemplate[] {
  const templates = features?.notification_sms?.templates;
  if (!Array.isArray(templates)) {
    return [];
  }
  return templates
    .map(template => normalizeNotificationSMSTemplate(template))
    .filter((template): template is NotificationSMSTemplate => Boolean(template));
}

export function getDefaultNotificationSMSTemplateKey(features?: SystemFeatures | null): string {
  const configured = features?.notification_sms?.template?.trim();
  if (configured) {
    return configured;
  }
  return getNotificationSMSTemplates(features)[0]?.key ?? '';
}

export function normalizeNotificationSMSTemplate(
  template: NotificationSMSTemplate | null | undefined
): NotificationSMSTemplate | null {
  const key = template?.key?.trim();
  if (!key) {
    return null;
  }

  return {
    key,
    name: template?.name?.trim() || key,
    preview_template: template?.preview_template?.trim() || undefined,
    params: Array.isArray(template?.params)
      ? template.params
          .map(param => ({
            key: param.key?.trim() ?? '',
            label: param.label?.trim() || param.key?.trim() || '',
            required: param.required !== false,
            max_length: param.max_length,
            pattern: param.pattern?.trim() || undefined,
          }))
          .filter(param => param.key)
      : [],
  };
}

export function normalizeNotificationSMSTemplateKey(value: string | undefined): string {
  return value?.trim() ?? '';
}
