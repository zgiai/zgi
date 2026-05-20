/**
 * @util Validation helpers for model identifiers used by custom model creation.
 * Supports common upstream naming styles such as namespaced repo ids and tagged local ids.
 */
export const CUSTOM_MODEL_ID_PATTERN = /^[-A-Za-z0-9._:/+]+$/;

export function isValidCustomModelId(value: string): boolean {
  const normalizedValue = value.trim();
  return normalizedValue.length > 0 && CUSTOM_MODEL_ID_PATTERN.test(normalizedValue);
}
