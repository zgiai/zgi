import type { ConversationVariable } from '../../../store/type';

export type ComplexOutputType = Extract<
  ConversationVariable['type'],
  'object' | 'array[string]' | 'array[number]' | 'array[boolean]' | 'array[object]'
>;

export type ComplexOutputValidationError =
  | 'invalidJson'
  | 'expectedObject'
  | 'expectedArray'
  | 'invalidArrayItems';

export function copyComplexOutputValue(value: unknown): unknown {
  try {
    return JSON.parse(JSON.stringify(value));
  } catch {
    return value;
  }
}

export function countComplexOutputValue(type: ComplexOutputType, value: unknown): number {
  if (type.startsWith('array')) return Array.isArray(value) ? value.length : 0;
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return Object.keys(value).length;
  }
  return 0;
}

export function serializeComplexOutputValue(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return '';
  }
}

function isObjectValue(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

export function validateComplexOutputValue(
  type: ComplexOutputType,
  raw: string
): { value?: unknown; error?: ComplexOutputValidationError } {
  let parsed: unknown;
  try {
    parsed = JSON.parse(raw);
  } catch {
    return { error: 'invalidJson' };
  }

  if (type === 'object') {
    return isObjectValue(parsed) ? { value: parsed } : { error: 'expectedObject' };
  }
  if (!Array.isArray(parsed)) return { error: 'expectedArray' };

  const itemsValid = parsed.every(item => {
    if (type === 'array[string]') return typeof item === 'string';
    if (type === 'array[number]') return typeof item === 'number' && Number.isFinite(item);
    if (type === 'array[boolean]') return typeof item === 'boolean';
    return isObjectValue(item);
  });

  return itemsValid ? { value: parsed } : { error: 'invalidArrayItems' };
}

export function defaultComplexOutputMode(type: ComplexOutputType): 'list' | 'json' {
  return type === 'array[string]' || type === 'array[number]' || type === 'array[boolean]'
    ? 'list'
    : 'json';
}
