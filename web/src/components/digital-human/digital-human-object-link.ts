'use client';

import { useCallback } from 'react';
import { usePathname, useRouter, useSearchParams } from 'next/navigation';

/** The things a console page can open in place: a run, a task, or an artifact. */
export type DigitalHumanObjectKind = 'run' | 'task' | 'artifact';

export const DIGITAL_HUMAN_VIEW_PARAM = 'view';
export const DIGITAL_HUMAN_VIEW_DH_PARAM = 'view_dh';
const KINDS: readonly DigitalHumanObjectKind[] = ['run', 'task', 'artifact'];
const digitalHumanPath = /^\/console\/digital-humans\/([0-9a-f-]{36})(?:\/|$)/i;

export function pathDigitalHuman(pathname: string | null) {
  return pathname?.match(digitalHumanPath)?.[1] ?? null;
}

export function parseDigitalHumanView(value: string | null): { kind: DigitalHumanObjectKind; id: string } | null {
  const [kind, id] = value?.split(':', 2) ?? [];
  return KINDS.includes(kind as DigitalHumanObjectKind) && id ? { kind: kind as DigitalHumanObjectKind, id } : null;
}

/**
 * The link that opens an object over the given page. It is a real URL, so a
 * modified click still opens it in a new tab, and back closes the drawer.
 */
export function digitalHumanObjectHref(
  page: string,
  kind: DigitalHumanObjectKind,
  id: string,
  digitalHumanId?: string
) {
  const [path, query = ''] = page.split('?', 2);
  const params = new URLSearchParams(query);
  params.set(DIGITAL_HUMAN_VIEW_PARAM, `${kind}:${id}`);
  // One drawer at a time: an object opened from a report takes its place.
  params.delete('report');
  params.delete('report_dh');
  if (digitalHumanId && digitalHumanId !== pathDigitalHuman(path)) params.set(DIGITAL_HUMAN_VIEW_DH_PARAM, digitalHumanId);
  else params.delete(DIGITAL_HUMAN_VIEW_DH_PARAM);
  return `${path}?${params.toString()}`;
}

/** Links that open a run, task, or artifact over the current page. */
export function useDigitalHumanObjectHref() {
  const pathname = usePathname();
  const searchParams = useSearchParams();
  const page = `${pathname}?${searchParams?.toString() ?? ''}`;
  return useCallback(
    (kind: DigitalHumanObjectKind, id: string, digitalHumanId?: string) =>
      digitalHumanObjectHref(page, kind, id, digitalHumanId),
    [page]
  );
}

/** Opens a run, task, or artifact over the current page. */
export function useOpenDigitalHumanObject() {
  const router = useRouter();
  const pathname = usePathname();
  return useCallback(
    (kind: DigitalHumanObjectKind, id: string, digitalHumanId?: string) => {
      // Read the query when opening, so callers need no Suspense boundary.
      const page = `${pathname}${window.location.search}`;
      router.push(digitalHumanObjectHref(page, kind, id, digitalHumanId), { scroll: false });
    },
    [pathname, router]
  );
}
