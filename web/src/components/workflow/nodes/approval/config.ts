import { isValidEmail } from '@/utils/validation';
import type { ValidationError, ValidationResult } from '../common/validation';

export type ApprovalFieldType = 'text' | 'textarea';
export type ApprovalActionStyle = 'primary' | 'secondary' | 'danger';
export type ApprovalTimeoutUnit = 'minute' | 'hour' | 'day';
export type ApprovalDefaultValue =
  | {
      type: 'constant';
      value: string;
    }
  | {
      type: 'variable';
      selector: string[];
    };

export interface ApprovalField {
  key: string;
  label: string;
  type: ApprovalFieldType;
  required: boolean;
  default?: ApprovalDefaultValue;
}

export interface ApprovalAction {
  id: string;
  label: string;
  style?: ApprovalActionStyle;
}

export interface ApprovalSubmitMethods {
  webapp: {
    enabled: boolean;
  };
  email: {
    enabled: boolean;
    subject: string;
    body: string;
    recipients: ApprovalEmailRecipient[];
  };
  sms: {
    enabled: boolean;
    provider: string;
    template: string;
    notification_title: string;
    template_params: Record<string, string>;
    recipients: ApprovalSMSRecipient[];
  };
}

export type ApprovalEmailRecipient =
  | {
      type: 'external';
      email: string;
    }
  | {
      type: 'member';
      account_id: string;
    };

export type ApprovalSMSRecipient =
  | {
      type: 'external';
      phone: string;
    }
  | {
      type: 'member';
      account_id: string;
    };

export interface ApprovalTimeout {
  duration: number;
  unit: ApprovalTimeoutUnit;
}

export interface ApprovalNodeData {
  type: 'approval';
  title: string;
  desc: string;
  version: 'v1';
  approval: {
    content: string;
    fields: ApprovalField[];
    actions: ApprovalAction[];
  };
  submit_methods: ApprovalSubmitMethods;
  timeout: ApprovalTimeout;
  isInLoop?: boolean;
  isInIteration?: boolean;
}

export const APPROVAL_TIMEOUT_HANDLE = 'expired';
export const APPROVAL_LEGACY_TIMEOUT_HANDLE = '__timeout';
export const APPROVAL_MAX_TIMEOUT_HOURS = 72;
export const APPROVAL_ACTION_MAX_LENGTH = 20;
export const APPROVAL_SMS_TEMPLATE = 'pending_action_notification';

const APPROVAL_SMS_RESERVED_TEMPLATE_PARAMS = new Set(['notification_title', 'link_suffix']);

export const APPROVAL_SYSTEM_OUTPUT_KEYS = new Set([
  'approval_action_id',
  'approval_action_label',
  'approval_rendered_content',
  '__approval_form',
  '__approval_token',
  '__approval_form_id',
  '__edge_source_handle__',
  '__action_id',
  '__rendered_content',
  APPROVAL_TIMEOUT_HANDLE,
  APPROVAL_LEGACY_TIMEOUT_HANDLE,
]);

export const APPROVAL_IDENTIFIER_PATTERN = /^[A-Za-z_][A-Za-z0-9_]*$/;

export function getApprovalTimeoutMaxDuration(unit: ApprovalTimeoutUnit): number {
  if (unit === 'day') return 3;
  if (unit === 'minute') return APPROVAL_MAX_TIMEOUT_HOURS * 60;
  return APPROVAL_MAX_TIMEOUT_HOURS;
}

export function normalizeApprovalSourceHandle(
  handle: string | null | undefined
): string | null | undefined {
  return handle === APPROVAL_LEGACY_TIMEOUT_HANDLE ? APPROVAL_TIMEOUT_HANDLE : handle;
}

function normalizeApprovalIdentifierBase(value: string): string {
  const normalized = value
    .trim()
    .replace(/[^A-Za-z0-9_]/g, '_')
    .replace(/_+/g, '_')
    .replace(/^[^A-Za-z_]+/, '')
    .replace(/_+$/g, '');

  return normalized || 'action';
}

export function createApprovalActionId(
  existingIds: Iterable<string> = [],
  preferredId = 'action'
): string {
  const ids = new Set(existingIds);
  const baseId = normalizeApprovalIdentifierBase(preferredId);
  if (!ids.has(baseId) && baseId !== APPROVAL_TIMEOUT_HANDLE) {
    return baseId;
  }

  let fallbackIndex = 2;
  let fallbackId = `${baseId}_${fallbackIndex}`;
  while (ids.has(fallbackId) || fallbackId === APPROVAL_TIMEOUT_HANDLE) {
    fallbackIndex += 1;
    fallbackId = `${baseId}_${fallbackIndex}`;
  }
  return fallbackId;
}

