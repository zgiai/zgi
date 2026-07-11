'use client';

import { DATABASE_READ_BINDING_PERMISSION_CODES } from '@/constants/permissions';
import { useAccountPermissions } from '@/hooks/organization/use-account-permissions';

export function useDatabaseNodePermissions() {
  const { hasAllPermissions, isLoading, isFetching } = useAccountPermissions();

  return {
    canReadDatabaseBinding: hasAllPermissions(DATABASE_READ_BINDING_PERMISSION_CODES),
    isLoadingDatabasePermissions: isLoading || isFetching,
  };
}
