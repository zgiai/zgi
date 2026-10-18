'use client';

import { useMemo } from 'react';
import { useT } from '@/i18n';
import type { RoleTextResolver } from './digital-human-templates';

/**
 * Bridges the strongly typed scoped translator and the blueprint key strings.
 *
 * Blueprint keys are data, not literals, so the translator receives a dynamic
 * key. The codebase already handles dynamic keys the same way (see
 * `app/console/workspace/page.tsx`); the cast is confined to this one boundary
 * so `digital-human-templates.ts` and `create-digital-human-payload.ts` stay pure.
 */
export function useRoleTextResolver(): RoleTextResolver {
  const t = useT('digitalHumans');
  return useMemo<RoleTextResolver>(
    () => (key: string) => t(key as Parameters<typeof t>[0]),
    [t]
  );
}
