import type { ValidationResult } from '../common/validation';
import {
  normalizeNotificationSMSTemplateKey,
  NOTIFICATION_SMS_TEMPLATE,
} from '@/lib/features/notification-sms';

export interface NotificationSMSNodeData {
  type: 'notification-sms';
  title: string;
  desc: string;
  phone: string;
  template: string;
  template_params: Record<string, string>;
  isInLoop: boolean;
  isInIteration: boolean;
}

export const DEFAULT_NOTIFICATION_SMS_NODE_DATA: NotificationSMSNodeData = {
  type: 'notification-sms',
  title: '',
  desc: '',
  phone: '',
  template: NOTIFICATION_SMS_TEMPLATE,
  template_params: {
    notification_title: '',
    link_code: '',
  },
  isInLoop: false,
  isInIteration: false,
};

export function normalizeNotificationSMSNodeData(
  value: Partial<NotificationSMSNodeData> | Record<string, unknown> | null | undefined
): NotificationSMSNodeData {
  const source = value && typeof value === 'object' && !Array.isArray(value) ? value : {};
  const templateParams = normalizeTemplateParams(source);

  return {
    ...DEFAULT_NOTIFICATION_SMS_NODE_DATA,
    title:
      typeof source.title === 'string' ? source.title : DEFAULT_NOTIFICATION_SMS_NODE_DATA.title,
    desc: typeof source.desc === 'string' ? source.desc : DEFAULT_NOTIFICATION_SMS_NODE_DATA.desc,
    phone:
      typeof source.phone === 'string' ? source.phone : DEFAULT_NOTIFICATION_SMS_NODE_DATA.phone,
    template: normalizeNotificationSMSTemplateKey(
      typeof source.template === 'string' ? source.template : undefined
    ),
    template_params: templateParams,
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

  if (!normalized.template.trim()) {
    errors.push({ code: 'notificationSms.validation.templateRequired' as const });
  }

  return { isValid: errors.length === 0, errors, warnings: [] };
};

function normalizeTemplateParams(
  source: Partial<NotificationSMSNodeData> | Record<string, unknown>
): Record<string, string> {
  const raw = source.template_params;
  const params =
    raw && typeof raw === 'object' && !Array.isArray(raw)
      ? Object.fromEntries(
          Object.entries(raw).filter(
            (entry): entry is [string, string] => typeof entry[1] === 'string'
          )
        )
      : {};
  return params;
}
