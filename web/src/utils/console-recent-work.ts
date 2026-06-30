import type { DashboardRecentWorkType } from '@/services/types/dashboard';

export function getRecentWorkHref(
  type: DashboardRecentWorkType,
  resourceId: string,
  parentId?: string
) {
  if (type === 'conversation') {
    const query = `conversation_id=${encodeURIComponent(resourceId)}`;
    return parentId ? `/console/agents/${parentId}/logs?${query}` : `/console/agents?${query}`;
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
