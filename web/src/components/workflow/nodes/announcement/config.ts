import type { ValidationError, ValidationResult } from '../common/validation';

export type AnnouncementTimeoutUnit = 'hour' | 'day';

export interface AnnouncementTimeout {
  duration: number;
  unit: AnnouncementTimeoutUnit;
}

export interface AnnouncementNodeData {
  type: 'announcement';
  title: string;
  desc: string;
  version: 'v1';
  announcement: {
    title: string;
    content: string;
  };
  timeout: AnnouncementTimeout;
  isInLoop?: boolean;
  isInIteration?: boolean;
}

export const ANNOUNCEMENT_MAX_TIMEOUT_HOURS = 168;
export const ANNOUNCEMENT_TITLE_MAX_LENGTH = 255;

export const DEFAULT_ANNOUNCEMENT_NODE_DATA: AnnouncementNodeData = {
  type: 'announcement',
  title: '',
  desc: '',
  version: 'v1',
  announcement: {
    title: '',
    content: '',
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

function isTimeoutUnit(value: unknown): value is AnnouncementTimeoutUnit {
  return value === 'hour' || value === 'day';
}

export function getAnnouncementTimeoutMaxDuration(unit: AnnouncementTimeoutUnit): number {
  return unit === 'day' ? 7 : ANNOUNCEMENT_MAX_TIMEOUT_HOURS;
}

export function normalizeAnnouncementNodeData(
  value: Partial<AnnouncementNodeData> | Record<string, unknown> | null | undefined
): AnnouncementNodeData {
  const fallback = DEFAULT_ANNOUNCEMENT_NODE_DATA;
  if (!isRecord(value)) return fallback;

  const announcement = isRecord(value.announcement) ? value.announcement : {};
  const timeout = isRecord(value.timeout) ? value.timeout : {};

  return {
    ...fallback,
    title: typeof value.title === 'string' ? value.title : fallback.title,
    desc: typeof value.desc === 'string' ? value.desc : fallback.desc,
    version: 'v1',
    announcement: {
      title:
        typeof announcement.title === 'string'
          ? announcement.title
          : typeof value.title === 'string'
            ? value.title
            : fallback.announcement.title,
      content:
        typeof announcement.content === 'string'
          ? announcement.content
          : fallback.announcement.content,
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

export function checkValid(data: AnnouncementNodeData): ValidationResult {
  const normalized = normalizeAnnouncementNodeData(data);
  const errors: ValidationError[] = [];
  const warnings: ValidationError[] = [];

  if (!normalized.announcement.title.trim()) {
    errors.push({ code: 'announcement.validation.titleRequired' });
  } else if (
    Array.from(normalized.announcement.title.trim()).length > ANNOUNCEMENT_TITLE_MAX_LENGTH
  ) {
    errors.push({
      code: 'announcement.validation.titleTooLong',
      params: { max: ANNOUNCEMENT_TITLE_MAX_LENGTH },
    });
  }
  if (!normalized.announcement.content.trim()) {
    errors.push({ code: 'announcement.validation.contentRequired' });
  }
  if (!Number.isInteger(normalized.timeout.duration) || normalized.timeout.duration <= 0) {
    errors.push({ code: 'announcement.validation.timeoutDurationInvalid' });
  }
  if (!isTimeoutUnit(normalized.timeout.unit)) {
    errors.push({ code: 'announcement.validation.timeoutUnitInvalid' });
  } else if (
    normalized.timeout.duration > getAnnouncementTimeoutMaxDuration(normalized.timeout.unit)
  ) {
    errors.push({ code: 'announcement.validation.timeoutDurationTooLong' });
  }

  return { isValid: errors.length === 0, errors, warnings };
}
