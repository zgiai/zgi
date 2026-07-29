interface SidebarWebAppItem {
  web_app_id: string;
}

interface MergeCurrentWebAppOptions {
  includeCurrent?: boolean;
  limit?: number;
}

export function mergeCurrentWebApp<T extends SidebarWebAppItem>(
  items: readonly T[],
  currentApp: T | null | undefined,
  options: MergeCurrentWebAppOptions = {}
): T[] {
  const deduplicated: T[] = [];
  const seen = new Set<string>();
  for (const item of items) {
    if (seen.has(item.web_app_id)) continue;
    seen.add(item.web_app_id);
    deduplicated.push(item);
  }

  const limit =
    typeof options.limit === 'number' ? Math.max(0, Math.trunc(options.limit)) : undefined;
  const visibleItems = typeof limit === 'number' ? deduplicated.slice(0, limit) : deduplicated;
  if (options.includeCurrent === false || !currentApp || limit === 0) return visibleItems;
  if (visibleItems.some(item => item.web_app_id === currentApp.web_app_id)) return visibleItems;
  if (typeof limit !== 'number') return [...visibleItems, currentApp];
  if (visibleItems.length < limit) return [...visibleItems, currentApp];
  return [...visibleItems.slice(0, limit - 1), currentApp];
}
