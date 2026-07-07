'use client';

import { KNOWLEDGE_BASE_READ_PERMISSION_CODES } from '@/constants/permissions';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

export function useKnowledgeNodePermissions() {
  const { hasAnyPermission, isLoading, isFetching } = useAccountPermissions();

  return {
    canReadKnowledgeBinding: hasAnyPermission(KNOWLEDGE_BASE_READ_PERMISSION_CODES),
    isLoadingKnowledgePermissions: isLoading || isFetching,
  };
}
