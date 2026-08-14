export const MODEL_USAGE_APP_TYPES = [
  'workflow',
  'dataset',
  'agent',
  'aichat',
  'image-runtime',
  'video-runtime',
  'data_library_file',
  'prompt_optimizer',
  'prompt_playground',
  'automation_task_draft',
  'unknown',
] as const;

export type ModelUsageAppType = (typeof MODEL_USAGE_APP_TYPES)[number];

export function isModelUsageAppType(value: unknown): value is ModelUsageAppType {
  return MODEL_USAGE_APP_TYPES.some(appType => appType === value);
}

export function normalizeModelUsageAppType(value: unknown): ModelUsageAppType {
  return isModelUsageAppType(value) ? value : 'unknown';
}