export const DEFAULT_APPROVAL_NODE_DATA: ApprovalNodeData = {
  type: 'approval',
  title: '',
  desc: '',
  version: 'v1',
  approval: {
    content: '',
    fields: [],
    actions: [
      { id: 'approve', label: 'Approve', style: 'primary' },
      { id: 'reject', label: 'Reject', style: 'danger' },
    ],
  },
  submit_methods: {
    webapp: { enabled: true },
    email: {
      enabled: false,
      subject: '',
      body: 'Please open the link to complete the review: {{#url#}}',
      recipients: [],
    },
    sms: {
      enabled: false,
      provider: '',
      template: APPROVAL_SMS_TEMPLATE,
      notification_title: '',
      template_params: {},
      recipients: [],
    },
  },
  timeout: {
    duration: 36,
    unit: 'hour',
  },
  isInLoop: false,
  isInIteration: false,
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === 'object' && !Array.isArray(value);
}

function isFieldType(value: unknown): value is ApprovalFieldType {
  return value === 'text' || value === 'textarea';
}

function isActionStyle(value: unknown): value is ApprovalActionStyle {
  return value === 'primary' || value === 'secondary' || value === 'danger';
}

function isTimeoutUnit(value: unknown): value is ApprovalTimeoutUnit {
  return value === 'minute' || value === 'hour' || value === 'day';
}

function normalizeDefaultValue(value: unknown): ApprovalDefaultValue | undefined {
  if (!isRecord(value)) return undefined;

  if (value.type === 'variable') {
    const selector = Array.isArray(value.selector)
      ? value.selector.filter((item): item is string => typeof item === 'string')
      : [];

    return {
      type: 'variable',
      selector,
    };
  }

  if (value.type === 'constant') {
    return {
      type: 'constant',
      value: typeof value.value === 'string' ? value.value : '',
    };
  }

  return undefined;
}

function normalizeFields(value: unknown): ApprovalField[] {
  if (!Array.isArray(value)) return DEFAULT_APPROVAL_NODE_DATA.approval.fields;

  const fields = value.map((item, index): ApprovalField => {
    const field = isRecord(item) ? item : {};
    const fallback = DEFAULT_APPROVAL_NODE_DATA.approval.fields[index] ?? {
      key: `field_${index + 1}`,
      label: '',
      type: 'text' as ApprovalFieldType,
      required: false,
    };

    return {
      key: typeof field.key === 'string' ? field.key : fallback.key,
      label: typeof field.label === 'string' ? field.label : fallback.label,
      type: isFieldType(field.type) ? field.type : fallback.type,
      required: typeof field.required === 'boolean' ? field.required : fallback.required,
      default: normalizeDefaultValue(field.default),
    };
  });

  return fields.filter(field => field.key || field.label);
}

function normalizeActions(value: unknown): ApprovalAction[] {
  if (!Array.isArray(value)) return DEFAULT_APPROVAL_NODE_DATA.approval.actions;

  const actions = value.map((item, index): ApprovalAction => {
    const action = isRecord(item) ? item : {};
    const fallback = DEFAULT_APPROVAL_NODE_DATA.approval.actions[index] ?? {
      id: `action_${index + 1}`,
      label: '',
      style: 'secondary' as ApprovalActionStyle,
    };

    return {
      id: typeof action.id === 'string' ? action.id : fallback.id,
      label: typeof action.label === 'string' ? action.label : fallback.label,
      style: isActionStyle(action.style) ? action.style : fallback.style,
    };
  });

  return actions;
}

function normalizeEmailRecipients(value: unknown): ApprovalEmailRecipient[] {
  if (!Array.isArray(value)) return [];

  const recipients: ApprovalEmailRecipient[] = [];
  const seen = new Set<string>();
  value.forEach(item => {
    if (!isRecord(item)) return;

    const recipient =
      item.type === 'member'
        ? ({
            type: 'member',
            account_id: typeof item.account_id === 'string' ? item.account_id.trim() : '',
          } satisfies ApprovalEmailRecipient)
        : item.type === 'external'
          ? ({
              type: 'external',
              email: typeof item.email === 'string' ? item.email.trim() : '',
            } satisfies ApprovalEmailRecipient)
          : null;

    if (!recipient) return;
    const key =
      recipient.type === 'member'
        ? `member:${recipient.account_id}`
        : `external:${recipient.email.toLowerCase()}`;
    if (seen.has(key)) return;
    seen.add(key);
    recipients.push(recipient);
  });

  return recipients;
}

