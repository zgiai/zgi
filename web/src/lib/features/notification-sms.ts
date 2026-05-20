import type { SystemFeatures } from '@/services/types/auth';

export const NOTIFICATION_SMS_NODE_TYPE = 'notification-sms' as const;
export const NOTIFICATION_SMS_CHANNEL_TYPE = 'sms' as const;
export const NOTIFICATION_SMS_TEMPLATE = 'pending_action_notification' as const;

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

export function getNotificationSMSPreviewTemplate(features?: SystemFeatures | null): string | undefined {
  const previewTemplate = features?.notification_sms?.preview_template?.trim();
  return previewTemplate || undefined;
}
