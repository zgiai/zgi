'use client';

import { usePageContextRegistration } from '@/components/aichat/page-context';
import type { FilesAIChatContextItem } from './types';

export function FilesAIChatContextRegistration({
  items,
}: {
  items: FilesAIChatContextItem[];
}) {
  usePageContextRegistration(items, { scopeId: 'console-files' });
  return null;
}