function normalizeSMSRecipients(value: unknown): ApprovalSMSRecipient[] {
  if (!Array.isArray(value)) return [];

  const recipients: ApprovalSMSRecipient[] = [];
  const seen = new Set<string>();
  value.forEach(item => {
    if (!isRecord(item)) return;

    const recipient =
      item.type === 'member'
        ? ({
            type: 'member',
            account_id: typeof item.account_id === 'string' ? item.account_id.trim() : '',
          } satisfies ApprovalSMSRecipient)
        : item.type === 'external'
          ? ({
              type: 'external',
              phone: typeof item.phone === 'string' ? item.phone.trim() : '',
            } satisfies ApprovalSMSRecipient)
          : null;

    if (!recipient) return;
    const key =
      recipient.type === 'member'
        ? `member:${recipient.account_id}`
        : `external:${recipient.phone}`;
    if (seen.has(key)) return;
    seen.add(key);
    recipients.push(recipient);
  });

  return recipients;
}

function normalizeTemplateParams(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {};

  return Object.entries(value).reduce<Record<string, string>>((acc, [key, paramValue]) => {
    const normalizedKey = key.trim();
    const normalizedValue = typeof paramValue === 'string' ? paramValue.trim() : '';
    if (normalizedKey) {
      acc[normalizedKey] = normalizedValue;
    }
    return acc;
  }, {});
}

export function normalizeApprovalNodeData(
  value: Partial<ApprovalNodeData> | Record<string, unknown> | null | undefined
): ApprovalNodeData {
  const fallback = DEFAULT_APPROVAL_NODE_DATA;
  if (!isRecord(value)) return fallback;

  const approval = isRecord(value.approval) ? value.approval : {};
  const submitMethods = isRecord(value.submit_methods) ? value.submit_methods : {};
  const webapp = isRecord(submitMethods.webapp) ? submitMethods.webapp : {};
  const email = isRecord(submitMethods.email) ? submitMethods.email : {};
  const sms = isRecord(submitMethods.sms) ? submitMethods.sms : {};
  const timeout = isRecord(value.timeout) ? value.timeout : {};

  return {
    ...fallback,
    title: typeof value.title === 'string' ? value.title : fallback.title,
    desc: typeof value.desc === 'string' ? value.desc : fallback.desc,
    version: 'v1',
    approval: {
      content: typeof approval.content === 'string' ? approval.content : fallback.approval.content,
      fields: normalizeFields(approval.fields),
      actions: normalizeActions(approval.actions),
    },
    submit_methods: {
      webapp: {
        enabled:
          typeof webapp.enabled === 'boolean'
            ? webapp.enabled
            : fallback.submit_methods.webapp.enabled,
      },
      email: {
        enabled:
          typeof email.enabled === 'boolean'
            ? email.enabled
            : fallback.submit_methods.email.enabled,
        subject:
          typeof email.subject === 'string' ? email.subject : fallback.submit_methods.email.subject,
        body: typeof email.body === 'string' ? email.body : fallback.submit_methods.email.body,
        recipients: normalizeEmailRecipients(email.recipients),
      },
      sms: {
        enabled:
          typeof sms.enabled === 'boolean' ? sms.enabled : fallback.submit_methods.sms.enabled,
        provider:
          typeof sms.provider === 'string' ? sms.provider : fallback.submit_methods.sms.provider,
        template:
          typeof sms.template === 'string' ? sms.template : fallback.submit_methods.sms.template,
        notification_title:
          typeof sms.notification_title === 'string'
            ? sms.notification_title
            : fallback.submit_methods.sms.notification_title,
        template_params: normalizeTemplateParams(sms.template_params),
        recipients: normalizeSMSRecipients(sms.recipients),
      },
    },
    timeout: {
      duration:
        typeof timeout.duration === 'number' && Number.isFinite(timeout.duration)
          ? timeout.duration
          : fallback.timeout.duration,
      unit: isTimeoutUnit(timeout.unit) ? timeout.unit : fallback.timeout.unit,
    },
    isInLoop: Boolean(value.isInLoop),
    isInIteration: Boolean(value.isInIteration),
  };
}

