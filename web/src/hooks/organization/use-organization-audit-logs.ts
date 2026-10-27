'use client';

import { keepPreviousData, useQuery } from '@tanstack/react-query';

import { ORGANIZATION_KEYS } from '@/hooks/query-keys';
import { organizationService } from '@/services/organization.service';
import type { OrgAuditLogParams } from '@/services/types/organization';

/**
 * Pages through an organization's member, role and permission change records.
 */
export function useOrganizationAuditLogs(
  organizationId: string | undefined,
  params: OrgAuditLogParams,
  enabled = true
) {
  return useQuery({
    queryKey: ORGANIZATION_KEYS.auditLogs(organizationId || '', params),
    queryFn: () => organizationService.getAuditLogs(organizationId as string, params),
    enabled: Boolean(organizationId) && enabled,
    placeholderData: keepPreviousData,
    retry: false,
  });
}
