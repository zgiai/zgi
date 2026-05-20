import type { ValidationResult } from '../common/validation';
import { isNotificationSMSLinkCodeValid } from '@/components/notification-sms/validation';
import { NOTIFICATION_SMS_TEMPLATE } from '@/lib/features/notification-sms';

export interface NotificationSMSNodeData {
  type: 'notification-sms';
  title: string;
  desc: string;
  phone: string;
  template: typeof NOTIFICATION_SMS_TEMPLATE;
  notification_title: string;
  link_code: string;
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_NOTIFICATION_SMS_NODE_DATA: NotificationSMSNodeData = {
  type: 'notification-sms',
  title: '',
  desc: '',
  phone: '',
  template: NOTIFICATION_SMS_TEMPLATE,
  notification_title: '',
  link_code: '',
  isInLoop: false,
  isInIteration: false,
};

export function normalizeNotificationSMSNodeData(
  value: Partial<NotificationSMSNodeData> | Record<string, unknown> | null | undefined
): NotificationSMSNodeData {
  const source = value && typeof value === 'object' && !Array.isArray(value) ? value : {};

  return {
    ...DEFAULT_NOTIFICATION_SMS_NODE_DATA,
    title: typeof source.title === 'string' ? source.title : DEFAULT_NOTIFICATION_SMS_NODE_DATA.title,
    desc: typeof source.desc === 'string' ? source.desc : DEFAULT_NOTIFICATION_SMS_NODE_DATA.desc,
    phone: typeof source.phone === 'string' ? source.phone : DEFAULT_NOTIFICATION_SMS_NODE_DATA.phone,
    template: NOTIFICATION_SMS_TEMPLATE,
    notification_title:
      typeof source.notification_title === 'string'
        ? source.notification_title
        : DEFAULT_NOTIFICATION_SMS_NODE_DATA.notification_title,
    link_code:
      typeof source.link_code === 'string'
        ? source.link_code
        : DEFAULT_NOTIFICATION_SMS_NODE_DATA.link_code,
    isInLoop: Boolean(source.isInLoop),
    isInIteration: Boolean(source.isInIteration),
  };
}

export const checkValid = (data: NotificationSMSNodeData): ValidationResult => {
  const normalized = normalizeNotificationSMSNodeData(data);
  const errors = [];

  if (!normalized.phone.trim()) {
    errors.push({ code: 'notificationSms.validation.phoneRequired' as const });
  }

  if (!normalized.notification_title.trim()) {
    errors.push({ code: 'notificationSms.validation.notificationTitleRequired' as const });
  }

  if (!normalized.link_code.trim()) {
    errors.push({ code: 'notificationSms.validation.linkCodeRequired' as const });
  } else if (
    !isNotificationSMSLinkCodeValid(normalized.link_code, { allowWorkflowToken: true })
  ) {
    errors.push({ code: 'notificationSms.validation.linkCodeInvalid' as const });
  }

  return { isValid: errors.length === 0, errors, warnings: [] };
};
