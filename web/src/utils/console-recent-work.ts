import type { DashboardRecentWorkType } from '@/services/types/dashboard';

export function getRecentWorkHref(
  type: DashboardRecentWorkType,
  resourceId: string,
  parentId?: string
) {
  if (type === 'conversation') {
    if (!parentId) {
      return `/console/work/chat?convId=${encodeURIComponent(resourceId)}`;
    }
    const query = `conversation_id=${encodeURIComponent(resourceId)}`;
    return `/console/agents/${parentId}/logs?${query}`;
  }

  if (type === 'workflow') {
    return `/console/workflows/${resourceId}`;
  }

  if (type === 'agent') {
    return `/console/agents/${resourceId}`;
  }

  if (type === 'dataset') {
    return `/console/dataset/${resourceId}`;
  }

  return `/console/db/${resourceId}`;
}

export function getRecentWorkNavigationHref(
  hasWorkspace: boolean,
  type: DashboardRecentWorkType,
  resourceId: string,
  parentId?: string
): string | null {
  return hasWorkspace ? getRecentWorkHref(type, resourceId, parentId) : null;
}
