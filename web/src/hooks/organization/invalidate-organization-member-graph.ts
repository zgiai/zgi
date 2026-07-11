'use client';

import type { QueryClient } from '@tanstack/react-query';
import { ORGANIZATION_KEYS, WORKSPACE_KEYS } from '@/hooks/query-keys';

export function invalidateOrganizationMemberGraph(
  queryClient: QueryClient,
  organizationId?: string | null
) {
  queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.currentMembers() });
  queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.current() });

  if (!organizationId) {
    queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.all });
    queryClient.invalidateQueries({ queryKey: WORKSPACE_KEYS.all });
    return;
  }

  queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.roles(organizationId) });
  queryClient.invalidateQueries({
    queryKey: [...ORGANIZATION_KEYS.all, 'role-members', organizationId],
  });
  queryClient.invalidateQueries({ queryKey: ORGANIZATION_KEYS.departments(organizationId) });
  queryClient.invalidateQueries({
    queryKey: ORGANIZATION_KEYS.departmentMembers(organizationId),
  });
  queryClient.invalidateQueries({ queryKey: WORKSPACE_KEYS.all });
  queryClient.invalidateQueries({ queryKey: WORKSPACE_KEYS.forSwitcher(organizationId) });
}
