'use client';

export type AIChatContextItemType =
  | 'agent'
  | 'workflow'
  | 'file'
  | 'task'
  | 'dataset'
  | 'database'
  | 'page'
  | 'selection'
  | 'log'
  | 'custom';

export type AIChatContextRisk = 'low' | 'medium' | 'high';

export interface AIChatContextItem {
  id: string;
  type: AIChatContextItemType;
  title: string;
  subtitle?: string;
  description?: string;
  href?: string;
  source?: string;
  risk?: AIChatContextRisk;
  permissions?: string[];
  metadata?: Record<string, string | number | boolean | null | undefined>;
}

export interface AIChatContextRegistrationOptions {
  scopeId?: string;
  replace?: boolean;
}
