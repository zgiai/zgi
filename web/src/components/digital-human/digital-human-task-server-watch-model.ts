/**
 * Pure helpers for a task's server watch (ADR 0031). They stay free of
 * runtime imports so a Node script can test them.
 */

export const SERVER_WATCH_INTERVALS = [1, 5, 15, 30, 60] as const;
export const SERVER_WATCH_THRESHOLDS = [1, 2, 3] as const;

/** Splits the services a person typed by commas, spaces, or the Chinese comma. */
export function parseWatchedServices(text: string): string[] {
  return [
    ...new Set(
      text
        .split(/[\s,，、]+/)
        .map(name => name.trim())
        .filter(Boolean)
    ),
  ];
}

interface WatchStateLike {
  state: Record<string, { healthy: boolean; observed?: string }>;
}

/** What the last check found down, the host first. */
export function serverWatchProblems(
  watch: WatchStateLike
): Array<{ name: string; observed: string }> {
  return Object.entries(watch.state ?? {})
    .filter(([, target]) => !target.healthy)
    .map(([name, target]) => ({ name, observed: target.observed ?? '' }))
    .sort((a, b) =>
      a.name === '@host' ? -1 : b.name === '@host' ? 1 : a.name.localeCompare(b.name)
    );
}
