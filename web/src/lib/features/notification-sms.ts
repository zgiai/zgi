import type { SystemFeatures } from '@/services/types/auth';

export const NOTIFICATION_SMS_NODE_TYPE = 'notification-sms' as const;
export const NOTIFICATION_SMS_CHANNEL_TYPE = 'sms' as const;
export const NOTIFICATION_SMS_TEMPLATE = 'pending_action_notification' as const;
export const NOTIFICATION_SMS_WORKFLOW_ALERT_TEMPLATE = 'workflow_alert' as const;
export const NOTIFICATION_SMS_AUTH_PHONE_REGISTER_TEMPLATE = 'auth_phone_register_code' as const;
export const NOTIFICATION_SMS_AUTH_PHONE_LOGIN_TEMPLATE = 'auth_phone_login_code' as const;

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

export type NotificationSMSParamDisplayKey =
  | 'notificationTitle'
  | 'linkCode'
  | 'remark'
  | 'summary';

export type NotificationSMSTemplateParamValidationReason = 'required' | 'max_length' | 'pattern';

export interface NotificationSMSTemplateParamValidationIssue {
  param: NotificationSMSTemplateParam;
  reason: NotificationSMSTemplateParamValidationReason;
  max?: number;
}

const FALLBACK_PENDING_ACTION_TEMPLATE_PARAMS: NotificationSMSTemplateParam[] = [
  { key: 'notification_title', label: 'Notification title', required: true, max_length: 64 },
  { key: 'link_code', label: 'Link code', required: true, pattern: '^[A-Za-z0-9]+$' },
];

const WORKFLOW_VALUE_TOKEN_PATTERN = /^\{\{#[^#]+#\}\}$/;
const TEMPLATE_PARAM_DISPLAY_KEY_BY_PARAM_KEY: Record<string, NotificationSMSParamDisplayKey> = {
  notification_title: 'notificationTitle',
  title: 'notificationTitle',
  link_code: 'linkCode',
  link: 'linkCode',
  link_suffix: 'linkCode',
  remark: 'remark',
  summary: 'summary',
};

export function isNotificationSMSEnabled(features?: SystemFeatures | null): boolean {
  if (!features?.notification_sms?.enabled) {
    return false;
  }

  return true;
}

export function isNotificationSMSConfigured(features?: SystemFeatures | null): boolean {
  return isNotificationSMSEnabled(features) && getNotificationSMSTemplates(features).length > 0;
}

export function hasNotificationSMSTemplate(
  features: SystemFeatures | null | undefined,
  templateKey: string
): boolean {
  if (!isNotificationSMSEnabled(features)) {
    return false;
  }

  return getNotificationSMSTemplates(features).some(template => template.key === templateKey);
}

export function getNotificationSMSParamDisplayKey(
  param: NotificationSMSTemplateParam | string
): NotificationSMSParamDisplayKey | null {
  const key = (typeof param === 'string' ? param : param.key).trim().toLowerCase();
  return TEMPLATE_PARAM_DISPLAY_KEY_BY_PARAM_KEY[key] ?? null;
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

export function getNotificationSMSTemplateParamValidationIssues(
  templateKey: string,
  templateParams: Record<string, string>,
  templates: NotificationSMSTemplate[] = []
): NotificationSMSTemplateParamValidationIssue[] {
  const normalizedTemplateKey = normalizeNotificationSMSTemplateKey(templateKey);
  const template = templates.find(item => item.key === normalizedTemplateKey);
  const params =
    template?.params ??
    (normalizedTemplateKey === NOTIFICATION_SMS_TEMPLATE
      ? FALLBACK_PENDING_ACTION_TEMPLATE_PARAMS
      : []);

  const issues: NotificationSMSTemplateParamValidationIssue[] = [];
  for (const param of params) {
    const value = templateParams[param.key]?.trim() ?? '';
    if (param.required !== false && !value) {
      issues.push({ param, reason: 'required' });
      continue;
    }
    if (!value || isNotificationSMSWorkflowValueToken(value)) {
      continue;
    }
    if (param.max_length && [...value].length > param.max_length) {
      issues.push({ param, reason: 'max_length', max: param.max_length });
      continue;
    }
    if (param.pattern && !matchesNotificationSMSPattern(value, param.pattern)) {
      issues.push({ param, reason: 'pattern' });
    }
  }
  return issues;
}

export function isNotificationSMSWorkflowValueToken(value: string): boolean {
  return WORKFLOW_VALUE_TOKEN_PATTERN.test(value.trim());
}

function matchesNotificationSMSPattern(value: string, pattern: string): boolean {
  try {
    return new RegExp(pattern).test(value);
  } catch {
    return true;
  }
}
