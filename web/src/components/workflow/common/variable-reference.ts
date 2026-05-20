'use client';

import type { WorkflowVariable } from '../store/type';
import type { StructuredTypeField } from '../types/input-var';

export type WorkflowPrimitiveType = WorkflowVariable['type'];

export interface ResolvedVariableReference {
  sourceId: string;
  sourceTitle: string;
  keyPath: string[];
  displayPath: string;
  displayText: string;
  invalid: boolean;
  isSpecialSource: boolean;
  type?: WorkflowPrimitiveType;
}

export const SPECIAL_VARIABLE_SOURCE_IDS = ['sys', 'conversation', 'environment'] as const;

export function isSpecialVariableSource(sourceId?: string | null): boolean {
  return SPECIAL_VARIABLE_SOURCE_IDS.includes(sourceId as (typeof SPECIAL_VARIABLE_SOURCE_IDS)[number]);
}

export function parseTemplateToSelector(template?: string): string[] | null {
  if (!template || typeof template !== 'string') return null;
  const trimmed = template.trim();
  if (!trimmed.startsWith('{{#') || !trimmed.endsWith('#}}')) return null;

  const inner = trimmed.slice(3, -3);
  const dotIndex = inner.indexOf('.');
  if (dotIndex <= 0) return null;

  const sourceId = inner.slice(0, dotIndex);
  const keyPath = inner.slice(dotIndex + 1);
  if (!sourceId || !keyPath) return null;

  return normalizeVariableSelector([sourceId, ...keyPath.split('.')]);
}

export function normalizeVariableSelector(selector?: string[] | null): string[] | null {
  if (!Array.isArray(selector) || selector.length < 2) return null;

  const [sourceIdRaw, keyRaw, ...rest] = selector;
  if (typeof sourceIdRaw !== 'string' || typeof keyRaw !== 'string') return null;

  if (sourceIdRaw !== 'sys' && keyRaw.startsWith('sys.')) {
    return ['sys', keyRaw.slice(4), ...rest];
  }

  return [sourceIdRaw, keyRaw, ...rest];
}

export function buildVariableSelectionKey(selector?: string[] | null): string | null {
  const normalized = normalizeVariableSelector(selector);
  return normalized ? normalized.join('::') : null;
}

export function findNestedStructuredField(
  fields: StructuredTypeField[] | undefined,
  path: string[]
): StructuredTypeField | null {
  if (!fields || path.length === 0) return null;

  const [current, ...rest] = path;
  const field = fields.find(item => item.key === current);
  if (!field) return null;
  if (rest.length === 0) return field;

  return findNestedStructuredField(field.children, rest);
}

export function hasMatchingStructuredField(
  fields: StructuredTypeField[] | undefined,
  matcher: (field: StructuredTypeField) => boolean
): boolean {
  if (!fields || fields.length === 0) return false;

  return fields.some(field => {
    if (matcher(field)) return true;
    return hasMatchingStructuredField(field.children, matcher);
  });
}

export function resolveVariableReference(args: {
  selector: string[];
  sourceTitle: string;
  invalid?: boolean;
  type?: WorkflowPrimitiveType;
}): ResolvedVariableReference {
  const normalized = normalizeVariableSelector(args.selector);
  if (!normalized) {
    throw new Error('resolveVariableReference requires a valid selector.');
  }

  const [sourceId, ...keyPath] = normalized;
  const displayPath = keyPath.join('.');

  return {
    sourceId,
    sourceTitle: args.sourceTitle,
    keyPath,
    displayPath,
    displayText: `${args.sourceTitle} (${displayPath})`,
    invalid: Boolean(args.invalid),
    isSpecialSource: isSpecialVariableSource(sourceId),
    type: args.type,
  };
}