export function checkValid(data: ApprovalNodeData): ValidationResult {
  const normalized = normalizeApprovalNodeData(data);
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!normalized.title.trim()) {
    errors.push({ code: 'approval.validation.titleRequired' });
  }

  if (!normalized.approval.content.trim()) {
    warnings.push({ code: 'approval.validation.contentRecommended' });
  }

  if (normalized.approval.actions.length === 0) {
    errors.push({ code: 'approval.validation.actionRequired' });
  }

  const actionIds = new Set<string>();
  normalized.approval.actions.forEach((action, index) => {
    const params = { index: index + 1 };
    if (!action.id.trim() || !APPROVAL_IDENTIFIER_PATTERN.test(action.id)) {
      errors.push({ code: 'approval.validation.actionIdInvalid', params });
    }
    if (action.id.length > APPROVAL_ACTION_MAX_LENGTH) {
      errors.push({ code: 'approval.validation.actionIdTooLong', params });
    }
    if (action.id === APPROVAL_TIMEOUT_HANDLE) {
      errors.push({ code: 'approval.validation.actionIdReserved', params });
    }
    if (actionIds.has(action.id)) {
      errors.push({ code: 'approval.validation.actionIdDuplicate', params });
    }
    actionIds.add(action.id);
    if (!action.label.trim()) {
      errors.push({ code: 'approval.validation.actionLabelRequired', params });
    }
    if (action.label.length > APPROVAL_ACTION_MAX_LENGTH) {
      errors.push({ code: 'approval.validation.actionLabelTooLong', params });
    }
  });

  const fieldKeys = new Set<string>();
  normalized.approval.fields.forEach((field, index) => {
    const params = { index: index + 1 };
    if (!field.key.trim() || !APPROVAL_IDENTIFIER_PATTERN.test(field.key)) {
      errors.push({ code: 'approval.validation.fieldKeyInvalid', params });
    }
    if (field.key.startsWith('__') || APPROVAL_SYSTEM_OUTPUT_KEYS.has(field.key)) {
      errors.push({ code: 'approval.validation.fieldKeyReserved', params });
    }
    if (fieldKeys.has(field.key)) {
      errors.push({ code: 'approval.validation.fieldKeyDuplicate', params });
    }
    fieldKeys.add(field.key);
    if (!isFieldType(field.type)) {
      errors.push({ code: 'approval.validation.fieldTypeInvalid', params });
    }
    if (field.default?.type === 'variable' && field.default.selector.length < 2) {
      errors.push({ code: 'approval.validation.defaultSelectorRequired', params });
    }
  });

  if (!Number.isInteger(normalized.timeout.duration) || normalized.timeout.duration <= 0) {
    errors.push({ code: 'approval.validation.timeoutDurationInvalid' });
  }

  if (!isTimeoutUnit(normalized.timeout.unit)) {
    errors.push({ code: 'approval.validation.timeoutUnitInvalid' });
  } else if (normalized.timeout.duration > getApprovalTimeoutMaxDuration(normalized.timeout.unit)) {
    errors.push({ code: 'approval.validation.timeoutDurationTooLong' });
  }

  if (normalized.submit_methods.email.enabled) {
    if (!normalized.submit_methods.email.body.includes('{{#url#}}')) {
      warnings.push({ code: 'approval.validation.emailBodyUrlRecommended' });
    }
    normalized.submit_methods.email.recipients.forEach((recipient, index) => {
      const params = { index: index + 1 };
      if (recipient.type === 'member') {
        if (!recipient.account_id.trim()) {
          errors.push({ code: 'approval.validation.recipientEmailRequired', params });
        }
        return;
      }

      if (!recipient.email.trim()) {
        errors.push({ code: 'approval.validation.recipientEmailRequired', params });
      } else if (!isValidEmail(recipient.email)) {
        errors.push({ code: 'approval.validation.recipientEmailInvalid', params });
      }
    });
  }

  if (normalized.submit_methods.sms.enabled) {
    if (!normalized.submit_methods.sms.notification_title.trim()) {
      errors.push({ code: 'approval.validation.smsTitleRequired' });
    }
    if (normalized.submit_methods.sms.recipients.length === 0) {
      errors.push({ code: 'approval.validation.smsRecipientRequired' });
    }
    Object.keys(normalized.submit_methods.sms.template_params).forEach(key => {
      const normalizedKey = key.trim();
      const params = { key: normalizedKey };
      if (!normalizedKey) {
        errors.push({ code: 'approval.validation.smsTemplateParamKeyRequired' });
      } else if (APPROVAL_SMS_RESERVED_TEMPLATE_PARAMS.has(normalizedKey)) {
        errors.push({ code: 'approval.validation.smsTemplateParamKeyReserved', params });
      } else if (!normalized.submit_methods.sms.template_params[key].trim()) {
        errors.push({ code: 'approval.validation.smsTemplateParamValueRequired', params });
      }
    });
    normalized.submit_methods.sms.recipients.forEach((recipient, index) => {
      const params = { index: index + 1 };
      if (recipient.type === 'member') {
        if (!recipient.account_id.trim()) {
          errors.push({ code: 'approval.validation.smsMemberRecipientRequired', params });
        }
        return;
      }

      if (!recipient.phone.trim()) {
        errors.push({ code: 'approval.validation.smsExternalRecipientRequired', params });
      }
    });
  }

  return { isValid: errors.length === 0, errors, warnings };
}
